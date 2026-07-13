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

	"github.com/enterprise-digital-platform/purchasing-service/internal/model"
)

const requisitionColumns = `id, company_id, branch_id, pr_number, requested_by, pr_date, status, subtotal_amount, notes, created_at, updated_at`

func scanRequisition(row pgx.Row, pr *model.Requisition) error {
	return row.Scan(&pr.ID, &pr.CompanyID, &pr.BranchID, &pr.PRNumber, &pr.RequestedBy, &pr.PRDate, &pr.Status, &pr.SubtotalAmount, &pr.Notes, &pr.CreatedAt, &pr.UpdatedAt)
}

func (h *Handler) listRequisitions(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + requisitionColumns + ` FROM purchase_requisitions WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY pr_date DESC, pr_number DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data purchase requisition")
		return
	}
	defer rows.Close()

	requisitions := []model.Requisition{}
	for rows.Next() {
		var pr model.Requisition
		if err := scanRequisition(rows, &pr); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data purchase requisition")
			return
		}
		requisitions = append(requisitions, pr)
	}
	writeJSON(w, http.StatusOK, requisitions)
}

type reqLineInput struct {
	ProductName    string  `json:"product_name"`
	Description    string  `json:"description"`
	Quantity       float64 `json:"quantity"`
	EstimatedPrice float64 `json:"estimated_price"`
}

type createRequisitionRequest struct {
	CompanyID   string         `json:"company_id"`
	BranchID    *string        `json:"branch_id"`
	RequestedBy string         `json:"requested_by"`
	PRDate      string         `json:"pr_date"`
	Notes       string         `json:"notes"`
	Lines       []reqLineInput `json:"lines"`
}

type requisitionWithLines struct {
	model.Requisition
	Lines []model.RequisitionLine `json:"lines"`
}

func (h *Handler) createRequisition(w http.ResponseWriter, r *http.Request) {
	var req createRequisitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.PRDate == "" || len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "company_id, pr_date, dan minimal 1 baris wajib diisi")
		return
	}
	prDate, err := time.Parse("2006-01-02", req.PRDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "pr_date harus format YYYY-MM-DD")
		return
	}

	var subtotal float64
	for _, l := range req.Lines {
		if strings.TrimSpace(l.ProductName) == "" {
			writeError(w, http.StatusBadRequest, "Setiap baris wajib punya product_name")
			return
		}
		subtotal += l.Quantity * l.EstimatedPrice
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	period := req.PRDate[:7]
	prNumber, err := nextSequence(ctx, tx, req.CompanyID, "purchase_requisitions", "pr_number", "PR", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor requisition")
		return
	}

	var pr model.Requisition
	err = tx.QueryRow(ctx, `
		INSERT INTO purchase_requisitions (company_id, branch_id, pr_number, requested_by, pr_date, subtotal_amount, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+requisitionColumns,
		req.CompanyID, req.BranchID, prNumber, req.RequestedBy, prDate, subtotal, req.Notes,
	).Scan(&pr.ID, &pr.CompanyID, &pr.BranchID, &pr.PRNumber, &pr.RequestedBy, &pr.PRDate, &pr.Status, &pr.SubtotalAmount, &pr.Notes, &pr.CreatedAt, &pr.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat purchase requisition")
		return
	}

	lines := make([]model.RequisitionLine, 0, len(req.Lines))
	for i, l := range req.Lines {
		amount := l.Quantity * l.EstimatedPrice
		var line model.RequisitionLine
		err := tx.QueryRow(ctx, `
			INSERT INTO purchase_requisition_lines (requisition_id, line_number, product_name, description, quantity, estimated_price, amount)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, requisition_id, line_number, product_name, description, quantity, estimated_price, amount`,
			pr.ID, i+1, l.ProductName, l.Description, l.Quantity, l.EstimatedPrice, amount,
		).Scan(&line.ID, &line.RequisitionID, &line.LineNumber, &line.ProductName, &line.Description, &line.Quantity, &line.EstimatedPrice, &line.Amount)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris requisition")
			return
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan purchase requisition")
		return
	}

	h.events.Publish("purchasing.requisition.created", newAuditEvent("purchasing.requisition.created", actorFromHeader(r), &pr.CompanyID, "create", "requisition", pr.ID, pr))
	writeJSON(w, http.StatusCreated, requisitionWithLines{Requisition: pr, Lines: lines})
}

func (h *Handler) getRequisition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var pr model.Requisition
	err := scanRequisition(h.pool.QueryRow(ctx, `SELECT `+requisitionColumns+` FROM purchase_requisitions WHERE id = $1`, id), &pr)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Purchase requisition tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat purchase requisition")
		return
	}

	lines, err := h.fetchRequisitionLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris requisition")
		return
	}
	writeJSON(w, http.StatusOK, requisitionWithLines{Requisition: pr, Lines: lines})
}

func (h *Handler) fetchRequisitionLines(ctx context.Context, requisitionID string) ([]model.RequisitionLine, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, requisition_id, line_number, product_name, description, quantity, estimated_price, amount
		FROM purchase_requisition_lines WHERE requisition_id = $1 ORDER BY line_number ASC`, requisitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []model.RequisitionLine{}
	for rows.Next() {
		var l model.RequisitionLine
		if err := rows.Scan(&l.ID, &l.RequisitionID, &l.LineNumber, &l.ProductName, &l.Description, &l.Quantity, &l.EstimatedPrice, &l.Amount); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (h *Handler) transitionRequisition(w http.ResponseWriter, r *http.Request, from, to, eventType string) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var pr model.Requisition
	err := scanRequisition(h.pool.QueryRow(r.Context(), `
		UPDATE purchase_requisitions SET status = $1, updated_at = now() WHERE id = $2 AND status = $3
		RETURNING `+requisitionColumns, to, id, from), &pr)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, fmt.Sprintf("Purchase requisition tidak ditemukan atau tidak berstatus %s", from))
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status requisition")
		return
	}

	h.events.Publish(eventType, newAuditEvent(eventType, actor, &pr.CompanyID, "update", "requisition", pr.ID, pr))
	writeJSON(w, http.StatusOK, pr)
}

func (h *Handler) submitRequisition(w http.ResponseWriter, r *http.Request) {
	h.transitionRequisition(w, r, "DRAFT", "SUBMITTED", "purchasing.requisition.submitted")
}

func (h *Handler) approveRequisition(w http.ResponseWriter, r *http.Request) {
	h.transitionRequisition(w, r, "SUBMITTED", "APPROVED", "purchasing.requisition.approved")
}

func (h *Handler) rejectRequisition(w http.ResponseWriter, r *http.Request) {
	h.transitionRequisition(w, r, "SUBMITTED", "REJECTED", "purchasing.requisition.rejected")
}

type convertRequisitionRequest struct {
	SupplierID string `json:"supplier_id"`
}

// convertRequisition membuat purchase_order baru dari requisition berstatus
// APPROVED, menyalin seluruh baris requisition apa adanya (estimated_price
// jadi unit_price awal, bisa diedit user sebelum PO dikonfirmasi), lalu
// menandai requisition sebagai CONVERTED. supplier_id wajib diisi di sini
// karena requisition sendiri belum terikat supplier tertentu.
func (h *Handler) convertRequisition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	var req convertRequisitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.SupplierID == "" {
		writeError(w, http.StatusBadRequest, "supplier_id wajib diisi")
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var pr model.Requisition
	err = scanRequisition(tx.QueryRow(ctx, `SELECT `+requisitionColumns+` FROM purchase_requisitions WHERE id = $1 FOR UPDATE`, id), &pr)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Purchase requisition tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat purchase requisition")
		return
	}
	if pr.Status != "APPROVED" {
		writeError(w, http.StatusConflict, "Purchase requisition harus berstatus APPROVED sebelum dikonversi menjadi PO")
		return
	}

	linesRows, err := tx.Query(ctx, `SELECT product_name, description, quantity, estimated_price, amount FROM purchase_requisition_lines WHERE requisition_id = $1 ORDER BY line_number ASC`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris requisition")
		return
	}
	type lineAgg struct {
		productName, description string
		quantity, price, amount  float64
	}
	var prLines []lineAgg
	for linesRows.Next() {
		var l lineAgg
		if err := linesRows.Scan(&l.productName, &l.description, &l.quantity, &l.price, &l.amount); err != nil {
			linesRows.Close()
			writeError(w, http.StatusInternalServerError, "Gagal membaca baris requisition")
			return
		}
		prLines = append(prLines, l)
	}
	linesRows.Close()

	period := pr.PRDate.Format("2006-01")
	poNumber, err := nextSequence(ctx, tx, pr.CompanyID, "purchase_orders", "po_number", "PO", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor purchase order")
		return
	}

	var po model.PurchaseOrder
	err = tx.QueryRow(ctx, `
		INSERT INTO purchase_orders (company_id, branch_id, po_number, supplier_id, requisition_id, order_date, subtotal_amount, tax_amount, total_amount)
		VALUES ($1, $2, $3, $4, $5, CURRENT_DATE, $6, 0, $6)
		RETURNING `+purchaseOrderColumns,
		pr.CompanyID, pr.BranchID, poNumber, req.SupplierID, pr.ID, pr.SubtotalAmount,
	).Scan(&po.ID, &po.CompanyID, &po.BranchID, &po.PONumber, &po.SupplierID, &po.RequisitionID, &po.OrderDate,
		&po.Status, &po.SubtotalAmount, &po.TaxAmount, &po.TotalAmount, &po.InvoiceID, &po.CreatedAt, &po.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat purchase order")
		return
	}

	for i, l := range prLines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_order_lines (purchase_order_id, line_number, product_name, description, quantity, unit_price, amount)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			po.ID, i+1, l.productName, l.description, l.quantity, l.price, l.amount); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris purchase order")
			return
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE purchase_requisitions SET status = 'CONVERTED', updated_at = now() WHERE id = $1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status requisition")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan konversi requisition")
		return
	}

	h.events.Publish("purchasing.requisition.converted", newAuditEvent("purchasing.requisition.converted", actor, &pr.CompanyID, "convert", "requisition", pr.ID, po))
	writeJSON(w, http.StatusCreated, po)
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
