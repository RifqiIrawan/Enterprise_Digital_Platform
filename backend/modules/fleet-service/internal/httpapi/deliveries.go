package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/fleet-service/internal/model"
)

const deliveryColumns = `id, company_id, branch_id, delivery_number, vehicle_id, driver_id, ecommerce_order_id, reference_number, recipient_name, recipient_phone, destination_address, scheduled_date, status, dispatched_at, delivered_at, cancelled_at, notes, created_by_user_id, created_at, updated_at`

func scanDelivery(row pgx.Row, d *model.DeliveryOrder) error {
	return row.Scan(&d.ID, &d.CompanyID, &d.BranchID, &d.DeliveryNumber, &d.VehicleID, &d.DriverID, &d.EcommerceOrderID, &d.ReferenceNumber, &d.RecipientName, &d.RecipientPhone, &d.DestinationAddress, &d.ScheduledDate, &d.Status, &d.DispatchedAt, &d.DeliveredAt, &d.CancelledAt, &d.Notes, &d.CreatedByUserID, &d.CreatedAt, &d.UpdatedAt)
}

func (h *Handler) listDeliveryOrders(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + deliveryColumns + ` FROM delivery_orders WHERE company_id = $1`
	args := []any{companyID}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data surat jalan")
		return
	}
	defer rows.Close()

	deliveries := []model.DeliveryOrder{}
	for rows.Next() {
		var d model.DeliveryOrder
		if err := scanDelivery(rows, &d); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data surat jalan")
			return
		}
		deliveries = append(deliveries, d)
	}
	writeJSON(w, http.StatusOK, deliveries)
}

func (h *Handler) getDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	var d model.DeliveryOrder
	err := scanDelivery(h.pool.QueryRow(r.Context(), `SELECT `+deliveryColumns+` FROM delivery_orders WHERE id = $1`, r.PathValue("id")), &d)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Surat jalan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat surat jalan")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type deliveryRequest struct {
	CompanyID          string  `json:"company_id"`
	BranchID           *string `json:"branch_id"`
	VehicleID          string  `json:"vehicle_id"`
	DriverID           string  `json:"driver_id"`
	EcommerceOrderID   string  `json:"ecommerce_order_id"`
	RecipientName      string  `json:"recipient_name"`
	RecipientPhone     string  `json:"recipient_phone"`
	DestinationAddress string  `json:"destination_address"`
	ScheduledDate      string  `json:"scheduled_date"`
	Notes              string  `json:"notes"`
}

// assertAssignable memastikan kendaraan/pengemudi ada, milik company yang
// sama (guard lintas-company: tanpa filter company_id, UUID milik company
// lain bisa ditebak dan dipakai), dan tidak sedang MAINTENANCE/INACTIVE.
// TIDAK mensyaratkan AVAILABLE -- surat jalan boleh dijadwalkan lebih dulu
// selagi kendaraannya masih mengantar yang lain; ketersediaan baru benar
// benar ditegakkan saat dispatch (lihat dispatchDeliveryOrder).
func (h *Handler) assertAssignable(ctx context.Context, companyID, vehicleID, driverID string) (int, string) {
	var vehicleStatus string
	err := h.pool.QueryRow(ctx, `SELECT status FROM vehicles WHERE id = $1 AND company_id = $2`, vehicleID, companyID).Scan(&vehicleStatus)
	if err == pgx.ErrNoRows {
		return http.StatusBadRequest, "Kendaraan tidak ditemukan di company ini"
	} else if err != nil {
		return http.StatusInternalServerError, "Gagal memuat kendaraan"
	}
	if vehicleStatus == "MAINTENANCE" {
		return http.StatusConflict, "Kendaraan sedang MAINTENANCE, tidak bisa dijadwalkan"
	}

	var driverStatus string
	err = h.pool.QueryRow(ctx, `SELECT status FROM drivers WHERE id = $1 AND company_id = $2`, driverID, companyID).Scan(&driverStatus)
	if err == pgx.ErrNoRows {
		return http.StatusBadRequest, "Pengemudi tidak ditemukan di company ini"
	} else if err != nil {
		return http.StatusInternalServerError, "Gagal memuat pengemudi"
	}
	if driverStatus == "INACTIVE" {
		return http.StatusConflict, "Pengemudi berstatus INACTIVE, tidak bisa dijadwalkan"
	}
	return 0, ""
}

// createDeliveryOrder. Kalau ecommerce_order_id diisi, fleet-service memanggil
// ecommerce-service untuk mengambil nomor order + nama/alamat penerima sebagai
// snapshot. Order WAJIB berstatus SHIPPED: barangnya baru benar-benar keluar
// gudang di titik itu (ship-lah yang mencatat stok keluar di
// warehouse-service), dan `POST /orders/{id}/deliver` yang nanti dipanggil
// saat surat jalan selesai HANYA menerima order SHIPPED -- kalau di sini
// dibiarkan longgar, kegagalannya baru muncul jauh belakangan saat deliver.
func (h *Handler) createDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	var req deliveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.RecipientName = strings.TrimSpace(req.RecipientName)
	req.DestinationAddress = strings.TrimSpace(req.DestinationAddress)
	if req.CompanyID == "" || req.VehicleID == "" || req.DriverID == "" {
		writeError(w, http.StatusBadRequest, "company_id, vehicle_id, dan driver_id wajib diisi")
		return
	}

	scheduledDate := time.Now()
	if req.ScheduledDate != "" {
		parsed, err := time.Parse("2006-01-02", req.ScheduledDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "scheduled_date harus format YYYY-MM-DD")
			return
		}
		scheduledDate = parsed
	}

	ctx := r.Context()
	if status, msg := h.assertAssignable(ctx, req.CompanyID, req.VehicleID, req.DriverID); status != 0 {
		writeError(w, status, msg)
		return
	}

	var ecommerceOrderID, referenceNumber *string
	if req.EcommerceOrderID != "" {
		order, err := h.ecommerce.GetOrder(req.EcommerceOrderID)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("Gagal memuat order dari ecommerce-service: %v", err))
			return
		}
		// Guard lintas-company: order milik company lain tidak boleh dijadikan
		// surat jalan di sini walaupun UUID-nya benar.
		if order.CompanyID != req.CompanyID {
			writeError(w, http.StatusBadRequest, "Order e-commerce tersebut milik company lain")
			return
		}
		if order.Status != "SHIPPED" {
			writeError(w, http.StatusConflict, "Order e-commerce harus berstatus SHIPPED sebelum dibuatkan surat jalan (status sekarang: "+order.Status+")")
			return
		}
		ecommerceOrderID = &req.EcommerceOrderID
		referenceNumber = &order.OrderNumber
		// Nama/alamat penerima diambil dari order KALAU tidak diisi manual --
		// pengiriman bisa saja dialihkan ke alamat lain daripada yang tertulis
		// di order, jadi input manual tetap menang.
		if req.RecipientName == "" {
			req.RecipientName = order.CustomerName
		}
		if req.DestinationAddress == "" {
			req.DestinationAddress = order.ShippingAddress
		}
	}

	if req.RecipientName == "" || req.DestinationAddress == "" {
		writeError(w, http.StatusBadRequest, "recipient_name dan destination_address wajib diisi (atau diambil dari order e-commerce)")
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	deliveryNumber, err := nextSequence(ctx, tx, req.CompanyID, "delivery_orders", "delivery_number", "DLV", time.Now().Format("2006-01"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor surat jalan")
		return
	}

	var d model.DeliveryOrder
	err = scanDelivery(tx.QueryRow(ctx, `
		INSERT INTO delivery_orders (company_id, branch_id, delivery_number, vehicle_id, driver_id, ecommerce_order_id, reference_number, recipient_name, recipient_phone, destination_address, scheduled_date, notes, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+deliveryColumns,
		req.CompanyID, req.BranchID, deliveryNumber, req.VehicleID, req.DriverID, ecommerceOrderID, referenceNumber, req.RecipientName, req.RecipientPhone, req.DestinationAddress, scheduledDate, req.Notes, actorFromHeader(r),
	), &d)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat surat jalan")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan surat jalan")
		return
	}

	h.events.Publish("fleet.delivery.created", newAuditEvent("fleet.delivery.created", actorFromHeader(r), &d.CompanyID, "create", "delivery_order", d.ID, d))
	writeJSON(w, http.StatusCreated, d)
}

// updateDeliveryOrder cuma untuk field non-status, dan hanya selagi PENDING --
// semua transisi status lewat endpoint khusus (dispatch/deliver/cancel),
// pola ecommerce-service. Kendaraan/pengemudi boleh diganti selagi PENDING
// (belum berangkat), divalidasi ulang dengan aturan yang sama seperti create.
func (h *Handler) updateDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req deliveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.RecipientName = strings.TrimSpace(req.RecipientName)
	req.DestinationAddress = strings.TrimSpace(req.DestinationAddress)
	if req.CompanyID == "" || req.VehicleID == "" || req.DriverID == "" || req.RecipientName == "" || req.DestinationAddress == "" {
		writeError(w, http.StatusBadRequest, "company_id, vehicle_id, driver_id, recipient_name, dan destination_address wajib diisi")
		return
	}

	ctx := r.Context()
	var before model.DeliveryOrder
	err := scanDelivery(h.pool.QueryRow(ctx, `SELECT `+deliveryColumns+` FROM delivery_orders WHERE id = $1 AND company_id = $2`, id, req.CompanyID), &before)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Surat jalan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat surat jalan")
		return
	}
	if before.Status != "PENDING" {
		writeError(w, http.StatusConflict, "Surat jalan hanya bisa diubah selagi PENDING")
		return
	}

	if status, msg := h.assertAssignable(ctx, req.CompanyID, req.VehicleID, req.DriverID); status != 0 {
		writeError(w, status, msg)
		return
	}

	scheduledDate := before.ScheduledDate
	if req.ScheduledDate != "" {
		parsed, err := time.Parse("2006-01-02", req.ScheduledDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "scheduled_date harus format YYYY-MM-DD")
			return
		}
		scheduledDate = parsed
	}

	var d model.DeliveryOrder
	err = scanDelivery(h.pool.QueryRow(ctx, `
		UPDATE delivery_orders SET vehicle_id = $1, driver_id = $2, recipient_name = $3, recipient_phone = $4, destination_address = $5, scheduled_date = $6, notes = $7, updated_at = now()
		WHERE id = $8 AND company_id = $9
		RETURNING `+deliveryColumns,
		req.VehicleID, req.DriverID, req.RecipientName, req.RecipientPhone, req.DestinationAddress, scheduledDate, req.Notes, id, req.CompanyID,
	), &d)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui surat jalan")
		return
	}

	h.events.Publish("fleet.delivery.updated", newAuditEvent("fleet.delivery.updated", actorFromHeader(r), &d.CompanyID, "update", "delivery_order", d.ID, d))
	writeJSON(w, http.StatusOK, d)
}

// dispatchDeliveryOrder adalah satu-satunya tempat kendaraan jadi IN_USE dan
// pengemudi jadi ON_DELIVERY. Ketiga baris (surat jalan, kendaraan, pengemudi)
// dikunci SELECT ... FOR UPDATE lalu diubah dalam SATU transaksi -- tanpa itu
// dua surat jalan yang di-dispatch bersamaan bisa sama-sama lolos cek
// "AVAILABLE" lalu menugaskan kendaraan yang sama dua kali.
func (h *Handler) dispatchDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var d model.DeliveryOrder
	err = scanDelivery(tx.QueryRow(ctx, `SELECT `+deliveryColumns+` FROM delivery_orders WHERE id = $1 FOR UPDATE`, id), &d)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Surat jalan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat surat jalan")
		return
	}
	if d.Status != "PENDING" {
		writeError(w, http.StatusConflict, "Surat jalan hanya bisa diberangkatkan dari status PENDING")
		return
	}

	var vehicleStatus, driverStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM vehicles WHERE id = $1 FOR UPDATE`, d.VehicleID).Scan(&vehicleStatus); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengunci data kendaraan")
		return
	}
	if vehicleStatus != "AVAILABLE" {
		writeError(w, http.StatusConflict, "Kendaraan tidak tersedia (status: "+vehicleStatus+")")
		return
	}
	if err := tx.QueryRow(ctx, `SELECT status FROM drivers WHERE id = $1 FOR UPDATE`, d.DriverID).Scan(&driverStatus); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengunci data pengemudi")
		return
	}
	if driverStatus != "AVAILABLE" {
		writeError(w, http.StatusConflict, "Pengemudi tidak tersedia (status: "+driverStatus+")")
		return
	}

	if _, err := tx.Exec(ctx, `UPDATE vehicles SET status = 'IN_USE', updated_at = now() WHERE id = $1`, d.VehicleID); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status kendaraan")
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE drivers SET status = 'ON_DELIVERY', updated_at = now() WHERE id = $1`, d.DriverID); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status pengemudi")
		return
	}
	err = scanDelivery(tx.QueryRow(ctx, `
		UPDATE delivery_orders SET status = 'DISPATCHED', dispatched_at = now(), updated_at = now()
		WHERE id = $1 RETURNING `+deliveryColumns, id), &d)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memberangkatkan surat jalan")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan perubahan surat jalan")
		return
	}

	h.events.Publish("fleet.delivery.dispatched", newAuditEvent("fleet.delivery.dispatched", actorFromHeader(r), &d.CompanyID, "update", "delivery_order", d.ID, d))
	writeJSON(w, http.StatusOK, d)
}

// deliverDeliveryOrder menutup surat jalan DAN, kalau surat jalan ini berasal
// dari order e-commerce, memajukan order itu ke DELIVERED di ecommerce-service.
//
// Urutannya SENGAJA: panggil ecommerce-service DULU, baru commit perubahan
// lokal -- pola identik shipOrder di ecommerce-service (yang memanggil
// warehouse-service dulu baru update status order lokal). Kalau dibalik,
// kegagalan panggilan lintas service meninggalkan surat jalan DELIVERED
// sementara order-nya masih SHIPPED, dan tidak ada yang membereskan itu.
func (h *Handler) deliverDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var d model.DeliveryOrder
	err := scanDelivery(h.pool.QueryRow(ctx, `SELECT `+deliveryColumns+` FROM delivery_orders WHERE id = $1`, id), &d)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Surat jalan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat surat jalan")
		return
	}
	if d.Status != "DISPATCHED" {
		writeError(w, http.StatusConflict, "Surat jalan hanya bisa diselesaikan dari status DISPATCHED")
		return
	}

	if d.EcommerceOrderID != nil {
		if err := h.ecommerce.MarkDelivered(headerValue(actor), *d.EcommerceOrderID); err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("Gagal menandai order DELIVERED di ecommerce-service: %v", err))
			return
		}
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	if err := releaseVehicleAndDriver(ctx, tx, d.VehicleID, d.DriverID); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membebaskan kendaraan/pengemudi")
		return
	}
	err = scanDelivery(tx.QueryRow(ctx, `
		UPDATE delivery_orders SET status = 'DELIVERED', delivered_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'DISPATCHED' RETURNING `+deliveryColumns, id), &d)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Order sudah ditandai DELIVERED di ecommerce-service, tetapi gagal memperbarui surat jalan lokal")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan perubahan surat jalan")
		return
	}

	h.events.Publish("fleet.delivery.delivered", newAuditEvent("fleet.delivery.delivered", actor, &d.CompanyID, "update", "delivery_order", d.ID, d))
	writeJSON(w, http.StatusOK, d)
}

// cancelDeliveryOrder. Order e-commerce yang terhubung SENGAJA tidak ikut
// diubah: membatalkan surat jalan berarti pengirimannya dijadwalkan ulang
// (mis. ganti kendaraan), bukan bahwa order-nya batal -- order tetap SHIPPED
// dan bisa dibuatkan surat jalan baru.
func (h *Handler) cancelDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var d model.DeliveryOrder
	err = scanDelivery(tx.QueryRow(ctx, `SELECT `+deliveryColumns+` FROM delivery_orders WHERE id = $1 FOR UPDATE`, id), &d)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Surat jalan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat surat jalan")
		return
	}
	if d.Status != "PENDING" && d.Status != "DISPATCHED" {
		writeError(w, http.StatusConflict, "Surat jalan hanya bisa dibatalkan dari status PENDING atau DISPATCHED")
		return
	}

	// Kendaraan/pengemudi cuma perlu dibebaskan kalau surat jalannya memang
	// sudah sempat berangkat -- surat jalan PENDING tidak pernah menguncinya.
	if d.Status == "DISPATCHED" {
		if err := releaseVehicleAndDriver(ctx, tx, d.VehicleID, d.DriverID); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membebaskan kendaraan/pengemudi")
			return
		}
	}

	err = scanDelivery(tx.QueryRow(ctx, `
		UPDATE delivery_orders SET status = 'CANCELLED', cancelled_at = now(), updated_at = now()
		WHERE id = $1 RETURNING `+deliveryColumns, id), &d)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membatalkan surat jalan")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan perubahan surat jalan")
		return
	}

	h.events.Publish("fleet.delivery.cancelled", newAuditEvent("fleet.delivery.cancelled", actorFromHeader(r), &d.CompanyID, "update", "delivery_order", d.ID, d))
	writeJSON(w, http.StatusOK, d)
}

// releaseVehicleAndDriver mengembalikan keduanya ke AVAILABLE. Guard
// `WHERE status = '<status sibuk>'` disengaja: kalau kendaraannya sudah
// terlanjur dipindah ke MAINTENANCE lewat jalur lain, penyelesaian surat
// jalan ini tidak boleh diam-diam menariknya kembali jadi AVAILABLE.
func releaseVehicleAndDriver(ctx context.Context, tx pgx.Tx, vehicleID, driverID string) error {
	if _, err := tx.Exec(ctx, `UPDATE vehicles SET status = 'AVAILABLE', updated_at = now() WHERE id = $1 AND status = 'IN_USE'`, vehicleID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE drivers SET status = 'AVAILABLE', updated_at = now() WHERE id = $1 AND status = 'ON_DELIVERY'`, driverID); err != nil {
		return err
	}
	return nil
}
