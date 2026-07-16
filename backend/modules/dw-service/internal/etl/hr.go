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

const hrSourceTable = "hr_payroll_details"

const hrExtractSQL = `
	SELECT pd.id, pd.payroll_run_id, pr.company_id, pr.branch_id, pr.period, pr.status,
	       pd.employee_id, e.employee_code, pd.employee_name, COALESCE(e.department, ''),
	       pd.basic_salary, pd.gross_salary, pd.total_deduction, pd.net_salary,
	       pd.working_days, pd.present_days, pr.posted_at,
	       COALESCE(pr.posted_at, pd.created_at) AS watermark
	FROM payroll_details pd
	JOIN payroll_runs pr ON pr.id = pd.payroll_run_id
	JOIN employees e ON e.id = pd.employee_id
	WHERE COALESCE(pr.posted_at, pd.created_at) >= $1
	ORDER BY COALESCE(pr.posted_at, pd.created_at)`

// SyncHR mengekstrak payroll_details (di-join ke payroll_runs dan employees)
// dari hr_service, lalu load ke fact_hr_payroll_details di ClickHouse.
// Watermark sama persis polanya dengan finance (COALESCE posted_at/
// created_at) -- payroll_details/payroll_runs juga tidak punya kolom
// updated_at, cuma berubah lewat status DRAFT->POSTED yang tercermin di
// payroll_runs.posted_at, lihat migrations/001_init.sql hr-service.
func SyncHR(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, hrSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get hr watermark: %w", err)
	}

	rows, err := source.Query(ctx, hrExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract hr rows: %w", err)
	}
	defer rows.Close()

	var out []ch.HRPayrollDetailRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.HRPayrollDetailRow
		var wm time.Time
		if err := rows.Scan(
			&r.DetailID, &r.PayrollRunID, &r.CompanyID, &r.BranchID, &r.Period, &r.RunStatus,
			&r.EmployeeID, &r.EmployeeCode, &r.EmployeeName, &r.Department,
			&r.BasicSalary, &r.GrossSalary, &r.TotalDeduction, &r.NetSalary,
			&r.WorkingDays, &r.PresentDays, &r.PostedAt, &wm,
		); err != nil {
			return 0, fmt.Errorf("scan hr row: %w", err)
		}
		out = append(out, r)
		if wm.After(maxWatermark) {
			maxWatermark = wm
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate hr rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertHRPayrollDetails(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load hr rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, hrSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", hrSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, hrSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance hr watermark: %w", err)
	}
	return len(out), nil
}
