package etl

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

const inventorySourceTable = "inventory_movements"

const inventoryExtractSQL = `
	SELECT sm.id, sm.company_id, sm.branch_id, sm.warehouse_id, w.code, w.name, sm.product_id,
	       p.sku, p.name, sm.movement_type, sm.quantity, sm.reference_type, sm.reference_id,
	       sm.movement_date, sm.created_at
	FROM stock_movements sm
	JOIN warehouses w ON w.id = sm.warehouse_id
	JOIN products p ON p.id = sm.product_id
	WHERE sm.created_at >= $1
	ORDER BY sm.created_at`

// SyncInventory mengekstrak stock_movements (di-join ke warehouses dan
// products) dari warehouse_service, lalu load ke fact_inventory_movements di
// ClickHouse. Watermark pakai created_at -- stock_movements append-only,
// tidak pernah di-UPDATE setelah dibuat (dikonfirmasi dari
// migrations/001_init.sql warehouse-service, tidak ada kolom updated_at
// sama sekali di tabel itu), jadi ini watermark paling sederhana dari
// ketiga fact yang ada.
func SyncInventory(ctx context.Context, source *pgxpool.Pool, dest *ch.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, inventorySourceTable)
	if err != nil {
		return 0, fmt.Errorf("get inventory watermark: %w", err)
	}

	rows, err := source.Query(ctx, inventoryExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract inventory rows: %w", err)
	}
	defer rows.Close()

	var out []ch.InventoryMovementRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.InventoryMovementRow
		var createdAt time.Time
		if err := rows.Scan(
			&r.MovementID, &r.CompanyID, &r.BranchID, &r.WarehouseID, &r.WarehouseCode, &r.WarehouseName,
			&r.ProductID, &r.ProductSKU, &r.ProductName, &r.MovementType, &r.Quantity, &r.ReferenceType,
			&r.ReferenceID, &r.MovementDate, &createdAt,
		); err != nil {
			return 0, fmt.Errorf("scan inventory row: %w", err)
		}
		out = append(out, r)
		if createdAt.After(maxWatermark) {
			maxWatermark = createdAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate inventory rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertInventoryMovements(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load inventory rows: %w", err)
	}
	if err := dest.SetWatermark(ctx, inventorySourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance inventory watermark: %w", err)
	}
	return len(out), nil
}
