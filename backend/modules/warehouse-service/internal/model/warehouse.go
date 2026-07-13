package model

import "time"

type Product struct {
	ID        string    `json:"id" db:"id"`
	CompanyID string    `json:"company_id" db:"company_id"`
	BranchID  *string   `json:"branch_id" db:"branch_id"`
	SKU       string    `json:"sku" db:"sku"`
	Name      string    `json:"name" db:"name"`
	Unit      string    `json:"unit" db:"unit"`
	Category  string    `json:"category" db:"category"`
	CostPrice float64   `json:"cost_price" db:"cost_price"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Warehouse struct {
	ID        string    `json:"id" db:"id"`
	CompanyID string    `json:"company_id" db:"company_id"`
	BranchID  *string   `json:"branch_id" db:"branch_id"`
	Code      string    `json:"code" db:"code"`
	Name      string    `json:"name" db:"name"`
	Address   string    `json:"address" db:"address"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type StockBalance struct {
	ID          string    `json:"id" db:"id"`
	WarehouseID string    `json:"warehouse_id" db:"warehouse_id"`
	ProductID   string    `json:"product_id" db:"product_id"`
	Quantity    float64   `json:"quantity" db:"quantity"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`

	ProductSKU  string `json:"product_sku,omitempty" db:"-"`
	ProductName string `json:"product_name,omitempty" db:"-"`
	ProductUnit string `json:"product_unit,omitempty" db:"-"`
}

type StockMovement struct {
	ID            string    `json:"id" db:"id"`
	CompanyID     string    `json:"company_id" db:"company_id"`
	BranchID      *string   `json:"branch_id" db:"branch_id"`
	WarehouseID   string    `json:"warehouse_id" db:"warehouse_id"`
	ProductID     string    `json:"product_id" db:"product_id"`
	MovementType  string    `json:"movement_type" db:"movement_type"`
	Quantity      float64   `json:"quantity" db:"quantity"`
	ReferenceType string    `json:"reference_type" db:"reference_type"`
	ReferenceID   *string   `json:"reference_id" db:"reference_id"`
	Notes         string    `json:"notes" db:"notes"`
	MovementDate  time.Time `json:"movement_date" db:"movement_date"`
	ActorUserID   *string   `json:"actor_user_id" db:"actor_user_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`

	ProductSKU  string `json:"product_sku,omitempty" db:"-"`
	ProductName string `json:"product_name,omitempty" db:"-"`
}

type StockTransfer struct {
	ID              string    `json:"id" db:"id"`
	CompanyID       string    `json:"company_id" db:"company_id"`
	BranchID        *string   `json:"branch_id" db:"branch_id"`
	TransferNumber  string    `json:"transfer_number" db:"transfer_number"`
	FromWarehouseID string    `json:"from_warehouse_id" db:"from_warehouse_id"`
	ToWarehouseID   string    `json:"to_warehouse_id" db:"to_warehouse_id"`
	TransferDate    time.Time `json:"transfer_date" db:"transfer_date"`
	Status          string    `json:"status" db:"status"`
	Notes           string    `json:"notes" db:"notes"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

type StockTransferLine struct {
	ID         string  `json:"id" db:"id"`
	TransferID string  `json:"transfer_id" db:"transfer_id"`
	LineNumber int16   `json:"line_number" db:"line_number"`
	ProductID  string  `json:"product_id" db:"product_id"`
	Quantity   float64 `json:"quantity" db:"quantity"`

	ProductSKU  string `json:"product_sku,omitempty" db:"-"`
	ProductName string `json:"product_name,omitempty" db:"-"`
}

type StockOpname struct {
	ID           string    `json:"id" db:"id"`
	CompanyID    string    `json:"company_id" db:"company_id"`
	BranchID     *string   `json:"branch_id" db:"branch_id"`
	WarehouseID  string    `json:"warehouse_id" db:"warehouse_id"`
	OpnameNumber string    `json:"opname_number" db:"opname_number"`
	OpnameDate   time.Time `json:"opname_date" db:"opname_date"`
	Status       string    `json:"status" db:"status"`
	Notes        string    `json:"notes" db:"notes"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type StockOpnameLine struct {
	ID              string  `json:"id" db:"id"`
	OpnameID        string  `json:"opname_id" db:"opname_id"`
	ProductID       string  `json:"product_id" db:"product_id"`
	SystemQuantity  float64 `json:"system_quantity" db:"system_quantity"`
	CountedQuantity float64 `json:"counted_quantity" db:"counted_quantity"`

	ProductSKU  string `json:"product_sku,omitempty" db:"-"`
	ProductName string `json:"product_name,omitempty" db:"-"`
}
