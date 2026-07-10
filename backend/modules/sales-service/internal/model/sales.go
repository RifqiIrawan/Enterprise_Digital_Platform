package model

import "time"

type Customer struct {
	ID           string    `json:"id" db:"id"`
	CompanyID    string    `json:"company_id" db:"company_id"`
	BranchID     *string   `json:"branch_id" db:"branch_id"`
	CustomerCode string    `json:"customer_code" db:"customer_code"`
	Name         string    `json:"name" db:"name"`
	Email        string    `json:"email" db:"email"`
	Phone        string    `json:"phone" db:"phone"`
	Address      string    `json:"address" db:"address"`
	TaxID        string    `json:"tax_id" db:"tax_id"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type Quotation struct {
	ID              string     `json:"id" db:"id"`
	CompanyID       string     `json:"company_id" db:"company_id"`
	BranchID        *string    `json:"branch_id" db:"branch_id"`
	QuotationNumber string     `json:"quotation_number" db:"quotation_number"`
	CustomerID      string     `json:"customer_id" db:"customer_id"`
	QuotationDate   time.Time  `json:"quotation_date" db:"quotation_date"`
	ValidUntil      *time.Time `json:"valid_until" db:"valid_until"`
	Status          string     `json:"status" db:"status"`
	SubtotalAmount  float64    `json:"subtotal_amount" db:"subtotal_amount"`
	TaxAmount       float64    `json:"tax_amount" db:"tax_amount"`
	TotalAmount     float64    `json:"total_amount" db:"total_amount"`
	Notes           string     `json:"notes" db:"notes"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type QuotationLine struct {
	ID          string  `json:"id" db:"id"`
	QuotationID string  `json:"quotation_id" db:"quotation_id"`
	LineNumber  int16   `json:"line_number" db:"line_number"`
	ProductName string  `json:"product_name" db:"product_name"`
	Description string  `json:"description" db:"description"`
	Quantity    float64 `json:"quantity" db:"quantity"`
	UnitPrice   float64 `json:"unit_price" db:"unit_price"`
	Amount      float64 `json:"amount" db:"amount"`
}

type SalesOrder struct {
	ID             string    `json:"id" db:"id"`
	CompanyID      string    `json:"company_id" db:"company_id"`
	BranchID       *string   `json:"branch_id" db:"branch_id"`
	SONumber       string    `json:"so_number" db:"so_number"`
	CustomerID     string    `json:"customer_id" db:"customer_id"`
	QuotationID    *string   `json:"quotation_id" db:"quotation_id"`
	OrderDate      time.Time `json:"order_date" db:"order_date"`
	Status         string    `json:"status" db:"status"`
	SubtotalAmount float64   `json:"subtotal_amount" db:"subtotal_amount"`
	TaxAmount      float64   `json:"tax_amount" db:"tax_amount"`
	TotalAmount    float64   `json:"total_amount" db:"total_amount"`
	InvoiceID      *string   `json:"invoice_id" db:"invoice_id"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type SalesOrderLine struct {
	ID           string  `json:"id" db:"id"`
	SalesOrderID string  `json:"sales_order_id" db:"sales_order_id"`
	LineNumber   int16   `json:"line_number" db:"line_number"`
	ProductName  string  `json:"product_name" db:"product_name"`
	Description  string  `json:"description" db:"description"`
	Quantity     float64 `json:"quantity" db:"quantity"`
	UnitPrice    float64 `json:"unit_price" db:"unit_price"`
	Amount       float64 `json:"amount" db:"amount"`
}
