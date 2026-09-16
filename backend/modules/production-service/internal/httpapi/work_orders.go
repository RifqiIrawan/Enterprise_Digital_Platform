package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/production-service/internal/model"
	"github.com/enterprise-digital-platform/production-service/internal/warehouseclient"
)

const workOrderColumns = `id, company_id, branch_id, wo_number, bom_id, product_id, warehouse_id, quantity_planned, batch_count, quantity_produced, status, planned_start_date, planned_end_date, notes, created_at, updated_at`

func scanWorkOrder(row pgx.Row, wo *model.WorkOrder) error {
	return row.Scan(&wo.ID, &wo.CompanyID, &wo.BranchID, &wo.WONumber, &wo.BOMID, &wo.ProductID, &wo.WarehouseID,
		&wo.QuantityPlanned, &wo.BatchCount, &wo.QuantityProduced, &wo.Status, &wo.PlannedStartDate, &wo.PlannedEndDate, &wo.Notes, &wo.CreatedAt, &wo.UpdatedAt)
}

func (h *Handler) listWorkOrders(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + workOrderColumns + ` FROM work_orders WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY planned_start_date ASC, wo_number ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data work order")
		return
	}
	defer rows.Close()

	orders := []model.WorkOrder{}
	for rows.Next() {
		var wo model.WorkOrder
		if err := scanWorkOrder(rows, &wo); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data work order")
			return
		}
		orders = append(orders, wo)
	}
	writeJSON(w, http.StatusOK, orders)
}

type createWorkOrderRequest struct {
	CompanyID        string  `json:"company_id"`
	BranchID         *string `json:"branch_id"`
	BOMID            string  `json:"bom_id"`
	WarehouseID      string  `json:"warehouse_id"`
	QuantityPlanned  float64 `json:"quantity_planned"`
	PlannedStartDate string  `json:"planned_start_date"`
	PlannedEndDate   string  `json:"planned_end_date"`
	Notes            string  `json:"notes"`
}

type workOrderWithLines struct {
	model.WorkOrder
	Lines []model.WorkOrderLine `json:"lines"`
}

// batchPlan menerjemahkan quantity_planned sebuah work order menjadi jumlah
// batch untuk BOM formula (bom_type = BATCH). Satu batch adalah isi satu alat
// -- tangki 1.000 kg tidak bisa diisi 1.500 kg hanya karena permintaannya
// segitu -- jadi quantity_planned yang bukan kelipatan bulat batch_size
// DITOLAK, bukan diam-diam dibulatkan. Pesannya menyebut dua angka terdekat
// yang sah supaya orangnya tahu harus mengetik apa, bukan hanya bahwa dia
// salah. BOM UNIT tidak punya batch; batch_count-nya nil dan perilakunya
// persis seperti sebelum Formula ada.
func batchPlan(bom model.BillOfMaterial, quantityPlanned float64) (*int, string) {
	if bom.BOMType != model.BOMTypeBatch || bom.BatchSize == nil {
		return nil, ""
	}
	size := *bom.BatchSize
	if size <= 0 {
		return nil, "BOM formula ini tidak punya batch_size yang sah"
	}
	exact := quantityPlanned / size
	rounded := math.Round(exact)
	// Toleransi 0,0001 batch: sisa pembulatan pecahan biner (mis. 3 x 0,1),
	// bukan setengah batch yang benar-benar diminta.
	if math.Abs(exact-rounded) > 0.0001 || rounded < 1 {
		lower := math.Floor(exact)
		if lower < 1 {
			lower = 1
		}
		return nil, fmt.Sprintf("quantity_planned (%g) harus kelipatan bulat batch_size (%g) -- BOM ini formula batch; gunakan %g atau %g",
			quantityPlanned, size, lower*size, (lower+1)*size)
	}
	count := int(rounded)
	return &count, ""
}

// createWorkOrder men-snapshot kebutuhan komponen ke work_order_lines saat
// dibuat, supaya perubahan BOM setelahnya tidak mengubah kebutuhan work order
// yang sudah berjalan. Dua cara menghitungnya, sesuai tipe BOM-nya:
//
//   - UNIT  : quantity_per_unit x quantity_planned, seperti sejak Fase 2.
//   - BATCH : percentage / 100 x batch_size x batch_count -- lewat percentage
//     yang ditulis orang, BUKAN lewat quantity_per_unit turunannya, supaya
//     persentase pecahan (mis. 33,3333%) tidak dibulatkan dua kali: sekali
//     saat disimpan ke NUMERIC(15,4), sekali lagi saat dikalikan.
func (h *Handler) createWorkOrder(w http.ResponseWriter, r *http.Request) {
	var req createWorkOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.BOMID == "" || req.WarehouseID == "" || req.PlannedStartDate == "" || req.QuantityPlanned <= 0 {
		writeError(w, http.StatusBadRequest, "company_id, bom_id, warehouse_id, planned_start_date, dan quantity_planned > 0 wajib diisi")
		return
	}
	plannedStart, err := time.Parse("2006-01-02", req.PlannedStartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "planned_start_date harus format YYYY-MM-DD")
		return
	}
	var plannedEnd *time.Time
	if req.PlannedEndDate != "" {
		parsed, err := time.Parse("2006-01-02", req.PlannedEndDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "planned_end_date harus format YYYY-MM-DD")
			return
		}
		plannedEnd = &parsed
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var bom model.BillOfMaterial
	err = scanBOM(tx.QueryRow(ctx, `SELECT `+bomColumns+` FROM bill_of_materials WHERE id = $1`, req.BOMID), &bom)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "BOM tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat BOM")
		return
	}
	if !bom.IsActive {
		writeError(w, http.StatusConflict, "BOM ini sudah nonaktif")
		return
	}

	batchCount, msg := batchPlan(bom, req.QuantityPlanned)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	bomLines, err := h.fetchBOMLines(ctx, bom.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat komponen BOM")
		return
	}
	if len(bomLines) == 0 {
		writeError(w, http.StatusConflict, "BOM ini belum punya komponen")
		return
	}

	period := req.PlannedStartDate[:7]
	woNumber, err := nextSequence(ctx, tx, req.CompanyID, "work_orders", "wo_number", "WO", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor work order")
		return
	}

	var wo model.WorkOrder
	err = scanWorkOrder(tx.QueryRow(ctx, `
		INSERT INTO work_orders (company_id, branch_id, wo_number, bom_id, product_id, warehouse_id, quantity_planned, batch_count, planned_start_date, planned_end_date, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+workOrderColumns,
		req.CompanyID, req.BranchID, woNumber, bom.ID, bom.ProductID, req.WarehouseID, req.QuantityPlanned, batchCount, plannedStart, plannedEnd, req.Notes,
	), &wo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat work order")
		return
	}

	lines := make([]model.WorkOrderLine, 0, len(bomLines))
	for i, bl := range bomLines {
		quantityRequired := bl.QuantityPerUnit * req.QuantityPlanned
		if batchCount != nil && bl.Percentage != nil {
			quantityRequired = *bl.Percentage / 100 * *bom.BatchSize * float64(*batchCount)
		}
		var line model.WorkOrderLine
		err := tx.QueryRow(ctx, `
			INSERT INTO work_order_lines (work_order_id, line_number, component_product_id, quantity_required)
			VALUES ($1, $2, $3, $4)
			RETURNING id, work_order_id, line_number, component_product_id, quantity_required`,
			wo.ID, i+1, bl.ComponentProductID, quantityRequired,
		).Scan(&line.ID, &line.WorkOrderID, &line.LineNumber, &line.ComponentProductID, &line.QuantityRequired)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris kebutuhan komponen")
			return
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan work order")
		return
	}

	h.events.Publish("production.work_order.created", newAuditEvent("production.work_order.created", actorFromHeader(r), &wo.CompanyID, "create", "work_order", wo.ID, wo))
	writeJSON(w, http.StatusCreated, workOrderWithLines{WorkOrder: wo, Lines: lines})
}

func (h *Handler) fetchWorkOrderLines(ctx context.Context, workOrderID string) ([]model.WorkOrderLine, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, work_order_id, line_number, component_product_id, quantity_required
		FROM work_order_lines WHERE work_order_id = $1 ORDER BY line_number ASC`, workOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []model.WorkOrderLine{}
	for rows.Next() {
		var l model.WorkOrderLine
		if err := rows.Scan(&l.ID, &l.WorkOrderID, &l.LineNumber, &l.ComponentProductID, &l.QuantityRequired); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (h *Handler) getWorkOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var wo model.WorkOrder
	err := scanWorkOrder(h.pool.QueryRow(ctx, `SELECT `+workOrderColumns+` FROM work_orders WHERE id = $1`, id), &wo)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Work order tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat work order")
		return
	}

	lines, err := h.fetchWorkOrderLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris kebutuhan komponen")
		return
	}
	writeJSON(w, http.StatusOK, workOrderWithLines{WorkOrder: wo, Lines: lines})
}

func (h *Handler) startWorkOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var wo model.WorkOrder
	err := scanWorkOrder(h.pool.QueryRow(r.Context(), `
		UPDATE work_orders SET status = 'IN_PROGRESS', updated_at = now() WHERE id = $1 AND status = 'DRAFT'
		RETURNING `+workOrderColumns, id), &wo)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Work order tidak ditemukan atau tidak berstatus DRAFT")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status work order")
		return
	}

	h.events.Publish("production.work_order.started", newAuditEvent("production.work_order.started", actor, &wo.CompanyID, "update", "work_order", wo.ID, wo))
	writeJSON(w, http.StatusOK, wo)
}

type completeWorkOrderRequest struct {
	QuantityProduced float64 `json:"quantity_produced"`
}

// completeWorkOrder mengonsumsi komponen (stock OUT per work_order_lines)
// dan menambah produk jadi (stock IN sebanyak quantity_produced) di
// warehouse-service -- panggilan warehouse-service dulu, baru update status
// lokal setelah sukses, konsisten dengan pola invoicePurchaseOrder/
// receivePurchaseOrder saat memanggil service lain.
func (h *Handler) completeWorkOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req completeWorkOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.QuantityProduced <= 0 {
		writeError(w, http.StatusBadRequest, "quantity_produced harus lebih besar dari 0")
		return
	}

	var wo model.WorkOrder
	err := scanWorkOrder(h.pool.QueryRow(ctx, `SELECT `+workOrderColumns+` FROM work_orders WHERE id = $1`, id), &wo)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Work order tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat work order")
		return
	}
	if wo.Status != "IN_PROGRESS" {
		writeError(w, http.StatusConflict, "Work order tidak ditemukan atau tidak berstatus IN_PROGRESS")
		return
	}

	if msg, err := h.checkProductionRuns(ctx, wo.ID, req.QuantityProduced); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa production run")
		return
	} else if msg != "" {
		writeError(w, http.StatusConflict, msg)
		return
	}

	lines, err := h.fetchWorkOrderLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris kebutuhan komponen")
		return
	}

	componentLines := make([]warehouseclient.MovementLineInput, 0, len(lines))
	for _, l := range lines {
		componentLines = append(componentLines, warehouseclient.MovementLineInput{
			ProductID: l.ComponentProductID,
			Quantity:  l.QuantityRequired,
		})
	}

	err = h.warehouse.PostMovementBatch(headerValue(actor), warehouseclient.PostMovementBatchRequest{
		CompanyID:     wo.CompanyID,
		BranchID:      wo.BranchID,
		WarehouseID:   wo.WarehouseID,
		MovementType:  "OUT",
		ReferenceType: "WORK_ORDER",
		ReferenceID:   wo.ID,
		Notes:         "Konsumsi bahan baku " + wo.WONumber,
		Lines:         componentLines,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Gagal mencatat konsumsi bahan baku di warehouse-service: %v", err))
		return
	}

	err = h.warehouse.PostMovementBatch(headerValue(actor), warehouseclient.PostMovementBatchRequest{
		CompanyID:     wo.CompanyID,
		BranchID:      wo.BranchID,
		WarehouseID:   wo.WarehouseID,
		MovementType:  "IN",
		ReferenceType: "WORK_ORDER",
		ReferenceID:   wo.ID,
		Notes:         "Hasil produksi " + wo.WONumber,
		Lines: []warehouseclient.MovementLineInput{
			{ProductID: wo.ProductID, Quantity: req.QuantityProduced},
		},
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Bahan baku sudah dikonsumsi, tetapi gagal mencatat hasil produksi di warehouse-service: %v", err))
		return
	}

	err = scanWorkOrder(h.pool.QueryRow(ctx, `
		UPDATE work_orders SET status = 'COMPLETED', quantity_produced = $1, updated_at = now()
		WHERE id = $2 AND status = 'IN_PROGRESS'
		RETURNING `+workOrderColumns, req.QuantityProduced, id), &wo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Stok berhasil dicatat di warehouse-service, tetapi gagal memperbarui status work order lokal")
		return
	}

	h.events.Publish("production.work_order.completed", newAuditEvent("production.work_order.completed", actor, &wo.CompanyID, "update", "work_order", wo.ID, wo))
	writeJSON(w, http.StatusOK, wo)
}

// checkProductionRuns menjahit lapisan MES (Fase 3) ke penyelesaian work
// order. Aturannya: SELAMA sebuah WO tidak punya production run, perilakunya
// persis seperti sebelum MES ada -- angka hasil produksi diketik saat
// menyelesaikan WO. Begitu ada run, lantai produksi yang jadi sumber
// kebenarannya:
//
//   - masih ada run OPEN berarti shift-nya belum ditutup; menyelesaikan WO
//     sekarang akan memutasi stok memakai angka yang belum lengkap.
//   - quantity_produced harus sama dengan jumlah quantity_good seluruh run.
//     Bukan dipaksa diam-diam sama: kalau operator mengetik angka lain,
//     salah satu dari keduanya salah, dan itu harus diketahui orangnya --
//     bukan ditimpa oleh service dan hilang begitu saja.
//
// Unit reject sengaja TIDAK ikut ditambahkan ke stok produk jadi; yang masuk
// gudang hanya yang lolos.
func (h *Handler) checkProductionRuns(ctx context.Context, workOrderID string, quantityProduced float64) (string, error) {
	var runCount, openRuns int
	var totalGood float64
	err := h.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'OPEN'),
		       COALESCE(SUM(quantity_good), 0)
		FROM production_runs WHERE work_order_id = $1`, workOrderID,
	).Scan(&runCount, &openRuns, &totalGood)
	if err != nil {
		return "", err
	}
	if runCount == 0 {
		return "", nil
	}
	if openRuns > 0 {
		return fmt.Sprintf("Masih ada %d production run yang belum ditutup untuk work order ini", openRuns), nil
	}
	// Perbandingan angka pecahan: beda di bawah 0,0001 unit adalah sisa
	// pembulatan NUMERIC, bukan selisih yang berarti di lantai produksi.
	if math.Abs(totalGood-quantityProduced) > 0.0001 {
		return fmt.Sprintf("quantity_produced (%g) tidak sama dengan total hasil bagus seluruh production run (%g)", quantityProduced, totalGood), nil
	}
	return "", nil
}

func nextSequence(ctx context.Context, tx pgx.Tx, companyID, table, column, prefix, period string) (string, error) {
	var count int
	likePattern := prefix + "-" + strings.ReplaceAll(period, "-", "") + "-%"
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE company_id = $1 AND %s LIKE $2`, table, column)
	if err := tx.QueryRow(ctx, query, companyID, likePattern).Scan(&count); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%04d", prefix, strings.ReplaceAll(period, "-", ""), count+1), nil
}
