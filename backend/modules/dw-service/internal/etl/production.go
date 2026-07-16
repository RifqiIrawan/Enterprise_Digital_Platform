package etl

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

const productionSourceTable = "production_work_orders"

const productionExtractSQL = `
	SELECT wo.id, wo.company_id, wo.branch_id, wo.wo_number, wo.bom_id, wo.product_id,
	       wo.warehouse_id, wo.quantity_planned, wo.quantity_produced, wo.status,
	       wo.planned_start_date, wo.planned_end_date, wo.updated_at
	FROM work_orders wo
	WHERE wo.updated_at >= $1
	ORDER BY wo.updated_at`

// SyncProduction mengekstrak work_orders (satu baris per work order, bukan
// work_order_lines -- lihat NEXT_SESSION.md untuk alasan pilihan fact ini)
// dari production_service, lalu load ke fact_production_work_orders di
// ClickHouse. product_id/warehouse_id TIDAK di-join ke nama (SKU/kode)
// seperti fact_inventory_movements -- production_service sendiri tidak
// bisa JOIN ke situ, product & warehouse ada di database warehouse_service
// yang beda (lihat komentar migrations/001_init.sql production-service),
// jadi dw-service pun sengaja tidak memaksa join lintas source pool untuk
// satu extract SQL. Watermark pakai updated_at (ada di work_orders, tidak
// seperti finance/hr yang tidak punya kolom itu).
func SyncProduction(ctx context.Context, source *pgxpool.Pool, dest *ch.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, productionSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get production watermark: %w", err)
	}

	rows, err := source.Query(ctx, productionExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract production rows: %w", err)
	}
	defer rows.Close()

	var out []ch.ProductionWorkOrderRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.ProductionWorkOrderRow
		if err := rows.Scan(
			&r.WOID, &r.CompanyID, &r.BranchID, &r.WONumber, &r.BOMID, &r.ProductID,
			&r.WarehouseID, &r.QuantityPlanned, &r.QuantityProduced, &r.Status,
			&r.PlannedStartDate, &r.PlannedEndDate, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan production row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate production rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertProductionWorkOrders(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load production rows: %w", err)
	}
	if err := dest.SetWatermark(ctx, productionSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance production watermark: %w", err)
	}
	return len(out), nil
}
