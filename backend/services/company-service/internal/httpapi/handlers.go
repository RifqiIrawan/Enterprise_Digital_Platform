package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enterprise-digital-platform/company-service/internal/eventbus"
	"github.com/enterprise-digital-platform/company-service/internal/metrics"
	"github.com/enterprise-digital-platform/company-service/internal/model"
)

type Handler struct {
	pool   *pgxpool.Pool
	events *eventbus.Publisher
}

func NewHandler(pool *pgxpool.Pool, events *eventbus.Publisher) *Handler {
	return &Handler{pool: pool, events: events}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /companies", h.listCompanies)
	mux.HandleFunc("POST /companies", h.createCompany)
	mux.HandleFunc("GET /companies/{id}", h.getCompany)
	mux.HandleFunc("PUT /companies/{id}", h.updateCompany)

	mux.HandleFunc("GET /companies/{id}/branches", h.listBranches)
	mux.HandleFunc("POST /companies/{id}/branches", h.createBranch)

	mux.HandleFunc("GET /companies/{id}/departments", h.listDepartments)
	mux.HandleFunc("POST /companies/{id}/departments", h.createDepartment)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "company-service"})
}

func (h *Handler) listCompanies(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, code, name, status, created_at, updated_at FROM companies ORDER BY name ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar company")
		return
	}
	defer rows.Close()

	companies := []model.Company{}
	for rows.Next() {
		var c model.Company
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data company")
			return
		}
		companies = append(companies, c)
	}
	writeJSON(w, http.StatusOK, companies)
}

type createCompanyRequest struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func (h *Handler) createCompany(w http.ResponseWriter, r *http.Request) {
	var req createCompanyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Code = strings.TrimSpace(strings.ToUpper(req.Code))
	req.Name = strings.TrimSpace(req.Name)
	if req.Code == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "Code dan nama wajib diisi")
		return
	}

	var c model.Company
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO companies (code, name) VALUES ($1, $2)
		 RETURNING id, code, name, status, created_at, updated_at`,
		req.Code, req.Name,
	).Scan(&c.ID, &c.Code, &c.Name, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Code company sudah dipakai")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat company")
		return
	}

	h.events.Publish("company.company.created", newAuditEvent("company.company.created", &c.ID, "create", "company", c.ID, c))
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) getCompany(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var c model.Company
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, code, name, status, created_at, updated_at FROM companies WHERE id = $1`, id,
	).Scan(&c.ID, &c.Code, &c.Name, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Company tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat company")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type updateCompanyRequest struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (h *Handler) updateCompany(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateCompanyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Nama wajib diisi")
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}

	var c model.Company
	err := h.pool.QueryRow(r.Context(),
		`UPDATE companies SET name = $1, status = $2, updated_at = now() WHERE id = $3
		 RETURNING id, code, name, status, created_at, updated_at`,
		req.Name, req.Status, id,
	).Scan(&c.ID, &c.Code, &c.Name, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Company tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui company")
		return
	}

	h.events.Publish("company.company.updated", newAuditEvent("company.company.updated", &c.ID, "update", "company", c.ID, c))
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) listBranches(w http.ResponseWriter, r *http.Request) {
	companyID := r.PathValue("id")
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, company_id, code, name, address, status, created_at, updated_at
		 FROM branches WHERE company_id = $1 ORDER BY name ASC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar branch")
		return
	}
	defer rows.Close()

	branches := []model.Branch{}
	for rows.Next() {
		var b model.Branch
		if err := rows.Scan(&b.ID, &b.CompanyID, &b.Code, &b.Name, &b.Address, &b.Status, &b.CreatedAt, &b.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data branch")
			return
		}
		branches = append(branches, b)
	}
	writeJSON(w, http.StatusOK, branches)
}

type createBranchRequest struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

func (h *Handler) createBranch(w http.ResponseWriter, r *http.Request) {
	companyID := r.PathValue("id")
	var req createBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Code = strings.TrimSpace(strings.ToUpper(req.Code))
	req.Name = strings.TrimSpace(req.Name)
	if req.Code == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "Code dan nama wajib diisi")
		return
	}

	var b model.Branch
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO branches (company_id, code, name, address) VALUES ($1, $2, $3, $4)
		 RETURNING id, company_id, code, name, address, status, created_at, updated_at`,
		companyID, req.Code, req.Name, req.Address,
	).Scan(&b.ID, &b.CompanyID, &b.Code, &b.Name, &b.Address, &b.Status, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Code branch sudah dipakai di company ini")
			return
		}
		if strings.Contains(err.Error(), "foreign key") {
			writeError(w, http.StatusBadRequest, "Company tidak ditemukan")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat branch")
		return
	}

	h.events.Publish("company.branch.created", newAuditEvent("company.branch.created", &companyID, "create", "branch", b.ID, b))
	writeJSON(w, http.StatusCreated, b)
}

func (h *Handler) listDepartments(w http.ResponseWriter, r *http.Request) {
	companyID := r.PathValue("id")
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, company_id, branch_id, code, name, status, created_at, updated_at
		 FROM departments WHERE company_id = $1 ORDER BY name ASC`, companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar department")
		return
	}
	defer rows.Close()

	departments := []model.Department{}
	for rows.Next() {
		var d model.Department
		if err := rows.Scan(&d.ID, &d.CompanyID, &d.BranchID, &d.Code, &d.Name, &d.Status, &d.CreatedAt, &d.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data department")
			return
		}
		departments = append(departments, d)
	}
	writeJSON(w, http.StatusOK, departments)
}

type createDepartmentRequest struct {
	BranchID *string `json:"branch_id"`
	Code     string  `json:"code"`
	Name     string  `json:"name"`
}

func (h *Handler) createDepartment(w http.ResponseWriter, r *http.Request) {
	companyID := r.PathValue("id")
	var req createDepartmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Code = strings.TrimSpace(strings.ToUpper(req.Code))
	req.Name = strings.TrimSpace(req.Name)
	if req.Code == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "Code dan nama wajib diisi")
		return
	}

	var d model.Department
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO departments (company_id, branch_id, code, name) VALUES ($1, $2, $3, $4)
		 RETURNING id, company_id, branch_id, code, name, status, created_at, updated_at`,
		companyID, req.BranchID, req.Code, req.Name,
	).Scan(&d.ID, &d.CompanyID, &d.BranchID, &d.Code, &d.Name, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Code department sudah dipakai di company ini")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat department")
		return
	}

	h.events.Publish("company.department.created", newAuditEvent("company.department.created", &companyID, "create", "department", d.ID, d))
	writeJSON(w, http.StatusCreated, d)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
