package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// mustSeedTimesheet menyiapkan proyek + (opsional) tugas + satu timesheet.
// withTask=false sengaja menyisakan task_id NULL: itulah kasus "jam kerja umum
// proyek" yang akan HILANG kalau ETL-nya memakai INNER JOIN ke tasks.
func mustSeedTimesheet(t *testing.T, companyID uuid.UUID, withTask bool, hours, rate, amount float64) (timesheetID uuid.UUID, projectCode string) {
	t.Helper()
	ctx := context.Background()

	projectCode = "PRJ-" + uuid.NewString()[:8]
	var projectID uuid.UUID
	if err := sourcePool.QueryRow(ctx,
		`INSERT INTO projects (company_id, project_code, name, status, budget_amount)
		 VALUES ($1, $2, 'Migrasi Data Center', 'ACTIVE', 50000000) RETURNING id`,
		companyID, projectCode,
	).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	var taskID *uuid.UUID
	if withTask {
		var id uuid.UUID
		if err := sourcePool.QueryRow(ctx,
			`INSERT INTO tasks (company_id, project_id, task_number, title)
			 VALUES ($1, $2, $3, 'Inventarisasi server') RETURNING id`,
			companyID, projectID, "TSK-"+uuid.NewString()[:8],
		).Scan(&id); err != nil {
			t.Fatalf("seed task: %v", err)
		}
		taskID = &id
	}

	if err := sourcePool.QueryRow(ctx, `
		INSERT INTO timesheets (company_id, project_id, task_id, employee_id, employee_name, hours, hourly_rate, amount, status)
		VALUES ($1, $2, $3, $4, 'Dewi Lestari', $5, $6, $7, 'APPROVED')
		RETURNING id`,
		companyID, projectID, taskID, uuid.New(), hours, rate, amount,
	).Scan(&timesheetID); err != nil {
		t.Fatalf("seed timesheet: %v", err)
	}
	return timesheetID, projectCode
}

func TestSyncProject_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	// Angka sengaja bisa dihitung tangan: 6 jam x 100.000 = 600.000.
	timesheetID, projectCode := mustSeedTimesheet(t, companyID, true, 6, 100000, 600000)

	n, err := SyncProject(context.Background(), sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotProjectCode, gotStatus string
	var gotHours, gotAmount float64
	row := chClient.QueryRow(context.Background(),
		"SELECT project_code, status, toFloat64(hours), toFloat64(amount) FROM fact_project_timesheets FINAL WHERE timesheet_id = ?", timesheetID)
	if err := row.Scan(&gotProjectCode, &gotStatus, &gotHours, &gotAmount); err != nil {
		t.Fatalf("query synced project row: %v", err)
	}
	if gotProjectCode != projectCode {
		t.Errorf("project_code = %q, want %q", gotProjectCode, projectCode)
	}
	if gotStatus != "APPROVED" {
		t.Errorf("status = %q, want APPROVED", gotStatus)
	}
	if gotHours != 6 {
		t.Errorf("hours = %v, want 6", gotHours)
	}
	if gotAmount != 600000 {
		t.Errorf("amount = %v, want 600000", gotAmount)
	}
}

// Timesheet TANPA tugas (jam kerja umum proyek) harus tetap masuk warehouse.
// Kalau ETL-nya memakai INNER JOIN ke tasks, baris ini hilang diam-diam dan
// total biaya di warehouse tidak akan pernah cocok dengan actual_cost proyek.
func TestSyncProject_KeepsTimesheetsWithoutTask(t *testing.T) {
	companyID := uuid.New()
	timesheetID, _ := mustSeedTimesheet(t, companyID, false, 2, 250000, 500000)

	if _, err := SyncProject(context.Background(), sourcePool, chClient, nil); err != nil {
		t.Fatalf("SyncProject: %v", err)
	}

	var n uint64
	row := chClient.QueryRow(context.Background(),
		"SELECT count(*) FROM fact_project_timesheets FINAL WHERE timesheet_id = ?", timesheetID)
	if err := row.Scan(&n); err != nil {
		t.Fatalf("query synced project row: %v", err)
	}
	if n != 1 {
		t.Fatalf("timesheet without a task must still be synced, got %d rows", n)
	}

	var taskNumberIsNull uint8
	row = chClient.QueryRow(context.Background(),
		"SELECT isNull(task_number) FROM fact_project_timesheets FINAL WHERE timesheet_id = ?", timesheetID)
	if err := row.Scan(&taskNumberIsNull); err != nil {
		t.Fatalf("query task_number: %v", err)
	}
	if taskNumberIsNull != 1 {
		t.Error("task_number should be NULL for a timesheet with no task")
	}
}
