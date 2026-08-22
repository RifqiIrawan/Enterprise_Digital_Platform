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

const (
	hrLeaveSourceTable = "hr_leave_requests"
	hrKPISourceTable   = "hr_kpi_reviews"
)

// Berbeda dari payroll (yang tidak punya updated_at dan memakai
// COALESCE(posted_at, created_at)), leave_requests dan kpi_reviews SUDAH punya
// updated_at yang ikut berubah di setiap transisi status. Jadi watermark-nya
// langsung updated_at -- perubahan status DRAFT->APPROVED otomatis terangkut di
// sync berikutnya tanpa perlu kolom turunan.
const hrLeaveExtractSQL = `
	SELECT lr.id, lr.company_id, lr.branch_id, lr.employee_id, e.employee_code,
	       lr.employee_name, COALESCE(e.department, ''), lr.leave_type, lr.status,
	       lr.start_date, lr.end_date, lr.total_days, lr.decided_at,
	       lr.created_at, lr.updated_at
	FROM leave_requests lr
	JOIN employees e ON e.id = lr.employee_id
	WHERE lr.updated_at >= $1
	ORDER BY lr.updated_at`

const hrKPIExtractSQL = `
	SELECT kr.id, kr.company_id, kr.branch_id, kr.employee_id, e.employee_code,
	       kr.employee_name, COALESCE(e.department, ''), kr.period, kr.status,
	       kr.total_score, kr.rating, kr.decided_at, kr.created_at, kr.updated_at
	FROM kpi_reviews kr
	JOIN employees e ON e.id = kr.employee_id
	WHERE kr.updated_at >= $1
	ORDER BY kr.updated_at`

// SyncHRLeave memuat pengajuan cuti (seluruh status) ke
// fact_hr_leave_requests. Penyaringan status dilakukan di query ringkasan,
// bukan di sini -- lihat HRLeaveMonthlySummary.
func SyncHRLeave(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, hrLeaveSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get hr leave watermark: %w", err)
	}

	rows, err := source.Query(ctx, hrLeaveExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract hr leave rows: %w", err)
	}
	defer rows.Close()

	var out []ch.HRLeaveRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.HRLeaveRow
		if err := rows.Scan(
			&r.LeaveID, &r.CompanyID, &r.BranchID, &r.EmployeeID, &r.EmployeeCode,
			&r.EmployeeName, &r.Department, &r.LeaveType, &r.Status,
			&r.StartDate, &r.EndDate, &r.TotalDays, &r.DecidedAt, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan hr leave row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate hr leave rows: %w", err)
	}
	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertHRLeaveRequests(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load hr leave rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, hrLeaveSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", hrLeaveSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, hrLeaveSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("set hr leave watermark: %w", err)
	}
	return len(out), nil
}

// SyncHRKPI memuat kepala penilaian KPI (nilai total & rating) ke
// fact_hr_kpi_reviews. Rincian per indikator sengaja tidak ikut -- lihat
// komentar HRKPIReviewRow.
func SyncHRKPI(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, hrKPISourceTable)
	if err != nil {
		return 0, fmt.Errorf("get hr kpi watermark: %w", err)
	}

	rows, err := source.Query(ctx, hrKPIExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract hr kpi rows: %w", err)
	}
	defer rows.Close()

	var out []ch.HRKPIReviewRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.HRKPIReviewRow
		if err := rows.Scan(
			&r.ReviewID, &r.CompanyID, &r.BranchID, &r.EmployeeID, &r.EmployeeCode,
			&r.EmployeeName, &r.Department, &r.Period, &r.Status,
			&r.TotalScore, &r.Rating, &r.DecidedAt, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan hr kpi row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate hr kpi rows: %w", err)
	}
	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertHRKPIReviews(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load hr kpi rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, hrKPISourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", hrKPISourceTable, err)
	}
	if err := dest.SetWatermark(ctx, hrKPISourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("set hr kpi watermark: %w", err)
	}
	return len(out), nil
}
