package model

import "time"

type Supplier struct {
	ID           string    `json:"id" db:"id"`
	CompanyID    string    `json:"company_id" db:"company_id"`
	BranchID     *string   `json:"branch_id" db:"branch_id"`
	SupplierCode string    `json:"supplier_code" db:"supplier_code"`
	Name         string    `json:"name" db:"name"`
	Email        string    `json:"email" db:"email"`
	Phone        string    `json:"phone" db:"phone"`
	Address      string    `json:"address" db:"address"`
	TaxID        string    `json:"tax_id" db:"tax_id"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type Requisition struct {
	ID             string    `json:"id" db:"id"`
	CompanyID      string    `json:"company_id" db:"company_id"`
	BranchID       *string   `json:"branch_id" db:"branch_id"`
	PRNumber       string    `json:"pr_number" db:"pr_number"`
	RequestedBy    string    `json:"requested_by" db:"requested_by"`
	PRDate         time.Time `json:"pr_date" db:"pr_date"`
	Status         string    `json:"status" db:"status"`
	SubtotalAmount float64   `json:"subtotal_amount" db:"subtotal_amount"`
	Notes          string    `json:"notes" db:"notes"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type RequisitionLine struct {
	ID             string  `json:"id" db:"id"`
	RequisitionID  string  `json:"requisition_id" db:"requisition_id"`
	LineNumber     int16   `json:"line_number" db:"line_number"`
	ProductName    string  `json:"product_name" db:"product_name"`
	Description    string  `json:"description" db:"description"`
	Quantity       float64 `json:"quantity" db:"quantity"`
	EstimatedPrice float64 `json:"estimated_price" db:"estimated_price"`
	Amount         float64 `json:"amount" db:"amount"`
}

type PurchaseOrder struct {
	ID             string    `json:"id" db:"id"`
	CompanyID      string    `json:"company_id" db:"company_id"`
	BranchID       *string   `json:"branch_id" db:"branch_id"`
	PONumber       string    `json:"po_number" db:"po_number"`
	SupplierID     string    `json:"supplier_id" db:"supplier_id"`
	RequisitionID  *string   `json:"requisition_id" db:"requisition_id"`
	OrderDate      time.Time `json:"order_date" db:"order_date"`
	Status         string    `json:"status" db:"status"`
	SubtotalAmount float64   `json:"subtotal_amount" db:"subtotal_amount"`
	TaxAmount      float64   `json:"tax_amount" db:"tax_amount"`
	TotalAmount    float64   `json:"total_amount" db:"total_amount"`
	InvoiceID      *string   `json:"invoice_id" db:"invoice_id"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type PurchaseOrderLine struct {
	ID              string  `json:"id" db:"id"`
	PurchaseOrderID string  `json:"purchase_order_id" db:"purchase_order_id"`
	LineNumber      int16   `json:"line_number" db:"line_number"`
	ProductName     string  `json:"product_name" db:"product_name"`
	Description     string  `json:"description" db:"description"`
	Quantity        float64 `json:"quantity" db:"quantity"`
	UnitPrice       float64 `json:"unit_price" db:"unit_price"`
	Amount          float64 `json:"amount" db:"amount"`
}
