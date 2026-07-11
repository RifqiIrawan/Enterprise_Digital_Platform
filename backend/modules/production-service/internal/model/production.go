package model

import "time"

type BillOfMaterial struct {
	ID        string    `json:"id" db:"id"`
	CompanyID string    `json:"company_id" db:"company_id"`
	BranchID  *string   `json:"branch_id" db:"branch_id"`
	BOMCode   string    `json:"bom_code" db:"bom_code"`
	Name      string    `json:"name" db:"name"`
	ProductID string    `json:"product_id" db:"product_id"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type BOMLine struct {
	ID                 string  `json:"id" db:"id"`
	BOMID              string  `json:"bom_id" db:"bom_id"`
	LineNumber         int16   `json:"line_number" db:"line_number"`
	ComponentProductID string  `json:"component_product_id" db:"component_product_id"`
	QuantityPerUnit    float64 `json:"quantity_per_unit" db:"quantity_per_unit"`
}

type WorkOrder struct {
	ID                string     `json:"id" db:"id"`
	CompanyID         string     `json:"company_id" db:"company_id"`
	BranchID          *string    `json:"branch_id" db:"branch_id"`
	WONumber          string     `json:"wo_number" db:"wo_number"`
	BOMID             string     `json:"bom_id" db:"bom_id"`
	ProductID         string     `json:"product_id" db:"product_id"`
	WarehouseID       string     `json:"warehouse_id" db:"warehouse_id"`
	QuantityPlanned   float64    `json:"quantity_planned" db:"quantity_planned"`
	QuantityProduced  *float64   `json:"quantity_produced" db:"quantity_produced"`
	Status            string     `json:"status" db:"status"`
	PlannedStartDate  time.Time  `json:"planned_start_date" db:"planned_start_date"`
	PlannedEndDate    *time.Time `json:"planned_end_date" db:"planned_end_date"`
	Notes             string     `json:"notes" db:"notes"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

type WorkOrderLine struct {
	ID                 string  `json:"id" db:"id"`
	WorkOrderID        string  `json:"work_order_id" db:"work_order_id"`
	LineNumber         int16   `json:"line_number" db:"line_number"`
	ComponentProductID string  `json:"component_product_id" db:"component_product_id"`
	QuantityRequired   float64 `json:"quantity_required" db:"quantity_required"`
}
