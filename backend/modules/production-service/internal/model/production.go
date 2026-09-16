package model

import "time"

// BOMType membedakan BOM diskrit (UNIT, "sekian per unit produk") dari
// formula industri proses (BATCH, "sekian persen dari satu batch"). BatchSize
// hanya terisi untuk BATCH; lihat migrations/003_formula_bom.sql.
const (
	BOMTypeUnit  = "UNIT"
	BOMTypeBatch = "BATCH"
)

type BillOfMaterial struct {
	ID        string    `json:"id" db:"id"`
	CompanyID string    `json:"company_id" db:"company_id"`
	BranchID  *string   `json:"branch_id" db:"branch_id"`
	BOMCode   string    `json:"bom_code" db:"bom_code"`
	Name      string    `json:"name" db:"name"`
	ProductID string    `json:"product_id" db:"product_id"`
	BOMType   string    `json:"bom_type" db:"bom_type"`
	BatchSize *float64  `json:"batch_size" db:"batch_size"`
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
	// Percentage terisi hanya untuk baris BOM BATCH (formula): bagian baris
	// ini dari satu batch. Itulah angka yang dipakai menghitung kebutuhan
	// work order -- QuantityPerUnit-nya turunan (percentage / 100).
	Percentage *float64 `json:"percentage" db:"percentage"`
}

type WorkOrder struct {
	ID               string     `json:"id" db:"id"`
	CompanyID        string     `json:"company_id" db:"company_id"`
	BranchID         *string    `json:"branch_id" db:"branch_id"`
	WONumber         string     `json:"wo_number" db:"wo_number"`
	BOMID            string     `json:"bom_id" db:"bom_id"`
	ProductID        string     `json:"product_id" db:"product_id"`
	WarehouseID      string     `json:"warehouse_id" db:"warehouse_id"`
	QuantityPlanned  float64    `json:"quantity_planned" db:"quantity_planned"`
	BatchCount       *int       `json:"batch_count" db:"batch_count"`
	QuantityProduced *float64   `json:"quantity_produced" db:"quantity_produced"`
	Status           string     `json:"status" db:"status"`
	PlannedStartDate time.Time  `json:"planned_start_date" db:"planned_start_date"`
	PlannedEndDate   *time.Time `json:"planned_end_date" db:"planned_end_date"`
	Notes            string     `json:"notes" db:"notes"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

type WorkOrderLine struct {
	ID                 string  `json:"id" db:"id"`
	WorkOrderID        string  `json:"work_order_id" db:"work_order_id"`
	LineNumber         int16   `json:"line_number" db:"line_number"`
	ComponentProductID string  `json:"component_product_id" db:"component_product_id"`
	QuantityRequired   float64 `json:"quantity_required" db:"quantity_required"`
}

// ---------- Fase 3 (MES) ----------

type Machine struct {
	ID                    string    `json:"id" db:"id"`
	CompanyID             string    `json:"company_id" db:"company_id"`
	BranchID              *string   `json:"branch_id" db:"branch_id"`
	Code                  string    `json:"code" db:"code"`
	Name                  string    `json:"name" db:"name"`
	MachineType           string    `json:"machine_type" db:"machine_type"`
	Location              *string   `json:"location" db:"location"`
	IdealCycleTimeMinutes float64   `json:"ideal_cycle_time_minutes" db:"ideal_cycle_time_minutes"`
	Status                string    `json:"status" db:"status"`
	Notes                 string    `json:"notes" db:"notes"`
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
}

// Shift menyimpan StartTime/EndTime sebagai string "HH:MM" -- kolomnya TIME di
// Postgres dan dibaca lewat to_char(), supaya frontend menerima jam yang siap
// ditampilkan tanpa zona waktu yang tidak berarti apa-apa untuk jam shift.
// PlannedMinutes tidak disimpan di tabel shifts; dihitung dari jam & istirahat
// (lihat shiftPlannedMinutes) dan ikut dikirim supaya UI tidak menghitung
// ulang dengan rumus versinya sendiri.
type Shift struct {
	ID             string    `json:"id" db:"id"`
	CompanyID      string    `json:"company_id" db:"company_id"`
	Code           string    `json:"code" db:"code"`
	Name           string    `json:"name" db:"name"`
	StartTime      string    `json:"start_time" db:"start_time"`
	EndTime        string    `json:"end_time" db:"end_time"`
	BreakMinutes   int       `json:"break_minutes" db:"break_minutes"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	PlannedMinutes int       `json:"planned_minutes"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type ProductionRun struct {
	ID             string    `json:"id" db:"id"`
	CompanyID      string    `json:"company_id" db:"company_id"`
	BranchID       *string   `json:"branch_id" db:"branch_id"`
	RunNumber      string    `json:"run_number" db:"run_number"`
	WorkOrderID    string    `json:"work_order_id" db:"work_order_id"`
	MachineID      string    `json:"machine_id" db:"machine_id"`
	ShiftID        string    `json:"shift_id" db:"shift_id"`
	RunDate        time.Time `json:"run_date" db:"run_date"`
	PlannedMinutes int       `json:"planned_minutes" db:"planned_minutes"`
	QuantityGood   *float64  `json:"quantity_good" db:"quantity_good"`
	QuantityReject *float64  `json:"quantity_reject" db:"quantity_reject"`
	Status         string    `json:"status" db:"status"`
	Notes          string    `json:"notes" db:"notes"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type DowntimeLog struct {
	ID              string    `json:"id" db:"id"`
	ProductionRunID string    `json:"production_run_id" db:"production_run_id"`
	ReasonCode      string    `json:"reason_code" db:"reason_code"`
	Minutes         int       `json:"minutes" db:"minutes"`
	Notes           string    `json:"notes" db:"notes"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}
