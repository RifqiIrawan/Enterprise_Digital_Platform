package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/iot-service/internal/model"
)

const deviceColumns = `id, company_id, branch_id, warehouse_id, device_code, device_type, name, status, threshold_min, threshold_max, created_at, updated_at`

var validDeviceTypes = map[string]bool{
	"TEMPERATURE": true,
	"HUMIDITY":    true,
	"VIBRATION":   true,
	"RFID":        true,
	"GPS":         true,
	"BARCODE":     true,
}

var numericDeviceTypes = map[string]bool{
	"TEMPERATURE": true,
	"HUMIDITY":    true,
	"VIBRATION":   true,
}

func scanDevice(row pgx.Row, d *model.Device) error {
	return row.Scan(&d.ID, &d.CompanyID, &d.BranchID, &d.WarehouseID, &d.DeviceCode, &d.DeviceType, &d.Name, &d.Status, &d.ThresholdMin, &d.ThresholdMax, &d.CreatedAt, &d.UpdatedAt)
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + deviceColumns + ` FROM devices WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	if deviceType := r.URL.Query().Get("device_type"); deviceType != "" {
		args = append(args, deviceType)
		query += ` AND device_type = $` + strconv.Itoa(len(args))
	}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	query += ` ORDER BY device_code ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data device")
		return
	}
	defer rows.Close()

	devices := []model.Device{}
	for rows.Next() {
		var d model.Device
		if err := scanDevice(rows, &d); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data device")
			return
		}
		devices = append(devices, d)
	}
	writeJSON(w, http.StatusOK, devices)
}

func (h *Handler) getDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var d model.Device
	err := scanDevice(h.pool.QueryRow(r.Context(), `SELECT `+deviceColumns+` FROM devices WHERE id = $1`, id), &d)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Device tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat device")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type deviceRequest struct {
	CompanyID    string   `json:"company_id"`
	BranchID     *string  `json:"branch_id"`
	WarehouseID  *string  `json:"warehouse_id"`
	DeviceCode   string   `json:"device_code"`
	DeviceType   string   `json:"device_type"`
	Name         string   `json:"name"`
	ThresholdMin *float64 `json:"threshold_min"`
	ThresholdMax *float64 `json:"threshold_max"`
}

func (req *deviceRequest) validate() (string, bool) {
	req.DeviceCode = strings.TrimSpace(req.DeviceCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.DeviceCode == "" || req.DeviceType == "" || req.Name == "" {
		return "company_id, device_code, device_type, dan name wajib diisi", false
	}
	if !validDeviceTypes[req.DeviceType] {
		return "device_type harus salah satu dari TEMPERATURE, HUMIDITY, VIBRATION, RFID, GPS, BARCODE", false
	}
	if !numericDeviceTypes[req.DeviceType] && (req.ThresholdMin != nil || req.ThresholdMax != nil) {
		return "threshold_min/threshold_max hanya berlaku untuk device_type TEMPERATURE, HUMIDITY, atau VIBRATION", false
	}
	if req.ThresholdMin != nil && req.ThresholdMax != nil && *req.ThresholdMin >= *req.ThresholdMax {
		return "threshold_min harus lebih kecil dari threshold_max", false
	}
	return "", true
}

func (h *Handler) createDevice(w http.ResponseWriter, r *http.Request) {
	var req deviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if msg, ok := req.validate(); !ok {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var d model.Device
	err := scanDevice(h.pool.QueryRow(r.Context(), `
		INSERT INTO devices (company_id, branch_id, warehouse_id, device_code, device_type, name, threshold_min, threshold_max)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+deviceColumns,
		req.CompanyID, req.BranchID, req.WarehouseID, req.DeviceCode, req.DeviceType, req.Name, req.ThresholdMin, req.ThresholdMax,
	), &d)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Kode device sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat device")
		return
	}

	h.events.Publish("iot.device.registered", newAuditEvent("iot.device.registered", actorFromHeader(r), &d.CompanyID, "create", "device", d.ID, d))
	writeJSON(w, http.StatusCreated, d)
}

type updateDeviceRequest struct {
	WarehouseID  *string  `json:"warehouse_id"`
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	ThresholdMin *float64 `json:"threshold_min"`
	ThresholdMax *float64 `json:"threshold_max"`
}

func (h *Handler) updateDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name wajib diisi")
		return
	}
	if req.Status != "ACTIVE" && req.Status != "INACTIVE" && req.Status != "MAINTENANCE" {
		writeError(w, http.StatusBadRequest, "status harus ACTIVE, INACTIVE, atau MAINTENANCE")
		return
	}
	if req.ThresholdMin != nil && req.ThresholdMax != nil && *req.ThresholdMin >= *req.ThresholdMax {
		writeError(w, http.StatusBadRequest, "threshold_min harus lebih kecil dari threshold_max")
		return
	}

	var d model.Device
	err := scanDevice(h.pool.QueryRow(r.Context(), `
		UPDATE devices SET warehouse_id = $1, name = $2, status = $3, threshold_min = $4, threshold_max = $5, updated_at = now()
		WHERE id = $6
		RETURNING `+deviceColumns,
		req.WarehouseID, req.Name, req.Status, req.ThresholdMin, req.ThresholdMax, id,
	), &d)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Device tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui device")
		return
	}

	h.events.Publish("iot.device.updated", newAuditEvent("iot.device.updated", actorFromHeader(r), &d.CompanyID, "update", "device", d.ID, d))
	writeJSON(w, http.StatusOK, d)
}
