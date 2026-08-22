package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/crm-service/internal/model"
)

const accountColumns = `id, company_id, branch_id, account_code, name, industry, website, phone, email, address, account_type, status, owner_user_id, notes, created_at, updated_at`

func scanAccount(row pgx.Row, a *model.Account) error {
	return row.Scan(&a.ID, &a.CompanyID, &a.BranchID, &a.AccountCode, &a.Name, &a.Industry, &a.Website, &a.Phone, &a.Email, &a.Address, &a.AccountType, &a.Status, &a.OwnerUserID, &a.Notes, &a.CreatedAt, &a.UpdatedAt)
}

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + accountColumns + ` FROM accounts WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	// Tanpa filter, SELURUH account ikut terbawa (termasuk yang nonaktif) --
	// halaman master memang perlu melihatnya untuk bisa mengaktifkan kembali.
	// Pemilih account di form lain yang mengirim status=ACTIVE.
	if status := strings.ToUpper(r.URL.Query().Get("status")); status != "" {
		if !validAccountStatuses[status] {
			writeError(w, http.StatusBadRequest, "status harus ACTIVE atau INACTIVE")
			return
		}
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	query += ` ORDER BY name ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data account")
		return
	}
	defer rows.Close()

	accounts := []model.Account{}
	for rows.Next() {
		var a model.Account
		if err := scanAccount(rows, &a); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data account")
			return
		}
		accounts = append(accounts, a)
	}
	writeJSON(w, http.StatusOK, accounts)
}

type accountRequest struct {
	CompanyID   string  `json:"company_id"`
	BranchID    *string `json:"branch_id"`
	AccountCode string  `json:"account_code"`
	Name        string  `json:"name"`
	Industry    string  `json:"industry"`
	Website     string  `json:"website"`
	Phone       string  `json:"phone"`
	Email       string  `json:"email"`
	Address     string  `json:"address"`
	AccountType string  `json:"account_type"`
	OwnerUserID *string `json:"owner_user_id"`
	Notes       string  `json:"notes"`
}

var validAccountTypes = map[string]bool{"PROSPECT": true, "CUSTOMER": true, "PARTNER": true, "OTHER": true}

var validAccountStatuses = map[string]bool{"ACTIVE": true, "INACTIVE": true}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req accountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.AccountCode = strings.TrimSpace(req.AccountCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.AccountCode == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, account_code, dan name wajib diisi")
		return
	}
	if req.AccountType == "" {
		req.AccountType = "PROSPECT"
	}
	if !validAccountTypes[req.AccountType] {
		writeError(w, http.StatusBadRequest, "account_type harus PROSPECT, CUSTOMER, PARTNER, atau OTHER")
		return
	}

	var a model.Account
	err := scanAccount(h.pool.QueryRow(r.Context(), `
		INSERT INTO accounts (company_id, branch_id, account_code, name, industry, website, phone, email, address, account_type, owner_user_id, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+accountColumns,
		req.CompanyID, req.BranchID, req.AccountCode, req.Name, req.Industry, req.Website, req.Phone, req.Email, req.Address, req.AccountType, req.OwnerUserID, req.Notes,
	), &a)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Kode account sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat account")
		return
	}

	h.events.Publish("crm.account.created", newAuditEvent("crm.account.created", actorFromHeader(r), &a.CompanyID, "create", "account", a.ID, a))
	writeJSON(w, http.StatusCreated, a)
}

type updateAccountRequest struct {
	Name        string `json:"name"`
	Industry    string `json:"industry"`
	Website     string `json:"website"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Address     string `json:"address"`
	AccountType string `json:"account_type"`
	// Status kosong berarti TIDAK DIUBAH -- menyunting nomor telepon tidak
	// boleh diam-diam mengaktifkan kembali account yang sengaja dinonaktifkan.
	Status      string  `json:"status"`
	OwnerUserID *string `json:"owner_user_id"`
	Notes       string  `json:"notes"`
}

func (h *Handler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}
	if !validAccountTypes[req.AccountType] {
		writeError(w, http.StatusBadRequest, "account_type harus PROSPECT, CUSTOMER, PARTNER, atau OTHER")
		return
	}
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	if req.Status != "" && !validAccountStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status harus ACTIVE atau INACTIVE")
		return
	}

	ctx := r.Context()

	// Menonaktifkan account yang masih punya opportunity berjalan akan
	// menyembunyikan pekerjaan yang sedang dikerjakan orang: pipeline-nya masih
	// hidup, tapi account-nya tidak bisa lagi dipilih di form mana pun.
	// Tutup dulu (WON/LOST) opportunity-nya, baru account-nya dinonaktifkan.
	if req.Status == "INACTIVE" {
		var openCount int
		if err := h.pool.QueryRow(ctx,
			`SELECT count(*) FROM opportunities WHERE account_id = $1 AND stage NOT IN ('WON', 'LOST')`, id).Scan(&openCount); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memeriksa opportunity terkait")
			return
		}
		if openCount > 0 {
			writeError(w, http.StatusConflict,
				"Account ini masih punya "+strconv.Itoa(openCount)+" opportunity yang belum ditutup; selesaikan (WON/LOST) dulu sebelum menonaktifkan")
			return
		}
	}

	var a model.Account
	err := scanAccount(h.pool.QueryRow(ctx, `
		UPDATE accounts SET name = $1, industry = $2, website = $3, phone = $4, email = $5, address = $6, account_type = $7,
		    status = COALESCE(NULLIF($8, ''), status), owner_user_id = $9, notes = $10, updated_at = now()
		WHERE id = $11
		RETURNING `+accountColumns,
		req.Name, req.Industry, req.Website, req.Phone, req.Email, req.Address, req.AccountType, req.Status, req.OwnerUserID, req.Notes, id,
	), &a)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Account tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui account")
		return
	}

	h.events.Publish("crm.account.updated", newAuditEvent("crm.account.updated", actorFromHeader(r), &a.CompanyID, "update", "account", a.ID, a))
	writeJSON(w, http.StatusOK, a)
}
