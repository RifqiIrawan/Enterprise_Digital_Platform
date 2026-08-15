package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/project-service/internal/model"
)

const projectColumns = `id, company_id, branch_id, project_code, name, description, customer_name, manager_employee_id, manager_name, start_date, end_date, status, budget_amount, actual_cost, completed_at, cancelled_at, notes, created_by_user_id, created_at, updated_at`

func scanProject(row pgx.Row, p *model.Project) error {
	return row.Scan(&p.ID, &p.CompanyID, &p.BranchID, &p.ProjectCode, &p.Name, &p.Description, &p.CustomerName, &p.ManagerEmployeeID, &p.ManagerName, &p.StartDate, &p.EndDate, &p.Status, &p.BudgetAmount, &p.ActualCost, &p.CompletedAt, &p.CancelledAt, &p.Notes, &p.CreatedByUserID, &p.CreatedAt, &p.UpdatedAt)
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + projectColumns + ` FROM projects WHERE company_id = $1`
	args := []any{companyID}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data proyek")
		return
	}
	defer rows.Close()

	projects := []model.Project{}
	for rows.Next() {
		var p model.Project
		if err := scanProject(rows, &p); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data proyek")
			return
		}
		projects = append(projects, p)
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) {
	var p model.Project
	err := scanProject(h.pool.QueryRow(r.Context(), `SELECT `+projectColumns+` FROM projects WHERE id = $1`, r.PathValue("id")), &p)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Proyek tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat proyek")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type projectRequest struct {
	CompanyID         string  `json:"company_id"`
	BranchID          *string `json:"branch_id"`
	ProjectCode       string  `json:"project_code"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	CustomerName      string  `json:"customer_name"`
	ManagerEmployeeID string  `json:"manager_employee_id"`
	StartDate         string  `json:"start_date"`
	EndDate           string  `json:"end_date"`
	BudgetAmount      float64 `json:"budget_amount"`
	Notes             string  `json:"notes"`
}

// resolveEmployee memvalidasi karyawan ke hr-service dan mengembalikan nama
// untuk di-snapshot. Karyawan WAJIB milik company yang sama (guard
// lintas-company: tanpa itu UUID milik company lain bisa dipakai) dan WAJIB
// berstatus ACTIVE -- menugaskan pekerjaan ke karyawan TERMINATED/INACTIVE
// tidak masuk akal, dan kalau dibiarkan, biayanya nanti tetap ikut terposting
// ke GL lewat timesheet.
//
// Mengembalikan (status HTTP, pesan) seperti assertAssignable di fleet-service:
// status 0 berarti lolos.
func (h *Handler) resolveEmployee(companyID, employeeID string) (name string, httpStatus int, msg string) {
	employee, err := h.hr.GetEmployee(employeeID)
	if err != nil {
		return "", http.StatusBadGateway, fmt.Sprintf("Gagal memuat karyawan dari hr-service: %v", err)
	}
	if employee.CompanyID != companyID {
		return "", http.StatusBadRequest, "Karyawan tersebut milik company lain"
	}
	if employee.Status != "ACTIVE" {
		return "", http.StatusConflict, "Karyawan tidak berstatus ACTIVE (status sekarang: " + employee.Status + ")"
	}
	return employee.FullName(), 0, ""
}

func parseOptionalDate(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var req projectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.ProjectCode = strings.TrimSpace(req.ProjectCode)
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.ProjectCode == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id, project_code, dan name wajib diisi")
		return
	}
	if req.BudgetAmount < 0 {
		writeError(w, http.StatusBadRequest, "budget_amount tidak boleh negatif")
		return
	}

	startDate := time.Now()
	if req.StartDate != "" {
		parsed, err := time.Parse("2006-01-02", req.StartDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "start_date harus format YYYY-MM-DD")
			return
		}
		startDate = parsed
	}
	endDate, err := parseOptionalDate(req.EndDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "end_date harus format YYYY-MM-DD")
		return
	}
	if endDate != nil && endDate.Before(startDate) {
		writeError(w, http.StatusBadRequest, "end_date tidak boleh lebih awal dari start_date")
		return
	}

	var managerID, managerName *string
	if req.ManagerEmployeeID != "" {
		name, status, msg := h.resolveEmployee(req.CompanyID, req.ManagerEmployeeID)
		if status != 0 {
			writeError(w, status, msg)
			return
		}
		managerID, managerName = &req.ManagerEmployeeID, &name
	}

	var p model.Project
	err = scanProject(h.pool.QueryRow(r.Context(), `
		INSERT INTO projects (company_id, branch_id, project_code, name, description, customer_name, manager_employee_id, manager_name, start_date, end_date, budget_amount, notes, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+projectColumns,
		req.CompanyID, req.BranchID, req.ProjectCode, req.Name, req.Description, req.CustomerName, managerID, managerName, startDate, endDate, req.BudgetAmount, req.Notes, actorFromHeader(r),
	), &p)
	if err != nil {
		if strings.Contains(err.Error(), "projects_company_id_project_code_key") {
			writeError(w, http.StatusConflict, "project_code sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat proyek")
		return
	}

	h.events.Publish("project.project.created", newAuditEvent("project.project.created", actorFromHeader(r), &p.CompanyID, "create", "project", p.ID, p))
	writeJSON(w, http.StatusCreated, p)
}

// updateProject cuma untuk field non-status dan non-uang-realisasi: status
// pindah lewat endpoint transisi, dan actual_cost hanya bergerak lewat posting
// timesheet ke GL. Proyek yang sudah COMPLETED/CANCELLED tidak bisa diubah
// lagi -- riwayatnya sudah jadi arsip.
func (h *Handler) updateProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req projectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.CompanyID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "company_id dan name wajib diisi")
		return
	}
	if req.BudgetAmount < 0 {
		writeError(w, http.StatusBadRequest, "budget_amount tidak boleh negatif")
		return
	}

	ctx := r.Context()
	var before model.Project
	err := scanProject(h.pool.QueryRow(ctx, `SELECT `+projectColumns+` FROM projects WHERE id = $1 AND company_id = $2`, id, req.CompanyID), &before)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Proyek tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat proyek")
		return
	}
	if before.Status == "COMPLETED" || before.Status == "CANCELLED" {
		writeError(w, http.StatusConflict, "Proyek yang sudah "+before.Status+" tidak bisa diubah lagi")
		return
	}

	startDate := before.StartDate
	if req.StartDate != "" {
		parsed, err := time.Parse("2006-01-02", req.StartDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "start_date harus format YYYY-MM-DD")
			return
		}
		startDate = parsed
	}
	endDate, err := parseOptionalDate(req.EndDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "end_date harus format YYYY-MM-DD")
		return
	}
	if endDate != nil && endDate.Before(startDate) {
		writeError(w, http.StatusBadRequest, "end_date tidak boleh lebih awal dari start_date")
		return
	}

	// Manajer boleh dikosongkan (dilepas) dengan mengirim string kosong.
	var managerID, managerName *string
	if req.ManagerEmployeeID != "" {
		name, status, msg := h.resolveEmployee(req.CompanyID, req.ManagerEmployeeID)
		if status != 0 {
			writeError(w, status, msg)
			return
		}
		managerID, managerName = &req.ManagerEmployeeID, &name
	}

	var p model.Project
	err = scanProject(h.pool.QueryRow(ctx, `
		UPDATE projects SET name = $1, description = $2, customer_name = $3, manager_employee_id = $4, manager_name = $5, start_date = $6, end_date = $7, budget_amount = $8, notes = $9, updated_at = now()
		WHERE id = $10 AND company_id = $11
		RETURNING `+projectColumns,
		req.Name, req.Description, req.CustomerName, managerID, managerName, startDate, endDate, req.BudgetAmount, req.Notes, id, req.CompanyID,
	), &p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui proyek")
		return
	}

	h.events.Publish("project.project.updated", newAuditEvent("project.project.updated", actorFromHeader(r), &p.CompanyID, "update", "project", p.ID, p))
	writeJSON(w, http.StatusOK, p)
}

// transitionProject adalah jalur bersama untuk activate/hold/cancel: baca +
// kunci barisnya, cek status asalnya diizinkan, lalu tulis status barunya.
// completeProject TIDAK memakai ini karena dia punya guard lintas-entitas
// tersendiri (lihat di bawah).
func (h *Handler) transitionProject(w http.ResponseWriter, r *http.Request, from []string, to, timestampColumn, eventType, notAllowedMsg string) {
	id := r.PathValue("id")
	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var p model.Project
	err = scanProject(tx.QueryRow(ctx, `SELECT `+projectColumns+` FROM projects WHERE id = $1 FOR UPDATE`, id), &p)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Proyek tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat proyek")
		return
	}
	if !contains(from, p.Status) {
		writeError(w, http.StatusConflict, notAllowedMsg+" (status sekarang: "+p.Status+")")
		return
	}

	setClause := `status = $1, updated_at = now()`
	if timestampColumn != "" {
		setClause += `, ` + timestampColumn + ` = now()`
	}
	err = scanProject(tx.QueryRow(ctx, `UPDATE projects SET `+setClause+` WHERE id = $2 RETURNING `+projectColumns, to, id), &p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status proyek")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan perubahan proyek")
		return
	}

	h.events.Publish(eventType, newAuditEvent(eventType, actorFromHeader(r), &p.CompanyID, "update", "project", p.ID, p))
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) activateProject(w http.ResponseWriter, r *http.Request) {
	h.transitionProject(w, r, []string{"PLANNING", "ON_HOLD"}, "ACTIVE", "", "project.project.activated", "Proyek hanya bisa diaktifkan dari status PLANNING atau ON_HOLD")
}

func (h *Handler) holdProject(w http.ResponseWriter, r *http.Request) {
	h.transitionProject(w, r, []string{"ACTIVE"}, "ON_HOLD", "", "project.project.held", "Proyek hanya bisa ditahan dari status ACTIVE")
}

func (h *Handler) cancelProject(w http.ResponseWriter, r *http.Request) {
	h.transitionProject(w, r, []string{"PLANNING", "ACTIVE", "ON_HOLD"}, "CANCELLED", "cancelled_at", "project.project.cancelled", "Proyek yang sudah selesai atau batal tidak bisa dibatalkan lagi")
}

// completeProject adalah satu-satunya transisi dengan guard lintas-entitas:
// proyek tidak boleh ditutup selagi masih ada tugas yang belum selesai
// (TODO/IN_PROGRESS) atau timesheet yang belum tuntas diproses
// (DRAFT/APPROVED). Yang kedua penting secara akuntansi -- timesheet APPROVED
// yang belum diposting adalah biaya yang sudah diakui tapi belum masuk GL;
// kalau proyeknya keburu ditutup, actual_cost-nya permanen understated dan
// tidak ada lagi jalur untuk mempostingnya (post-cost menolak proyek non-ACTIVE).
//
// Semuanya dibaca di dalam transaksi yang sama dengan SELECT ... FOR UPDATE
// pada barisnya, supaya tugas/timesheet baru yang masuk bersamaan tidak lolos
// di antara pengecekan dan penulisan status.
func (h *Handler) completeProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var p model.Project
	err = scanProject(tx.QueryRow(ctx, `SELECT `+projectColumns+` FROM projects WHERE id = $1 FOR UPDATE`, id), &p)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Proyek tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat proyek")
		return
	}
	if p.Status != "ACTIVE" {
		writeError(w, http.StatusConflict, "Proyek hanya bisa diselesaikan dari status ACTIVE (status sekarang: "+p.Status+")")
		return
	}

	var openTasks int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE project_id = $1 AND status IN ('TODO', 'IN_PROGRESS')`, id).Scan(&openTasks); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa tugas proyek")
		return
	}
	if openTasks > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("Masih ada %d tugas yang belum selesai; selesaikan atau batalkan dulu", openTasks))
		return
	}

	var openTimesheets int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM timesheets WHERE project_id = $1 AND status IN ('DRAFT', 'APPROVED')`, id).Scan(&openTimesheets); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa timesheet proyek")
		return
	}
	if openTimesheets > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("Masih ada %d timesheet yang belum diposting ke GL atau belum ditolak; proses dulu sebelum menutup proyek", openTimesheets))
		return
	}

	err = scanProject(tx.QueryRow(ctx, `
		UPDATE projects SET status = 'COMPLETED', completed_at = now(), updated_at = now()
		WHERE id = $1 RETURNING `+projectColumns, id), &p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyelesaikan proyek")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan perubahan proyek")
		return
	}

	h.events.Publish("project.project.completed", newAuditEvent("project.project.completed", actorFromHeader(r), &p.CompanyID, "update", "project", p.ID, p))
	writeJSON(w, http.StatusOK, p)
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
