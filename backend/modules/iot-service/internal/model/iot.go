package model

import "time"

type Device struct {
	ID           string    `json:"id" db:"id"`
	CompanyID    string    `json:"company_id" db:"company_id"`
	BranchID     *string   `json:"branch_id" db:"branch_id"`
	WarehouseID  *string   `json:"warehouse_id" db:"warehouse_id"`
	DeviceCode   string    `json:"device_code" db:"device_code"`
	DeviceType   string    `json:"device_type" db:"device_type"`
	Name         string    `json:"name" db:"name"`
	Status       string    `json:"status" db:"status"`
	ThresholdMin *float64  `json:"threshold_min" db:"threshold_min"`
	ThresholdMax *float64  `json:"threshold_max" db:"threshold_max"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type Reading struct {
	ID           string    `json:"id" db:"id"`
	DeviceID     string    `json:"device_id" db:"device_id"`
	CompanyID    string    `json:"company_id" db:"company_id"`
	BranchID     *string   `json:"branch_id" db:"branch_id"`
	ReadingType  string    `json:"reading_type" db:"reading_type"`
	ValueNumeric *float64  `json:"value_numeric" db:"value_numeric"`
	ValueText    *string   `json:"value_text" db:"value_text"`
	RecordedAt   time.Time `json:"recorded_at" db:"recorded_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`

	DeviceCode string `json:"device_code,omitempty" db:"-"`
	DeviceName string `json:"device_name,omitempty" db:"-"`
}

type Alert struct {
	ID             string     `json:"id" db:"id"`
	DeviceID       string     `json:"device_id" db:"device_id"`
	ReadingID      *string    `json:"reading_id" db:"reading_id"`
	CompanyID      string     `json:"company_id" db:"company_id"`
	BranchID       *string    `json:"branch_id" db:"branch_id"`
	AlertType      string     `json:"alert_type" db:"alert_type"`
	Severity       string     `json:"severity" db:"severity"`
	Message        string     `json:"message" db:"message"`
	Status         string     `json:"status" db:"status"`
	TriggeredAt    time.Time  `json:"triggered_at" db:"triggered_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at" db:"acknowledged_at"`
	AcknowledgedBy *string    `json:"acknowledged_by" db:"acknowledged_by"`
	ResolvedAt     *time.Time `json:"resolved_at" db:"resolved_at"`
	ResolvedBy     *string    `json:"resolved_by" db:"resolved_by"`

	DeviceCode string `json:"device_code,omitempty" db:"-"`
	DeviceName string `json:"device_name,omitempty" db:"-"`
}
