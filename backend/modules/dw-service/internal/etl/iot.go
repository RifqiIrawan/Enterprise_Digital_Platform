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

const iotSourceTable = "iot_readings"

const iotExtractSQL = `
	SELECT r.id, r.company_id, r.branch_id, r.device_id, d.device_code, d.device_type,
	       r.reading_type, r.value_numeric, r.value_text, r.recorded_at, r.created_at
	FROM readings r
	JOIN devices d ON d.id = r.device_id
	WHERE r.created_at >= $1
	ORDER BY r.created_at`

// SyncIoT mengekstrak readings (di-join ke devices untuk device_code/type)
// dari iot_service, lalu load ke fact_iot_readings di ClickHouse. Watermark
// pakai created_at, bukan recorded_at -- readings append-only, tidak pernah
// di-UPDATE setelah dibuat (tidak ada kolom updated_at, lihat
// migrations/001_init.sql iot-service), sama seperti fact_inventory_
// movements. recorded_at (waktu sensor membaca nilai) TIDAK dipakai sebagai
// watermark karena readings bisa datang out-of-order dari MQTT (recorded_at
// device bisa lebih lambat/cepat dari created_at server), created_at
// server-side lebih aman untuk menjamin "tidak pernah terlewat" saat sync
// berikutnya.
func SyncIoT(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, iotSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get iot watermark: %w", err)
	}

	rows, err := source.Query(ctx, iotExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract iot rows: %w", err)
	}
	defer rows.Close()

	var out []ch.IoTReadingRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.IoTReadingRow
		var createdAt time.Time
		if err := rows.Scan(
			&r.ReadingID, &r.CompanyID, &r.BranchID, &r.DeviceID, &r.DeviceCode, &r.DeviceType,
			&r.ReadingType, &r.ValueNumeric, &r.ValueText, &r.RecordedAt, &createdAt,
		); err != nil {
			return 0, fmt.Errorf("scan iot row: %w", err)
		}
		out = append(out, r)
		if createdAt.After(maxWatermark) {
			maxWatermark = createdAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate iot rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertIoTReadings(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load iot rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, iotSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", iotSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, iotSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance iot watermark: %w", err)
	}
	return len(out), nil
}
