package model

import "time"

type Asset struct {
	ID              string     `json:"id" db:"id"`
	CompanyID       string     `json:"company_id" db:"company_id"`
	BranchID        *string    `json:"branch_id" db:"branch_id"`
	WarehouseID     *string    `json:"warehouse_id" db:"warehouse_id"`
	AssetCode       string     `json:"asset_code" db:"asset_code"`
	Name            string     `json:"name" db:"name"`
	Category        string     `json:"category" db:"category"`
	AcquisitionDate *time.Time `json:"acquisition_date" db:"acquisition_date"`
	AcquisitionCost float64    `json:"acquisition_cost" db:"acquisition_cost"`
	Status          string     `json:"status" db:"status"`
	Notes           string     `json:"notes" db:"notes"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type MaintenanceSchedule struct {
	ID              string     `json:"id" db:"id"`
	CompanyID       string     `json:"company_id" db:"company_id"`
	BranchID        *string    `json:"branch_id" db:"branch_id"`
	AssetID         string     `json:"asset_id" db:"asset_id"`
	MaintenanceType string     `json:"maintenance_type" db:"maintenance_type"`
	ScheduledDate   time.Time  `json:"scheduled_date" db:"scheduled_date"`
	CompletedDate   *time.Time `json:"completed_date" db:"completed_date"`
	Status          string     `json:"status" db:"status"`
	Notes           string     `json:"notes" db:"notes"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}
