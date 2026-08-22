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

const opportunityColumns = `id, company_id, branch_id, opportunity_number, account_id, contact_id, name, stage, amount, probability, expected_close_date, owner_user_id, lost_reason, notes, created_at, updated_at`

var openStages = map[string]bool{"PROSPECTING": true, "QUALIFICATION": true, "PROPOSAL": true, "NEGOTIATION": true}

func scanOpportunity(row pgx.Row, o *model.Opportunity) error {
	return row.Scan(&o.ID, &o.CompanyID, &o.BranchID, &o.OpportunityNumber, &o.AccountID, &o.ContactID, &o.Name, &o.Stage, &o.Amount, &o.Probability, &o.ExpectedCloseDate, &o.OwnerUserID, &o.LostReason, &o.Notes, &o.CreatedAt, &o.UpdatedAt)
}

func (h *Handler) listOpportunities(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + opportunityColumns + ` FROM opportunities WHERE company_id = $1`
	args := []any{companyID}
	if accountID := r.URL.Query().Get("account_id"); accountID != "" {
		args = append(args, accountID)
		query += ` AND account_id = $` + strconv.Itoa(len(args))
	}
	if stage := r.URL.Query().Get("stage"); stage != "" {
		args = append(args, stage)
		query += ` AND stage = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data opportunity")
		return
	}
	defer rows.Close()

	opportunities := []model.Opportunity{}
	for rows.Next() {
		var o model.Opportunity
		if err := scanOpportunity(rows, &o); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data opportunity")
			return
		}
		opportunities = append(opportunities, o)
	}
	writeJSON(w, http.StatusOK, opportunities)
}

type opportunityRequest struct {
	CompanyID         string  `json:"company_id"`
	BranchID          *string `json:"branch_id"`
	AccountID         string  `json:"account_id"`
	ContactID         *string `json:"contact_id"`
	Name              string  `json:"name"`
	Amount            float64 `json:"amount"`
	Probability       int16   `json:"probability"`
	ExpectedCloseDate string  `json:"expected_close_date"`
	OwnerUserID       *string `json:"owner_user_id"`
	Notes             string  `json:"notes"`
}

func parseOptionalDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (h *Handler) createOpportunity(w http.ResponseWriter, r *http.Request) {
	var req opportunityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.AccountID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, account_id, dan name wajib diisi")
		return
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

	var account model.Account
	err = scanAccount(tx.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = $1 AND company_id = $2`, req.AccountID, req.CompanyID), &account)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Account tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat account")
		return
	}

	// Opportunity baru selalu pekerjaan yang akan datang -- account yang sudah
	// dinonaktifkan tidak boleh menerimanya. Opportunity LAMA milik account itu
	// tetap bisa dilanjutkan sampai ditutup (lihat pagar di updateAccount).
	if account.Status != "ACTIVE" {
		writeError(w, http.StatusConflict, "Account tersebut nonaktif, tidak bisa dipakai untuk opportunity baru")
		return
	}
	if req.ContactID != nil && *req.ContactID != "" {
		var contact model.Contact
		err = scanContact(tx.QueryRow(ctx, `SELECT `+contactColumns+` FROM contacts WHERE id = $1 AND company_id = $2`, *req.ContactID, req.CompanyID), &contact)
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Contact tidak ditemukan")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memuat contact")
			return
		}
		if contact.Status != "ACTIVE" {
			writeError(w, http.StatusConflict, "Contact tersebut nonaktif, tidak bisa dipakai untuk opportunity baru")
			return
		}
	} else {
		req.ContactID = nil
	}

	period := time.Now().Format("2006-01")
	opportunityNumber, err := nextSequence(ctx, tx, req.CompanyID, "opportunities", "opportunity_number", "OPP", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor opportunity")
		return
	}

	var o model.Opportunity
	err = scanOpportunity(tx.QueryRow(ctx, `
		INSERT INTO opportunities (company_id, branch_id, opportunity_number, account_id, contact_id, name, amount, probability, expected_close_date, owner_user_id, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+opportunityColumns,
		req.CompanyID, req.BranchID, opportunityNumber, req.AccountID, req.ContactID, req.Name, req.Amount, req.Probability, expectedCloseDate, req.OwnerUserID, req.Notes,
	), &o)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat opportunity")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan opportunity")
		return
	}

	h.events.Publish("crm.opportunity.created", newAuditEvent("crm.opportunity.created", actorFromHeader(r), &o.CompanyID, "create", "opportunity", o.ID, o))
	writeJSON(w, http.StatusCreated, o)
}

type updateOpportunityRequest struct {
	ContactID         *string `json:"contact_id"`
	Name              string  `json:"name"`
	Stage             string  `json:"stage"`
	Amount            float64 `json:"amount"`
	Probability       int16   `json:"probability"`
	ExpectedCloseDate string  `json:"expected_close_date"`
	OwnerUserID       *string `json:"owner_user_id"`
	Notes             string  `json:"notes"`
}

// updateOpportunity mengizinkan edit field + pindah antar 4 stage terbuka
// (PROSPECTING..NEGOTIATION) -- freeform, cocok untuk pipeline yang kartunya
// bisa maju-mundur, BEDA dari transisi satu-arah dokumen lain di platform
// ini (quotation send/accept/reject). WON/LOST HANYA lewat /win /lose
// (terminal, lihat di bawah) -- PUT terhadap opportunity yang sudah
// WON/LOST ditolak 409. Event yang dipublikasikan beda tergantung apakah
// stage benar-benar berubah (stage_changed) atau tidak (updated) -- pola
// "diff sebelum publish" yang sama dengan completeMaintenanceSchedule di
// asset-service membandingkan status aset sebelum revert.
func (h *Handler) updateOpportunity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateOpportunityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}
	if !openStages[req.Stage] {
		writeError(w, http.StatusBadRequest, "stage harus PROSPECTING, QUALIFICATION, PROPOSAL, atau NEGOTIATION (pakai /win atau /lose untuk stage final)")
		return
	}
	expectedCloseDate, err := parseOptionalDate(req.ExpectedCloseDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected_close_date harus format YYYY-MM-DD")
		return
	}

	ctx := r.Context()
	var before model.Opportunity
	err = scanOpportunity(h.pool.QueryRow(ctx, `SELECT `+opportunityColumns+` FROM opportunities WHERE id = $1`, id), &before)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Opportunity tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat opportunity")
		return
	}
	if before.Stage == "WON" || before.Stage == "LOST" {
		writeError(w, http.StatusConflict, "Opportunity yang sudah WON/LOST tidak bisa diedit lagi")
		return
	}

	var o model.Opportunity
	err = scanOpportunity(h.pool.QueryRow(ctx, `
		UPDATE opportunities SET contact_id = $1, name = $2, stage = $3, amount = $4, probability = $5, expected_close_date = $6, owner_user_id = $7, notes = $8, updated_at = now()
		WHERE id = $9
		RETURNING `+opportunityColumns,
		req.ContactID, req.Name, req.Stage, req.Amount, req.Probability, expectedCloseDate, req.OwnerUserID, req.Notes, id,
	), &o)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui opportunity")
		return
	}

	eventType := "crm.opportunity.updated"
	if before.Stage != o.Stage {
		eventType = "crm.opportunity.stage_changed"
	}
	h.events.Publish(eventType, newAuditEvent(eventType, actorFromHeader(r), &o.CompanyID, "update", "opportunity", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}

func (h *Handler) winOpportunity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var o model.Opportunity
	err := scanOpportunity(h.pool.QueryRow(r.Context(), `
		UPDATE opportunities SET stage = 'WON', probability = 100, updated_at = now()
		WHERE id = $1 AND stage NOT IN ('WON', 'LOST')
		RETURNING `+opportunityColumns, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Opportunity tidak ditemukan atau sudah berstatus final (WON/LOST)")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menandai opportunity sebagai won")
		return
	}

	h.events.Publish("crm.opportunity.won", newAuditEvent("crm.opportunity.won", actorFromHeader(r), &o.CompanyID, "update", "opportunity", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}

type loseOpportunityRequest struct {
	LostReason string `json:"lost_reason"`
}

func (h *Handler) loseOpportunity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req loseOpportunityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}

	var o model.Opportunity
	err := scanOpportunity(h.pool.QueryRow(r.Context(), `
		UPDATE opportunities SET stage = 'LOST', lost_reason = $1, updated_at = now()
		WHERE id = $2 AND stage NOT IN ('WON', 'LOST')
		RETURNING `+opportunityColumns, req.LostReason, id), &o)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Opportunity tidak ditemukan atau sudah berstatus final (WON/LOST)")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menandai opportunity sebagai lost")
		return
	}

	h.events.Publish("crm.opportunity.lost", newAuditEvent("crm.opportunity.lost", actorFromHeader(r), &o.CompanyID, "update", "opportunity", o.ID, o))
	writeJSON(w, http.StatusOK, o)
}
