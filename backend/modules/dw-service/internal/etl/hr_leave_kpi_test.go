package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func mustSeedEmployeeRow(t *testing.T, department string) (uuid.UUID, string) {
	t.Helper()
	code := "EMP-" + uuid.NewString()[:8]
	var id uuid.UUID
	if err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO employees (employee_code, department) VALUES ($1, $2) RETURNING id`,
		code, department).Scan(&id); err != nil {
		t.Fatalf("seed employee: %v", err)
	}
	return id, code
}

func mustSeedLeave(t *testing.T, companyID uuid.UUID, leaveType, status string, days int) uuid.UUID {
	t.Helper()
	employeeID, _ := mustSeedEmployeeRow(t, "HR")
	var id uuid.UUID
	if err := sourcePool.QueryRow(context.Background(), `
		INSERT INTO leave_requests (company_id, employee_id, employee_name, leave_type, start_date, end_date, total_days, status)
		VALUES ($1, $2, 'Test Employee', $3, '2026-08-17', '2026-08-19', $4, $5)
		RETURNING id`, companyID, employeeID, leaveType, days, status).Scan(&id); err != nil {
		t.Fatalf("seed leave request: %v", err)
	}
	return id
}

func mustSeedKPIReview(t *testing.T, companyID uuid.UUID, period, status string, score float64, rating string) uuid.UUID {
	t.Helper()
	employeeID, _ := mustSeedEmployeeRow(t, "Sales")
	var id uuid.UUID
	if err := sourcePool.QueryRow(context.Background(), `
		INSERT INTO kpi_reviews (company_id, employee_id, employee_name, period, status, total_score, rating)
		VALUES ($1, $2, 'Test Employee', $3, $4, $5, $6)
		RETURNING id`, companyID, employeeID, period, status, score, rating).Scan(&id); err != nil {
		t.Fatalf("seed kpi review: %v", err)
	}
	return id
}

func TestSyncHRLeave_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	leaveID := mustSeedLeave(t, companyID, "ANNUAL", "APPROVED", 3)

	n, err := SyncHRLeave(context.Background(), sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncHRLeave: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var leaveType, status, department string
	var totalDays int16
	row := chClient.QueryRow(context.Background(),
		"SELECT leave_type, status, department, total_days FROM fact_hr_leave_requests FINAL WHERE leave_id = ?", leaveID)
	if err := row.Scan(&leaveType, &status, &department, &totalDays); err != nil {
		t.Fatalf("query synced leave row: %v", err)
	}
	if leaveType != "ANNUAL" || status != "APPROVED" || totalDays != 3 {
		t.Errorf("baris tersinkron tidak sesuai: %s/%s/%d", leaveType, status, totalDays)
	}
	if department != "HR" {
		t.Errorf("department = %q, want HR (hasil join ke employees)", department)
	}
}

// Cuti yang DITOLAK tetap ikut disalin: penyaringan status adalah urusan query
// ringkasan, bukan ETL.
func TestSyncHRLeave_KeepsNonApprovedStatuses(t *testing.T) {
	companyID := uuid.New()
	rejected := mustSeedLeave(t, companyID, "SICK", "REJECTED", 2)

	if _, err := SyncHRLeave(context.Background(), sourcePool, chClient, nil); err != nil {
		t.Fatalf("SyncHRLeave: %v", err)
	}

	var status string
	row := chClient.QueryRow(context.Background(),
		"SELECT status FROM fact_hr_leave_requests FINAL WHERE leave_id = ?", rejected)
	if err := row.Scan(&status); err != nil {
		t.Fatalf("query rejected leave row: %v", err)
	}
	if status != "REJECTED" {
		t.Errorf("status = %q, want REJECTED", status)
	}
}

// Perubahan status di sumber terangkut lewat watermark updated_at, dan
// ReplacingMergeTree menggantikan versi lamanya (FINAL memaksa penggabungan).
func TestSyncHRLeave_StatusChangeReplacesRow(t *testing.T) {
	companyID := uuid.New()
	leaveID := mustSeedLeave(t, companyID, "ANNUAL", "SUBMITTED", 3)

	if _, err := SyncHRLeave(context.Background(), sourcePool, chClient, nil); err != nil {
		t.Fatalf("sync pertama: %v", err)
	}

	if _, err := sourcePool.Exec(context.Background(),
		`UPDATE leave_requests SET status = 'APPROVED', decided_at = now(), updated_at = now() WHERE id = $1`,
		leaveID); err != nil {
		t.Fatalf("ubah status: %v", err)
	}
	if _, err := SyncHRLeave(context.Background(), sourcePool, chClient, nil); err != nil {
		t.Fatalf("sync kedua: %v", err)
	}

	var status string
	var count uint64
	row := chClient.QueryRow(context.Background(),
		"SELECT status, count() FROM fact_hr_leave_requests FINAL WHERE leave_id = ? GROUP BY status", leaveID)
	if err := row.Scan(&status, &count); err != nil {
		t.Fatalf("query leave row: %v", err)
	}
	if status != "APPROVED" {
		t.Errorf("status = %q, want APPROVED setelah sync kedua", status)
	}
	if count != 1 {
		t.Errorf("expected 1 baris setelah FINAL, got %d", count)
	}
}

func TestSyncHRKPI_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	reviewID := mustSeedKPIReview(t, companyID, "2026-08", "APPROVED", 88.5, "BAIK")

	n, err := SyncHRKPI(context.Background(), sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncHRKPI: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var period, rating, department string
	var score decimal.Decimal
	row := chClient.QueryRow(context.Background(),
		"SELECT period, rating, department, total_score FROM fact_hr_kpi_reviews FINAL WHERE review_id = ?", reviewID)
	if err := row.Scan(&period, &rating, &department, &score); err != nil {
		t.Fatalf("query synced kpi row: %v", err)
	}
	if period != "2026-08" || rating != "BAIK" {
		t.Errorf("baris tersinkron tidak sesuai: %s/%s", period, rating)
	}
	if !score.Equal(decimal.NewFromFloat(88.5)) {
		t.Errorf("total_score = %v, want 88.5", score)
	}
	if department != "Sales" {
		t.Errorf("department = %q, want Sales", department)
	}
}

// Watermark-nya INKLUSIF (">="), jadi sync ulang tanpa data baru memang
// memproses lagi baris di batas terakhir -- yang harus dijaga bukan "0 baris",
// melainkan tidak ada duplikat setelah FINAL. Pola yang sama dengan
// TestSyncFinance_RerunIsIdempotent.
func TestSyncHRKPI_RerunIsIdempotent(t *testing.T) {
	companyID := uuid.New()
	reviewID := mustSeedKPIReview(t, companyID, "2026-09", "APPROVED", 91, "SANGAT BAIK")

	if _, err := SyncHRKPI(context.Background(), sourcePool, chClient, nil); err != nil {
		t.Fatalf("sync pertama: %v", err)
	}
	if _, err := SyncHRKPI(context.Background(), sourcePool, chClient, nil); err != nil {
		t.Fatalf("sync kedua: %v", err)
	}

	var count uint64
	row := chClient.QueryRow(context.Background(),
		"SELECT count(*) FROM fact_hr_kpi_reviews FINAL WHERE review_id = ?", reviewID)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("hitung baris tersinkron: %v", err)
	}
	if count != 1 {
		t.Errorf("expected tepat 1 baris untuk review %s setelah 2 sync, got %d", reviewID, count)
	}
}
