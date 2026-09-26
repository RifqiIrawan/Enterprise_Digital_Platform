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
func SyncProduction(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, productionSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get production watermark: %w", err)
	}

	out, maxWatermark, err := extractProduction(ctx, source, watermark)
	if err != nil {
		return 0, err
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertProductionWorkOrders(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load production rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, productionSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", productionSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, productionSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance production watermark: %w", err)
	}
	return len(out), nil
}

// extractProduction adalah bagian "tarik dari Postgres" milik SyncProduction, dipisah supaya
// Backfill bisa memakai SQL dan pemetaan kolom yang PERSIS sama (watermark nol
// = seluruh baris) tanpa menyentuh ClickHouse.
func extractProduction(ctx context.Context, source *pgxpool.Pool, watermark time.Time) ([]ch.ProductionWorkOrderRow, time.Time, error) {
	rows, err := source.Query(ctx, productionExtractSQL, watermark)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("extract production rows: %w", err)
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
			return nil, time.Time{}, fmt.Errorf("scan production row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, fmt.Errorf("iterate production rows: %w", err)
	}

	return out, maxWatermark, nil
}

const productionOEESourceTable = "production_oee"

// Hanya run CLOSED yang diekstrak. Itu aturan yang sama dengan halaman OEE di
// production-service: run yang masih berjalan belum punya jumlah unit, dan
// downtime-nya masih boleh bertambah (menambah downtime TIDAK menyentuh
// production_runs.updated_at, jadi baris OPEN yang terlanjur tersalin bisa
// ketinggalan menitnya). Begitu run ditutup, updated_at-nya naik -- jadi tidak
// ada run yang lolos dari watermark, hanya tertunda sampai angkanya final.
//
// LATERAL, bukan JOIN biasa ke downtime_logs: satu run dengan 3 catatan
// downtime akan menggandakan barisnya 3 kali kalau di-join langsung, dan
// planned_minutes-nya ikut tergandakan.
const productionOEEExtractSQL = `
	SELECT pr.id, pr.company_id, pr.branch_id, pr.run_number,
	       pr.work_order_id, wo.wo_number, pr.machine_id, m.code, m.name,
	       pr.shift_id, s.code, pr.run_date, pr.planned_minutes,
	       COALESCE(dt.minutes, 0), COALESCE(pr.quantity_good, 0), COALESCE(pr.quantity_reject, 0),
	       m.ideal_cycle_time_minutes, pr.status, pr.updated_at
	FROM production_runs pr
	JOIN machines m ON m.id = pr.machine_id
	JOIN shifts s ON s.id = pr.shift_id
	JOIN work_orders wo ON wo.id = pr.work_order_id
	LEFT JOIN LATERAL (
	    SELECT COALESCE(SUM(minutes), 0) AS minutes FROM downtime_logs WHERE production_run_id = pr.id
	) dt ON TRUE
	WHERE pr.status = 'CLOSED' AND pr.updated_at >= $1
	ORDER BY pr.updated_at`

// SyncProductionOEE memuat fact_production_oee: satu baris per production run
// yang sudah ditutup, lengkap dengan menit downtime-nya dan ideal cycle time
// mesinnya. Berbeda dari SyncProduction (yang fact-nya adalah RENCANA: work
// order), fact ini adalah PELAKSANAAN di lantai produksi -- keduanya berdiri
// sendiri karena satu work order bisa punya banyak run, dan menggabungkannya
// akan memaksa salah satu pertanyaan dijawab dengan angka yang digandakan.
//
// Berbeda juga dari fact lain yang tidak berani JOIN lintas service: mesin,
// shift, dan work order semuanya ada di database production_service yang sama
// dengan production_runs, jadi ini join biasa, bukan penyeberangan sumber.
func SyncProductionOEE(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, productionOEESourceTable)
	if err != nil {
		return 0, fmt.Errorf("get production oee watermark: %w", err)
	}

	out, maxWatermark, err := extractProductionOEE(ctx, source, watermark)
	if err != nil {
		return 0, err
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertProductionOEE(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load production oee rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, productionOEESourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", productionOEESourceTable, err)
	}
	if err := dest.SetWatermark(ctx, productionOEESourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance production oee watermark: %w", err)
	}
	return len(out), nil
}

// extractProductionOEE adalah bagian "tarik dari Postgres" milik SyncProductionOEE, dipisah supaya
// Backfill bisa memakai SQL dan pemetaan kolom yang PERSIS sama (watermark nol
// = seluruh baris) tanpa menyentuh ClickHouse.
func extractProductionOEE(ctx context.Context, source *pgxpool.Pool, watermark time.Time) ([]ch.ProductionOEERow, time.Time, error) {
	rows, err := source.Query(ctx, productionOEEExtractSQL, watermark)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("extract production oee rows: %w", err)
	}
	defer rows.Close()

	var out []ch.ProductionOEERow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.ProductionOEERow
		if err := rows.Scan(
			&r.RunID, &r.CompanyID, &r.BranchID, &r.RunNumber,
			&r.WorkOrderID, &r.WONumber, &r.MachineID, &r.MachineCode, &r.MachineName,
			&r.ShiftID, &r.ShiftCode, &r.RunDate, &r.PlannedMinutes,
			&r.DowntimeMinutes, &r.QuantityGood, &r.QuantityReject,
			&r.IdealCycleTimeMinutes, &r.Status, &r.UpdatedAt,
		); err != nil {
			return nil, time.Time{}, fmt.Errorf("scan production oee row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, fmt.Errorf("iterate production oee rows: %w", err)
	}

	return out, maxWatermark, nil
}
