package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/fleet-service/internal/model"
)

const vehicleColumns = `id, company_id, branch_id, vehicle_code, plate_number, name, vehicle_type, capacity_kg, status, notes, created_at, updated_at`

var validVehicleTypes = map[string]bool{"MOTORCYCLE": true, "VAN": true, "TRUCK": true}

// validVehicleStatuses SENGAJA tidak memuat IN_USE: status itu hanya boleh
// dipasang oleh lifecycle delivery order (dispatch), bukan diketik manual
// lewat PUT -- kalau boleh, kendaraan bisa ditandai bebas padahal sedang
// dalam perjalanan, dan sebaliknya. MAINTENANCE tetap manual karena itu
// keputusan operasional di luar sistem ini.
var validVehicleStatuses = map[string]bool{"AVAILABLE": true, "MAINTENANCE": true}

func scanVehicle(row pgx.Row, v *model.Vehicle) error {
	return row.Scan(&v.ID, &v.CompanyID, &v.BranchID, &v.VehicleCode, &v.PlateNumber, &v.Name, &v.VehicleType, &v.CapacityKg, &v.Status, &v.Notes, &v.CreatedAt, &v.UpdatedAt)
}

func (h *Handler) listVehicles(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + vehicleColumns + ` FROM vehicles WHERE company_id = $1`
	args := []any{companyID}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY vehicle_code ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data kendaraan")
		return
	}
	defer rows.Close()

	vehicles := []model.Vehicle{}
	for rows.Next() {
		var v model.Vehicle
		if err := scanVehicle(rows, &v); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data kendaraan")
			return
		}
		vehicles = append(vehicles, v)
	}
	writeJSON(w, http.StatusOK, vehicles)
}

type vehicleRequest struct {
	CompanyID   string  `json:"company_id"`
	BranchID    *string `json:"branch_id"`
	VehicleCode string  `json:"vehicle_code"`
	PlateNumber string  `json:"plate_number"`
	Name        string  `json:"name"`
	VehicleType string  `json:"vehicle_type"`
	CapacityKg  float64 `json:"capacity_kg"`
	Status      string  `json:"status"`
	Notes       string  `json:"notes"`
}

// vehicle_code diisi manual oleh user (master data, sama seperti
// customers.customer_code di sales-service dan accounts.account_code di
// crm-service) -- BUKAN auto-generate seperti delivery_number, yang memang
// nomor dokumen transaksional.
func (h *Handler) createVehicle(w http.ResponseWriter, r *http.Request) {
	var req vehicleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.VehicleCode = strings.TrimSpace(req.VehicleCode)
	req.PlateNumber = strings.TrimSpace(req.PlateNumber)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.VehicleCode == "" || req.PlateNumber == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, vehicle_code, plate_number, dan name wajib diisi")
		return
	}
	if req.VehicleType == "" {
		req.VehicleType = "VAN"
	}
	if !validVehicleTypes[req.VehicleType] {
		writeError(w, http.StatusBadRequest, "vehicle_type harus MOTORCYCLE, VAN, atau TRUCK")
		return
	}
	if req.CapacityKg < 0 {
		writeError(w, http.StatusBadRequest, "capacity_kg tidak boleh negatif")
		return
	}

	var v model.Vehicle
	err := scanVehicle(h.pool.QueryRow(r.Context(), `
		INSERT INTO vehicles (company_id, branch_id, vehicle_code, plate_number, name, vehicle_type, capacity_kg, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+vehicleColumns,
		req.CompanyID, req.BranchID, req.VehicleCode, req.PlateNumber, req.Name, req.VehicleType, req.CapacityKg, req.Notes,
	), &v)
	if err != nil {
		if strings.Contains(err.Error(), "vehicles_company_id_vehicle_code_key") {
			writeError(w, http.StatusConflict, "vehicle_code sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat kendaraan")
		return
	}

	h.events.Publish("fleet.vehicle.created", newAuditEvent("fleet.vehicle.created", actorFromHeader(r), &v.CompanyID, "create", "vehicle", v.ID, v))
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handler) updateVehicle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req vehicleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.PlateNumber = strings.TrimSpace(req.PlateNumber)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.PlateNumber == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, plate_number, dan name wajib diisi")
		return
	}
	if !validVehicleTypes[req.VehicleType] {
		writeError(w, http.StatusBadRequest, "vehicle_type harus MOTORCYCLE, VAN, atau TRUCK")
		return
	}
	if req.Status == "" {
		req.Status = "AVAILABLE"
	}
	if !validVehicleStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status hanya bisa diubah manual ke AVAILABLE atau MAINTENANCE (IN_USE digerakkan otomatis oleh surat jalan)")
		return
	}

	ctx := r.Context()
	var current model.Vehicle
	err := scanVehicle(h.pool.QueryRow(ctx, `SELECT `+vehicleColumns+` FROM vehicles WHERE id = $1 AND company_id = $2`, id, req.CompanyID), &current)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Kendaraan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat kendaraan")
		return
	}
	// Kendaraan yang sedang jalan tidak boleh dipindah statusnya lewat PUT --
	// selesaikan atau batalkan dulu surat jalannya.
	if current.Status == "IN_USE" {
		writeError(w, http.StatusConflict, "Kendaraan sedang dipakai surat jalan aktif, selesaikan atau batalkan surat jalannya dulu")
		return
	}

	var v model.Vehicle
	err = scanVehicle(h.pool.QueryRow(ctx, `
		UPDATE vehicles SET plate_number = $1, name = $2, vehicle_type = $3, capacity_kg = $4, status = $5, notes = $6, updated_at = now()
		WHERE id = $7 AND company_id = $8
		RETURNING `+vehicleColumns,
		req.PlateNumber, req.Name, req.VehicleType, req.CapacityKg, req.Status, req.Notes, id, req.CompanyID,
	), &v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui kendaraan")
		return
	}

	h.events.Publish("fleet.vehicle.updated", newAuditEvent("fleet.vehicle.updated", actorFromHeader(r), &v.CompanyID, "update", "vehicle", v.ID, v))
	writeJSON(w, http.StatusOK, v)
}
