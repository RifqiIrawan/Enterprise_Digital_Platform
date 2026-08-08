package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/crm-service/internal/model"
)

const contactColumns = `id, company_id, branch_id, account_id, first_name, last_name, job_title, email, phone, is_primary, notes, created_at, updated_at`

func scanContact(row pgx.Row, c *model.Contact) error {
	return row.Scan(&c.ID, &c.CompanyID, &c.BranchID, &c.AccountID, &c.FirstName, &c.LastName, &c.JobTitle, &c.Email, &c.Phone, &c.IsPrimary, &c.Notes, &c.CreatedAt, &c.UpdatedAt)
}

func (h *Handler) listContacts(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + contactColumns + ` FROM contacts WHERE company_id = $1`
	args := []any{companyID}
	if accountID := r.URL.Query().Get("account_id"); accountID != "" {
		args = append(args, accountID)
		query += ` AND account_id = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY first_name ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data contact")
		return
	}
	defer rows.Close()

	contacts := []model.Contact{}
	for rows.Next() {
		var c model.Contact
		if err := scanContact(rows, &c); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data contact")
			return
		}
		contacts = append(contacts, c)
	}
	writeJSON(w, http.StatusOK, contacts)
}

type contactRequest struct {
	CompanyID string  `json:"company_id"`
	BranchID  *string `json:"branch_id"`
	AccountID *string `json:"account_id"`
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	JobTitle  string  `json:"job_title"`
	Email     string  `json:"email"`
	Phone     string  `json:"phone"`
	IsPrimary bool    `json:"is_primary"`
	Notes     string  `json:"notes"`
}

func (h *Handler) createContact(w http.ResponseWriter, r *http.Request) {
	var req contactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.FirstName = strings.TrimSpace(req.FirstName)
	if req.CompanyID == "" || req.FirstName == "" {
		writeError(w, http.StatusBadRequest, "company_id dan first_name wajib diisi")
		return
	}

	if req.AccountID != nil && *req.AccountID != "" {
		var a model.Account
		err := scanAccount(h.pool.QueryRow(r.Context(), `SELECT `+accountColumns+` FROM accounts WHERE id = $1 AND company_id = $2`, *req.AccountID, req.CompanyID), &a)
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Account tidak ditemukan")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memuat account")
			return
		}
	} else {
		req.AccountID = nil
	}

	var c model.Contact
	err := scanContact(h.pool.QueryRow(r.Context(), `
		INSERT INTO contacts (company_id, branch_id, account_id, first_name, last_name, job_title, email, phone, is_primary, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+contactColumns,
		req.CompanyID, req.BranchID, req.AccountID, req.FirstName, req.LastName, req.JobTitle, req.Email, req.Phone, req.IsPrimary, req.Notes,
	), &c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat contact")
		return
	}

	h.events.Publish("crm.contact.created", newAuditEvent("crm.contact.created", actorFromHeader(r), &c.CompanyID, "create", "contact", c.ID, c))
	writeJSON(w, http.StatusCreated, c)
}

type updateContactRequest struct {
	AccountID *string `json:"account_id"`
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	JobTitle  string  `json:"job_title"`
	Email     string  `json:"email"`
	Phone     string  `json:"phone"`
	IsPrimary bool    `json:"is_primary"`
	Notes     string  `json:"notes"`
}

func (h *Handler) updateContact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.FirstName = strings.TrimSpace(req.FirstName)
	if req.FirstName == "" {
		writeError(w, http.StatusBadRequest, "first_name wajib diisi")
		return
	}

	var c model.Contact
	err := scanContact(h.pool.QueryRow(r.Context(), `
		UPDATE contacts SET account_id = $1, first_name = $2, last_name = $3, job_title = $4, email = $5, phone = $6, is_primary = $7, notes = $8, updated_at = now()
		WHERE id = $9
		RETURNING `+contactColumns,
		req.AccountID, req.FirstName, req.LastName, req.JobTitle, req.Email, req.Phone, req.IsPrimary, req.Notes, id,
	), &c)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Contact tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui contact")
		return
	}

	h.events.Publish("crm.contact.updated", newAuditEvent("crm.contact.updated", actorFromHeader(r), &c.CompanyID, "update", "contact", c.ID, c))
	writeJSON(w, http.StatusOK, c)
}
