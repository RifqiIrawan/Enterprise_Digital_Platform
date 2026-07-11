package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/asset-service/internal/model"
)

const maintenanceColumns = `id, company_id, asset_id, maintenance_type, scheduled_date, completed_date, status, notes, created_at, updated_at`

func scanMaintenance(row pgx.Row, m *model.MaintenanceSchedule) error {
	return row.Scan(&m.ID, &m.CompanyID, &m.AssetID, &m.MaintenanceType, &m.ScheduledDate, &m.CompletedDate, &m.Status, &m.Notes, &m.CreatedAt, &m.UpdatedAt)
}

func (h *Handler) listMaintenanceSchedules(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	assetID := r.URL.Query().Get("asset_id")

	query := `SELECT ` + maintenanceColumns + ` FROM maintenance_schedules WHERE company_id = $1`
	args := []any{companyID}
	if assetID != "" {
		query += ` AND asset_id = $2`
		args = append(args, assetID)
	}
	query += ` ORDER BY scheduled_date ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data jadwal maintenance")
		return
	}
	defer rows.Close()

	schedules := []model.MaintenanceSchedule{}
	for rows.Next() {
		var m model.MaintenanceSchedule
		if err := scanMaintenance(rows, &m); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data jadwal maintenance")
			return
		}
		schedules = append(schedules, m)
	}
	writeJSON(w, http.StatusOK, schedules)
}

type maintenanceRequest struct {
	CompanyID       string `json:"company_id"`
	AssetID         string `json:"asset_id"`
	MaintenanceType string `json:"maintenance_type"`
	ScheduledDate   string `json:"scheduled_date"`
	Notes           string `json:"notes"`
}

func (h *Handler) createMaintenanceSchedule(w http.ResponseWriter, r *http.Request) {
	var req maintenanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.MaintenanceType = strings.TrimSpace(req.MaintenanceType)
	if req.CompanyID == "" || req.AssetID == "" || req.MaintenanceType == "" || req.ScheduledDate == "" {
		writeError(w, http.StatusBadRequest, "company_id, asset_id, maintenance_type, dan scheduled_date wajib diisi")
		return
	}
	scheduledDate, err := time.Parse("2006-01-02", req.ScheduledDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "scheduled_date harus format YYYY-MM-DD")
		return
	}

	var asset model.Asset
	err = scanAsset(h.pool.QueryRow(r.Context(), `SELECT `+assetColumns+` FROM assets WHERE id = $1`, req.AssetID), &asset)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Aset tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat aset")
		return
	}

	var m model.MaintenanceSchedule
	err = scanMaintenance(h.pool.QueryRow(r.Context(), `
		INSERT INTO maintenance_schedules (company_id, asset_id, maintenance_type, scheduled_date, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+maintenanceColumns,
		req.CompanyID, req.AssetID, req.MaintenanceType, scheduledDate, req.Notes,
	), &m)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat jadwal maintenance")
		return
	}

	h.events.Publish("asset.maintenance.scheduled", newAuditEvent("asset.maintenance.scheduled", actorFromHeader(r), &m.CompanyID, "create", "maintenance_schedule", m.ID, m))
	writeJSON(w, http.StatusCreated, m)
}

// completeMaintenanceSchedule menandai jadwal selesai dan otomatis
// mengembalikan status aset ke ACTIVE kalau sebelumnya MAINTENANCE --
// keduanya di database yang sama (assets & maintenance_schedules), jadi
// bisa satu transaksi tanpa perlu panggilan HTTP lintas service.
func (h *Handler) completeMaintenanceSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)
	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var m model.MaintenanceSchedule
	err = scanMaintenance(tx.QueryRow(ctx, `
		UPDATE maintenance_schedules SET status = 'COMPLETED', completed_date = CURRENT_DATE, updated_at = now()
		WHERE id = $1 AND status = 'SCHEDULED'
		RETURNING `+maintenanceColumns, id), &m)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Jadwal maintenance tidak ditemukan atau tidak berstatus SCHEDULED")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui jadwal maintenance")
		return
	}

	if _, err := tx.Exec(ctx, `UPDATE assets SET status = 'ACTIVE', updated_at = now() WHERE id = $1 AND status = 'MAINTENANCE'`, m.AssetID); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status aset")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan penyelesaian maintenance")
		return
	}

	h.events.Publish("asset.maintenance.completed", newAuditEvent("asset.maintenance.completed", actor, &m.CompanyID, "update", "maintenance_schedule", m.ID, m))
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) cancelMaintenanceSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := actorFromHeader(r)

	var m model.MaintenanceSchedule
	err := scanMaintenance(h.pool.QueryRow(r.Context(), `
		UPDATE maintenance_schedules SET status = 'CANCELLED', updated_at = now()
		WHERE id = $1 AND status = 'SCHEDULED'
		RETURNING `+maintenanceColumns, id), &m)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Jadwal maintenance tidak ditemukan atau tidak berstatus SCHEDULED")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membatalkan jadwal maintenance")
		return
	}

	h.events.Publish("asset.maintenance.cancelled", newAuditEvent("asset.maintenance.cancelled", actor, &m.CompanyID, "update", "maintenance_schedule", m.ID, m))
	writeJSON(w, http.StatusOK, m)
}
