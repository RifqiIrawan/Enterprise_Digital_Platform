package model

import "time"

type TicketCategory struct {
	ID           string    `json:"id" db:"id"`
	CompanyID    string    `json:"company_id" db:"company_id"`
	BranchID     *string   `json:"branch_id" db:"branch_id"`
	CategoryCode string    `json:"category_code" db:"category_code"`
	Name         string    `json:"name" db:"name"`
	Description  string    `json:"description" db:"description"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type Ticket struct {
	ID             string     `json:"id" db:"id"`
	CompanyID      string     `json:"company_id" db:"company_id"`
	BranchID       *string    `json:"branch_id" db:"branch_id"`
	TicketNumber   string     `json:"ticket_number" db:"ticket_number"`
	CategoryID     string     `json:"category_id" db:"category_id"`
	Subject        string     `json:"subject" db:"subject"`
	Description    string     `json:"description" db:"description"`
	Priority       string     `json:"priority" db:"priority"`
	Status         string     `json:"status" db:"status"`
	RequesterName  string     `json:"requester_name" db:"requester_name"`
	RequesterEmail string     `json:"requester_email" db:"requester_email"`
	AssignedTo     *string    `json:"assigned_to" db:"assigned_to"`
	ResolvedAt     *time.Time `json:"resolved_at" db:"resolved_at"`
	ClosedAt       *time.Time `json:"closed_at" db:"closed_at"`
	Notes          string     `json:"notes" db:"notes"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type TicketComment struct {
	ID           string    `json:"id" db:"id"`
	CompanyID    string    `json:"company_id" db:"company_id"`
	BranchID     *string   `json:"branch_id" db:"branch_id"`
	TicketID     string    `json:"ticket_id" db:"ticket_id"`
	CommentText  string    `json:"comment_text" db:"comment_text"`
	IsInternal   bool      `json:"is_internal" db:"is_internal"`
	AuthorUserID *string   `json:"author_user_id" db:"author_user_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}
