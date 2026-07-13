package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/warehouse-service/internal/model"
)

const stockOpnameColumns = `id, company_id, branch_id, warehouse_id, opname_number, opname_date, status, notes, created_at, updated_at`

func scanStockOpname(row pgx.Row, o *model.StockOpname) error {
	return row.Scan(&o.ID, &o.CompanyID, &o.BranchID, &o.WarehouseID, &o.OpnameNumber, &o.OpnameDate, &o.Status, &o.Notes, &o.CreatedAt, &o.UpdatedAt)
}

func (h *Handler) listStockOpnames(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + stockOpnameColumns + ` FROM stock_opnames WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY opname_date DESC, opname_number DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data stock opname")
		return
	}
	defer rows.Close()

	opnames := []model.StockOpname{}
	for rows.Next() {
		var o model.StockOpname
		if err := scanStockOpname(rows, &o); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data stock opname")
			return
		}
		opnames = append(opnames, o)
	}
	writeJSON(w, http.StatusOK, opnames)
}

type opnameLineInput struct {
	ProductID       string  `json:"product_id"`
	CountedQuantity float64 `json:"counted_quantity"`
}

type createStockOpnameRequest struct {
	CompanyID   string            `json:"company_id"`
	BranchID    *string           `json:"branch_id"`
	WarehouseID string            `json:"warehouse_id"`
	OpnameDate  string            `json:"opname_date"`
	Notes       string            `json:"notes"`
	Lines       []opnameLineInput `json:"lines"`
}

type stockOpnameWithLines struct {
	model.StockOpname
	Lines []model.StockOpnameLine `json:"lines"`
}

// createStockOpname men-snapshot system_quantity dari stock_balances saat
// baris dibuat (status DRAFT). Selisih baru dihitung & diterapkan ke stok
// saat di-POST (lihat postStockOpname), supaya user bisa mengoreksi hasil
// hitung fisik sebelum benar-benar menyentuh saldo.
func (h *Handler) createStockOpname(w http.ResponseWriter, r *http.Request) {
	var req createStockOpnameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.WarehouseID == "" || req.OpnameDate == "" || len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "company_id, warehouse_id, opname_date, dan minimal 1 baris wajib diisi")
		return
	}
	opnameDate, err := time.Parse("2006-01-02", req.OpnameDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "opname_date harus format YYYY-MM-DD")
		return
	}
	for _, l := range req.Lines {
		if l.ProductID == "" || l.CountedQuantity < 0 {
			writeError(w, http.StatusBadRequest, "Setiap baris wajib punya product_id dan counted_quantity >= 0")
			return
		}
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	period := req.OpnameDate[:7]
	opnameNumber, err := nextSequence(ctx, tx, req.CompanyID, "stock_opnames", "opname_number", "OPN", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor stock opname")
		return
	}

	var o model.StockOpname
	err = scanStockOpname(tx.QueryRow(ctx, `
		INSERT INTO stock_opnames (company_id, branch_id, warehouse_id, opname_number, opname_date, notes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+stockOpnameColumns,
		req.CompanyID, req.BranchID, req.WarehouseID, opnameNumber, opnameDate, req.Notes,
	), &o)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat stock opname")
		return
	}

	lines := make([]model.StockOpnameLine, 0, len(req.Lines))
	for _, l := range req.Lines {
		var systemQuantity float64
		err := tx.QueryRow(ctx, `SELECT quantity FROM stock_balances WHERE warehouse_id = $1 AND product_id = $2`, req.WarehouseID, l.ProductID).Scan(&systemQuantity)
		if err != nil && err != pgx.ErrNoRows {
			writeError(w, http.StatusInternalServerError, "Gagal memuat saldo stok")
			return
		}

		var line model.StockOpnameLine
		err = tx.QueryRow(ctx, `
			INSERT INTO stock_opname_lines (opname_id, product_id, system_quantity, counted_quantity)
			VALUES ($1, $2, $3, $4)
			RETURNING id, opname_id, product_id, system_quantity, counted_quantity`,
			o.ID, l.ProductID, systemQuantity, l.CountedQuantity,
		).Scan(&line.ID, &line.OpnameID, &line.ProductID, &line.SystemQuantity, &line.CountedQuantity)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris stock opname")
			return
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan stock opname")
		return
	}

	h.events.Publish("warehouse.opname.created", newAuditEvent("warehouse.opname.created", actorFromHeader(r), &o.CompanyID, "create", "stock_opname", o.ID, o))
	writeJSON(w, http.StatusCreated, stockOpnameWithLines{StockOpname: o, Lines: lines})
}

func (h *Handler) fetchStockOpnameLines(ctx context.Context, opnameID string) ([]model.StockOpnameLine, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT sol.id, sol.opname_id, sol.product_id, sol.system_quantity, sol.counted_quantity, p.sku, p.name
		FROM stock_opname_lines sol
		JOIN products p ON p.id = sol.product_id
		WHERE sol.opname_id = $1 ORDER BY p.name ASC`, opnameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []model.StockOpnameLine{}
	for rows.Next() {
		var l model.StockOpnameLine
		if err := rows.Scan(&l.ID, &l.OpnameID, &l.ProductID, &l.SystemQuantity, &l.CountedQuantity, &l.ProductSKU, &l.ProductName); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (h *Handler) getStockOpname(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var o model.StockOpname
	err := scanStockOpname(h.pool.QueryRow(ctx, `SELECT `+stockOpnameColumns+` FROM stock_opnames WHERE id = $1`, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Stock opname tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat stock opname")
		return
	}

	lines, err := h.fetchStockOpnameLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris stock opname")
		return
	}
	writeJSON(w, http.StatusOK, stockOpnameWithLines{StockOpname: o, Lines: lines})
}

// postStockOpname menghitung selisih (counted_quantity - system_quantity)
// tiap baris dan menerapkannya sebagai stock_movements ADJUSTMENT (reference
// type OPNAME), sehingga saldo stok akhirnya sama dengan hasil hitung fisik.
// Baris dengan selisih 0 dilewati (tidak ada mutasi yang perlu dicatat).
func (h *Handler) postStockOpname(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var o model.StockOpname
	err := scanStockOpname(h.pool.QueryRow(ctx, `SELECT `+stockOpnameColumns+` FROM stock_opnames WHERE id = $1`, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Stock opname tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat stock opname")
		return
	}
	if o.Status != "DRAFT" {
		writeError(w, http.StatusConflict, fmt.Sprintf("Stock opname tidak berstatus DRAFT (saat ini %s)", o.Status))
		return
	}

	lines, err := h.fetchStockOpnameLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris stock opname")
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	for _, l := range lines {
		diff := l.CountedQuantity - l.SystemQuantity
		if diff == 0 {
			continue
		}
		movementType := "IN"
		quantity := diff
		if diff < 0 {
			movementType = "OUT"
			quantity = -diff
		}
		if _, err := applyStockMovement(ctx, tx, o.CompanyID, o.BranchID, o.WarehouseID, l.ProductID, movementType, quantity, "OPNAME", &o.ID, "Stock opname "+o.OpnameNumber, o.OpnameDate, actor); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mencatat penyesuaian stok")
			return
		}
	}

	err = scanStockOpname(tx.QueryRow(ctx, `
		UPDATE stock_opnames SET status = 'POSTED', updated_at = now() WHERE id = $1 AND status = 'DRAFT'
		RETURNING `+stockOpnameColumns, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Stock opname sudah diproses pihak lain")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status stock opname")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan hasil stock opname")
		return
	}

	h.events.Publish("warehouse.opname.posted", newAuditEvent("warehouse.opname.posted", actor, &o.CompanyID, "update", "stock_opname", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}
