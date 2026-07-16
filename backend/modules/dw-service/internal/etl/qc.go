package etl

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

const qcSourceTable = "qc_inspections"

const qcExtractSQL = `
	SELECT qi.id, qi.company_id, qi.branch_id, qi.inspection_number, qi.standard_id, qs.standard_code,
	       qi.product_id, qi.reference_type, qi.reference_id, qi.reference_number,
	       qi.inspected_quantity, qi.passed_quantity, qi.failed_quantity, qi.result,
	       qi.inspection_date, qi.updated_at
	FROM quality_inspections qi
	JOIN quality_standards qs ON qs.id = qi.standard_id
	WHERE qi.updated_at >= $1
	ORDER BY qi.updated_at`

// SyncQC mengekstrak quality_inspections (di-join ke quality_standards untuk
// standard_code) dari qc_service, lalu load ke fact_qc_inspections di
// ClickHouse. Berbeda dari modul lain, inspeksi qc-service sengaja dibuat
// final saat dibuat (tidak ada status DRAFT/POSTED, lihat komentar
// migrations/001_init.sql qc-service) -- tapi tabelnya tetap punya
// updated_at (dipakai kalau nanti ada edit pasca-buat), jadi watermark
// pakai itu, konsisten dengan pola production/purchasing.
func SyncQC(ctx context.Context, source *pgxpool.Pool, dest *ch.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, qcSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get qc watermark: %w", err)
	}

	rows, err := source.Query(ctx, qcExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract qc rows: %w", err)
	}
	defer rows.Close()

	var out []ch.QCInspectionRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.QCInspectionRow
		if err := rows.Scan(
			&r.InspectionID, &r.CompanyID, &r.BranchID, &r.InspectionNumber, &r.StandardID, &r.StandardCode,
			&r.ProductID, &r.ReferenceType, &r.ReferenceID, &r.ReferenceNumber,
			&r.InspectedQuantity, &r.PassedQuantity, &r.FailedQuantity, &r.Result,
			&r.InspectionDate, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan qc row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate qc rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertQCInspections(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load qc rows: %w", err)
	}
	if err := dest.SetWatermark(ctx, qcSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance qc watermark: %w", err)
	}
	return len(out), nil
}
