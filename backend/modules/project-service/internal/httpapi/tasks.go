package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/project-service/internal/model"
)

const taskColumns = `id, company_id, branch_id, project_id, task_number, title, description, assignee_employee_id, assignee_name, status, priority, due_date, estimated_hours, completed_at, created_by_user_id, created_at, updated_at`

func scanTask(row pgx.Row, t *model.Task) error {
	return row.Scan(&t.ID, &t.CompanyID, &t.BranchID, &t.ProjectID, &t.TaskNumber, &t.Title, &t.Description, &t.AssigneeEmployeeID, &t.AssigneeName, &t.Status, &t.Priority, &t.DueDate, &t.EstimatedHours, &t.CompletedAt, &t.CreatedByUserID, &t.CreatedAt, &t.UpdatedAt)
}

var validTaskStatuses = []string{"TODO", "IN_PROGRESS", "DONE", "CANCELLED"}
var validPriorities = []string{"LOW", "MEDIUM", "HIGH"}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + taskColumns + ` FROM tasks WHERE company_id = $1`
	args := []any{companyID}
	if projectID := r.URL.Query().Get("project_id"); projectID != "" {
		args = append(args, projectID)
		query += ` AND project_id = $` + strconv.Itoa(len(args))
	}
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
		writeError(w, http.StatusInternalServerError, "Gagal memuat data tugas")
		return
	}
	defer rows.Close()

	tasks := []model.Task{}
	for rows.Next() {
		var t model.Task
		if err := scanTask(rows, &t); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data tugas")
			return
		}
		tasks = append(tasks, t)
	}
	writeJSON(w, http.StatusOK, tasks)
}

type taskRequest struct {
	CompanyID          string  `json:"company_id"`
	BranchID           *string `json:"branch_id"`
	ProjectID          string  `json:"project_id"`
	Title              string  `json:"title"`
	Description        string  `json:"description"`
	AssigneeEmployeeID string  `json:"assignee_employee_id"`
	Status             string  `json:"status"`
	Priority           string  `json:"priority"`
	DueDate            string  `json:"due_date"`
	EstimatedHours     float64 `json:"estimated_hours"`
}

// assertProjectOpen memastikan proyeknya ada, milik company yang sama, dan
// belum ditutup. Tugas baru pada proyek COMPLETED/CANCELLED tidak masuk akal:
// COMPLETED sudah lolos guard "tidak ada tugas terbuka", jadi menambah tugas
// setelahnya diam-diam membatalkan arti guard itu.
func assertProjectOpen(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, companyID, projectID string) (int, string) {
	var status string
	err := q.QueryRow(ctx, `SELECT status FROM projects WHERE id = $1 AND company_id = $2`, projectID, companyID).Scan(&status)
	if err == pgx.ErrNoRows {
		return http.StatusBadRequest, "Proyek tidak ditemukan di company ini"
	} else if err != nil {
		return http.StatusInternalServerError, "Gagal memuat proyek"
	}
	if status == "COMPLETED" || status == "CANCELLED" {
		return http.StatusConflict, "Proyek sudah " + status + ", tidak menerima perubahan baru"
	}
	return 0, ""
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	var req taskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.CompanyID == "" || req.ProjectID == "" || req.Title == "" {
		writeError(w, http.StatusBadRequest, "company_id, project_id, dan title wajib diisi")
		return
	}
	if req.Priority == "" {
		req.Priority = "MEDIUM"
	}
	if !contains(validPriorities, req.Priority) {
		writeError(w, http.StatusBadRequest, "priority harus salah satu dari LOW, MEDIUM, HIGH")
		return
	}
	if req.EstimatedHours < 0 {
		writeError(w, http.StatusBadRequest, "estimated_hours tidak boleh negatif")
		return
	}
	dueDate, err := parseOptionalDate(req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_date harus format YYYY-MM-DD")
		return
	}

	ctx := r.Context()
	if status, msg := assertProjectOpen(ctx, h.pool, req.CompanyID, req.ProjectID); status != 0 {
		writeError(w, status, msg)
		return
	}

	var assigneeID, assigneeName *string
	if req.AssigneeEmployeeID != "" {
		name, status, msg := h.resolveEmployee(req.CompanyID, req.AssigneeEmployeeID)
		if status != 0 {
			writeError(w, status, msg)
			return
		}
		assigneeID, assigneeName = &req.AssigneeEmployeeID, &name
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	taskNumber, err := nextSequence(ctx, tx, req.CompanyID, "tasks", "task_number", "TSK", time.Now().Format("2006-01"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor tugas")
		return
	}

	var t model.Task
	err = scanTask(tx.QueryRow(ctx, `
		INSERT INTO tasks (company_id, branch_id, project_id, task_number, title, description, assignee_employee_id, assignee_name, priority, due_date, estimated_hours, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+taskColumns,
		req.CompanyID, req.BranchID, req.ProjectID, taskNumber, req.Title, req.Description, assigneeID, assigneeName, req.Priority, dueDate, req.EstimatedHours, actorFromHeader(r),
	), &t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat tugas")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan tugas")
		return
	}

	h.events.Publish("project.task.created", newAuditEvent("project.task.created", actorFromHeader(r), &t.CompanyID, "create", "task", t.ID, t))
	writeJSON(w, http.StatusCreated, t)
}

// updateTask SENGAJA menerima status lewat PUT biasa (pola tickets di
// ticketing-service), berbeda dari proyek yang pakai endpoint transisi: tugas
// berpindah status berkali-kali sehari dan tidak punya efek samping lintas
// entitas. Satu-satunya otomatisasi: completed_at diisi saat status jadi DONE
// dan dikosongkan lagi kalau tugasnya dibuka kembali -- kalau tidak, tugas
// yang di-reopen akan tetap membawa tanggal selesai yang sudah tidak benar.
func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req taskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.CompanyID == "" || req.Title == "" {
		writeError(w, http.StatusBadRequest, "company_id dan title wajib diisi")
		return
	}
	if req.Status != "" && !contains(validTaskStatuses, req.Status) {
		writeError(w, http.StatusBadRequest, "status harus salah satu dari TODO, IN_PROGRESS, DONE, CANCELLED")
		return
	}
	if req.Priority != "" && !contains(validPriorities, req.Priority) {
		writeError(w, http.StatusBadRequest, "priority harus salah satu dari LOW, MEDIUM, HIGH")
		return
	}
	if req.EstimatedHours < 0 {
		writeError(w, http.StatusBadRequest, "estimated_hours tidak boleh negatif")
		return
	}
	dueDate, err := parseOptionalDate(req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_date harus format YYYY-MM-DD")
		return
	}

	ctx := r.Context()
	var before model.Task
	err = scanTask(h.pool.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = $1 AND company_id = $2`, id, req.CompanyID), &before)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Tugas tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat tugas")
		return
	}
	if status, msg := assertProjectOpen(ctx, h.pool, req.CompanyID, before.ProjectID); status != 0 {
		writeError(w, status, msg)
		return
	}

	status := before.Status
	if req.Status != "" {
		status = req.Status
	}
	priority := before.Priority
	if req.Priority != "" {
		priority = req.Priority
	}

	// Penugasan boleh dilepas dengan mengirim assignee_employee_id kosong.
	var assigneeID, assigneeName *string
	if req.AssigneeEmployeeID != "" {
		name, code, msg := h.resolveEmployee(req.CompanyID, req.AssigneeEmployeeID)
		if code != 0 {
			writeError(w, code, msg)
			return
		}
		assigneeID, assigneeName = &req.AssigneeEmployeeID, &name
	}

	// $5 (status) dipakai DUA kali: sekali sebagai nilai kolom, sekali di dalam
	// CASE. Tanpa cast eksplisit, Postgres menyimpulkan tipe yang berbeda untuk
	// parameter yang sama di kedua tempat (varchar dari `status = $5`, text dari
	// perbandingan literal) dan menolak query-nya dengan SQLSTATE 42P08
	// "inconsistent types deduced for parameter" -- ditemukan lewat test, bukan
	// diantisipasi. `$5::varchar` di kedua tempat menyamakannya.
	var t model.Task
	err = scanTask(h.pool.QueryRow(ctx, `
		UPDATE tasks SET title = $1, description = $2, assignee_employee_id = $3, assignee_name = $4, status = $5::varchar, priority = $6, due_date = $7, estimated_hours = $8,
			completed_at = CASE WHEN $5::varchar = 'DONE' THEN COALESCE(completed_at, now()) ELSE NULL END,
			updated_at = now()
		WHERE id = $9 AND company_id = $10
		RETURNING `+taskColumns,
		req.Title, req.Description, assigneeID, assigneeName, status, priority, dueDate, req.EstimatedHours, id, req.CompanyID,
	), &t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui tugas")
		return
	}

	h.events.Publish("project.task.updated", newAuditEvent("project.task.updated", actorFromHeader(r), &t.CompanyID, "update", "task", t.ID, t))
	writeJSON(w, http.StatusOK, t)
}
