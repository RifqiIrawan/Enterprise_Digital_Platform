package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/audit-service/internal/model"
)

type Handler struct {
	pool *pgxpool.Pool
}

func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /audit-logs", h.listAuditLogs)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "audit-service"})
}

func (h *Handler) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 100
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}

	conditions := []string{"1=1"}
	args := []any{}
	addFilter := func(column, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		conditions = append(conditions, column+" = $"+strconv.Itoa(len(args)))
	}
	addFilter("company_id", q.Get("company_id"))
	addFilter("actor_user_id", q.Get("actor_user_id"))
	addFilter("event_type", q.Get("event_type"))
	addFilter("entity_type", q.Get("entity_type"))

	if from := q.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			args = append(args, t)
			conditions = append(conditions, "occurred_at >= $"+strconv.Itoa(len(args)))
		}
	}
	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			args = append(args, t)
			conditions = append(conditions, "occurred_at <= $"+strconv.Itoa(len(args)))
		}
	}

	args = append(args, limit)
	query := `SELECT id, event_id, event_type, source_service, actor_user_id, actor_email, company_id, branch_id,
	                 action, entity_type, entity_id, payload, occurred_at, recorded_at
	          FROM audit_logs WHERE ` + strings.Join(conditions, " AND ") +
		" ORDER BY occurred_at DESC LIMIT $" + strconv.Itoa(len(args))

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat audit log")
		return
	}
	defer rows.Close()

	logs := []model.AuditLog{}
	for rows.Next() {
		var l model.AuditLog
		if err := rows.Scan(&l.ID, &l.EventID, &l.EventType, &l.SourceService, &l.ActorUserID, &l.ActorEmail,
			&l.CompanyID, &l.BranchID, &l.Action, &l.EntityType, &l.EntityID, &l.Payload, &l.OccurredAt, &l.RecordedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca audit log")
			return
		}
		logs = append(logs, l)
	}
	writeJSON(w, http.StatusOK, logs)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
