package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/enterprise-digital-platform/ticketing-service/internal/model"
)

const ticketCommentColumns = `id, company_id, branch_id, ticket_id, comment_text, is_internal, author_user_id, created_at`

func scanTicketComment(row pgx.Row, c *model.TicketComment) error {
	return row.Scan(&c.ID, &c.CompanyID, &c.BranchID, &c.TicketID, &c.CommentText, &c.IsInternal, &c.AuthorUserID, &c.CreatedAt)
}

func (h *Handler) listTicketComments(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		writeError(w, http.StatusBadRequest, "company_id wajib diisi")
		return
	}
	query := `SELECT ` + ticketCommentColumns + ` FROM ticket_comments WHERE company_id = $1`
	args := []any{companyID}
	if ticketID := r.URL.Query().Get("ticket_id"); ticketID != "" {
		args = append(args, ticketID)
		query += ` AND ticket_id = $` + strconv.Itoa(len(args))
	}
	if branchID := r.URL.Query().Get("branch_id"); branchID != "" {
		args = append(args, branchID)
		query += ` AND (branch_id = $` + strconv.Itoa(len(args)) + ` OR branch_id IS NULL)`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data comment")
		return
	}
	defer rows.Close()

	comments := []model.TicketComment{}
	for rows.Next() {
		var c model.TicketComment
		if err := scanTicketComment(rows, &c); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data comment")
			return
		}
		comments = append(comments, c)
	}
	writeJSON(w, http.StatusOK, comments)
}

type ticketCommentRequest struct {
	CompanyID   string  `json:"company_id"`
	BranchID    *string `json:"branch_id"`
	TicketID    string  `json:"ticket_id"`
	CommentText string  `json:"comment_text"`
	IsInternal  bool    `json:"is_internal"`
}

func (h *Handler) createTicketComment(w http.ResponseWriter, r *http.Request) {
	var req ticketCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.CommentText = strings.TrimSpace(req.CommentText)
	if req.CompanyID == "" || req.TicketID == "" || req.CommentText == "" {
		writeError(w, http.StatusBadRequest, "company_id, ticket_id, dan comment_text wajib diisi")
		return
	}

	ctx := r.Context()
	var ticket model.Ticket
	err := scanTicket(h.pool.QueryRow(ctx, `SELECT `+ticketColumns+` FROM tickets WHERE id = $1 AND company_id = $2`, req.TicketID, req.CompanyID), &ticket)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "Tiket tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat tiket")
		return
	}

	actor := actorFromHeader(r)
	var c model.TicketComment
	err = scanTicketComment(h.pool.QueryRow(ctx, `
		INSERT INTO ticket_comments (company_id, branch_id, ticket_id, comment_text, is_internal, author_user_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+ticketCommentColumns,
		req.CompanyID, req.BranchID, req.TicketID, req.CommentText, req.IsInternal, actor,
	), &c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat comment")
		return
	}

	h.events.Publish("ticketing.comment.created", newAuditEvent("ticketing.comment.created", actor, &c.CompanyID, "create", "ticket_comment", c.ID, c))
	writeJSON(w, http.StatusCreated, c)
}
