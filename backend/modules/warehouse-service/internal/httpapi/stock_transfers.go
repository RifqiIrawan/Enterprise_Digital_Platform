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

	"github.com/enterprise-digital-platform/warehouse-service/internal/model"
)

const stockTransferColumns = `id, company_id, branch_id, transfer_number, from_warehouse_id, to_warehouse_id, transfer_date, status, notes, created_at, updated_at`

func scanStockTransfer(row pgx.Row, t *model.StockTransfer) error {
	return row.Scan(&t.ID, &t.CompanyID, &t.BranchID, &t.TransferNumber, &t.FromWarehouseID, &t.ToWarehouseID, &t.TransferDate, &t.Status, &t.Notes, &t.CreatedAt, &t.UpdatedAt)
}

func (h *Handler) listStockTransfers(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + stockTransferColumns + ` FROM stock_transfers WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY transfer_date DESC, transfer_number DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data mutasi antar gudang")
		return
	}
	defer rows.Close()

	transfers := []model.StockTransfer{}
	for rows.Next() {
		var t model.StockTransfer
		if err := scanStockTransfer(rows, &t); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data mutasi antar gudang")
			return
		}
		transfers = append(transfers, t)
	}
	writeJSON(w, http.StatusOK, transfers)
}

type transferLineInput struct {
	ProductID string  `json:"product_id"`
	Quantity  float64 `json:"quantity"`
}

type createStockTransferRequest struct {
	CompanyID       string              `json:"company_id"`
	BranchID        *string             `json:"branch_id"`
	FromWarehouseID string              `json:"from_warehouse_id"`
	ToWarehouseID   string              `json:"to_warehouse_id"`
	TransferDate    string              `json:"transfer_date"`
	Notes           string              `json:"notes"`
	Lines           []transferLineInput `json:"lines"`
}

type stockTransferWithLines struct {
	model.StockTransfer
	Lines []model.StockTransferLine `json:"lines"`
}

func (h *Handler) createStockTransfer(w http.ResponseWriter, r *http.Request) {
	var req createStockTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.FromWarehouseID == "" || req.ToWarehouseID == "" || req.TransferDate == "" || len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "company_id, from_warehouse_id, to_warehouse_id, transfer_date, dan minimal 1 baris wajib diisi")
		return
	}
	if req.FromWarehouseID == req.ToWarehouseID {
		writeError(w, http.StatusBadRequest, "Gudang asal dan tujuan tidak boleh sama")
		return
	}
	transferDate, err := time.Parse("2006-01-02", req.TransferDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "transfer_date harus format YYYY-MM-DD")
		return
	}
	for _, l := range req.Lines {
		if l.ProductID == "" || l.Quantity <= 0 {
			writeError(w, http.StatusBadRequest, "Setiap baris wajib punya product_id dan quantity > 0")
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

	period := req.TransferDate[:7]
	transferNumber, err := nextSequence(ctx, tx, req.CompanyID, "stock_transfers", "transfer_number", "TRF", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor transfer")
		return
	}

	var t model.StockTransfer
	err = scanStockTransfer(tx.QueryRow(ctx, `
		INSERT INTO stock_transfers (company_id, branch_id, transfer_number, from_warehouse_id, to_warehouse_id, transfer_date, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+stockTransferColumns,
		req.CompanyID, req.BranchID, transferNumber, req.FromWarehouseID, req.ToWarehouseID, transferDate, req.Notes,
	), &t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat mutasi antar gudang")
		return
	}

	lines := make([]model.StockTransferLine, 0, len(req.Lines))
	for i, l := range req.Lines {
		var line model.StockTransferLine
		err := tx.QueryRow(ctx, `
			INSERT INTO stock_transfer_lines (transfer_id, line_number, product_id, quantity)
			VALUES ($1, $2, $3, $4)
			RETURNING id, transfer_id, line_number, product_id, quantity`,
			t.ID, i+1, l.ProductID, l.Quantity,
		).Scan(&line.ID, &line.TransferID, &line.LineNumber, &line.ProductID, &line.Quantity)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris mutasi")
			return
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan mutasi antar gudang")
		return
	}

	h.events.Publish("warehouse.transfer.created", newAuditEvent("warehouse.transfer.created", actorFromHeader(r), &t.CompanyID, "create", "stock_transfer", t.ID, t))
	writeJSON(w, http.StatusCreated, stockTransferWithLines{StockTransfer: t, Lines: lines})
}

func (h *Handler) fetchStockTransferLines(ctx context.Context, transferID string) ([]model.StockTransferLine, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT stl.id, stl.transfer_id, stl.line_number, stl.product_id, stl.quantity, p.sku, p.name
		FROM stock_transfer_lines stl
		JOIN products p ON p.id = stl.product_id
		WHERE stl.transfer_id = $1 ORDER BY stl.line_number ASC`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []model.StockTransferLine{}
	for rows.Next() {
		var l model.StockTransferLine
		if err := rows.Scan(&l.ID, &l.TransferID, &l.LineNumber, &l.ProductID, &l.Quantity, &l.ProductSKU, &l.ProductName); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (h *Handler) getStockTransfer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var t model.StockTransfer
	err := scanStockTransfer(h.pool.QueryRow(ctx, `SELECT `+stockTransferColumns+` FROM stock_transfers WHERE id = $1`, id), &t)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Mutasi antar gudang tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat mutasi antar gudang")
		return
	}

	lines, err := h.fetchStockTransferLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris mutasi")
		return
	}
	writeJSON(w, http.StatusOK, stockTransferWithLines{StockTransfer: t, Lines: lines})
}

// confirmStockTransfer memindahkan stok secara nyata: tiap baris transfer
// menghasilkan sepasang stock_movements (OUT di gudang asal, IN di gudang
// tujuan), reference_type=TRANSFER menunjuk ke transfer.id.
func (h *Handler) confirmStockTransfer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var t model.StockTransfer
	err := scanStockTransfer(h.pool.QueryRow(ctx, `SELECT `+stockTransferColumns+` FROM stock_transfers WHERE id = $1`, id), &t)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Mutasi antar gudang tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat mutasi antar gudang")
		return
	}
	if t.Status != "DRAFT" {
		writeError(w, http.StatusConflict, fmt.Sprintf("Mutasi antar gudang tidak berstatus DRAFT (saat ini %s)", t.Status))
		return
	}

	lines, err := h.fetchStockTransferLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris mutasi")
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	for _, l := range lines {
		if _, err := applyStockMovement(ctx, tx, t.CompanyID, t.BranchID, t.FromWarehouseID, l.ProductID, "OUT", l.Quantity, "TRANSFER", &t.ID, "Mutasi "+t.TransferNumber, t.TransferDate, actor); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mencatat stok keluar dari gudang asal")
			return
		}
		if _, err := applyStockMovement(ctx, tx, t.CompanyID, t.BranchID, t.ToWarehouseID, l.ProductID, "IN", l.Quantity, "TRANSFER", &t.ID, "Mutasi "+t.TransferNumber, t.TransferDate, actor); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mencatat stok masuk ke gudang tujuan")
			return
		}
	}

	err = scanStockTransfer(tx.QueryRow(ctx, `
		UPDATE stock_transfers SET status = 'CONFIRMED', updated_at = now() WHERE id = $1 AND status = 'DRAFT'
		RETURNING `+stockTransferColumns, id), &t)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Mutasi antar gudang sudah diproses pihak lain")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status mutasi antar gudang")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan konfirmasi mutasi antar gudang")
		return
	}

	h.events.Publish("warehouse.transfer.confirmed", newAuditEvent("warehouse.transfer.confirmed", actor, &t.CompanyID, "update", "stock_transfer", t.ID, t))
	writeJSON(w, http.StatusOK, t)
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
