package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/hr-service/internal/model"
)

var validEmploymentTypes = map[string]bool{
	"PERMANENT": true, "CONTRACT": true, "INTERN": true, "OUTSOURCE": true,
}

var validPTKPStatuses = map[string]bool{
	"TK/0": true, "TK/1": true, "TK/2": true, "TK/3": true,
	"K/0": true, "K/1": true, "K/2": true, "K/3": true,
}

const employeeColumns = `id, company_id, branch_id, employee_code, first_name, last_name, email, phone, department, job_title,
	manager_id, employment_type, status, hire_date, termination_date, basic_salary, monthly_allowance, ptkp_status,
	is_active, created_at, updated_at`

func scanEmployee(row pgx.Row, e *model.Employee) error {
	return row.Scan(&e.ID, &e.CompanyID, &e.BranchID, &e.EmployeeCode, &e.FirstName, &e.LastName, &e.Email, &e.Phone,
		&e.Department, &e.JobTitle, &e.ManagerID, &e.EmploymentType, &e.Status, &e.HireDate, &e.TerminationDate,
		&e.BasicSalary, &e.MonthlyAllowance, &e.PTKPStatus, &e.IsActive, &e.CreatedAt, &e.UpdatedAt)
}

func (h *Handler) listEmployees(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}

	query := `SELECT ` + employeeColumns + ` FROM employees WHERE company_id = $1`
	args := []any{companyID}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += " AND status = $2"
	}
	query += " ORDER BY employee_code ASC"

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data karyawan")
		return
	}
	defer rows.Close()

	employees := []model.Employee{}
	for rows.Next() {
		var e model.Employee
		if err := scanEmployee(rows, &e); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data karyawan")
			return
		}
		employees = append(employees, e)
	}
	writeJSON(w, http.StatusOK, employees)
}

type employeeRequest struct {
	CompanyID        string  `json:"company_id"`
	BranchID         *string `json:"branch_id"`
	EmployeeCode     string  `json:"employee_code"`
	FirstName        string  `json:"first_name"`
	LastName         string  `json:"last_name"`
	Email            string  `json:"email"`
	Phone            string  `json:"phone"`
	Department       string  `json:"department"`
	JobTitle         string  `json:"job_title"`
	ManagerID        *string `json:"manager_id"`
	EmploymentType   string  `json:"employment_type"`
	HireDate         string  `json:"hire_date"` // YYYY-MM-DD
	BasicSalary      float64 `json:"basic_salary"`
	MonthlyAllowance float64 `json:"monthly_allowance"`
	PTKPStatus       string  `json:"ptkp_status"`
}

func (h *Handler) createEmployee(w http.ResponseWriter, r *http.Request) {
	var req employeeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.EmployeeCode = strings.TrimSpace(req.EmployeeCode)
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.Email = strings.TrimSpace(req.Email)
	req.EmploymentType = strings.ToUpper(strings.TrimSpace(req.EmploymentType))
	if req.EmploymentType == "" {
		req.EmploymentType = "PERMANENT"
	}
	if req.PTKPStatus == "" {
		req.PTKPStatus = "TK/0"
	}
	if req.CompanyID == "" || req.EmployeeCode == "" || req.FirstName == "" || req.Email == "" || req.HireDate == "" {
		writeError(w, http.StatusBadRequest, "company_id, employee_code, first_name, email, dan hire_date wajib diisi")
		return
	}
	if !validEmploymentTypes[req.EmploymentType] {
		writeError(w, http.StatusBadRequest, "employment_type harus salah satu dari PERMANENT, CONTRACT, INTERN, OUTSOURCE")
		return
	}
	if !validPTKPStatuses[req.PTKPStatus] {
		writeError(w, http.StatusBadRequest, "ptkp_status tidak valid")
		return
	}
	hireDate, err := time.Parse("2006-01-02", req.HireDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "hire_date harus format YYYY-MM-DD")
		return
	}
	if req.BasicSalary < 0 || req.MonthlyAllowance < 0 {
		writeError(w, http.StatusBadRequest, "basic_salary dan monthly_allowance tidak boleh negatif")
		return
	}

	var e model.Employee
	err = h.pool.QueryRow(r.Context(), `
		INSERT INTO employees (company_id, branch_id, employee_code, first_name, last_name, email, phone, department,
		                       job_title, manager_id, employment_type, hire_date, basic_salary, monthly_allowance, ptkp_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING `+employeeColumns,
		req.CompanyID, req.BranchID, req.EmployeeCode, req.FirstName, req.LastName, req.Email, req.Phone, req.Department,
		req.JobTitle, req.ManagerID, req.EmploymentType, hireDate, req.BasicSalary, req.MonthlyAllowance, req.PTKPStatus,
	).Scan(&e.ID, &e.CompanyID, &e.BranchID, &e.EmployeeCode, &e.FirstName, &e.LastName, &e.Email, &e.Phone,
		&e.Department, &e.JobTitle, &e.ManagerID, &e.EmploymentType, &e.Status, &e.HireDate, &e.TerminationDate,
		&e.BasicSalary, &e.MonthlyAllowance, &e.PTKPStatus, &e.IsActive, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Employee code atau email sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat data karyawan")
		return
	}

	h.events.Publish("hr.employee.created", newAuditEvent("hr.employee.created", actorFromHeader(r), &e.CompanyID, "create", "employee", e.ID, e))
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handler) getEmployee(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var e model.Employee
	err := scanEmployee(h.pool.QueryRow(r.Context(), `SELECT `+employeeColumns+` FROM employees WHERE id = $1`, id), &e)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Karyawan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data karyawan")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

type updateEmployeeRequest struct {
	FirstName        string  `json:"first_name"`
	LastName         string  `json:"last_name"`
	Phone            string  `json:"phone"`
	Department       string  `json:"department"`
	JobTitle         string  `json:"job_title"`
	ManagerID        *string `json:"manager_id"`
	EmploymentType   string  `json:"employment_type"`
	Status           string  `json:"status"`
	BasicSalary      float64 `json:"basic_salary"`
	MonthlyAllowance float64 `json:"monthly_allowance"`
	PTKPStatus       string  `json:"ptkp_status"`
	TerminationDate  *string `json:"termination_date"`
	IsActive         bool    `json:"is_active"`
}

func (h *Handler) updateEmployee(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateEmployeeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.EmploymentType = strings.ToUpper(strings.TrimSpace(req.EmploymentType))
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	if req.FirstName == "" {
		writeError(w, http.StatusBadRequest, "first_name wajib diisi")
		return
	}
	if !validEmploymentTypes[req.EmploymentType] {
		writeError(w, http.StatusBadRequest, "employment_type tidak valid")
		return
	}
	if !validPTKPStatuses[req.PTKPStatus] {
		writeError(w, http.StatusBadRequest, "ptkp_status tidak valid")
		return
	}
	validStatuses := map[string]bool{"ACTIVE": true, "INACTIVE": true, "TERMINATED": true, "ON_LEAVE": true}
	if !validStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status tidak valid")
		return
	}
	if req.BasicSalary < 0 || req.MonthlyAllowance < 0 {
		writeError(w, http.StatusBadRequest, "basic_salary dan monthly_allowance tidak boleh negatif")
		return
	}
	var terminationDate *time.Time
	if req.TerminationDate != nil && *req.TerminationDate != "" {
		d, err := time.Parse("2006-01-02", *req.TerminationDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "termination_date harus format YYYY-MM-DD")
			return
		}
		terminationDate = &d
	}

	var e model.Employee
	err := h.pool.QueryRow(r.Context(), `
		UPDATE employees SET first_name = $1, last_name = $2, phone = $3, department = $4, job_title = $5,
		       manager_id = $6, employment_type = $7, status = $8, basic_salary = $9, monthly_allowance = $10,
		       ptkp_status = $11, termination_date = $12, is_active = $13, updated_at = now()
		WHERE id = $14
		RETURNING `+employeeColumns,
		req.FirstName, req.LastName, req.Phone, req.Department, req.JobTitle, req.ManagerID, req.EmploymentType,
		req.Status, req.BasicSalary, req.MonthlyAllowance, req.PTKPStatus, terminationDate, req.IsActive, id,
	).Scan(&e.ID, &e.CompanyID, &e.BranchID, &e.EmployeeCode, &e.FirstName, &e.LastName, &e.Email, &e.Phone,
		&e.Department, &e.JobTitle, &e.ManagerID, &e.EmploymentType, &e.Status, &e.HireDate, &e.TerminationDate,
		&e.BasicSalary, &e.MonthlyAllowance, &e.PTKPStatus, &e.IsActive, &e.CreatedAt, &e.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Karyawan tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui data karyawan")
		return
	}

	h.events.Publish("hr.employee.updated", newAuditEvent("hr.employee.updated", actorFromHeader(r), &e.CompanyID, "update", "employee", e.ID, e))
	writeJSON(w, http.StatusOK, e)
}
