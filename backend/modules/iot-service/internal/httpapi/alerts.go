package httpapi

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/iot-service/internal/model"
)

const alertColumns = `a.id, a.device_id, a.reading_id, a.company_id, a.branch_id, a.alert_type, a.severity, a.message, a.status, a.triggered_at, a.acknowledged_at, a.acknowledged_by, a.resolved_at, a.resolved_by, d.device_code, d.name`

func scanAlert(row pgx.Row, a *model.Alert) error {
	return row.Scan(&a.ID, &a.DeviceID, &a.ReadingID, &a.CompanyID, &a.BranchID, &a.AlertType, &a.Severity, &a.Message, &a.Status, &a.TriggeredAt, &a.AcknowledgedAt, &a.AcknowledgedBy, &a.ResolvedAt, &a.ResolvedBy, &a.DeviceCode, &a.DeviceName)
}

func (h *Handler) listAlerts(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + alertColumns + ` FROM alerts a JOIN devices d ON d.id = a.device_id WHERE a.company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (a.branch_id = $` + strconv.Itoa(len(args)) + ` OR a.branch_id IS NULL)`
	}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += ` AND a.status = $` + strconv.Itoa(len(args))
	}
	query += ` ORDER BY a.triggered_at DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data alert")
		return
	}
	defer rows.Close()

	alerts := []model.Alert{}
	for rows.Next() {
		var a model.Alert
		if err := scanAlert(rows, &a); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data alert")
			return
		}
		alerts = append(alerts, a)
	}
	writeJSON(w, http.StatusOK, alerts)
}

func (h *Handler) acknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var a model.Alert
	err := scanAlert(h.pool.QueryRow(r.Context(), `
		UPDATE alerts a SET status = 'ACKNOWLEDGED', acknowledged_at = now(), acknowledged_by = $1
		FROM devices d
		WHERE a.device_id = d.id AND a.id = $2 AND a.status = 'OPEN'
		RETURNING `+alertColumns,
		actor, id,
	), &a)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Alert tidak ditemukan atau tidak berstatus OPEN")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengubah status alert")
		return
	}

	h.events.Publish("iot.alert.acknowledged", newAuditEvent("iot.alert.acknowledged", actor, &a.CompanyID, "update", "alert", a.ID, a))
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) resolveAlert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var a model.Alert
	err := scanAlert(h.pool.QueryRow(r.Context(), `
		UPDATE alerts a SET status = 'RESOLVED', resolved_at = now(), resolved_by = $1
		FROM devices d
		WHERE a.device_id = d.id AND a.id = $2 AND a.status IN ('OPEN', 'ACKNOWLEDGED')
		RETURNING `+alertColumns,
		actor, id,
	), &a)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Alert tidak ditemukan atau sudah RESOLVED")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengubah status alert")
		return
	}

	h.events.Publish("iot.alert.resolved", newAuditEvent("iot.alert.resolved", actor, &a.CompanyID, "update", "alert", a.ID, a))
	writeJSON(w, http.StatusOK, a)
}
