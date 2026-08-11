package model

import "time"

type Vehicle struct {
	ID          string    `json:"id"`
	CompanyID   string    `json:"company_id"`
	BranchID    *string   `json:"branch_id"`
	VehicleCode string    `json:"vehicle_code"`
	PlateNumber string    `json:"plate_number"`
	Name        string    `json:"name"`
	VehicleType string    `json:"vehicle_type"`
	CapacityKg  float64   `json:"capacity_kg"`
	Status      string    `json:"status"`
	Notes       string    `json:"notes"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Driver struct {
	ID            string    `json:"id"`
	CompanyID     string    `json:"company_id"`
	BranchID      *string   `json:"branch_id"`
	DriverCode    string    `json:"driver_code"`
	Name          string    `json:"name"`
	Phone         string    `json:"phone"`
	LicenseNumber string    `json:"license_number"`
	Status        string    `json:"status"`
	Notes         string    `json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// DeliveryOrder -- EcommerceOrderID dan ReferenceNumber nullable karena
// pengiriman tidak selalu berasal dari order e-commerce (lihat komentar
// migrations/001_init.sql). Timestamp transisi (DispatchedAt/DeliveredAt/
// CancelledAt) pointer karena genuinely NULL sampai transisinya terjadi --
// bukan string kosong, pelajaran dari bug scan-NULL opportunities.lost_reason
// di crm-service.
type DeliveryOrder struct {
	ID                 string     `json:"id"`
	CompanyID          string     `json:"company_id"`
	BranchID           *string    `json:"branch_id"`
	DeliveryNumber     string     `json:"delivery_number"`
	VehicleID          string     `json:"vehicle_id"`
	DriverID           string     `json:"driver_id"`
	EcommerceOrderID   *string    `json:"ecommerce_order_id"`
	ReferenceNumber    *string    `json:"reference_number"`
	RecipientName      string     `json:"recipient_name"`
	RecipientPhone     string     `json:"recipient_phone"`
	DestinationAddress string     `json:"destination_address"`
	ScheduledDate      time.Time  `json:"scheduled_date"`
	Status             string     `json:"status"`
	DispatchedAt       *time.Time `json:"dispatched_at"`
	DeliveredAt        *time.Time `json:"delivered_at"`
	CancelledAt        *time.Time `json:"cancelled_at"`
	Notes              string     `json:"notes"`
	CreatedByUserID    *string    `json:"created_by_user_id"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
