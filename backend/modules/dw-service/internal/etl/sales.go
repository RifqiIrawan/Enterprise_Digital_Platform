package etl

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

const salesSourceTable = "sales_order_lines"

const salesExtractSQL = `
	SELECT sol.id, sol.sales_order_id, so.company_id, so.branch_id, so.so_number, so.order_date,
	       so.status, so.customer_id, c.customer_code, c.name, sol.product_name, sol.quantity,
	       sol.unit_price, sol.amount, so.updated_at
	FROM sales_order_lines sol
	JOIN sales_orders so ON so.id = sol.sales_order_id
	JOIN customers c ON c.id = so.customer_id
	WHERE so.updated_at >= $1
	ORDER BY so.updated_at`

// SyncSales mengekstrak sales_order_lines (di-join ke sales_orders dan
// customers) dari sales_service, lalu load ke fact_sales_order_lines di
// ClickHouse. Watermark pakai sales_orders.updated_at -- baris line sendiri
// tidak punya timestamp, jadi kalau status order berubah (mis. CONFIRMED ->
// FULFILLED), updated_at parent-nya ikut berubah dan SEMUA baris line order
// itu ikut ter-extract ulang -- benar secara desain (ReplacingMergeTree di
// tujuan akan meng-upsert baris yang sama, bukan duplikat).
func SyncSales(ctx context.Context, source *pgxpool.Pool, dest *ch.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, salesSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get sales watermark: %w", err)
	}

	rows, err := source.Query(ctx, salesExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract sales rows: %w", err)
	}
	defer rows.Close()

	var out []ch.SalesOrderLineRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.SalesOrderLineRow
		if err := rows.Scan(
			&r.LineID, &r.SalesOrderID, &r.CompanyID, &r.BranchID, &r.SONumber, &r.OrderDate,
			&r.OrderStatus, &r.CustomerID, &r.CustomerCode, &r.CustomerName, &r.ProductName,
			&r.Quantity, &r.UnitPrice, &r.Amount, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan sales row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate sales rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertSalesOrderLines(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load sales rows: %w", err)
	}
	if err := dest.SetWatermark(ctx, salesSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance sales watermark: %w", err)
	}
	return len(out), nil
}
