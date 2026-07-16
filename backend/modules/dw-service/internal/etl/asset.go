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

const assetSourceTable = "asset_maintenance"

const assetExtractSQL = `
	SELECT ms.id, ms.company_id, ms.branch_id, ms.asset_id, a.asset_code, a.name,
	       ms.maintenance_type, ms.scheduled_date, ms.completed_date, ms.status, ms.updated_at
	FROM maintenance_schedules ms
	JOIN assets a ON a.id = ms.asset_id
	WHERE ms.updated_at >= $1
	ORDER BY ms.updated_at`

// SyncAsset mengekstrak maintenance_schedules (di-join ke assets untuk
// asset_code/nama) dari asset_service, lalu load ke fact_asset_maintenance
// di ClickHouse. Watermark pakai updated_at (ada di maintenance_schedules
// sejak 001_init.sql) -- status SCHEDULED/COMPLETED/CANCELLED "overdue"
// dihitung on the fly di asset-service sendiri (bukan status tersendiri),
// jadi tidak ada kolom tambahan untuk itu di sini, sama seperti sumbernya.
func SyncAsset(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, assetSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get asset watermark: %w", err)
	}

	rows, err := source.Query(ctx, assetExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract asset rows: %w", err)
	}
	defer rows.Close()

	var out []ch.AssetMaintenanceRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.AssetMaintenanceRow
		if err := rows.Scan(
			&r.ScheduleID, &r.CompanyID, &r.BranchID, &r.AssetID, &r.AssetCode, &r.AssetName,
			&r.MaintenanceType, &r.ScheduledDate, &r.CompletedDate, &r.Status, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan asset row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate asset rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertAssetMaintenance(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load asset rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, assetSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", assetSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, assetSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance asset watermark: %w", err)
	}
	return len(out), nil
}
