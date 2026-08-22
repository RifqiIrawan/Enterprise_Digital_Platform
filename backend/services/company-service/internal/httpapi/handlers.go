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
	mux.HandleFunc("PUT /companies/{id}/branches/{branchId}", h.updateBranch)
	mux.HandleFunc("DELETE /companies/{id}/branches/{branchId}", h.deleteBranch)

	mux.HandleFunc("GET /companies/{id}/departments", h.listDepartments)
	mux.HandleFunc("POST /companies/{id}/departments", h.createDepartment)
	mux.HandleFunc("PUT /companies/{id}/departments/{departmentId}", h.updateDepartment)
	mux.HandleFunc("DELETE /companies/{id}/departments/{departmentId}", h.deleteDepartment)
}

// Catatan yang berlaku untuk KEDUA endpoint DELETE di bawah: branch dan
// department dirujuk lewat kolom `branch_id`/`department_id` di database
// service LAIN (finance, hr, sales, dst) yang tidak punya foreign key ke sini
// -- tiap service punya database sendiri. Jadi guard di sini hanya bisa
// menahan referensi yang ada di database company-service saja; menghapus
// branch yang sudah dipakai transaksi di modul lain tetap akan meninggalkan
// `branch_id` yatim di sana. Karena itu UI menawarkan "Nonaktifkan"
// (status = inactive) sebagai jalur normal dan menempatkan hapus sebagai
// tindakan terpisah yang eksplisit.

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

	h.events.Publish("company.company.created", newAuditEvent(r, "company.company.created", &c.ID, "create", "company", c.ID, c))
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

	h.events.Publish("company.company.updated", newAuditEvent(r, "company.company.updated", &c.ID, "update", "company", c.ID, c))
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

	h.events.Publish("company.branch.created", newAuditEvent(r, "company.branch.created", &companyID, "create", "branch", b.ID, b))
	writeJSON(w, http.StatusCreated, b)
}

type updateBranchRequest struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Status  string `json:"status"`
}

func (h *Handler) updateBranch(w http.ResponseWriter, r *http.Request) {
	companyID := r.PathValue("id")
	branchID := r.PathValue("branchId")
	var req updateBranchRequest
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
	if req.Status != "active" && req.Status != "inactive" {
		writeError(w, http.StatusBadRequest, "Status harus active atau inactive")
		return
	}

	// company_id ikut jadi predikat (bukan cuma id) supaya branch milik company
	// lain tidak bisa diubah lewat URL company yang salah -- jalur yang sama
	// dipakai semua handler bertingkat di bawah ini.
	var b model.Branch
	err := h.pool.QueryRow(r.Context(),
		`UPDATE branches SET name = $1, address = $2, status = $3, updated_at = now()
		 WHERE id = $4 AND company_id = $5
		 RETURNING id, company_id, code, name, address, status, created_at, updated_at`,
		req.Name, req.Address, req.Status, branchID, companyID,
	).Scan(&b.ID, &b.CompanyID, &b.Code, &b.Name, &b.Address, &b.Status, &b.CreatedAt, &b.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Branch tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui branch")
		return
	}

	h.events.Publish("company.branch.updated", newAuditEvent(r, "company.branch.updated", &companyID, "update", "branch", b.ID, b))
	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) deleteBranch(w http.ResponseWriter, r *http.Request) {
	companyID := r.PathValue("id")
	branchID := r.PathValue("branchId")

	// departments.branch_id dideklarasikan ON DELETE CASCADE di 001_init.sql.
	// Tanpa guard ini, menghapus satu branch akan ikut menghapus seluruh
	// department di bawahnya tanpa peringatan apa pun -- kehilangan data diam-
	// diam yang tidak bisa dibatalkan. Jadi penghapusannya ditolak selama masih
	// ada department yang menempel, dan penggunanya harus memindahkan atau
	// menghapus department itu lebih dulu secara sadar.
	var departmentCount int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT count(*) FROM departments WHERE branch_id = $1`, branchID,
	).Scan(&departmentCount); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memeriksa department pada branch")
		return
	}
	if departmentCount > 0 {
		writeError(w, http.StatusConflict, "Branch masih dipakai oleh department, pindahkan atau hapus department-nya dulu")
		return
	}

	var b model.Branch
	err := h.pool.QueryRow(r.Context(),
		`DELETE FROM branches WHERE id = $1 AND company_id = $2
		 RETURNING id, company_id, code, name, address, status, created_at, updated_at`,
		branchID, companyID,
	).Scan(&b.ID, &b.CompanyID, &b.Code, &b.Name, &b.Address, &b.Status, &b.CreatedAt, &b.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Branch tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghapus branch")
		return
	}

	h.events.Publish("company.branch.deleted", newAuditEvent(r, "company.branch.deleted", &companyID, "delete", "branch", b.ID, b))
	writeJSON(w, http.StatusOK, b)
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
	// Normalisasi + validasi kepemilikan branch, alasannya sama persis dengan
	// updateDepartment (lihat komentar di sana).
	if req.BranchID != nil && strings.TrimSpace(*req.BranchID) == "" {
		req.BranchID = nil
	}
	if req.BranchID != nil {
		var owner string
		err := h.pool.QueryRow(r.Context(), `SELECT company_id FROM branches WHERE id = $1`, *req.BranchID).Scan(&owner)
		if err == pgx.ErrNoRows || (err == nil && owner != companyID) {
			writeError(w, http.StatusBadRequest, "Branch tidak ditemukan di company ini")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memeriksa branch")
			return
		}
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

	h.events.Publish("company.department.created", newAuditEvent(r, "company.department.created", &companyID, "create", "department", d.ID, d))
	writeJSON(w, http.StatusCreated, d)
}

type updateDepartmentRequest struct {
	BranchID *string `json:"branch_id"`
	Name     string  `json:"name"`
	Status   string  `json:"status"`
}

func (h *Handler) updateDepartment(w http.ResponseWriter, r *http.Request) {
	companyID := r.PathValue("id")
	departmentID := r.PathValue("departmentId")
	var req updateDepartmentRequest
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
	if req.Status != "active" && req.Status != "inactive" {
		writeError(w, http.StatusBadRequest, "Status harus active atau inactive")
		return
	}
	// String kosong dari form HTML berarti "company-wide", sama dengan NULL.
	// Tanpa normalisasi ini, "" akan sampai ke Postgres sebagai UUID tidak
	// valid dan gagal sebagai 500, bukan sebagai pilihan yang sah.
	if req.BranchID != nil && strings.TrimSpace(*req.BranchID) == "" {
		req.BranchID = nil
	}

	// Branch tujuan wajib milik company yang sama. FK di skema hanya menjamin
	// branch-nya ADA, bukan bahwa dia milik company ini -- tanpa cek ini sebuah
	// department bisa dipindahkan ke branch milik company lain.
	if req.BranchID != nil {
		var owner string
		err := h.pool.QueryRow(r.Context(), `SELECT company_id FROM branches WHERE id = $1`, *req.BranchID).Scan(&owner)
		if err == pgx.ErrNoRows || (err == nil && owner != companyID) {
			writeError(w, http.StatusBadRequest, "Branch tidak ditemukan di company ini")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memeriksa branch")
			return
		}
	}

	var d model.Department
	err := h.pool.QueryRow(r.Context(),
		`UPDATE departments SET branch_id = $1, name = $2, status = $3, updated_at = now()
		 WHERE id = $4 AND company_id = $5
		 RETURNING id, company_id, branch_id, code, name, status, created_at, updated_at`,
		req.BranchID, req.Name, req.Status, departmentID, companyID,
	).Scan(&d.ID, &d.CompanyID, &d.BranchID, &d.Code, &d.Name, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Department tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui department")
		return
	}

	h.events.Publish("company.department.updated", newAuditEvent(r, "company.department.updated", &companyID, "update", "department", d.ID, d))
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) deleteDepartment(w http.ResponseWriter, r *http.Request) {
	companyID := r.PathValue("id")
	departmentID := r.PathValue("departmentId")

	var d model.Department
	err := h.pool.QueryRow(r.Context(),
		`DELETE FROM departments WHERE id = $1 AND company_id = $2
		 RETURNING id, company_id, branch_id, code, name, status, created_at, updated_at`,
		departmentID, companyID,
	).Scan(&d.ID, &d.CompanyID, &d.BranchID, &d.Code, &d.Name, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Department tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghapus department")
		return
	}

	h.events.Publish("company.department.deleted", newAuditEvent(r, "company.department.deleted", &companyID, "delete", "department", d.ID, d))
	writeJSON(w, http.StatusOK, d)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
