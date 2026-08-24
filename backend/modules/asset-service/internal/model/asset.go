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
	// Penyusutan (Fase 5). UsefulLifeMonths nil = aset ini memang tidak
	// disusutkan. BookValue tidak disimpan di tabel; dihitung
	// (acquisition_cost - accumulated_depreciation) supaya tidak ada dua
	// angka yang bisa berbeda.
	SalvageValue            float64    `json:"salvage_value" db:"salvage_value"`
	UsefulLifeMonths        *int       `json:"useful_life_months" db:"useful_life_months"`
	DepreciationMethod      string     `json:"depreciation_method" db:"depreciation_method"`
	DepreciationStartDate   *time.Time `json:"depreciation_start_date" db:"depreciation_start_date"`
	AccumulatedDepreciation float64    `json:"accumulated_depreciation" db:"accumulated_depreciation"`
	BookValue               float64    `json:"book_value"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
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

// ---------- Fase 5: penyusutan & kalibrasi ----------

type DepreciationRun struct {
	ID             string     `json:"id" db:"id"`
	CompanyID      string     `json:"company_id" db:"company_id"`
	Period         string     `json:"period" db:"period"`
	Status         string     `json:"status" db:"status"`
	AssetCount     int        `json:"asset_count" db:"asset_count"`
	TotalAmount    float64    `json:"total_amount" db:"total_amount"`
	JournalEntryID *string    `json:"journal_entry_id" db:"journal_entry_id"`
	PostedAt       *time.Time `json:"posted_at" db:"posted_at"`
	Notes          string     `json:"notes" db:"notes"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type DepreciationEntry struct {
	ID              string    `json:"id" db:"id"`
	RunID           string    `json:"run_id" db:"run_id"`
	AssetID         string    `json:"asset_id" db:"asset_id"`
	Method          string    `json:"method" db:"method"`
	Amount          float64   `json:"amount" db:"amount"`
	BookValueBefore float64   `json:"book_value_before" db:"book_value_before"`
	BookValueAfter  float64   `json:"book_value_after" db:"book_value_after"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type Calibration struct {
	ID                string     `json:"id" db:"id"`
	CompanyID         string     `json:"company_id" db:"company_id"`
	BranchID          *string    `json:"branch_id" db:"branch_id"`
	AssetID           string     `json:"asset_id" db:"asset_id"`
	ScheduledDate     time.Time  `json:"scheduled_date" db:"scheduled_date"`
	PerformedDate     *time.Time `json:"performed_date" db:"performed_date"`
	IntervalMonths    *int       `json:"interval_months" db:"interval_months"`
	NextDueDate       *time.Time `json:"next_due_date" db:"next_due_date"`
	Result            *string    `json:"result" db:"result"`
	CertificateNumber *string    `json:"certificate_number" db:"certificate_number"`
	PerformedBy       *string    `json:"performed_by" db:"performed_by"`
	Status            string     `json:"status" db:"status"`
	Notes             string     `json:"notes" db:"notes"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}
