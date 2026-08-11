package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/fleet-service/internal/model"
)

const driverColumns = `id, company_id, branch_id, driver_code, name, phone, license_number, status, notes, created_at, updated_at`

// Sama seperti validVehicleStatuses: ON_DELIVERY SENGAJA tidak ada di sini,
// status itu hanya digerakkan oleh lifecycle surat jalan.
var validDriverStatuses = map[string]bool{"AVAILABLE": true, "INACTIVE": true}

func scanDriver(row pgx.Row, d *model.Driver) error {
	return row.Scan(&d.ID, &d.CompanyID, &d.BranchID, &d.DriverCode, &d.Name, &d.Phone, &d.LicenseNumber, &d.Status, &d.Notes, &d.CreatedAt, &d.UpdatedAt)
}

func (h *Handler) listDrivers(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + driverColumns + ` FROM drivers WHERE company_id = $1`
	args := []any{companyID}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY driver_code ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data pengemudi")
		return
	}
	defer rows.Close()

	drivers := []model.Driver{}
	for rows.Next() {
		var d model.Driver
		if err := scanDriver(rows, &d); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data pengemudi")
			return
		}
		drivers = append(drivers, d)
	}
	writeJSON(w, http.StatusOK, drivers)
}

type driverRequest struct {
	CompanyID     string  `json:"company_id"`
	BranchID      *string `json:"branch_id"`
	DriverCode    string  `json:"driver_code"`
	Name          string  `json:"name"`
	Phone         string  `json:"phone"`
	LicenseNumber string  `json:"license_number"`
	Status        string  `json:"status"`
	Notes         string  `json:"notes"`
}

func (h *Handler) createDriver(w http.ResponseWriter, r *http.Request) {
	var req driverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.DriverCode = strings.TrimSpace(req.DriverCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.DriverCode == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, driver_code, dan name wajib diisi")
		return
	}

	var d model.Driver
	err := scanDriver(h.pool.QueryRow(r.Context(), `
		INSERT INTO drivers (company_id, branch_id, driver_code, name, phone, license_number, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+driverColumns,
		req.CompanyID, req.BranchID, req.DriverCode, req.Name, req.Phone, req.LicenseNumber, req.Notes,
	), &d)
	if err != nil {
		if strings.Contains(err.Error(), "drivers_company_id_driver_code_key") {
			writeError(w, http.StatusConflict, "driver_code sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat pengemudi")
		return
	}

	h.events.Publish("fleet.driver.created", newAuditEvent("fleet.driver.created", actorFromHeader(r), &d.CompanyID, "create", "driver", d.ID, d))
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handler) updateDriver(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req driverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id dan name wajib diisi")
		return
	}
	if req.Status == "" {
		req.Status = "AVAILABLE"
	}
	if !validDriverStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status hanya bisa diubah manual ke AVAILABLE atau INACTIVE (ON_DELIVERY digerakkan otomatis oleh surat jalan)")
		return
	}

	ctx := r.Context()
	var current model.Driver
	err := scanDriver(h.pool.QueryRow(ctx, `SELECT `+driverColumns+` FROM drivers WHERE id = $1 AND company_id = $2`, id, req.CompanyID), &current)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Pengemudi tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat pengemudi")
		return
	}
	if current.Status == "ON_DELIVERY" {
		writeError(w, http.StatusConflict, "Pengemudi sedang menjalankan surat jalan aktif, selesaikan atau batalkan surat jalannya dulu")
		return
	}

	var d model.Driver
	err = scanDriver(h.pool.QueryRow(ctx, `
		UPDATE drivers SET name = $1, phone = $2, license_number = $3, status = $4, notes = $5, updated_at = now()
		WHERE id = $6 AND company_id = $7
		RETURNING `+driverColumns,
		req.Name, req.Phone, req.LicenseNumber, req.Status, req.Notes, id, req.CompanyID,
	), &d)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui pengemudi")
		return
	}

	h.events.Publish("fleet.driver.updated", newAuditEvent("fleet.driver.updated", actorFromHeader(r), &d.CompanyID, "update", "driver", d.ID, d))
	writeJSON(w, http.StatusOK, d)
}
