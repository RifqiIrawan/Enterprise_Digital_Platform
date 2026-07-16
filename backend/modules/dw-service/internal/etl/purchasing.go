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

const purchasingSourceTable = "purchasing_order_lines"

const purchasingExtractSQL = `
	SELECT pol.id, pol.purchase_order_id, po.company_id, po.branch_id, po.po_number, po.order_date,
	       po.status, po.supplier_id, s.supplier_code, s.name, pol.product_name, pol.quantity,
	       pol.unit_price, pol.amount, po.updated_at
	FROM purchase_order_lines pol
	JOIN purchase_orders po ON po.id = pol.purchase_order_id
	JOIN suppliers s ON s.id = po.supplier_id
	WHERE po.updated_at >= $1
	ORDER BY po.updated_at`

// SyncPurchasing mengekstrak purchase_order_lines (di-join ke
// purchase_orders dan suppliers) dari purchasing_service, lalu load ke
// fact_purchasing_order_lines di ClickHouse. Watermark & desain identik
// dengan SyncSales (AP mirror dari AR) -- purchase_orders.updated_at
// dipakai, bukan created_at, dengan alasan yang sama: kalau status PO
// berubah (mis. CONFIRMED -> RECEIVED), SEMUA baris line PO itu ikut
// ter-extract ulang, dan ReplacingMergeTree di tujuan meng-upsert baris
// yang sama, bukan duplikat.
func SyncPurchasing(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, purchasingSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get purchasing watermark: %w", err)
	}

	rows, err := source.Query(ctx, purchasingExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract purchasing rows: %w", err)
	}
	defer rows.Close()

	var out []ch.PurchasingOrderLineRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.PurchasingOrderLineRow
		if err := rows.Scan(
			&r.LineID, &r.PurchaseOrderID, &r.CompanyID, &r.BranchID, &r.PONumber, &r.OrderDate,
			&r.OrderStatus, &r.SupplierID, &r.SupplierCode, &r.SupplierName, &r.ProductName,
			&r.Quantity, &r.UnitPrice, &r.Amount, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan purchasing row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate purchasing rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertPurchasingOrderLines(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load purchasing rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, purchasingSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", purchasingSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, purchasingSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance purchasing watermark: %w", err)
	}
	return len(out), nil
}
