package etl

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func mustSeedPayrollDetail(t *testing.T, companyID uuid.UUID, runStatus string) (detailID uuid.UUID, employeeCode string) {
	t.Helper()
	employeeCode = "EMP-" + uuid.NewString()[:8]
	var employeeID uuid.UUID
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO employees (employee_code, department) VALUES ($1, $2) RETURNING id`,
		employeeCode, "Finance",
	).Scan(&employeeID)
	if err != nil {
		t.Fatalf("seed employee: %v", err)
	}

	var postedAt *time.Time
	if runStatus == "POSTED" {
		now := time.Now()
		postedAt = &now
	}

	var runID uuid.UUID
	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO payroll_runs (company_id, period, status, posted_at)
		VALUES ($1, '2026-07', $2, $3)
		RETURNING id`,
		companyID, runStatus, postedAt,
	).Scan(&runID)
	if err != nil {
		t.Fatalf("seed payroll run: %v", err)
	}

	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO payroll_details (payroll_run_id, employee_id, employee_name, basic_salary, gross_salary, total_deduction, net_salary, working_days, present_days)
		VALUES ($1, $2, 'Test Employee', 5000000, 5500000, 500000, 5000000, 22, 20)
		RETURNING id`,
		runID, employeeID,
	).Scan(&detailID)
	if err != nil {
		t.Fatalf("seed payroll detail: %v", err)
	}
	return detailID, employeeCode
}

func TestSyncHR_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	detailID, employeeCode := mustSeedPayrollDetail(t, companyID, "POSTED")

	n, err := SyncHR(context.Background(), sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncHR: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotEmployeeCode string
	var gotNetSalary decimal.Decimal
	row := chClient.QueryRow(context.Background(),
		"SELECT employee_code, net_salary FROM fact_hr_payroll_details FINAL WHERE detail_id = ?", detailID)
	if err := row.Scan(&gotEmployeeCode, &gotNetSalary); err != nil {
		t.Fatalf("query synced hr row: %v", err)
	}
	if gotEmployeeCode != employeeCode {
		t.Errorf("employee_code = %q, want %q", gotEmployeeCode, employeeCode)
	}
	if !gotNetSalary.Equal(decimal.NewFromInt(5000000)) {
		t.Errorf("net_salary = %v, want 5000000", gotNetSalary)
	}
}
