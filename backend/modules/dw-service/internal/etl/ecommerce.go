package etl

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

const ecommerceSourceTable = "order_items"

// ecommerceExtractSQL watermark pakai orders.updated_at -- order_items
// sendiri tidak punya updated_at (immutable setelah dibuat), jadi kalau
// status order berubah (mis. PAID -> SHIPPED), updated_at parent-nya ikut
// berubah dan SEMUA baris item order itu ikut ter-extract ulang -- benar
// secara desain, sama pola dengan SyncSales (fact_sales_order_lines).
const ecommerceExtractSQL = `
	SELECT oi.id, oi.order_id, o.company_id, o.branch_id, o.order_number, o.order_date, o.status,
	       o.customer_name, o.customer_email, oi.product_id, oi.product_sku, oi.product_name,
	       oi.quantity, oi.unit_price, oi.line_total, o.updated_at
	FROM order_items oi
	JOIN orders o ON o.id = oi.order_id
	WHERE o.updated_at >= $1
	ORDER BY o.updated_at`

// SyncEcommerce mengekstrak order_items (di-join ke orders) dari
// ecommerce-service, lalu load ke fact_ecommerce_order_lines di ClickHouse
// -- grain dan pola identik dengan fact_sales_order_lines.
func SyncEcommerce(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, ecommerceSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get ecommerce watermark: %w", err)
	}

	rows, err := source.Query(ctx, ecommerceExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract ecommerce rows: %w", err)
	}
	defer rows.Close()

	var out []ch.EcommerceOrderLineRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.EcommerceOrderLineRow
		if err := rows.Scan(
			&r.LineID, &r.OrderID, &r.CompanyID, &r.BranchID, &r.OrderNumber, &r.OrderDate, &r.OrderStatus,
			&r.CustomerName, &r.CustomerEmail, &r.ProductID, &r.ProductSKU, &r.ProductName,
			&r.Quantity, &r.UnitPrice, &r.Amount, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan ecommerce row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate ecommerce rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertEcommerceOrderLines(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load ecommerce rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, ecommerceSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", ecommerceSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, ecommerceSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance ecommerce watermark: %w", err)
	}
	return len(out), nil
}
