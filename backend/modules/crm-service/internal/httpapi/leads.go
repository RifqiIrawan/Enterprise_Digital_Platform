package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/crm-service/internal/model"
)

const leadColumns = `id, company_id, branch_id, lead_number, first_name, last_name, company_name, email, phone, source, status, owner_user_id, notes, converted_account_id, converted_contact_id, converted_opportunity_id, converted_at, created_at, updated_at`

var validLeadSources = map[string]bool{"WEBSITE": true, "REFERRAL": true, "COLD_CALL": true, "EVENT": true, "ADVERTISEMENT": true, "SOCIAL_MEDIA": true, "OTHER": true}
var openLeadStatuses = map[string]bool{"NEW": true, "CONTACTED": true, "QUALIFIED": true, "UNQUALIFIED": true}

func scanLead(row pgx.Row, l *model.Lead) error {
	return row.Scan(&l.ID, &l.CompanyID, &l.BranchID, &l.LeadNumber, &l.FirstName, &l.LastName, &l.CompanyName, &l.Email, &l.Phone, &l.Source, &l.Status, &l.OwnerUserID, &l.Notes, &l.ConvertedAccountID, &l.ConvertedContactID, &l.ConvertedOpportunityID, &l.ConvertedAt, &l.CreatedAt, &l.UpdatedAt)
}

func (h *Handler) listLeads(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + leadColumns + ` FROM leads WHERE company_id = $1`
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
		writeError(w, http.StatusInternalServerError, "Gagal memuat data lead")
		return
	}
	defer rows.Close()

	leads := []model.Lead{}
	for rows.Next() {
		var l model.Lead
		if err := scanLead(rows, &l); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data lead")
			return
		}
		leads = append(leads, l)
	}
	writeJSON(w, http.StatusOK, leads)
}

type leadRequest struct {
	CompanyID   string  `json:"company_id"`
	BranchID    *string `json:"branch_id"`
	FirstName   string  `json:"first_name"`
	LastName    string  `json:"last_name"`
	CompanyName string  `json:"company_name"`
	Email       string  `json:"email"`
	Phone       string  `json:"phone"`
	Source      string  `json:"source"`
	OwnerUserID *string `json:"owner_user_id"`
	Notes       string  `json:"notes"`
}

func (h *Handler) createLead(w http.ResponseWriter, r *http.Request) {
	var req leadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.FirstName = strings.TrimSpace(req.FirstName)
	if req.CompanyID == "" || req.FirstName == "" {
		writeError(w, http.StatusBadRequest, "company_id dan first_name wajib diisi")
		return
	}
	if req.Source == "" {
		req.Source = "OTHER"
	}
	if !validLeadSources[req.Source] {
		writeError(w, http.StatusBadRequest, "source tidak valid")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	period := time.Now().Format("2006-01")
	leadNumber, err := nextSequence(ctx, tx, req.CompanyID, "leads", "lead_number", "LEAD", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor lead")
		return
	}

	var l model.Lead
	err = scanLead(tx.QueryRow(ctx, `
		INSERT INTO leads (company_id, branch_id, lead_number, first_name, last_name, company_name, email, phone, source, owner_user_id, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+leadColumns,
		req.CompanyID, req.BranchID, leadNumber, req.FirstName, req.LastName, req.CompanyName, req.Email, req.Phone, req.Source, req.OwnerUserID, req.Notes,
	), &l)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat lead")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan lead")
		return
	}

	h.events.Publish("crm.lead.created", newAuditEvent("crm.lead.created", actorFromHeader(r), &l.CompanyID, "create", "lead", l.ID, l))
	writeJSON(w, http.StatusCreated, l)
}

type updateLeadRequest struct {
	FirstName   string  `json:"first_name"`
	LastName    string  `json:"last_name"`
	CompanyName string  `json:"company_name"`
	Email       string  `json:"email"`
	Phone       string  `json:"phone"`
	Source      string  `json:"source"`
	Status      string  `json:"status"`
	OwnerUserID *string `json:"owner_user_id"`
	Notes       string  `json:"notes"`
}

func (h *Handler) updateLead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateLeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.FirstName = strings.TrimSpace(req.FirstName)
	if req.FirstName == "" {
		writeError(w, http.StatusBadRequest, "first_name wajib diisi")
		return
	}
	if !validLeadSources[req.Source] {
		writeError(w, http.StatusBadRequest, "source tidak valid")
		return
	}
	if !openLeadStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status harus NEW, CONTACTED, QUALIFIED, atau UNQUALIFIED (pakai /convert untuk CONVERTED)")
		return
	}

	ctx := r.Context()
	var before model.Lead
	err := scanLead(h.pool.QueryRow(ctx, `SELECT `+leadColumns+` FROM leads WHERE id = $1`, id), &before)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Lead tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat lead")
		return
	}
	if before.Status == "CONVERTED" {
		writeError(w, http.StatusConflict, "Lead yang sudah CONVERTED tidak bisa diedit lagi")
		return
	}

	var l model.Lead
	err = scanLead(h.pool.QueryRow(ctx, `
		UPDATE leads SET first_name = $1, last_name = $2, company_name = $3, email = $4, phone = $5, source = $6, status = $7, owner_user_id = $8, notes = $9, updated_at = now()
		WHERE id = $10
		RETURNING `+leadColumns,
		req.FirstName, req.LastName, req.CompanyName, req.Email, req.Phone, req.Source, req.Status, req.OwnerUserID, req.Notes, id,
	), &l)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui lead")
		return
	}

	h.events.Publish("crm.lead.updated", newAuditEvent("crm.lead.updated", actorFromHeader(r), &l.CompanyID, "update", "lead", l.ID, l))
	writeJSON(w, http.StatusOK, l)
}

type convertLeadRequest struct {
	OpportunityName   string  `json:"opportunity_name"`
	OpportunityAmount float64 `json:"opportunity_amount"`
	ExpectedCloseDate string  `json:"expected_close_date"`
}

// convertLead adalah satu-satunya business logic lintas-entitas di modul
// ini -- meniru pola convertQuotation di sales-service (SELECT ... FOR
// UPDATE, guard status, insert beruntun, update baris sumber, commit,
// publish event) persis, karena leads/accounts/contacts/opportunities
// semuanya di database crm_service yang sama sehingga bisa satu transaksi
// tanpa panggilan HTTP lintas service.
func (h *Handler) convertLead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var req convertLeadRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // semua field opsional, payload kosong valid
	}
	expectedCloseDate, err := parseOptionalDate(req.ExpectedCloseDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected_close_date harus format YYYY-MM-DD")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var lead model.Lead
	err = scanLead(tx.QueryRow(ctx, `SELECT `+leadColumns+` FROM leads WHERE id = $1 FOR UPDATE`, id), &lead)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Lead tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat lead")
		return
	}
	if lead.Status != "QUALIFIED" {
		writeError(w, http.StatusConflict, "Lead harus berstatus QUALIFIED sebelum dikonversi")
		return
	}

	period := time.Now().Format("2006-01")

	accountName := lead.CompanyName
	if accountName == "" {
		accountName = strings.TrimSpace(lead.FirstName + " " + lead.LastName)
	}
	accountCode, err := nextSequence(ctx, tx, lead.CompanyID, "accounts", "account_code", "ACC", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat kode account")
		return
	}
	var account model.Account
	err = scanAccount(tx.QueryRow(ctx, `
		INSERT INTO accounts (company_id, branch_id, account_code, name, phone, email, address, industry, website, account_type, owner_user_id, notes)
		VALUES ($1, $2, $3, $4, $5, $6, '', '', '', 'PROSPECT', $7, $8)
		RETURNING `+accountColumns,
		lead.CompanyID, lead.BranchID, accountCode, accountName, lead.Phone, lead.Email, lead.OwnerUserID, lead.Notes,
	), &account)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat account dari lead")
		return
	}

	var contact model.Contact
	err = scanContact(tx.QueryRow(ctx, `
		INSERT INTO contacts (company_id, branch_id, account_id, first_name, last_name, job_title, email, phone, is_primary, notes)
		VALUES ($1, $2, $3, $4, $5, '', $6, $7, TRUE, $8)
		RETURNING `+contactColumns,
		lead.CompanyID, lead.BranchID, account.ID, lead.FirstName, lead.LastName, lead.Email, lead.Phone, lead.Notes,
	), &contact)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat contact dari lead")
		return
	}

	opportunityName := strings.TrimSpace(req.OpportunityName)
	if opportunityName == "" {
		opportunityName = accountName + " - Deal"
	}
	opportunityNumber, err := nextSequence(ctx, tx, lead.CompanyID, "opportunities", "opportunity_number", "OPP", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor opportunity")
		return
	}
	var opportunity model.Opportunity
	err = scanOpportunity(tx.QueryRow(ctx, `
		INSERT INTO opportunities (company_id, branch_id, opportunity_number, account_id, contact_id, name, stage, amount, probability, expected_close_date, owner_user_id, notes)
		VALUES ($1, $2, $3, $4, $5, $6, 'PROSPECTING', $7, 10, $8, $9, '')
		RETURNING `+opportunityColumns,
		lead.CompanyID, lead.BranchID, opportunityNumber, account.ID, contact.ID, opportunityName, req.OpportunityAmount, expectedCloseDate, lead.OwnerUserID,
	), &opportunity)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat opportunity dari lead")
		return
	}

	err = scanLead(tx.QueryRow(ctx, `
		UPDATE leads SET status = 'CONVERTED', converted_account_id = $1, converted_contact_id = $2, converted_opportunity_id = $3, converted_at = now(), updated_at = now()
		WHERE id = $4
		RETURNING `+leadColumns,
		account.ID, contact.ID, opportunity.ID, id,
	), &lead)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status lead")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan konversi lead")
		return
	}

	result := map[string]any{"lead": lead, "account": account, "contact": contact, "opportunity": opportunity}
	h.events.Publish("crm.lead.converted", newAuditEvent("crm.lead.converted", actor, &lead.CompanyID, "convert", "lead", lead.ID, result))
	writeJSON(w, http.StatusCreated, result)
}
