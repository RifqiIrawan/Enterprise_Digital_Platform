package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/ticketing-service/internal/model"
)

const ticketColumns = `id, company_id, branch_id, ticket_number, category_id, subject, description, priority, status, requester_name, requester_email, assigned_to, resolved_at, closed_at, notes, created_at, updated_at`

var validTicketPriorities = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "URGENT": true}
var openTicketStatuses = map[string]bool{"OPEN": true, "IN_PROGRESS": true, "RESOLVED": true}

func scanTicket(row pgx.Row, t *model.Ticket) error {
	return row.Scan(&t.ID, &t.CompanyID, &t.BranchID, &t.TicketNumber, &t.CategoryID, &t.Subject, &t.Description, &t.Priority, &t.Status, &t.RequesterName, &t.RequesterEmail, &t.AssignedTo, &t.ResolvedAt, &t.ClosedAt, &t.Notes, &t.CreatedAt, &t.UpdatedAt)
}

func (h *Handler) listTickets(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + ticketColumns + ` FROM tickets WHERE company_id = $1`
	args := []any{companyID}
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += ` AND status = $` + strconv.Itoa(len(args))
	}
	if categoryID := r.URL.Query().Get("category_id"); categoryID != "" {
		args = append(args, categoryID)
		query += ` AND category_id = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data tiket")
		return
	}
	defer rows.Close()

	tickets := []model.Ticket{}
	for rows.Next() {
		var t model.Ticket
		if err := scanTicket(rows, &t); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data tiket")
			return
		}
		tickets = append(tickets, t)
	}
	writeJSON(w, http.StatusOK, tickets)
}

type ticketRequest struct {
	CompanyID      string  `json:"company_id"`
	BranchID       *string `json:"branch_id"`
	CategoryID     string  `json:"category_id"`
	Subject        string  `json:"subject"`
	Description    string  `json:"description"`
	Priority       string  `json:"priority"`
	RequesterName  string  `json:"requester_name"`
	RequesterEmail string  `json:"requester_email"`
	AssignedTo     *string `json:"assigned_to"`
	Notes          string  `json:"notes"`
}

func (h *Handler) createTicket(w http.ResponseWriter, r *http.Request) {
	var req ticketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.RequesterName = strings.TrimSpace(req.RequesterName)
	if req.CompanyID == "" || req.CategoryID == "" || req.Subject == "" || req.RequesterName == "" {
		writeError(w, http.StatusBadRequest, "company_id, category_id, subject, dan requester_name wajib diisi")
		return
	}
	if req.Priority == "" {
		req.Priority = "MEDIUM"
	}
	if !validTicketPriorities[req.Priority] {
		writeError(w, http.StatusBadRequest, "priority harus LOW, MEDIUM, HIGH, atau URGENT")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	var category model.TicketCategory
	err = scanTicketCategory(tx.QueryRow(ctx, `SELECT `+ticketCategoryColumns+` FROM ticket_categories WHERE id = $1 AND company_id = $2`, req.CategoryID, req.CompanyID), &category)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Category tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat category")
		return
	}

	period := time.Now().Format("2006-01")
	ticketNumber, err := nextSequence(ctx, tx, req.CompanyID, "tickets", "ticket_number", "TICKET", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat nomor tiket")
		return
	}

	var t model.Ticket
	err = scanTicket(tx.QueryRow(ctx, `
		INSERT INTO tickets (company_id, branch_id, ticket_number, category_id, subject, description, priority, requester_name, requester_email, assigned_to, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+ticketColumns,
		req.CompanyID, req.BranchID, ticketNumber, req.CategoryID, req.Subject, req.Description, req.Priority, req.RequesterName, req.RequesterEmail, req.AssignedTo, req.Notes,
	), &t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat tiket")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan tiket")
		return
	}

	h.events.Publish("ticketing.ticket.created", newAuditEvent("ticketing.ticket.created", actorFromHeader(r), &t.CompanyID, "create", "ticket", t.ID, t))
	writeJSON(w, http.StatusCreated, t)
}

type updateTicketRequest struct {
	CategoryID     string  `json:"category_id"`
	Subject        string  `json:"subject"`
	Description    string  `json:"description"`
	Priority       string  `json:"priority"`
	Status         string  `json:"status"`
	RequesterName  string  `json:"requester_name"`
	RequesterEmail string  `json:"requester_email"`
	AssignedTo     *string `json:"assigned_to"`
	Notes          string  `json:"notes"`
}

// updateTicket mengizinkan edit field + pindah bebas antar 3 status terbuka
// (OPEN/IN_PROGRESS/RESOLVED) -- workflow support memang wajar maju-mundur
// (mis. RESOLVED balik ke IN_PROGRESS kalau customer reply lagi), berbeda
// dari transisi satu-arah dokumen lain di platform ini. CLOSED HANYA lewat
// /close (terminal, lihat di bawah) -- PUT terhadap tiket yang sudah CLOSED
// ditolak 409, pola sama dengan updateOpportunity di crm-service.
func (h *Handler) updateTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Subject == "" {
		writeError(w, http.StatusBadRequest, "subject wajib diisi")
		return
	}
	if !validTicketPriorities[req.Priority] {
		writeError(w, http.StatusBadRequest, "priority harus LOW, MEDIUM, HIGH, atau URGENT")
		return
	}
	if !openTicketStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "status harus OPEN, IN_PROGRESS, atau RESOLVED (pakai /close untuk status final)")
		return
	}

	ctx := r.Context()
	var before model.Ticket
	err := scanTicket(h.pool.QueryRow(ctx, `SELECT `+ticketColumns+` FROM tickets WHERE id = $1`, id), &before)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Tiket tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat tiket")
		return
	}
	if before.Status == "CLOSED" {
		writeError(w, http.StatusConflict, "Tiket yang sudah CLOSED tidak bisa diedit lagi (pakai /reopen)")
		return
	}
	if req.CategoryID != "" && req.CategoryID != before.CategoryID {
		var category model.TicketCategory
		err = scanTicketCategory(h.pool.QueryRow(ctx, `SELECT `+ticketCategoryColumns+` FROM ticket_categories WHERE id = $1 AND company_id = $2`, req.CategoryID, before.CompanyID), &category)
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Category tidak ditemukan")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memuat category")
			return
		}
	} else {
		req.CategoryID = before.CategoryID
	}

	resolvedAt := before.ResolvedAt
	if req.Status == "RESOLVED" && before.Status != "RESOLVED" {
		now := time.Now()
		resolvedAt = &now
	} else if req.Status != "RESOLVED" {
		resolvedAt = nil
	}

	var t model.Ticket
	err = scanTicket(h.pool.QueryRow(ctx, `
		UPDATE tickets SET category_id = $1, subject = $2, description = $3, priority = $4, status = $5, requester_name = $6, requester_email = $7, assigned_to = $8, notes = $9, resolved_at = $10, updated_at = now()
		WHERE id = $11
		RETURNING `+ticketColumns,
		req.CategoryID, req.Subject, req.Description, req.Priority, req.Status, req.RequesterName, req.RequesterEmail, req.AssignedTo, req.Notes, resolvedAt, id,
	), &t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui tiket")
		return
	}

	h.events.Publish("ticketing.ticket.updated", newAuditEvent("ticketing.ticket.updated", actorFromHeader(r), &t.CompanyID, "update", "ticket", t.ID, t))
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) closeTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var t model.Ticket
	err := scanTicket(h.pool.QueryRow(r.Context(), `
		UPDATE tickets SET status = 'CLOSED', closed_at = now(), updated_at = now()
		WHERE id = $1 AND status != 'CLOSED'
		RETURNING `+ticketColumns, id), &t)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Tiket tidak ditemukan atau sudah CLOSED")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menutup tiket")
		return
	}

	h.events.Publish("ticketing.ticket.closed", newAuditEvent("ticketing.ticket.closed", actorFromHeader(r), &t.CompanyID, "update", "ticket", t.ID, t))
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) reopenTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var t model.Ticket
	err := scanTicket(h.pool.QueryRow(r.Context(), `
		UPDATE tickets SET status = 'OPEN', closed_at = NULL, resolved_at = NULL, updated_at = now()
		WHERE id = $1 AND status = 'CLOSED'
		RETURNING `+ticketColumns, id), &t)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusConflict, "Tiket tidak ditemukan atau belum CLOSED")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuka kembali tiket")
		return
	}

	h.events.Publish("ticketing.ticket.reopened", newAuditEvent("ticketing.ticket.reopened", actorFromHeader(r), &t.CompanyID, "update", "ticket", t.ID, t))
	writeJSON(w, http.StatusOK, t)
}
