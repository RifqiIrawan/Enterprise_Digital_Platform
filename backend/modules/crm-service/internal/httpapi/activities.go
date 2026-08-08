package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/crm-service/internal/model"
)

const activityColumns = `id, company_id, branch_id, reference_type, reference_id, activity_type, subject, description, activity_date, due_date, is_done, owner_user_id, created_at, updated_at`

var validReferenceTypes = map[string]string{
	"LEAD":        "leads",
	"ACCOUNT":     "accounts",
	"CONTACT":     "contacts",
	"OPPORTUNITY": "opportunities",
}
var validActivityTypes = map[string]bool{"CALL": true, "EMAIL": true, "MEETING": true, "NOTE": true, "TASK": true}

func scanActivity(row pgx.Row, a *model.Activity) error {
	return row.Scan(&a.ID, &a.CompanyID, &a.BranchID, &a.ReferenceType, &a.ReferenceID, &a.ActivityType, &a.Subject, &a.Description, &a.ActivityDate, &a.DueDate, &a.IsDone, &a.OwnerUserID, &a.CreatedAt, &a.UpdatedAt)
}

// referenceExists memvalidasi bahwa reference_id benar-benar ada DAN milik
// company_id yang sama -- reference_id polimorfik (bisa menunjuk ke salah
// satu dari 4 tabel tergantung reference_type) jadi tidak ada FK database
// yang bisa memaksa ini (lihat komentar migrations/001_init.sql). Filter
// company_id di sini WAJIB, bukan opsional -- tanpanya caller bisa menebak
// UUID milik company lain dan berhasil mengaitkan activity ke situ.
func referenceExists(ctx context.Context, pool *pgxpool.Pool, referenceType, referenceID, companyID string) (bool, error) {
	table, ok := validReferenceTypes[referenceType]
	if !ok {
		return false, nil
	}
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM ` + table + ` WHERE id = $1 AND company_id = $2)`
	if err := pool.QueryRow(ctx, query, referenceID, companyID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (h *Handler) listActivities(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + activityColumns + ` FROM activities WHERE company_id = $1`
	args := []any{companyID}
	if referenceType := r.URL.Query().Get("reference_type"); referenceType != "" {
		args = append(args, referenceType)
		query += ` AND reference_type = $` + strconv.Itoa(len(args))
	}
	if referenceID := r.URL.Query().Get("reference_id"); referenceID != "" {
		args = append(args, referenceID)
		query += ` AND reference_id = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY activity_date DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data activity")
		return
	}
	defer rows.Close()

	activities := []model.Activity{}
	for rows.Next() {
		var a model.Activity
		if err := scanActivity(rows, &a); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data activity")
			return
		}
		activities = append(activities, a)
	}
	writeJSON(w, http.StatusOK, activities)
}

type activityRequest struct {
	CompanyID     string  `json:"company_id"`
	BranchID      *string `json:"branch_id"`
	ReferenceType string  `json:"reference_type"`
	ReferenceID   string  `json:"reference_id"`
	ActivityType  string  `json:"activity_type"`
	Subject       string  `json:"subject"`
	Description   string  `json:"description"`
	ActivityDate  string  `json:"activity_date"`
	DueDate       string  `json:"due_date"`
	IsDone        bool    `json:"is_done"`
	OwnerUserID   *string `json:"owner_user_id"`
}

func (h *Handler) createActivity(w http.ResponseWriter, r *http.Request) {
	var req activityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	if req.CompanyID == "" || req.ReferenceID == "" || req.Subject == "" {
		writeError(w, http.StatusBadRequest, "company_id, reference_id, dan subject wajib diisi")
		return
	}
	if _, ok := validReferenceTypes[req.ReferenceType]; !ok {
		writeError(w, http.StatusBadRequest, "reference_type harus LEAD, ACCOUNT, CONTACT, atau OPPORTUNITY")
		return
	}
	if !validActivityTypes[req.ActivityType] {
		writeError(w, http.StatusBadRequest, "activity_type harus CALL, EMAIL, MEETING, NOTE, atau TASK")
		return
	}

	ctx := r.Context()
	exists, err := referenceExists(ctx, h.pool, req.ReferenceType, req.ReferenceID, req.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memvalidasi reference")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "Reference tidak ditemukan")
		return
	}

	activityDate := time.Now()
	if req.ActivityDate != "" {
		parsed, err := time.Parse(time.RFC3339, req.ActivityDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "activity_date harus format RFC3339")
			return
		}
		activityDate = parsed
	}
	dueDate, err := parseOptionalDateTime(req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_date harus format RFC3339")
		return
	}

	var a model.Activity
	err = scanActivity(h.pool.QueryRow(ctx, `
		INSERT INTO activities (company_id, branch_id, reference_type, reference_id, activity_type, subject, description, activity_date, due_date, is_done, owner_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+activityColumns,
		req.CompanyID, req.BranchID, req.ReferenceType, req.ReferenceID, req.ActivityType, req.Subject, req.Description, activityDate, dueDate, req.IsDone, req.OwnerUserID,
	), &a)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat activity")
		return
	}

	h.events.Publish("crm.activity.created", newAuditEvent("crm.activity.created", actorFromHeader(r), &a.CompanyID, "create", "activity", a.ID, a))
	writeJSON(w, http.StatusCreated, a)
}

func parseOptionalDateTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

type updateActivityRequest struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
	DueDate     string `json:"due_date"`
	IsDone      bool   `json:"is_done"`
}

func (h *Handler) updateActivity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateActivityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Subject == "" {
		writeError(w, http.StatusBadRequest, "subject wajib diisi")
		return
	}
	dueDate, err := parseOptionalDateTime(req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_date harus format RFC3339")
		return
	}

	var a model.Activity
	err = scanActivity(h.pool.QueryRow(r.Context(), `
		UPDATE activities SET subject = $1, description = $2, due_date = $3, is_done = $4, updated_at = now()
		WHERE id = $5
		RETURNING `+activityColumns,
		req.Subject, req.Description, dueDate, req.IsDone, id,
	), &a)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Activity tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui activity")
		return
	}

	h.events.Publish("crm.activity.updated", newAuditEvent("crm.activity.updated", actorFromHeader(r), &a.CompanyID, "update", "activity", a.ID, a))
	writeJSON(w, http.StatusOK, a)
}
