package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/warehouse-service/internal/model"
)

var validReferenceTypes = map[string]bool{
	"PURCHASE_ORDER": true,
	"SALES_ORDER":    true,
	"TRANSFER":       true,
	"OPNAME":         true,
	"MANUAL":         true,
}

// findOrCreateProduct mencocokkan product_name teks bebas (dari PO/SO
// purchasing-service/sales-service, atau input manual) ke product master
// lewat (company_id, name); kalau belum ada, dibuat otomatis dengan SKU
// auto-generate supaya PO/SO tidak perlu tahu/mengelola product_id.
func findOrCreateProduct(ctx context.Context, tx pgx.Tx, companyID, name, unit string) (model.Product, error) {
	var p model.Product
	err := scanProduct(tx.QueryRow(ctx, `SELECT `+productColumns+` FROM products WHERE company_id = $1 AND name = $2`, companyID, name), &p)
	if err == nil {
		return p, nil
	}
	if err != pgx.ErrNoRows {
		return p, err
	}

	if unit == "" {
		unit = "pcs"
	}
	sku := "AUTO-" + strings.ToUpper(uuid.NewString()[:8])
	err = scanProduct(tx.QueryRow(ctx, `
		INSERT INTO products (company_id, sku, name, unit)
		VALUES ($1, $2, $3, $4)
		RETURNING `+productColumns, companyID, sku, name, unit), &p)
	return p, err
}

// applyStockMovement mencatat satu baris stock_movements dan menyelaraskan
// stock_balances (increment untuk IN, decrement untuk OUT) dalam transaksi
// yang sama, dipakai oleh seluruh alur yang menggerakkan stok (manual entry,
// batch dari PO/SO, transfer confirm, opname post).
func applyStockMovement(ctx context.Context, tx pgx.Tx, companyID string, branchID *string, warehouseID, productID, movementType string, quantity float64, referenceType string, referenceID *string, notes string, movementDate time.Time, actor *string) (model.StockMovement, error) {
	var mv model.StockMovement
	err := tx.QueryRow(ctx, `
		INSERT INTO stock_movements (company_id, branch_id, warehouse_id, product_id, movement_type, quantity, reference_type, reference_id, notes, movement_date, actor_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, company_id, branch_id, warehouse_id, product_id, movement_type, quantity, reference_type, reference_id, notes, movement_date, actor_user_id, created_at`,
		companyID, branchID, warehouseID, productID, movementType, quantity, referenceType, referenceID, notes, movementDate, actor,
	).Scan(&mv.ID, &mv.CompanyID, &mv.BranchID, &mv.WarehouseID, &mv.ProductID, &mv.MovementType, &mv.Quantity, &mv.ReferenceType, &mv.ReferenceID, &mv.Notes, &mv.MovementDate, &mv.ActorUserID, &mv.CreatedAt)
	if err != nil {
		return mv, err
	}

	delta := quantity
	if movementType == "OUT" {
		delta = -quantity
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO stock_balances (warehouse_id, product_id, quantity, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (warehouse_id, product_id) DO UPDATE SET quantity = stock_balances.quantity + $3, updated_at = now()`,
		warehouseID, productID, delta,
	)
	return mv, err
}

func (h *Handler) listStockBalances(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	warehouseID := r.URL.Query().Get("warehouse_id")

	query := `
		SELECT sb.id, sb.warehouse_id, sb.product_id, sb.quantity, sb.updated_at, p.sku, p.name, p.unit
		FROM stock_balances sb
		JOIN warehouses wh ON wh.id = sb.warehouse_id
		JOIN products p ON p.id = sb.product_id
		WHERE wh.company_id = $1`
	args := []any{companyID}
	if warehouseID != "" {
		query += ` AND sb.warehouse_id = $2`
		args = append(args, warehouseID)
	}
	query += ` ORDER BY p.name ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data stok")
		return
	}
	defer rows.Close()

	balances := []model.StockBalance{}
	for rows.Next() {
		var b model.StockBalance
		if err := rows.Scan(&b.ID, &b.WarehouseID, &b.ProductID, &b.Quantity, &b.UpdatedAt, &b.ProductSKU, &b.ProductName, &b.ProductUnit); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data stok")
			return
		}
		balances = append(balances, b)
	}
	writeJSON(w, http.StatusOK, balances)
}

func (h *Handler) listStockMovements(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	warehouseID := r.URL.Query().Get("warehouse_id")

	query := `
		SELECT sm.id, sm.company_id, sm.branch_id, sm.warehouse_id, sm.product_id, sm.movement_type, sm.quantity,
		       sm.reference_type, sm.reference_id, sm.notes, sm.movement_date, sm.actor_user_id, sm.created_at,
		       p.sku, p.name
		FROM stock_movements sm
		JOIN products p ON p.id = sm.product_id
		WHERE sm.company_id = $1`
	args := []any{companyID}
	if warehouseID != "" {
		query += ` AND sm.warehouse_id = $2`
		args = append(args, warehouseID)
	}
	query += ` ORDER BY sm.movement_date DESC, sm.created_at DESC LIMIT 200`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data mutasi stok")
		return
	}
	defer rows.Close()

	movements := []model.StockMovement{}
	for rows.Next() {
		var m model.StockMovement
		if err := rows.Scan(&m.ID, &m.CompanyID, &m.BranchID, &m.WarehouseID, &m.ProductID, &m.MovementType, &m.Quantity,
			&m.ReferenceType, &m.ReferenceID, &m.Notes, &m.MovementDate, &m.ActorUserID, &m.CreatedAt, &m.ProductSKU, &m.ProductName); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data mutasi stok")
			return
		}
		movements = append(movements, m)
	}
	writeJSON(w, http.StatusOK, movements)
}

type manualStockMovementRequest struct {
	CompanyID    string  `json:"company_id"`
	BranchID     *string `json:"branch_id"`
	WarehouseID  string  `json:"warehouse_id"`
	ProductID    string  `json:"product_id"`
	MovementType string  `json:"movement_type"`
	Quantity     float64 `json:"quantity"`
	Notes        string  `json:"notes"`
	MovementDate string  `json:"movement_date"`
}

// createManualStockMovement adalah entry point untuk pencatatan stok manual
// dari UI (mis. stok awal, koreksi kecil) -- bukan hasil PO/SO/transfer/opname.
func (h *Handler) createManualStockMovement(w http.ResponseWriter, r *http.Request) {
	var req manualStockMovementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.WarehouseID == "" || req.ProductID == "" {
		writeError(w, http.StatusBadRequest, "company_id, warehouse_id, dan product_id wajib diisi")
		return
	}
	if req.MovementType != "IN" && req.MovementType != "OUT" {
		writeError(w, http.StatusBadRequest, "movement_type harus IN atau OUT")
		return
	}
	if req.Quantity <= 0 {
		writeError(w, http.StatusBadRequest, "quantity harus lebih besar dari 0")
		return
	}

	movementDate := time.Now()
	if req.MovementDate != "" {
		parsed, err := time.Parse("2006-01-02", req.MovementDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "movement_date harus format YYYY-MM-DD")
			return
		}
		movementDate = parsed
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	actor := actorFromHeader(r)
	mv, err := applyStockMovement(ctx, tx, req.CompanyID, req.BranchID, req.WarehouseID, req.ProductID, req.MovementType, req.Quantity, "MANUAL", nil, req.Notes, movementDate, actor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mencatat mutasi stok")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan mutasi stok")
		return
	}

	h.events.Publish("warehouse.stock.moved", newAuditEvent("warehouse.stock.moved", actor, &req.CompanyID, "create", "stock_movement", mv.ID, mv))
	writeJSON(w, http.StatusCreated, mv)
}

type stockMovementLineInput struct {
	ProductName string  `json:"product_name"`
	Unit        string  `json:"unit"`
	Quantity    float64 `json:"quantity"`
}

type stockMovementBatchRequest struct {
	CompanyID     string                   `json:"company_id"`
	BranchID      *string                  `json:"branch_id"`
	WarehouseID   string                   `json:"warehouse_id"`
	MovementType  string                   `json:"movement_type"`
	ReferenceType string                   `json:"reference_type"`
	ReferenceID   string                   `json:"reference_id"`
	Notes         string                   `json:"notes"`
	MovementDate  string                   `json:"movement_date"`
	Lines         []stockMovementLineInput `json:"lines"`
}

// postStockMovementBatch dipanggil langsung (service-to-service, tidak lewat
// api-gateway) oleh purchasing-service saat PO RECEIVED (movement_type=IN,
// reference_type=PURCHASE_ORDER) dan sales-service saat SO FULFILLED
// (movement_type=OUT, reference_type=SALES_ORDER) -- lihat
// internal/warehouseclient di kedua service tsb. Produk dicocokkan lewat
// product_name karena PO/SO belum punya product_id (lihat komentar di
// migrations/001_init.sql).
func (h *Handler) postStockMovementBatch(w http.ResponseWriter, r *http.Request) {
	var req stockMovementBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.WarehouseID == "" || len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "company_id, warehouse_id, dan minimal 1 baris wajib diisi")
		return
	}
	if req.MovementType != "IN" && req.MovementType != "OUT" {
		writeError(w, http.StatusBadRequest, "movement_type harus IN atau OUT")
		return
	}
	if !validReferenceTypes[req.ReferenceType] {
		writeError(w, http.StatusBadRequest, "reference_type tidak valid")
		return
	}
	for _, l := range req.Lines {
		if strings.TrimSpace(l.ProductName) == "" || l.Quantity <= 0 {
			writeError(w, http.StatusBadRequest, "Setiap baris wajib punya product_name dan quantity > 0")
			return
		}
	}

	movementDate := time.Now()
	if req.MovementDate != "" {
		parsed, err := time.Parse("2006-01-02", req.MovementDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "movement_date harus format YYYY-MM-DD")
			return
		}
		movementDate = parsed
	}

	var referenceID *string
	if req.ReferenceID != "" {
		referenceID = &req.ReferenceID
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	actor := actorFromHeader(r)
	movements := make([]model.StockMovement, 0, len(req.Lines))
	for _, l := range req.Lines {
		product, err := findOrCreateProduct(ctx, tx, req.CompanyID, strings.TrimSpace(l.ProductName), l.Unit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mencocokkan produk: "+l.ProductName)
			return
		}
		mv, err := applyStockMovement(ctx, tx, req.CompanyID, req.BranchID, req.WarehouseID, product.ID, req.MovementType, l.Quantity, req.ReferenceType, referenceID, req.Notes, movementDate, actor)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mencatat mutasi stok untuk produk: "+l.ProductName)
			return
		}
		mv.ProductSKU = product.SKU
		mv.ProductName = product.Name
		movements = append(movements, mv)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan mutasi stok")
		return
	}

	h.events.Publish("warehouse.stock.batch_moved", newAuditEvent("warehouse.stock.batch_moved", actor, &req.CompanyID, "create", "stock_movement_batch", req.ReferenceID, movements))
	writeJSON(w, http.StatusCreated, movements)
}
