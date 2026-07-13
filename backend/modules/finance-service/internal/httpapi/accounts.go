package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/finance-service/internal/model"
)

var validAccountTypes = map[string]bool{
	"ASSET": true, "LIABILITY": true, "EQUITY": true, "REVENUE": true, "EXPENSE": true,
}

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	query := `SELECT id, company_id, branch_id, account_code, account_name, account_type, parent_id, is_posting, is_active, created_at, updated_at
		FROM accounts WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY account_code ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat chart of accounts")
		return
	}
	defer rows.Close()

	accounts := []model.Account{}
	for rows.Next() {
		var a model.Account
		if err := rows.Scan(&a.ID, &a.CompanyID, &a.BranchID, &a.AccountCode, &a.AccountName, &a.AccountType, &a.ParentID, &a.IsPosting, &a.IsActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data account")
			return
		}
		accounts = append(accounts, a)
	}
	writeJSON(w, http.StatusOK, accounts)
}

type createAccountRequest struct {
	CompanyID   string  `json:"company_id"`
	BranchID    *string `json:"branch_id"`
	AccountCode string  `json:"account_code"`
	AccountName string  `json:"account_name"`
	AccountType string  `json:"account_type"`
	ParentID    *string `json:"parent_id"`
	IsPosting   *bool   `json:"is_posting"`
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.AccountCode = strings.TrimSpace(req.AccountCode)
	req.AccountName = strings.TrimSpace(req.AccountName)
	req.AccountType = strings.ToUpper(strings.TrimSpace(req.AccountType))
	if req.CompanyID == "" || req.AccountCode == "" || req.AccountName == "" {
		writeError(w, http.StatusBadRequest, "company_id, account_code, dan account_name wajib diisi")
		return
	}
	if !validAccountTypes[req.AccountType] {
		writeError(w, http.StatusBadRequest, "account_type harus salah satu dari ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE")
		return
	}
	isPosting := true
	if req.IsPosting != nil {
		isPosting = *req.IsPosting
	}

	var a model.Account
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO accounts (company_id, branch_id, account_code, account_name, account_type, parent_id, is_posting)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, company_id, branch_id, account_code, account_name, account_type, parent_id, is_posting, is_active, created_at, updated_at`,
		req.CompanyID, req.BranchID, req.AccountCode, req.AccountName, req.AccountType, req.ParentID, isPosting,
	).Scan(&a.ID, &a.CompanyID, &a.BranchID, &a.AccountCode, &a.AccountName, &a.AccountType, &a.ParentID, &a.IsPosting, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Account code sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat account")
		return
	}

	h.events.Publish("finance.account.created", newAuditEvent("finance.account.created", actorFromHeader(r), &a.CompanyID, "create", "account", a.ID, a))
	writeJSON(w, http.StatusCreated, a)
}

type updateAccountRequest struct {
	AccountName string `json:"account_name"`
	IsPosting   bool   `json:"is_posting"`
	IsActive    bool   `json:"is_active"`
}

func (h *Handler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.AccountName = strings.TrimSpace(req.AccountName)
	if req.AccountName == "" {
		writeError(w, http.StatusBadRequest, "account_name wajib diisi")
		return
	}

	var a model.Account
	err := h.pool.QueryRow(r.Context(), `
		UPDATE accounts SET account_name = $1, is_posting = $2, is_active = $3, updated_at = now()
		WHERE id = $4
		RETURNING id, company_id, branch_id, account_code, account_name, account_type, parent_id, is_posting, is_active, created_at, updated_at`,
		req.AccountName, req.IsPosting, req.IsActive, id,
	).Scan(&a.ID, &a.CompanyID, &a.BranchID, &a.AccountCode, &a.AccountName, &a.AccountType, &a.ParentID, &a.IsPosting, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Account tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui account")
		return
	}

	h.events.Publish("finance.account.updated", newAuditEvent("finance.account.updated", actorFromHeader(r), &a.CompanyID, "update", "account", a.ID, a))
	writeJSON(w, http.StatusOK, a)
}
