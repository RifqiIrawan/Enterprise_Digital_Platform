package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/warehouse-service/internal/model"
)

var validReferenceTypes = map[string]bool{
	"PURCHASE_ORDER":  true,
	"SALES_ORDER":     true,
	"TRANSFER":        true,
	"OPNAME":          true,
	"MANUAL":          true,
	"WORK_ORDER":      true,
	"ECOMMERCE_ORDER": true, // stok keluar dari ecommerce-service saat order SHIPPED, lihat internal/warehouseclient di sana
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
		INSERT INTO products (company_id, sku, name, unit, category)
		VALUES ($1, $2, $3, $4, '')
		RETURNING `+productColumns, companyID, sku, name, unit), &p)
	return p, err
}

// insufficientStockError: sebuah mutasi OUT akan membuat saldo di bawah nol.
// Stok tidak boleh negatif -- saldo negatif berarti barang keluar yang tidak
// pernah tercatat masuk, dan laporan stok serta HPP yang dibangun di atasnya
// ikut salah. Penanganannya di pemanggil: 409 dengan pesan yang menyebut angka.
type insufficientStockError struct {
	WarehouseID string
	ProductID   string
	Requested   float64
	Available   float64
	// Label diisi saat galat dibuat (SKU - nama produk, kode gudang) supaya
	// pesannya bisa dibaca orang, bukan UUID.
	ProductLabel   string
	WarehouseLabel string
}

func (e *insufficientStockError) Error() string {
	return fmt.Sprintf("Stok tidak cukup untuk %s di gudang %s: diminta %s, tersedia %s",
		e.ProductLabel, e.WarehouseLabel, formatQty(e.Requested), formatQty(e.Available))
}

func formatQty(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// writeStockError menerjemahkan galat applyStockMovement: stok tidak cukup
// adalah 409 yang bisa ditindaklanjuti pengguna, sisanya galat server.
func writeStockError(w http.ResponseWriter, err error, fallback string) {
	var ins *insufficientStockError
	if errors.As(err, &ins) {
		writeErrorCode(w, http.StatusConflict, ins.Error(), "INSUFFICIENT_STOCK")
		return
	}
	writeError(w, http.StatusInternalServerError, fallback)
}

// applyStockMovement mencatat satu baris stock_movements dan menyelaraskan
// stock_balances (increment untuk IN, decrement untuk OUT) dalam transaksi
// yang sama, dipakai oleh seluruh alur yang menggerakkan stok (manual entry,
// batch dari PO/SO/pesanan e-commerce/work order, transfer confirm, opname post).
//
// OUT hanya berhasil kalau saldo cukup: pengurangan ditulis sebagai UPDATE
// bersyarat (quantity >= diminta), jadi dua OUT bersamaan atas saldo yang sama
// tidak bisa sama-sama lolos -- baris saldo terkunci oleh UPDATE pertama dan
// yang kedua dievaluasi terhadap angka barunya. IN tidak pernah ditolak, juga
// pada saldo yang sudah negatif dari masa sebelum aturan ini: barang masuk
// justru cara saldo itu pulih.
func applyStockMovement(ctx context.Context, tx pgx.Tx, companyID string, branchID *string, warehouseID, productID, movementType string, quantity float64, referenceType string, referenceID *string, notes string, movementDate time.Time, actor *string) (model.StockMovement, error) {
	var mv model.StockMovement

	if movementType == "OUT" {
		tag, err := tx.Exec(ctx, `
			UPDATE stock_balances SET quantity = quantity - $3, updated_at = now()
			WHERE warehouse_id = $1 AND product_id = $2 AND quantity >= $3`,
			warehouseID, productID, quantity)
		if err != nil {
			return mv, err
		}
		if tag.RowsAffected() == 0 {
			return mv, newInsufficientStockError(ctx, tx, warehouseID, productID, quantity)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_balances (warehouse_id, product_id, quantity, updated_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (warehouse_id, product_id) DO UPDATE SET quantity = stock_balances.quantity + $3, updated_at = now()`,
			warehouseID, productID, quantity); err != nil {
			return mv, err
		}
	}

	err := tx.QueryRow(ctx, `
		INSERT INTO stock_movements (company_id, branch_id, warehouse_id, product_id, movement_type, quantity, reference_type, reference_id, notes, movement_date, actor_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, company_id, branch_id, warehouse_id, product_id, movement_type, quantity, reference_type, reference_id, notes, movement_date, actor_user_id, created_at`,
		companyID, branchID, warehouseID, productID, movementType, quantity, referenceType, referenceID, notes, movementDate, actor,
	).Scan(&mv.ID, &mv.CompanyID, &mv.BranchID, &mv.WarehouseID, &mv.ProductID, &mv.MovementType, &mv.Quantity, &mv.ReferenceType, &mv.ReferenceID, &mv.Notes, &mv.MovementDate, &mv.ActorUserID, &mv.CreatedAt)
	return mv, err
}

// newInsufficientStockError membaca saldo saat ini dan label produk/gudang
// untuk pesan galat. Hanya jalan di jalur gagal, jadi dua query tambahan itu
// tidak membebani mutasi yang normal.
func newInsufficientStockError(ctx context.Context, tx pgx.Tx, warehouseID, productID string, requested float64) error {
	e := &insufficientStockError{WarehouseID: warehouseID, ProductID: productID, Requested: requested,
		ProductLabel: productID, WarehouseLabel: warehouseID}
	_ = tx.QueryRow(ctx, `SELECT COALESCE((SELECT quantity FROM stock_balances WHERE warehouse_id = $1 AND product_id = $2), 0)`,
		warehouseID, productID).Scan(&e.Available)
	var sku, name string
	if tx.QueryRow(ctx, `SELECT sku, name FROM products WHERE id = $1`, productID).Scan(&sku, &name) == nil {
		e.ProductLabel = sku + " - " + name
	}
	var code string
	if tx.QueryRow(ctx, `SELECT code FROM warehouses WHERE id = $1`, warehouseID).Scan(&code) == nil {
		e.WarehouseLabel = code
	}
	return e
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
		args = append(args, warehouseID)
		query += ` AND sm.warehouse_id = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (sm.branch_id = $` + strconv.Itoa(len(args)) + ` OR sm.branch_id IS NULL)`
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
		writeStockError(w, err, "Gagal mencatat mutasi stok")
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
	ProductID   string  `json:"product_id"`
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
		if (l.ProductID == "" && strings.TrimSpace(l.ProductName) == "") || l.Quantity <= 0 {
			writeError(w, http.StatusBadRequest, "Setiap baris wajib punya product_id atau product_name, dan quantity > 0")
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
		var product model.Product
		var err error
		if l.ProductID != "" {
			err = scanProduct(tx.QueryRow(ctx, `SELECT `+productColumns+` FROM products WHERE id = $1`, l.ProductID), &product)
			if err == pgx.ErrNoRows {
				writeError(w, http.StatusBadRequest, "Produk tidak ditemukan: "+l.ProductID)
				return
			}
		} else {
			product, err = findOrCreateProduct(ctx, tx, req.CompanyID, strings.TrimSpace(l.ProductName), l.Unit)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mencocokkan produk")
			return
		}
		mv, err := applyStockMovement(ctx, tx, req.CompanyID, req.BranchID, req.WarehouseID, product.ID, req.MovementType, l.Quantity, req.ReferenceType, referenceID, req.Notes, movementDate, actor)
		if err != nil {
			writeStockError(w, err, "Gagal mencatat mutasi stok untuk produk: "+l.ProductName)
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
