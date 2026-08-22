package model

import "time"

type Lead struct {
	ID                     string     `json:"id" db:"id"`
	CompanyID              string     `json:"company_id" db:"company_id"`
	BranchID               *string    `json:"branch_id" db:"branch_id"`
	LeadNumber             string     `json:"lead_number" db:"lead_number"`
	FirstName              string     `json:"first_name" db:"first_name"`
	LastName               string     `json:"last_name" db:"last_name"`
	CompanyName            string     `json:"company_name" db:"company_name"`
	Email                  string     `json:"email" db:"email"`
	Phone                  string     `json:"phone" db:"phone"`
	Source                 string     `json:"source" db:"source"`
	Status                 string     `json:"status" db:"status"`
	OwnerUserID            *string    `json:"owner_user_id" db:"owner_user_id"`
	Notes                  string     `json:"notes" db:"notes"`
	ConvertedAccountID     *string    `json:"converted_account_id" db:"converted_account_id"`
	ConvertedContactID     *string    `json:"converted_contact_id" db:"converted_contact_id"`
	ConvertedOpportunityID *string    `json:"converted_opportunity_id" db:"converted_opportunity_id"`
	ConvertedAt            *time.Time `json:"converted_at" db:"converted_at"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at" db:"updated_at"`
}

type Account struct {
	ID          string    `json:"id" db:"id"`
	CompanyID   string    `json:"company_id" db:"company_id"`
	BranchID    *string   `json:"branch_id" db:"branch_id"`
	AccountCode string    `json:"account_code" db:"account_code"`
	Name        string    `json:"name" db:"name"`
	Industry    string    `json:"industry" db:"industry"`
	Website     string    `json:"website" db:"website"`
	Phone       string    `json:"phone" db:"phone"`
	Email       string    `json:"email" db:"email"`
	Address     string    `json:"address" db:"address"`
	AccountType string    `json:"account_type" db:"account_type"`
	Status      string    `json:"status" db:"status"` // ACTIVE | INACTIVE
	OwnerUserID *string   `json:"owner_user_id" db:"owner_user_id"`
	Notes       string    `json:"notes" db:"notes"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Contact struct {
	ID        string    `json:"id" db:"id"`
	CompanyID string    `json:"company_id" db:"company_id"`
	BranchID  *string   `json:"branch_id" db:"branch_id"`
	AccountID *string   `json:"account_id" db:"account_id"`
	FirstName string    `json:"first_name" db:"first_name"`
	LastName  string    `json:"last_name" db:"last_name"`
	JobTitle  string    `json:"job_title" db:"job_title"`
	Email     string    `json:"email" db:"email"`
	Phone     string    `json:"phone" db:"phone"`
	IsPrimary bool      `json:"is_primary" db:"is_primary"`
	Status    string    `json:"status" db:"status"` // ACTIVE | INACTIVE
	Notes     string    `json:"notes" db:"notes"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Opportunity struct {
	ID                string     `json:"id" db:"id"`
	CompanyID         string     `json:"company_id" db:"company_id"`
	BranchID          *string    `json:"branch_id" db:"branch_id"`
	OpportunityNumber string     `json:"opportunity_number" db:"opportunity_number"`
	AccountID         string     `json:"account_id" db:"account_id"`
	ContactID         *string    `json:"contact_id" db:"contact_id"`
	Name              string     `json:"name" db:"name"`
	Stage             string     `json:"stage" db:"stage"`
	Amount            float64    `json:"amount" db:"amount"`
	Probability       int16      `json:"probability" db:"probability"`
	ExpectedCloseDate *time.Time `json:"expected_close_date" db:"expected_close_date"`
	OwnerUserID       *string    `json:"owner_user_id" db:"owner_user_id"`
	LostReason        *string    `json:"lost_reason" db:"lost_reason"`
	Notes             string     `json:"notes" db:"notes"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

type Activity struct {
	ID            string     `json:"id" db:"id"`
	CompanyID     string     `json:"company_id" db:"company_id"`
	BranchID      *string    `json:"branch_id" db:"branch_id"`
	ReferenceType string     `json:"reference_type" db:"reference_type"`
	ReferenceID   string     `json:"reference_id" db:"reference_id"`
	ActivityType  string     `json:"activity_type" db:"activity_type"`
	Subject       string     `json:"subject" db:"subject"`
	Description   string     `json:"description" db:"description"`
	ActivityDate  time.Time  `json:"activity_date" db:"activity_date"`
	DueDate       *time.Time `json:"due_date" db:"due_date"`
	IsDone        bool       `json:"is_done" db:"is_done"`
	OwnerUserID   *string    `json:"owner_user_id" db:"owner_user_id"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}
