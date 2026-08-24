package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/production-service/internal/model"
)

const machineColumns = `id, company_id, branch_id, code, name, machine_type, location, ideal_cycle_time_minutes, status, notes, created_at, updated_at`

var machineStatuses = map[string]bool{"ACTIVE": true, "MAINTENANCE": true, "INACTIVE": true}

func scanMachine(row pgx.Row, m *model.Machine) error {
	return row.Scan(&m.ID, &m.CompanyID, &m.BranchID, &m.Code, &m.Name, &m.MachineType, &m.Location,
		&m.IdealCycleTimeMinutes, &m.Status, &m.Notes, &m.CreatedAt, &m.UpdatedAt)
}

func (h *Handler) listMachines(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + machineColumns + ` FROM machines WHERE company_id = $1`
	args := []any{companyID}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	query += ` ORDER BY code ASC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data mesin")
		return
	}
	defer rows.Close()

	machines := []model.Machine{}
	for rows.Next() {
		var m model.Machine
		if err := scanMachine(rows, &m); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data mesin")
			return
		}
		machines = append(machines, m)
	}
	writeJSON(w, http.StatusOK, machines)
}

type createMachineRequest struct {
	CompanyID             string  `json:"company_id"`
	BranchID              *string `json:"branch_id"`
	Code                  string  `json:"code"`
	Name                  string  `json:"name"`
	MachineType           string  `json:"machine_type"`
	Location              string  `json:"location"`
	IdealCycleTimeMinutes float64 `json:"ideal_cycle_time_minutes"`
	Notes                 string  `json:"notes"`
}

func (h *Handler) createMachine(w http.ResponseWriter, r *http.Request) {
	var req createMachineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	req.MachineType = strings.TrimSpace(req.MachineType)
	if req.CompanyID == "" || req.Code == "" || req.Name == "" || req.IdealCycleTimeMinutes <= 0 {
		writeError(w, http.StatusBadRequest, "company_id, code, name, dan ideal_cycle_time_minutes > 0 wajib diisi")
		return
	}
	if req.MachineType == "" {
		req.MachineType = "GENERAL"
	}

	var m model.Machine
	err := scanMachine(h.pool.QueryRow(r.Context(), `
		INSERT INTO machines (company_id, branch_id, code, name, machine_type, location, ideal_cycle_time_minutes, notes)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8)
		RETURNING `+machineColumns,
		req.CompanyID, req.BranchID, req.Code, req.Name, req.MachineType, strings.TrimSpace(req.Location), req.IdealCycleTimeMinutes, req.Notes,
	), &m)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Kode mesin sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat mesin")
		return
	}

	h.events.Publish("production.machine.created", newAuditEvent("production.machine.created", actorFromHeader(r), &m.CompanyID, "create", "machine", m.ID, m))
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handler) getMachine(w http.ResponseWriter, r *http.Request) {
	var m model.Machine
	err := scanMachine(h.pool.QueryRow(r.Context(), `SELECT `+machineColumns+` FROM machines WHERE id = $1`, r.PathValue("id")), &m)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Mesin tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat mesin")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

type updateMachineRequest struct {
	Name                  string  `json:"name"`
	MachineType           string  `json:"machine_type"`
	Location              string  `json:"location"`
	IdealCycleTimeMinutes float64 `json:"ideal_cycle_time_minutes"`
	Status                string  `json:"status"`
	Notes                 string  `json:"notes"`
}

// updateMachine menolak menonaktifkan mesin yang masih punya run berjalan.
// Mesin yang ditarik ke MAINTENANCE sementara run-nya masih OPEN berarti
// waktu shift-nya tetap berjalan tanpa ada yang bisa menutup run itu lewat
// UI -- lebih baik ditolak dengan alasan yang jelas daripada meninggalkan
// run menggantung yang merusak Availability mesin itu.
func (h *Handler) updateMachine(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateMachineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.MachineType = strings.TrimSpace(req.MachineType)
	req.Status = strings.TrimSpace(req.Status)
	if req.Name == "" || req.IdealCycleTimeMinutes <= 0 {
		writeError(w, http.StatusBadRequest, "name dan ideal_cycle_time_minutes > 0 wajib diisi")
		return
	}
	if req.MachineType == "" {
		req.MachineType = "GENERAL"
	}
	if req.Status == "" {
		req.Status = "ACTIVE"
	}
	if !machineStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status harus ACTIVE, MAINTENANCE, atau INACTIVE")
		return
	}

	ctx := r.Context()
	if req.Status != "ACTIVE" {
		var openRuns int
		if err := h.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM production_runs WHERE machine_id = $1 AND status = 'OPEN'`, id,
		).Scan(&openRuns); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memeriksa run berjalan")
			return
		}
		if openRuns > 0 {
			writeError(w, http.StatusConflict, "Mesin ini masih punya production run berjalan; tutup run-nya dulu")
			return
		}
	}

	var m model.Machine
	err := scanMachine(h.pool.QueryRow(ctx, `
		UPDATE machines SET name = $1, machine_type = $2, location = NULLIF($3, ''), ideal_cycle_time_minutes = $4,
		       status = $5, notes = $6, updated_at = now()
		WHERE id = $7
		RETURNING `+machineColumns,
		req.Name, req.MachineType, strings.TrimSpace(req.Location), req.IdealCycleTimeMinutes, req.Status, req.Notes, id,
	), &m)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Mesin tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui mesin")
		return
	}

	h.events.Publish("production.machine.updated", newAuditEvent("production.machine.updated", actorFromHeader(r), &m.CompanyID, "update", "machine", m.ID, m))
	writeJSON(w, http.StatusOK, m)
}
