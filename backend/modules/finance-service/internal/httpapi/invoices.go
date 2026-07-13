package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/finance-service/internal/model"
)

type invoiceWithLines struct {
	model.Invoice
	Lines []model.InvoiceLine `json:"lines"`
}

func (h *Handler) listInvoices(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	query := `SELECT id, company_id, branch_id, invoice_type, invoice_number, partner_name, invoice_date, due_date,
	                 control_account_id, tax_account_id, subtotal_amount, tax_amount, total_amount, paid_amount,
	                 status, journal_id, created_at, updated_at
	          FROM invoices WHERE company_id = $1`
	args := []any{companyID}

	if invoiceType := r.URL.Query().Get("invoice_type"); invoiceType != "" {
		args = append(args, invoiceType)
		query += " AND invoice_type = $" + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += " AND (branch_id = $" + strconv.Itoa(len(args)) + " OR branch_id IS NULL)"
	}
	query += " ORDER BY invoice_date DESC, invoice_number DESC"

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar invoice")
		return
	}
	defer rows.Close()

	invoices := []model.Invoice{}
	for rows.Next() {
		var inv model.Invoice
		if err := scanInvoice(rows, &inv); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data invoice")
			return
		}
		invoices = append(invoices, inv)
	}
	writeJSON(w, http.StatusOK, invoices)
}

type invoiceLineInput struct {
	AccountID   string  `json:"account_id"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type createInvoiceRequest struct {
	CompanyID        string             `json:"company_id"`
	BranchID         *string            `json:"branch_id"`
	InvoiceType      string             `json:"invoice_type"`
	PartnerName      string             `json:"partner_name"`
	InvoiceDate      string             `json:"invoice_date"`
	DueDate          *string            `json:"due_date"`
	ControlAccountID string             `json:"control_account_id"`
	TaxAccountID     *string            `json:"tax_account_id"`
	TaxAmount        float64            `json:"tax_amount"`
	Lines            []invoiceLineInput `json:"lines"`
}

func (h *Handler) createInvoice(w http.ResponseWriter, r *http.Request) {
	var req createInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.InvoiceType = strings.ToUpper(strings.TrimSpace(req.InvoiceType))
	req.PartnerName = strings.TrimSpace(req.PartnerName)
	if req.CompanyID == "" || req.PartnerName == "" || req.ControlAccountID == "" || len(req.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "company_id, partner_name, control_account_id, dan minimal 1 baris invoice wajib diisi")
		return
	}
	if req.InvoiceType != "AR" && req.InvoiceType != "AP" {
		writeError(w, http.StatusBadRequest, "invoice_type harus AR atau AP")
		return
	}
	invoiceDate, err := time.Parse("2006-01-02", req.InvoiceDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invoice_date harus format YYYY-MM-DD")
		return
	}
	var dueDate *time.Time
	if req.DueDate != nil && *req.DueDate != "" {
		d, err := time.Parse("2006-01-02", *req.DueDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "due_date harus format YYYY-MM-DD")
			return
		}
		dueDate = &d
	}

	var subtotal float64
	for _, l := range req.Lines {
		if l.AccountID == "" || l.Description == "" {
			writeError(w, http.StatusBadRequest, "Setiap baris invoice wajib punya account_id dan description")
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

	period := req.InvoiceDate[:7]
	prefix := "INV-" + req.InvoiceType
	invoiceNumber, err := nextSequence(ctx, tx, req.CompanyID, "invoices", "invoice_number", prefix, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor invoice")
		return
	}

	var inv model.Invoice
	err = tx.QueryRow(ctx, `
		INSERT INTO invoices (company_id, branch_id, invoice_type, invoice_number, partner_name, invoice_date, due_date,
		                      control_account_id, tax_account_id, subtotal_amount, tax_amount, total_amount)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, company_id, branch_id, invoice_type, invoice_number, partner_name, invoice_date, due_date,
		          control_account_id, tax_account_id, subtotal_amount, tax_amount, total_amount, paid_amount,
		          status, journal_id, created_at, updated_at`,
		req.CompanyID, req.BranchID, req.InvoiceType, invoiceNumber, req.PartnerName, invoiceDate, dueDate,
		req.ControlAccountID, req.TaxAccountID, subtotal, req.TaxAmount, total,
	).Scan(&inv.ID, &inv.CompanyID, &inv.BranchID, &inv.InvoiceType, &inv.InvoiceNumber, &inv.PartnerName, &inv.InvoiceDate, &inv.DueDate,
		&inv.ControlAccountID, &inv.TaxAccountID, &inv.SubtotalAmount, &inv.TaxAmount, &inv.TotalAmount, &inv.PaidAmount,
		&inv.Status, &inv.JournalID, &inv.CreatedAt, &inv.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat invoice")
		return
	}

	lines := make([]model.InvoiceLine, 0, len(req.Lines))
	for i, l := range req.Lines {
		amount := l.Quantity * l.UnitPrice
		var line model.InvoiceLine
		err := tx.QueryRow(ctx, `
			INSERT INTO invoice_lines (invoice_id, line_number, account_id, description, quantity, unit_price, amount)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, invoice_id, line_number, account_id, description, quantity, unit_price, amount`,
			inv.ID, i+1, l.AccountID, l.Description, l.Quantity, l.UnitPrice, amount,
		).Scan(&line.ID, &line.InvoiceID, &line.LineNumber, &line.AccountID, &line.Description, &line.Quantity, &line.UnitPrice, &line.Amount)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris invoice")
			return
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan invoice")
		return
	}

	h.events.Publish("finance.invoice.created", newAuditEvent("finance.invoice.created", actorFromHeader(r), &inv.CompanyID, "create", "invoice", inv.ID, inv))
	writeJSON(w, http.StatusCreated, invoiceWithLines{Invoice: inv, Lines: lines})
}

func (h *Handler) getInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var inv model.Invoice
	err := h.pool.QueryRow(ctx, `
		SELECT id, company_id, branch_id, invoice_type, invoice_number, partner_name, invoice_date, due_date,
		       control_account_id, tax_account_id, subtotal_amount, tax_amount, total_amount, paid_amount,
		       status, journal_id, created_at, updated_at
		FROM invoices WHERE id = $1`, id,
	).Scan(&inv.ID, &inv.CompanyID, &inv.BranchID, &inv.InvoiceType, &inv.InvoiceNumber, &inv.PartnerName, &inv.InvoiceDate, &inv.DueDate,
		&inv.ControlAccountID, &inv.TaxAccountID, &inv.SubtotalAmount, &inv.TaxAmount, &inv.TotalAmount, &inv.PaidAmount,
		&inv.Status, &inv.JournalID, &inv.CreatedAt, &inv.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Invoice tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat invoice")
		return
	}

	rows, err := h.pool.Query(ctx, `
		SELECT id, invoice_id, line_number, account_id, description, quantity, unit_price, amount
		FROM invoice_lines WHERE invoice_id = $1 ORDER BY line_number ASC`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris invoice")
		return
	}
	defer rows.Close()

	lines := []model.InvoiceLine{}
	for rows.Next() {
		var l model.InvoiceLine
		if err := rows.Scan(&l.ID, &l.InvoiceID, &l.LineNumber, &l.AccountID, &l.Description, &l.Quantity, &l.UnitPrice, &l.Amount); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca baris invoice")
			return
		}
		lines = append(lines, l)
	}
	writeJSON(w, http.StatusOK, invoiceWithLines{Invoice: inv, Lines: lines})
}

// postInvoice mem-posting invoice DRAFT menjadi POSTED sekaligus membuat
// journal entry berimbang secara otomatis:
//   - AR (piutang customer): debit control_account (Piutang Usaha) sebesar total,
//     credit tiap account baris (Revenue) sebesar amount, credit tax_account bila ada.
//   - AP (hutang vendor): debit tiap account baris (Expense) sebesar amount,
//     debit tax_account bila ada, credit control_account (Hutang Usaha) sebesar total.
func (h *Handler) postInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var inv model.Invoice
	err = tx.QueryRow(ctx, `
		SELECT id, company_id, branch_id, invoice_type, invoice_number, partner_name, invoice_date, due_date,
		       control_account_id, tax_account_id, subtotal_amount, tax_amount, total_amount, paid_amount, status
		FROM invoices WHERE id = $1 FOR UPDATE`, id,
	).Scan(&inv.ID, &inv.CompanyID, &inv.BranchID, &inv.InvoiceType, &inv.InvoiceNumber, &inv.PartnerName, &inv.InvoiceDate, &inv.DueDate,
		&inv.ControlAccountID, &inv.TaxAccountID, &inv.SubtotalAmount, &inv.TaxAmount, &inv.TotalAmount, &inv.PaidAmount, &inv.Status)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Invoice tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat invoice")
		return
	}
	if inv.Status != "DRAFT" {
		writeError(w, http.StatusConflict, "Invoice sudah tidak berstatus DRAFT")
		return
	}

	lineRows, err := tx.Query(ctx, `SELECT account_id, description, amount FROM invoice_lines WHERE invoice_id = $1 ORDER BY line_number ASC`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris invoice")
		return
	}
	type lineAgg struct {
		accountID   string
		description string
		amount      float64
	}
	var invLines []lineAgg
	for lineRows.Next() {
		var l lineAgg
		if err := lineRows.Scan(&l.accountID, &l.description, &l.amount); err != nil {
			lineRows.Close()
			writeError(w, http.StatusInternalServerError, "Gagal membaca baris invoice")
			return
		}
		invLines = append(invLines, l)
	}
	lineRows.Close()

	period := inv.InvoiceDate.Format("2006-01")
	entryNumber, err := nextSequence(ctx, tx, inv.CompanyID, "journal_entries", "entry_number", "JE", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor jurnal")
		return
	}

	var journal model.JournalEntry
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_entries (company_id, branch_id, entry_number, entry_date, period, description, reference_type, reference_id, status, total_debit, total_credit, posted_by, posted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'POSTED', $9, $9, $10, now())
		RETURNING id`,
		inv.CompanyID, inv.BranchID, entryNumber, inv.InvoiceDate, period,
		"Posting invoice "+inv.InvoiceNumber, referenceTypeFor(inv.InvoiceType), inv.ID, inv.TotalAmount, actor,
	).Scan(&journal.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat jurnal")
		return
	}

	lineNo := 1
	insertLine := func(accountID string, debit, credit float64, description string) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO journal_lines (journal_id, line_number, account_id, debit_amount, credit_amount, description)
			VALUES ($1, $2, $3, $4, $5, $6)`, journal.ID, lineNo, accountID, debit, credit, description)
		lineNo++
		return err
	}

	if inv.InvoiceType == "AR" {
		if err := insertLine(inv.ControlAccountID, inv.TotalAmount, 0, "Piutang "+inv.PartnerName); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris jurnal")
			return
		}
		for _, l := range invLines {
			if err := insertLine(l.accountID, 0, l.amount, l.description); err != nil {
				writeError(w, http.StatusInternalServerError, "Gagal membuat baris jurnal")
				return
			}
		}
		if inv.TaxAmount > 0 && inv.TaxAccountID != nil {
			if err := insertLine(*inv.TaxAccountID, 0, inv.TaxAmount, "PPN Keluaran "+inv.InvoiceNumber); err != nil {
				writeError(w, http.StatusInternalServerError, "Gagal membuat baris jurnal")
				return
			}
		}
	} else {
		for _, l := range invLines {
			if err := insertLine(l.accountID, l.amount, 0, l.description); err != nil {
				writeError(w, http.StatusInternalServerError, "Gagal membuat baris jurnal")
				return
			}
		}
		if inv.TaxAmount > 0 && inv.TaxAccountID != nil {
			if err := insertLine(*inv.TaxAccountID, inv.TaxAmount, 0, "PPN Masukan "+inv.InvoiceNumber); err != nil {
				writeError(w, http.StatusInternalServerError, "Gagal membuat baris jurnal")
				return
			}
		}
		if err := insertLine(inv.ControlAccountID, 0, inv.TotalAmount, "Hutang "+inv.PartnerName); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris jurnal")
			return
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE invoices SET status = 'POSTED', journal_id = $1, updated_at = now() WHERE id = $2`, journal.ID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status invoice")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan posting invoice")
		return
	}

	inv.Status = "POSTED"
	inv.JournalID = &journal.ID
	h.events.Publish("finance.invoice.posted", newAuditEvent("finance.invoice.posted", actor, &inv.CompanyID, "post", "invoice", inv.ID, inv))
	writeJSON(w, http.StatusOK, inv)
}

func referenceTypeFor(invoiceType string) string {
	if invoiceType == "AR" {
		return "invoice_ar"
	}
	return "invoice_ap"
}

func (h *Handler) arApSummary(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT invoice_type, COUNT(*), COALESCE(SUM(total_amount), 0), COALESCE(SUM(paid_amount), 0),
		       COALESCE(SUM(total_amount - paid_amount), 0)
		FROM invoices
		WHERE company_id = $1 AND status IN ('POSTED', 'PARTIALLY_PAID')
		GROUP BY invoice_type`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan AR/AP")
		return
	}
	defer rows.Close()

	type summary struct {
		InvoiceType       string  `json:"invoice_type"`
		Count             int     `json:"count"`
		TotalAmount       float64 `json:"total_amount"`
		PaidAmount        float64 `json:"paid_amount"`
		OutstandingAmount float64 `json:"outstanding_amount"`
	}
	results := []summary{}
	for rows.Next() {
		var s summary
		if err := rows.Scan(&s.InvoiceType, &s.Count, &s.TotalAmount, &s.PaidAmount, &s.OutstandingAmount); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca ringkasan AR/AP")
			return
		}
		results = append(results, s)
	}
	writeJSON(w, http.StatusOK, results)
}

func scanInvoice(rows pgx.Rows, inv *model.Invoice) error {
	return rows.Scan(&inv.ID, &inv.CompanyID, &inv.BranchID, &inv.InvoiceType, &inv.InvoiceNumber, &inv.PartnerName, &inv.InvoiceDate, &inv.DueDate,
		&inv.ControlAccountID, &inv.TaxAccountID, &inv.SubtotalAmount, &inv.TaxAmount, &inv.TotalAmount, &inv.PaidAmount,
		&inv.Status, &inv.JournalID, &inv.CreatedAt, &inv.UpdatedAt)
}
