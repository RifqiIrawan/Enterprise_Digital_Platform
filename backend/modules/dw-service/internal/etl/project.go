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

const projectSourceTable = "timesheets"

// projectExtractSQL watermark pakai timesheets.updated_at. Grain SATU baris per
// timesheet -- ini entitas project-service yang benar-benar membawa measure
// numerik (hours, hourly_rate, amount). `projects` sendiri TIDAK dijadikan fact
// terpisah: anggaran/realisasinya adalah agregat dari timesheet yang sudah
// diposting, jadi memodelkannya lagi sebagai fact berarti menyimpan angka yang
// sama dua kali dengan risiko keduanya berbeda.
//
// tasks di-LEFT JOIN karena task_id memang nullable di sumbernya (jam kerja
// umum proyek: rapat, koordinasi). INNER JOIN akan diam-diam MENGHILANGKAN
// baris-baris itu dari warehouse, dan totalnya tidak akan cocok lagi dengan
// actual_cost proyek di Postgres.
//
// journal_entry_id ikut dibawa supaya biaya di fact ini bisa direkonsiliasi
// langsung dengan fact_finance_journal_lines -- satu-satunya jembatan antara
// jam kerja proyek dan jurnal GL yang mencatat biayanya.
const projectExtractSQL = `
	SELECT ts.id, ts.company_id, ts.branch_id, ts.project_id,
	       p.project_code, p.name AS project_name, p.status AS project_status,
	       ts.task_id, t.task_number,
	       ts.employee_id, ts.employee_name, ts.work_date,
	       ts.hours, ts.hourly_rate, ts.amount,
	       ts.status, ts.approved_at, ts.posted_at, ts.journal_entry_id,
	       ts.created_at, ts.updated_at
	FROM timesheets ts
	JOIN projects p ON p.id = ts.project_id
	LEFT JOIN tasks t ON t.id = ts.task_id
	WHERE ts.updated_at >= $1
	ORDER BY ts.updated_at`

// SyncProject mengekstrak timesheets (di-join ke projects, LEFT JOIN tasks)
// dari project-service, lalu load ke fact_project_timesheets di ClickHouse.
func SyncProject(ctx context.Context, source *pgxpool.Pool, dest *ch.Client, lake *datalake.Client) (int, error) {
	watermark, err := dest.GetWatermark(ctx, projectSourceTable)
	if err != nil {
		return 0, fmt.Errorf("get project watermark: %w", err)
	}

	rows, err := source.Query(ctx, projectExtractSQL, watermark)
	if err != nil {
		return 0, fmt.Errorf("extract project rows: %w", err)
	}
	defer rows.Close()

	var out []ch.ProjectTimesheetRow
	maxWatermark := watermark
	for rows.Next() {
		var r ch.ProjectTimesheetRow
		if err := rows.Scan(
			&r.TimesheetID, &r.CompanyID, &r.BranchID, &r.ProjectID,
			&r.ProjectCode, &r.ProjectName, &r.ProjectStatus,
			&r.TaskID, &r.TaskNumber,
			&r.EmployeeID, &r.EmployeeName, &r.WorkDate,
			&r.Hours, &r.HourlyRate, &r.Amount,
			&r.Status, &r.ApprovedAt, &r.PostedAt, &r.JournalEntryID,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan project row: %w", err)
		}
		out = append(out, r)
		if r.UpdatedAt.After(maxWatermark) {
			maxWatermark = r.UpdatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate project rows: %w", err)
	}

	if len(out) == 0 {
		return 0, nil
	}

	syncedAt := time.Now()
	if err := dest.InsertProjectTimesheets(ctx, out, syncedAt); err != nil {
		return 0, fmt.Errorf("load project rows: %w", err)
	}
	if err := lake.WriteJSONLines(ctx, projectSourceTable, out, syncedAt); err != nil {
		log.Printf("dw-service: datalake write for %s failed (ClickHouse sync still succeeded): %v", projectSourceTable, err)
	}
	if err := dest.SetWatermark(ctx, projectSourceTable, maxWatermark); err != nil {
		return 0, fmt.Errorf("advance project watermark: %w", err)
	}
	return len(out), nil
}
