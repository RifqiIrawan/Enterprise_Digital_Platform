package model

import "time"

type Company struct {
	ID        string    `json:"id" db:"id"`
	Code      string    `json:"code" db:"code"`
	Name      string    `json:"name" db:"name"`
	Status    string    `json:"status" db:"status"` // active | inactive
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Branch struct {
	ID        string    `json:"id" db:"id"`
	CompanyID string    `json:"company_id" db:"company_id"`
	Code      string    `json:"code" db:"code"`
	Name      string    `json:"name" db:"name"`
	Address   string    `json:"address" db:"address"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Department struct {
	ID        string    `json:"id" db:"id"`
	CompanyID string    `json:"company_id" db:"company_id"`
	BranchID  *string   `json:"branch_id" db:"branch_id"` // nil = department company-wide
	Code      string    `json:"code" db:"code"`
	Name      string    `json:"name" db:"name"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
