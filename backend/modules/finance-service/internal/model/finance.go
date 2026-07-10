package model

import "time"

type Account struct {
	ID          string    `json:"id" db:"id"`
	CompanyID   string    `json:"company_id" db:"company_id"`
	AccountCode string    `json:"account_code" db:"account_code"`
	AccountName string    `json:"account_name" db:"account_name"`
	AccountType string    `json:"account_type" db:"account_type"` // ASSET | LIABILITY | EQUITY | REVENUE | EXPENSE
	ParentID    *string   `json:"parent_id" db:"parent_id"`
	IsPosting   bool      `json:"is_posting" db:"is_posting"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type JournalEntry struct {
	ID            string     `json:"id" db:"id"`
	CompanyID     string     `json:"company_id" db:"company_id"`
	BranchID      *string    `json:"branch_id" db:"branch_id"`
	EntryNumber   string     `json:"entry_number" db:"entry_number"`
	EntryDate     time.Time  `json:"entry_date" db:"entry_date"`
	Period        string     `json:"period" db:"period"`
	Description   string     `json:"description" db:"description"`
	ReferenceType string     `json:"reference_type" db:"reference_type"`
	ReferenceID   *string    `json:"reference_id" db:"reference_id"`
	Status        string     `json:"status" db:"status"` // DRAFT | POSTED | REVERSED
	TotalDebit    float64    `json:"total_debit" db:"total_debit"`
	TotalCredit   float64    `json:"total_credit" db:"total_credit"`
	PostedBy      *string    `json:"posted_by" db:"posted_by"`
	PostedAt      *time.Time `json:"posted_at" db:"posted_at"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
}

type JournalLine struct {
	ID           string  `json:"id" db:"id"`
	JournalID    string  `json:"journal_id" db:"journal_id"`
	LineNumber   int16   `json:"line_number" db:"line_number"`
	AccountID    string  `json:"account_id" db:"account_id"`
	DebitAmount  float64 `json:"debit_amount" db:"debit_amount"`
	CreditAmount float64 `json:"credit_amount" db:"credit_amount"`
	Description  string  `json:"description" db:"description"`
}

type Invoice struct {
	ID               string     `json:"id" db:"id"`
	CompanyID        string     `json:"company_id" db:"company_id"`
	BranchID         *string    `json:"branch_id" db:"branch_id"`
	InvoiceType      string     `json:"invoice_type" db:"invoice_type"` // AR | AP
	InvoiceNumber    string     `json:"invoice_number" db:"invoice_number"`
	PartnerName      string     `json:"partner_name" db:"partner_name"`
	InvoiceDate      time.Time  `json:"invoice_date" db:"invoice_date"`
	DueDate          *time.Time `json:"due_date" db:"due_date"`
	ControlAccountID string     `json:"control_account_id" db:"control_account_id"`
	TaxAccountID     *string    `json:"tax_account_id" db:"tax_account_id"`
	SubtotalAmount   float64    `json:"subtotal_amount" db:"subtotal_amount"`
	TaxAmount        float64    `json:"tax_amount" db:"tax_amount"`
	TotalAmount      float64    `json:"total_amount" db:"total_amount"`
	PaidAmount       float64    `json:"paid_amount" db:"paid_amount"`
	Status           string     `json:"status" db:"status"`
	JournalID        *string    `json:"journal_id" db:"journal_id"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

type InvoiceLine struct {
	ID          string  `json:"id" db:"id"`
	InvoiceID   string  `json:"invoice_id" db:"invoice_id"`
	LineNumber  int16   `json:"line_number" db:"line_number"`
	AccountID   string  `json:"account_id" db:"account_id"`
	Description string  `json:"description" db:"description"`
	Quantity    float64 `json:"quantity" db:"quantity"`
	UnitPrice   float64 `json:"unit_price" db:"unit_price"`
	Amount      float64 `json:"amount" db:"amount"`
}
