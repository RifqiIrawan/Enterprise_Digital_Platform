package model

import "time"

type Order struct {
	ID              string     `json:"id" db:"id"`
	CompanyID       string     `json:"company_id" db:"company_id"`
	BranchID        *string    `json:"branch_id" db:"branch_id"`
	OrderNumber     string     `json:"order_number" db:"order_number"`
	CustomerName    string     `json:"customer_name" db:"customer_name"`
	CustomerEmail   string     `json:"customer_email" db:"customer_email"`
	ShippingAddress string     `json:"shipping_address" db:"shipping_address"`
	Status          string     `json:"status" db:"status"`
	OrderDate       time.Time  `json:"order_date" db:"order_date"`
	TotalAmount     float64    `json:"total_amount" db:"total_amount"`
	Notes           string     `json:"notes" db:"notes"`
	PlacedByUserID  *string    `json:"placed_by_user_id" db:"placed_by_user_id"`
	PaidAt          *time.Time `json:"paid_at" db:"paid_at"`
	ShippedAt       *time.Time `json:"shipped_at" db:"shipped_at"`
	DeliveredAt     *time.Time `json:"delivered_at" db:"delivered_at"`
	CancelledAt     *time.Time `json:"cancelled_at" db:"cancelled_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type OrderItem struct {
	ID          string    `json:"id" db:"id"`
	CompanyID   string    `json:"company_id" db:"company_id"`
	BranchID    *string   `json:"branch_id" db:"branch_id"`
	OrderID     string    `json:"order_id" db:"order_id"`
	ProductID   string    `json:"product_id" db:"product_id"`
	ProductSKU  string    `json:"product_sku" db:"product_sku"`
	ProductName string    `json:"product_name" db:"product_name"`
	UnitPrice   float64   `json:"unit_price" db:"unit_price"`
	Quantity    float64   `json:"quantity" db:"quantity"`
	LineTotal   float64   `json:"line_total" db:"line_total"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// OrderWithItems adalah bentuk response GET /orders/{id} -- order header
// plus baris keranjangnya, sama pola dengan salesOrderWithLines di
// sales-service (SalesOrder embedded + Lines []SalesOrderLine).
type OrderWithItems struct {
	Order
	Items []OrderItem `json:"items"`
}
