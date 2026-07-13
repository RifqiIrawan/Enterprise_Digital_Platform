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

	"github.com/enterprise-digital-platform/finance-service/internal/model"
)

func (h *Handler) listJournalEntries(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	query := `
		SELECT id, company_id, branch_id, entry_number, entry_date, period, description, reference_type,
		       reference_id, status, total_debit, total_credit, posted_by, posted_at, created_at
		FROM journal_entries WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY entry_date DESC, entry_number DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat jurnal")
		return
	}
	defer rows.Close()

	entries := []model.JournalEntry{}
	for rows.Next() {
		var e model.JournalEntry
		if err := rows.Scan(&e.ID, &e.CompanyID, &e.BranchID, &e.EntryNumber, &e.EntryDate, &e.Period, &e.Description,
			&e.ReferenceType, &e.ReferenceID, &e.Status, &e.TotalDebit, &e.TotalCredit, &e.PostedBy, &e.PostedAt, &e.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data jurnal")
			return
		}
		entries = append(entries, e)
	}
	writeJSON(w, http.StatusOK, entries)
}

type journalLineInput struct {
	AccountID    string  `json:"account_id"`
	DebitAmount  float64 `json:"debit_amount"`
	CreditAmount float64 `json:"credit_amount"`
	Description  string  `json:"description"`
}

type createJournalEntryRequest struct {
	CompanyID     string             `json:"company_id"`
	BranchID      *string            `json:"branch_id"`
	EntryDate     string             `json:"entry_date"` // YYYY-MM-DD
	Description   string             `json:"description"`
	ReferenceType string             `json:"reference_type"`
	ReferenceID   *string            `json:"reference_id"`
	Lines         []journalLineInput `json:"lines"`
}

type journalEntryWithLines struct {
	model.JournalEntry
	Lines []model.JournalLine `json:"lines"`
}

// createJournalEntry membuat journal entry berstatus DRAFT. Dipakai baik
// dari UI manual maupun (nanti) service lain seperti payroll-service via
// panggilan HTTP langsung, mengikuti pola financeClient.postJournalEntry()
// di 20_Implementation_Guide.md.
func (h *Handler) createJournalEntry(w http.ResponseWriter, r *http.Request) {
	var req createJournalEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CompanyID == "" || req.EntryDate == "" || len(req.Lines) < 2 {
		writeError(w, http.StatusBadRequest, "company_id, entry_date, dan minimal 2 baris jurnal wajib diisi")
		return
	}
	entryDate, err := time.Parse("2006-01-02", req.EntryDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "entry_date harus format YYYY-MM-DD")
		return
	}
	if req.ReferenceType == "" {
		req.ReferenceType = "manual"
	}

	var totalDebit, totalCredit float64
	for _, l := range req.Lines {
		if l.AccountID == "" {
			writeError(w, http.StatusBadRequest, "Setiap baris jurnal wajib punya account_id")
			return
		}
		if l.DebitAmount < 0 || l.CreditAmount < 0 || (l.DebitAmount > 0 && l.CreditAmount > 0) {
			writeError(w, http.StatusBadRequest, "Setiap baris jurnal hanya boleh diisi debit ATAU credit, tidak dua-duanya")
			return
		}
		totalDebit += l.DebitAmount
		totalCredit += l.CreditAmount
	}
	if !amountsEqual(totalDebit, totalCredit) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Jurnal tidak balance: total debit %.2f, total credit %.2f", totalDebit, totalCredit))
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	period := req.EntryDate[:7]
	entryNumber, err := nextSequence(ctx, tx, req.CompanyID, "journal_entries", "entry_number", "JE", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor jurnal")
		return
	}

	var e model.JournalEntry
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_entries (company_id, branch_id, entry_number, entry_date, period, description, reference_type, reference_id, total_debit, total_credit)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, company_id, branch_id, entry_number, entry_date, period, description, reference_type, reference_id, status, total_debit, total_credit, posted_by, posted_at, created_at`,
		req.CompanyID, req.BranchID, entryNumber, entryDate, period, req.Description, req.ReferenceType, req.ReferenceID, totalDebit, totalCredit,
	).Scan(&e.ID, &e.CompanyID, &e.BranchID, &e.EntryNumber, &e.EntryDate, &e.Period, &e.Description, &e.ReferenceType,
		&e.ReferenceID, &e.Status, &e.TotalDebit, &e.TotalCredit, &e.PostedBy, &e.PostedAt, &e.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat jurnal")
		return
	}

	lines := make([]model.JournalLine, 0, len(req.Lines))
	for i, l := range req.Lines {
		var line model.JournalLine
		err := tx.QueryRow(ctx, `
			INSERT INTO journal_lines (journal_id, line_number, account_id, debit_amount, credit_amount, description)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, journal_id, line_number, account_id, debit_amount, credit_amount, description`,
			e.ID, i+1, l.AccountID, l.DebitAmount, l.CreditAmount, l.Description,
		).Scan(&line.ID, &line.JournalID, &line.LineNumber, &line.AccountID, &line.DebitAmount, &line.CreditAmount, &line.Description)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membuat baris jurnal")
			return
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan jurnal")
		return
	}

	h.events.Publish("finance.journal.created", newAuditEvent("finance.journal.created", actorFromHeader(r), &e.CompanyID, "create", "journal_entry", e.ID, e))
	writeJSON(w, http.StatusCreated, journalEntryWithLines{JournalEntry: e, Lines: lines})
}

func (h *Handler) getJournalEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var e model.JournalEntry
	err := h.pool.QueryRow(ctx, `
		SELECT id, company_id, branch_id, entry_number, entry_date, period, description, reference_type,
		       reference_id, status, total_debit, total_credit, posted_by, posted_at, created_at
		FROM journal_entries WHERE id = $1`, id,
	).Scan(&e.ID, &e.CompanyID, &e.BranchID, &e.EntryNumber, &e.EntryDate, &e.Period, &e.Description,
		&e.ReferenceType, &e.ReferenceID, &e.Status, &e.TotalDebit, &e.TotalCredit, &e.PostedBy, &e.PostedAt, &e.CreatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Jurnal tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat jurnal")
		return
	}

	lines, err := h.fetchJournalLines(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat baris jurnal")
		return
	}
	writeJSON(w, http.StatusOK, journalEntryWithLines{JournalEntry: e, Lines: lines})
}

func (h *Handler) fetchJournalLines(ctx context.Context, journalID string) ([]model.JournalLine, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT id, journal_id, line_number, account_id, debit_amount, credit_amount, description
		FROM journal_lines WHERE journal_id = $1 ORDER BY line_number ASC`, journalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []model.JournalLine{}
	for rows.Next() {
		var l model.JournalLine
		if err := rows.Scan(&l.ID, &l.JournalID, &l.LineNumber, &l.AccountID, &l.DebitAmount, &l.CreditAmount, &l.Description); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

// postJournalEntry mengunci jurnal (DRAFT -> POSTED). Setelah posted, jurnal
// tidak boleh diubah lagi (immutable), konsisten dengan prinsip GL: koreksi
// dilakukan lewat jurnal pembalik (reversal), bukan edit langsung.
func (h *Handler) postJournalEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var e model.JournalEntry
	err := h.pool.QueryRow(r.Context(), `
		UPDATE journal_entries SET status = 'POSTED', posted_by = $1, posted_at = now()
		WHERE id = $2 AND status = 'DRAFT'
		RETURNING id, company_id, branch_id, entry_number, entry_date, period, description, reference_type,
		          reference_id, status, total_debit, total_credit, posted_by, posted_at, created_at`,
		actor, id,
	).Scan(&e.ID, &e.CompanyID, &e.BranchID, &e.EntryNumber, &e.EntryDate, &e.Period, &e.Description,
		&e.ReferenceType, &e.ReferenceID, &e.Status, &e.TotalDebit, &e.TotalCredit, &e.PostedBy, &e.PostedAt, &e.CreatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Jurnal tidak ditemukan atau sudah tidak berstatus DRAFT")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal posting jurnal")
		return
	}

	h.events.Publish("finance.journal.posted", newAuditEvent("finance.journal.posted", actor, &e.CompanyID, "post", "journal_entry", e.ID, e))
	writeJSON(w, http.StatusOK, e)
}

func amountsEqual(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.01
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
