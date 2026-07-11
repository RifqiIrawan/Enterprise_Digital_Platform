package model

import "time"

type QualityStandard struct {
	ID           string    `json:"id" db:"id"`
	CompanyID    string    `json:"company_id" db:"company_id"`
	BranchID     *string   `json:"branch_id" db:"branch_id"`
	StandardCode string    `json:"standard_code" db:"standard_code"`
	Name         string    `json:"name" db:"name"`
	ProductID    string    `json:"product_id" db:"product_id"`
	Criteria     string    `json:"criteria" db:"criteria"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type QualityInspection struct {
	ID                string    `json:"id" db:"id"`
	CompanyID         string    `json:"company_id" db:"company_id"`
	BranchID          *string   `json:"branch_id" db:"branch_id"`
	InspectionNumber  string    `json:"inspection_number" db:"inspection_number"`
	StandardID        string    `json:"standard_id" db:"standard_id"`
	ProductID         string    `json:"product_id" db:"product_id"`
	ReferenceType     string    `json:"reference_type" db:"reference_type"`
	ReferenceID       *string   `json:"reference_id" db:"reference_id"`
	ReferenceNumber   *string   `json:"reference_number" db:"reference_number"`
	InspectedQuantity float64   `json:"inspected_quantity" db:"inspected_quantity"`
	PassedQuantity    float64   `json:"passed_quantity" db:"passed_quantity"`
	FailedQuantity    float64   `json:"failed_quantity" db:"failed_quantity"`
	Result            string    `json:"result" db:"result"`
	InspectionDate    time.Time `json:"inspection_date" db:"inspection_date"`
	Notes             string    `json:"notes" db:"notes"`
	InspectedBy       *string   `json:"inspected_by" db:"inspected_by"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}
