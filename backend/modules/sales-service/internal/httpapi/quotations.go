package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/sales-service/internal/model"
)

const quotationColumns = `id, company_id, branch_id, quotation_number, customer_id, quotation_date, valid_until, status, subtotal_amount, tax_amount, total_amount, notes, created_at, updated_at`

func scanQuotation(row pgx.Row, q *model.Quotation) error {
	return row.Scan(&q.ID, &q.CompanyID, &q.BranchID, &q.QuotationNumber, &q.CustomerID, &q.QuotationDate, &q.ValidUntil,
		&q.Status, &q.SubtotalAmount, &q.TaxAmount, &q.TotalAmount, &q.Notes, &q.CreatedAt, &q.UpdatedAt)
}

func (h *Handler) listQuotations(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT `+quotationColumns+` FROM quotations WHERE company_id = $1 ORDER BY quotation_date DESC, quotation_number DESC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data quotation")
		return
	}
	defer rows.Close()

	quotations := []model.Quotation{}
	for rows.Next() {
		var q model.Quotation
		if err := scanQuotation(rows, &q); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data quotation")
			return
		}
		quotations = append(quotations, q)
	}
	writeJSON(w, http.StatusOK, quotations)
}

type lineInput struct {
	ProductName string  `json:"product_name"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type createQuotationRequest struct {
	CompanyID     string      `json:"company_id"`
	BranchID      *string     `json:"branch_id"`
	CustomerID    string      `json:"customer_id"`
	QuotationDate string      `json:"quotation_date"`
	ValidUntil    *string     `json:"valid_until"`
	TaxAmount     float64     `json:"tax_amount"`
	Notes         string      `json:"notes"`
	Lines         []lineInput `json:"lines"`
}

type quotationWithLines struct {
	model.Quotation
	Lines []model.QuotationLine `json:"lines"`
}

func (h *Handler) createQuotation(w http.ResponseWriter, r *http.Request) {
	var req createQuotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.CustomerID == "" || req.QuotationDate == "" || len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "company_id, customer_id, quotation_date, dan minimal 1 baris wajib diisi")
		return
	}
	quotationDate, err := time.Parse("2006-01-02", req.QuotationDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "quotation_date harus format YYYY-MM-DD")
		return
	}
	var validUntil *time.Time
	if req.ValidUntil != nil && *req.ValidUntil != "" {
		d, err := time.Parse("2006-01-02", *req.ValidUntil)
		if err != nil {
			writeError(w, http.StatusBadRequest, "valid_until harus format YYYY-MM-DD")
			return
		}
		validUntil = &d
	}

	var subtotal float64
	for _, l := range req.Lines {
		if strings.TrimSpace(l.ProductName) == "" {
			writeError(w, http.StatusBadRequest, "Setiap baris wajib punya product_name")
			return
		}
		subtotal += l.Quantity * l.UnitPrice
	}
	total := subtotal + req.TaxAmount

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	period := req.QuotationDate[:7]
	quotationNumber, err := nextSequence(ctx, tx, req.CompanyID, "quotations", "quotation_number", "QUO", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor quotation")
		return
	}

	var q model.Quotation
	err = tx.QueryRow(ctx, `
		INSERT INTO quotations (company_id, branch_id, quotation_number, customer_id, quotation_date, valid_until, subtotal_amount, tax_amount, total_amount, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+quotationColumns,
		req.CompanyID, req.BranchID, quotationNumber, req.CustomerID, quotationDate, validUntil, subtotal, req.TaxAmount, total, req.Notes,
	).Scan(&q.ID, &q.CompanyID, &q.BranchID, &q.QuotationNumber, &q.CustomerID, &q.QuotationDate, &q.ValidUntil,
		&q.Status, &q.SubtotalAmount, &q.TaxAmount, &q.TotalAmount, &q.Notes, &q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat quotation")
		return
	}

	lines := make([]model.QuotationLine, 0, len(req.Lines))
	for i, l := range req.Lines {
		amount := l.Quantity * l.UnitPrice
		var line model.QuotationLine
		err := tx.QueryRow(ctx, `
			INSERT INTO quotation_lines (quotation_id, line_number, product_name, description, quantity, unit_price, amount)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, quotation_id, line_number, product_name, description, quantity, unit_price, amount`,
			q.ID, i+1, l.ProductName, l.Description, l.Quantity, l.UnitPrice, amount,
		).Scan(&line.ID, &line.QuotationID, &line.LineNumber, &line.ProductName, &line.Description, &line.Quantity, &line.UnitPrice, &line.Amount)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris quotation")
			return
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan quotation")
		return
	}

	h.events.Publish("sales.quotation.created", newAuditEvent("sales.quotation.created", actorFromHeader(r), &q.CompanyID, "create", "quotation", q.ID, q))
	writeJSON(w, http.StatusCreated, quotationWithLines{Quotation: q, Lines: lines})
}

func (h *Handler) getQuotation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var q model.Quotation
	err := scanQuotation(h.pool.QueryRow(ctx, `SELECT `+quotationColumns+` FROM quotations WHERE id = $1`, id), &q)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Quotation tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat quotation")
		return
	}

	lines, err := h.fetchQuotationLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris quotation")
		return
	}
	writeJSON(w, http.StatusOK, quotationWithLines{Quotation: q, Lines: lines})
}

func (h *Handler) fetchQuotationLines(ctx context.Context, quotationID string) ([]model.QuotationLine, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, quotation_id, line_number, product_name, description, quantity, unit_price, amount
		FROM quotation_lines WHERE quotation_id = $1 ORDER BY line_number ASC`, quotationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []model.QuotationLine{}
	for rows.Next() {
		var l model.QuotationLine
		if err := rows.Scan(&l.ID, &l.QuotationID, &l.LineNumber, &l.ProductName, &l.Description, &l.Quantity, &l.UnitPrice, &l.Amount); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (h *Handler) transitionQuotation(w http.ResponseWriter, r *http.Request, from, to, eventType string) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var q model.Quotation
	err := scanQuotation(h.pool.QueryRow(r.Context(), `
		UPDATE quotations SET status = $1, updated_at = now() WHERE id = $2 AND status = $3
		RETURNING `+quotationColumns, to, id, from), &q)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, fmt.Sprintf("Quotation tidak ditemukan atau tidak berstatus %s", from))
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status quotation")
		return
	}

	h.events.Publish(eventType, newAuditEvent(eventType, actor, &q.CompanyID, "update", "quotation", q.ID, q))
	writeJSON(w, http.StatusOK, q)
}

func (h *Handler) sendQuotation(w http.ResponseWriter, r *http.Request) {
	h.transitionQuotation(w, r, "DRAFT", "SENT", "sales.quotation.sent")
}

func (h *Handler) acceptQuotation(w http.ResponseWriter, r *http.Request) {
	h.transitionQuotation(w, r, "SENT", "ACCEPTED", "sales.quotation.accepted")
}

func (h *Handler) rejectQuotation(w http.ResponseWriter, r *http.Request) {
	h.transitionQuotation(w, r, "SENT", "REJECTED", "sales.quotation.rejected")
}

// convertQuotation membuat sales_order baru dari quotation berstatus
// ACCEPTED, menyalin seluruh baris quotation apa adanya, lalu menandai
// quotation sebagai CONVERTED supaya tidak dikonversi dua kali.
func (h *Handler) convertQuotation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var q model.Quotation
	err = scanQuotation(tx.QueryRow(ctx, `SELECT `+quotationColumns+` FROM quotations WHERE id = $1 FOR UPDATE`, id), &q)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Quotation tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat quotation")
		return
	}
	if q.Status != "ACCEPTED" {
		writeError(w, http.StatusConflict, "Quotation harus berstatus ACCEPTED sebelum dikonversi menjadi sales order")
		return
	}

	linesRows, err := tx.Query(ctx, `SELECT product_name, description, quantity, unit_price, amount FROM quotation_lines WHERE quotation_id = $1 ORDER BY line_number ASC`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris quotation")
		return
	}
	type lineAgg struct {
		productName, description    string
		quantity, unitPrice, amount float64
	}
	var qLines []lineAgg
	for linesRows.Next() {
		var l lineAgg
		if err := linesRows.Scan(&l.productName, &l.description, &l.quantity, &l.unitPrice, &l.amount); err != nil {
			linesRows.Close()
			writeError(w, http.StatusInternalServerError, "Gagal membaca baris quotation")
			return
		}
		qLines = append(qLines, l)
	}
	linesRows.Close()

	period := q.QuotationDate.Format("2006-01")
	soNumber, err := nextSequence(ctx, tx, q.CompanyID, "sales_orders", "so_number", "SO", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor sales order")
		return
	}

	var so model.SalesOrder
	err = tx.QueryRow(ctx, `
		INSERT INTO sales_orders (company_id, branch_id, so_number, customer_id, quotation_id, order_date, subtotal_amount, tax_amount, total_amount)
		VALUES ($1, $2, $3, $4, $5, CURRENT_DATE, $6, $7, $8)
		RETURNING `+salesOrderColumns,
		q.CompanyID, q.BranchID, soNumber, q.CustomerID, q.ID, q.SubtotalAmount, q.TaxAmount, q.TotalAmount,
	).Scan(&so.ID, &so.CompanyID, &so.BranchID, &so.SONumber, &so.CustomerID, &so.QuotationID, &so.OrderDate,
		&so.Status, &so.SubtotalAmount, &so.TaxAmount, &so.TotalAmount, &so.InvoiceID, &so.CreatedAt, &so.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat sales order")
		return
	}

	for i, l := range qLines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO sales_order_lines (sales_order_id, line_number, product_name, description, quantity, unit_price, amount)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			so.ID, i+1, l.productName, l.description, l.quantity, l.unitPrice, l.amount); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris sales order")
			return
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE quotations SET status = 'CONVERTED', updated_at = now() WHERE id = $1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status quotation")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan konversi quotation")
		return
	}

	h.events.Publish("sales.quotation.converted", newAuditEvent("sales.quotation.converted", actor, &q.CompanyID, "convert", "quotation", q.ID, so))
	writeJSON(w, http.StatusCreated, so)
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
