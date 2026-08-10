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

	"github.com/enterprise-digital-platform/ecommerce-service/internal/model"
	"github.com/enterprise-digital-platform/ecommerce-service/internal/warehouseclient"
)

const orderColumns = `id, company_id, branch_id, order_number, customer_name, customer_email, shipping_address, status, order_date, total_amount, notes, placed_by_user_id, paid_at, shipped_at, delivered_at, cancelled_at, created_at, updated_at`
const orderItemColumns = `id, company_id, branch_id, order_id, product_id, product_sku, product_name, unit_price, quantity, line_total, created_at`

func scanOrder(row pgx.Row, o *model.Order) error {
	return row.Scan(&o.ID, &o.CompanyID, &o.BranchID, &o.OrderNumber, &o.CustomerName, &o.CustomerEmail, &o.ShippingAddress, &o.Status, &o.OrderDate, &o.TotalAmount, &o.Notes, &o.PlacedByUserID, &o.PaidAt, &o.ShippedAt, &o.DeliveredAt, &o.CancelledAt, &o.CreatedAt, &o.UpdatedAt)
}

func scanOrderItem(row pgx.Row, i *model.OrderItem) error {
	return row.Scan(&i.ID, &i.CompanyID, &i.BranchID, &i.OrderID, &i.ProductID, &i.ProductSKU, &i.ProductName, &i.UnitPrice, &i.Quantity, &i.LineTotal, &i.CreatedAt)
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + orderColumns + ` FROM orders WHERE company_id = $1`
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
		writeError(w, http.StatusInternalServerError, "Gagal memuat data order")
		return
	}
	defer rows.Close()

	orders := []model.Order{}
	for rows.Next() {
		var o model.Order
		if err := scanOrder(rows, &o); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data order")
			return
		}
		orders = append(orders, o)
	}
	writeJSON(w, http.StatusOK, orders)
}

type orderLineRequest struct {
	ProductID   string  `json:"product_id"`
	ProductSKU  string  `json:"product_sku"`
	ProductName string  `json:"product_name"`
	UnitPrice   float64 `json:"unit_price"`
	Quantity    float64 `json:"quantity"`
}

type orderRequest struct {
	CompanyID       string             `json:"company_id"`
	BranchID        *string            `json:"branch_id"`
	CustomerName    string             `json:"customer_name"`
	CustomerEmail   string             `json:"customer_email"`
	ShippingAddress string             `json:"shipping_address"`
	OrderDate       string             `json:"order_date"`
	Notes           string             `json:"notes"`
	Lines           []orderLineRequest `json:"lines"`
}

// createOrder TIDAK memvalidasi product_id/unit_price ke warehouse-service
// secara live -- frontend sudah mengambil daftar produk lewat
// GET /api/warehouse/products untuk mengisi dropdown checkout, jadi
// product_id/product_sku/product_name/unit_price di sini diperlakukan
// sebagai snapshot yang dipercaya, sama seperti sales_order_lines.unit_price
// di sales-service tidak divalidasi ulang ke warehouse-service saat SO
// dibuat. Batasan ini didokumentasikan eksplisit di README, bukan
// diasumsikan aman.
func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var req orderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.CustomerName = strings.TrimSpace(req.CustomerName)
	if req.CompanyID == "" || req.CustomerName == "" {
		writeError(w, http.StatusBadRequest, "company_id dan customer_name wajib diisi")
		return
	}
	if len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "order harus punya minimal 1 baris produk")
		return
	}
	for i, l := range req.Lines {
		if l.ProductID == "" || strings.TrimSpace(l.ProductSKU) == "" || strings.TrimSpace(l.ProductName) == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("baris %d: product_id, product_sku, dan product_name wajib diisi", i+1))
			return
		}
		if l.Quantity <= 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("baris %d: quantity harus lebih dari 0", i+1))
			return
		}
		if l.UnitPrice < 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("baris %d: unit_price tidak boleh negatif", i+1))
			return
		}
	}

	orderDate := time.Now()
	if req.OrderDate != "" {
		parsed, err := time.Parse("2006-01-02", req.OrderDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "order_date harus format YYYY-MM-DD")
			return
		}
		orderDate = parsed
	}

	var totalAmount float64
	for _, l := range req.Lines {
		totalAmount += l.UnitPrice * l.Quantity
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	period := time.Now().Format("2006-01")
	orderNumber, err := nextSequence(ctx, tx, req.CompanyID, "orders", "order_number", "ORD", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor order")
		return
	}

	var o model.Order
	err = scanOrder(tx.QueryRow(ctx, `
		INSERT INTO orders (company_id, branch_id, order_number, customer_name, customer_email, shipping_address, order_date, total_amount, notes, placed_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+orderColumns,
		req.CompanyID, req.BranchID, orderNumber, req.CustomerName, req.CustomerEmail, req.ShippingAddress, orderDate, totalAmount, req.Notes, actorFromHeader(r),
	), &o)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat order")
		return
	}

	items := make([]model.OrderItem, 0, len(req.Lines))
	for _, l := range req.Lines {
		var item model.OrderItem
		err = scanOrderItem(tx.QueryRow(ctx, `
			INSERT INTO order_items (company_id, branch_id, order_id, product_id, product_sku, product_name, unit_price, quantity, line_total)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING `+orderItemColumns,
			req.CompanyID, req.BranchID, o.ID, l.ProductID, l.ProductSKU, l.ProductName, l.UnitPrice, l.Quantity, l.UnitPrice*l.Quantity,
		), &item)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris order")
			return
		}
		items = append(items, item)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan order")
		return
	}

	h.events.Publish("ecommerce.order.created", newAuditEvent("ecommerce.order.created", actorFromHeader(r), &o.CompanyID, "create", "order", o.ID, o))
	writeJSON(w, http.StatusCreated, model.OrderWithItems{Order: o, Items: items})
}

func (h *Handler) fetchOrderItems(ctx context.Context, orderID string) ([]model.OrderItem, error) {
	rows, err := h.pool.Query(ctx, `SELECT `+orderItemColumns+` FROM order_items WHERE order_id = $1 ORDER BY created_at ASC`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []model.OrderItem{}
	for rows.Next() {
		var i model.OrderItem
		if err := scanOrderItem(rows, &i); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, nil
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var o model.Order
	err := scanOrder(h.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id = $1`, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Order tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat order")
		return
	}

	items, err := h.fetchOrderItems(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris order")
		return
	}
	writeJSON(w, http.StatusOK, model.OrderWithItems{Order: o, Items: items})
}

type updateOrderRequest struct {
	CustomerName    string `json:"customer_name"`
	CustomerEmail   string `json:"customer_email"`
	ShippingAddress string `json:"shipping_address"`
	Notes           string `json:"notes"`
}

// updateOrder cuma mengizinkan edit field non-status (customer_name/email/
// shipping_address/notes), dan hanya selagi order masih PENDING -- semua
// transisi status lewat endpoint khusus (pay/ship/deliver/cancel), tidak ada
// PUT generik untuk status seperti tickets.go, mengikuti pola sales_orders
// (confirm/fulfill/invoice) yang lebih cocok untuk dokumen transaksional.
func (h *Handler) updateOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.CustomerName = strings.TrimSpace(req.CustomerName)
	if req.CustomerName == "" {
		writeError(w, http.StatusBadRequest, "customer_name wajib diisi")
		return
	}

	ctx := r.Context()
	var before model.Order
	err := scanOrder(h.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id = $1`, id), &before)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Order tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat order")
		return
	}
	if before.Status != "PENDING" {
		writeError(w, http.StatusConflict, "Order hanya bisa diubah selagi PENDING")
		return
	}

	var o model.Order
	err = scanOrder(h.pool.QueryRow(ctx, `
		UPDATE orders SET customer_name = $1, customer_email = $2, shipping_address = $3, notes = $4, updated_at = now()
		WHERE id = $5
		RETURNING `+orderColumns,
		req.CustomerName, req.CustomerEmail, req.ShippingAddress, req.Notes, id,
	), &o)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui order")
		return
	}

	h.events.Publish("ecommerce.order.updated", newAuditEvent("ecommerce.order.updated", actorFromHeader(r), &o.CompanyID, "update", "order", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) payOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var o model.Order
	err := scanOrder(h.pool.QueryRow(r.Context(), `
		UPDATE orders SET status = 'PAID', paid_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'PENDING'
		RETURNING `+orderColumns, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Order tidak ditemukan atau tidak berstatus PENDING")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses pembayaran order")
		return
	}

	h.events.Publish("ecommerce.order.paid", newAuditEvent("ecommerce.order.paid", actorFromHeader(r), &o.CompanyID, "update", "order", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}

type shipOrderRequest struct {
	WarehouseID string `json:"warehouse_id"`
}

// shipOrder mencatat stok keluar di warehouse-service (lihat
// internal/warehouseclient) sebelum mengubah status order ke SHIPPED --
// panggilan warehouse-service dulu, baru update status lokal setelah sukses,
// pola identik dengan fulfillSalesOrder di sales-service.
func (h *Handler) shipOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req shipOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.WarehouseID == "" {
		writeError(w, http.StatusBadRequest, "warehouse_id wajib diisi")
		return
	}

	var o model.Order
	err := scanOrder(h.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id = $1`, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Order tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat order")
		return
	}
	if o.Status != "PAID" {
		writeError(w, http.StatusConflict, "Order tidak ditemukan atau tidak berstatus PAID")
		return
	}

	items, err := h.fetchOrderItems(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris order")
		return
	}

	warehouseLines := make([]warehouseclient.MovementLineInput, 0, len(items))
	for _, item := range items {
		warehouseLines = append(warehouseLines, warehouseclient.MovementLineInput{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		})
	}

	err = h.warehouse.PostMovementBatch(headerValue(actor), warehouseclient.PostMovementBatchRequest{
		CompanyID:     o.CompanyID,
		BranchID:      o.BranchID,
		WarehouseID:   req.WarehouseID,
		MovementType:  "OUT",
		ReferenceType: "ECOMMERCE_ORDER",
		ReferenceID:   o.ID,
		Notes:         "Pengiriman " + o.OrderNumber,
		MovementDate:  o.OrderDate.Format("2006-01-02"),
		Lines:         warehouseLines,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Gagal mencatat stok keluar di warehouse-service: %v", err))
		return
	}

	err = scanOrder(h.pool.QueryRow(ctx, `
		UPDATE orders SET status = 'SHIPPED', shipped_at = now(), updated_at = now() WHERE id = $1 AND status = 'PAID'
		RETURNING `+orderColumns, id), &o)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Stok berhasil dicatat di warehouse-service, tetapi gagal memperbarui status order lokal")
		return
	}

	h.events.Publish("ecommerce.order.shipped", newAuditEvent("ecommerce.order.shipped", actor, &o.CompanyID, "update", "order", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) deliverOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var o model.Order
	err := scanOrder(h.pool.QueryRow(r.Context(), `
		UPDATE orders SET status = 'DELIVERED', delivered_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'SHIPPED'
		RETURNING `+orderColumns, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Order tidak ditemukan atau tidak berstatus SHIPPED")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses pengiriman selesai")
		return
	}

	h.events.Publish("ecommerce.order.delivered", newAuditEvent("ecommerce.order.delivered", actorFromHeader(r), &o.CompanyID, "update", "order", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}

// cancelOrder hanya diizinkan selagi PENDING atau PAID -- setelah SHIPPED,
// stok sudah keluar dari warehouse-service, jadi pembatalan tidak lagi
// sesederhana ubah status lokal (butuh stock-in balik / retur, di luar
// lingkup modul ini saat ini, lihat README "Belum ada").
func (h *Handler) cancelOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var o model.Order
	err := scanOrder(h.pool.QueryRow(r.Context(), `
		UPDATE orders SET status = 'CANCELLED', cancelled_at = now(), updated_at = now()
		WHERE id = $1 AND status IN ('PENDING', 'PAID')
		RETURNING `+orderColumns, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Order tidak ditemukan atau sudah SHIPPED/DELIVERED/CANCELLED")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membatalkan order")
		return
	}

	h.events.Publish("ecommerce.order.cancelled", newAuditEvent("ecommerce.order.cancelled", actorFromHeader(r), &o.CompanyID, "update", "order", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}
