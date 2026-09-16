package etl

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

// fact_production_oee (Fase 3). Dua hal yang dijaga di sini: extract-nya
// menjumlahkan downtime tanpa menggandakan baris run-nya, dan rumus OEE di
// gudang data memberi angka yang SAMA PERSIS dengan aggregateOEE milik
// production-service. Yang kedua itu alasan fact ini ada: kalau grafik BI dan
// halaman OEE operasional boleh menyimpang, salah satunya pasti dipercaya
// orang tanpa tahu yang mana yang benar.

type mesFixture struct {
	runID     uuid.UUID
	runNumber string
	machineID uuid.UUID
	companyID uuid.UUID
}

// mustSeedProductionRun menanam satu run lengkap dengan mesin, shift, dan work
// order-nya. downtimeMinutes ditanam sebagai BEBERAPA catatan supaya extract
// SQL-nya benar-benar diuji terhadap kasus yang membuat JOIN biasa salah.
func mustSeedProductionRun(t *testing.T, companyID uuid.UUID, status string, plannedMinutes int, good, reject *float64, idealCycle float64, downtimeMinutes []int) mesFixture {
	t.Helper()
	ctx := context.Background()
	woID, _ := mustSeedWorkOrder(t, companyID, nil)

	var machineID, shiftID, runID uuid.UUID
	code := uuid.NewString()[:8]
	if err := sourcePool.QueryRow(ctx,
		`INSERT INTO machines (company_id, code, name, ideal_cycle_time_minutes) VALUES ($1, $2, $3, $4) RETURNING id`,
		companyID, "MC-"+code, "Mesin "+code, idealCycle,
	).Scan(&machineID); err != nil {
		t.Fatalf("seed machine: %v", err)
	}
	if err := sourcePool.QueryRow(ctx,
		`INSERT INTO shifts (company_id, code, name) VALUES ($1, $2, $3) RETURNING id`,
		companyID, "SH-"+code, "Shift "+code,
	).Scan(&shiftID); err != nil {
		t.Fatalf("seed shift: %v", err)
	}

	runNumber := "PRUN-" + code
	if err := sourcePool.QueryRow(ctx, `
		INSERT INTO production_runs (company_id, run_number, work_order_id, machine_id, shift_id, run_date, planned_minutes, quantity_good, quantity_reject, status)
		VALUES ($1, $2, $3, $4, $5, CURRENT_DATE, $6, $7, $8, $9)
		RETURNING id`,
		companyID, runNumber, woID, machineID, shiftID, plannedMinutes, good, reject, status,
	).Scan(&runID); err != nil {
		t.Fatalf("seed production run: %v", err)
	}
	for _, m := range downtimeMinutes {
		if _, err := sourcePool.Exec(ctx,
			`INSERT INTO downtime_logs (production_run_id, reason_code, minutes) VALUES ($1, 'BREAKDOWN', $2)`,
			runID, m,
		); err != nil {
			t.Fatalf("seed downtime: %v", err)
		}
	}
	return mesFixture{runID: runID, runNumber: runNumber, machineID: machineID, companyID: companyID}
}

func TestSyncProductionOEE_ExtractsAndSumsDowntime(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	good, reject := 150.0, 10.0
	f := mustSeedProductionRun(t, companyID, "CLOSED", 420, &good, &reject, 2.0, []int{40, 20})

	n, err := SyncProductionOEE(ctx, sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncProductionOEE: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var (
		gotRunNumber              string
		gotPlanned, gotDowntime   int32
		gotGood, gotReject, ideal float64
	)
	row := chClient.QueryRow(ctx, `
		SELECT run_number, planned_minutes, downtime_minutes,
		       toFloat64(quantity_good), toFloat64(quantity_reject), toFloat64(ideal_cycle_time_minutes)
		FROM fact_production_oee FINAL WHERE run_id = ?`, f.runID)
	if err := row.Scan(&gotRunNumber, &gotPlanned, &gotDowntime, &gotGood, &gotReject, &ideal); err != nil {
		t.Fatalf("query synced oee row: %v", err)
	}
	if gotRunNumber != f.runNumber {
		t.Errorf("run_number = %q, want %q", gotRunNumber, f.runNumber)
	}
	// Dua catatan downtime menjadi SATU baris dengan 60 menit -- bukan dua
	// baris yang masing-masing membawa planned_minutes 420.
	if gotDowntime != 60 {
		t.Errorf("downtime_minutes = %d, want 60 (40 + 20 dalam satu baris)", gotDowntime)
	}
	if gotPlanned != 420 {
		t.Errorf("planned_minutes = %d, want 420", gotPlanned)
	}
	if gotGood != 150 || gotReject != 10 || ideal != 2 {
		t.Errorf("good/reject/ideal = %v/%v/%v, want 150/10/2", gotGood, gotReject, ideal)
	}
}

// Run yang masih OPEN belum punya angka unit, dan downtime-nya masih boleh
// bertambah tanpa menyentuh updated_at. Menyalinnya lebih awal berarti gudang
// data menyimpan versi yang belum final -- jadi extract-nya menunggu CLOSED.
func TestSyncProductionOEE_SkipsOpenRuns(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	f := mustSeedProductionRun(t, companyID, "OPEN", 480, nil, nil, 1.5, []int{15})

	if _, err := SyncProductionOEE(ctx, sourcePool, chClient, nil); err != nil {
		t.Fatalf("SyncProductionOEE: %v", err)
	}

	var count uint64
	if err := chClient.QueryRow(ctx,
		"SELECT count() FROM fact_production_oee FINAL WHERE run_id = ?", f.runID).Scan(&count); err != nil {
		t.Fatalf("count open run: %v", err)
	}
	if count != 0 {
		t.Errorf("run OPEN ikut tersalin (%d baris), mau 0", count)
	}
}

func oeeRow(companyID uuid.UUID, date, status string, planned, downtime int32, good, reject, idealCycle float64, t *testing.T) ch.ProductionOEERow {
	t.Helper()
	return ch.ProductionOEERow{
		RunID: uuid.New(), CompanyID: companyID, RunNumber: "PRUN-" + uuid.NewString()[:6],
		WorkOrderID: uuid.New(), WONumber: "WO-X", MachineID: uuid.New(), MachineCode: "MC-1",
		MachineName: "Mesin 1", ShiftID: uuid.New(), ShiftCode: "SH-1",
		RunDate: mustDate(t, date), PlannedMinutes: planned, DowntimeMinutes: downtime,
		QuantityGood: good, QuantityReject: reject, IdealCycleTimeMinutes: idealCycle,
		Status: status, UpdatedAt: time.Now(),
	}
}

// Angka acuannya diambil dari verifikasi end-to-end MES: planned 420, downtime
// 60, 150 bagus + 10 reject pada mesin ber-cycle 2 menit menghasilkan
// A 0,8571 / P 0,8889 / Q 0,9375 / OEE 0,7143 di production-service. Gudang
// data harus sampai ke angka yang sama, dalam persen.
func TestProductionOEEMonthlySummary_MatchesServiceFormula(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	rows := []ch.ProductionOEERow{
		oeeRow(companyID, "2026-08-03", "CLOSED", 420, 60, 150, 10, 2.0, t),
		// Run OPEN tidak boleh ikut: angkanya belum final.
		oeeRow(companyID, "2026-08-04", "OPEN", 480, 0, 0, 0, 2.0, t),
	}
	if err := chClient.InsertProductionOEE(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertProductionOEE: %v", err)
	}

	summary, err := chClient.ProductionOEEMonthlySummary(ctx, companyID)
	if err != nil {
		t.Fatalf("ProductionOEEMonthlySummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected 1 bulan, got %d", len(summary))
	}
	m := summary[0]
	if m.RunCount != 1 {
		t.Errorf("run_count = %d, want 1 (run OPEN tidak ikut)", m.RunCount)
	}
	if m.RunTimeMinutes != 360 {
		t.Errorf("run_time_minutes = %d, want 360 (420 - 60)", m.RunTimeMinutes)
	}
	check := func(label string, got *float64, want float64) {
		t.Helper()
		if got == nil {
			t.Errorf("%s = nil, want %.2f", label, want)
			return
		}
		if *got != want {
			t.Errorf("%s = %.2f, want %.2f", label, *got, want)
		}
	}
	check("availability_pct", m.AvailabilityPct, 85.71)
	check("performance_pct", m.PerformancePct, 88.89)
	check("quality_pct", m.QualityPct, 93.75)
	check("oee_pct", m.OEEPct, 71.43)
	if m.PerformanceCapped {
		t.Error("performance_capped = true, want false")
	}
}

// Performance di atas 100% selalu berarti salah satu angkanya keliru (cycle
// time terlalu lambat, atau downtime yang tidak dicatat). Dipotong di 100% DAN
// ditandai -- kalau cuma dipotong, OEE-nya berbohong ke atas tanpa jejak.
func TestProductionOEEMonthlySummary_FlagsCappedPerformance(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	// 200 unit x 3 menit ideal = 600 menit ideal untuk run time 180 menit.
	rows := []ch.ProductionOEERow{oeeRow(companyID, "2026-09-01", "CLOSED", 240, 60, 200, 0, 3.0, t)}
	if err := chClient.InsertProductionOEE(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertProductionOEE: %v", err)
	}

	summary, err := chClient.ProductionOEEMonthlySummary(ctx, companyID)
	if err != nil {
		t.Fatalf("ProductionOEEMonthlySummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected 1 bulan, got %d", len(summary))
	}
	m := summary[0]
	if m.PerformancePct == nil || *m.PerformancePct != 100 {
		t.Errorf("performance_pct = %v, want 100 (dipotong)", m.PerformancePct)
	}
	if !m.PerformanceCapped {
		t.Error("performance_capped = false, want true")
	}
	// Availability 180/240 = 75%, Quality 100% -> OEE = 75%.
	if m.OEEPct == nil || *m.OEEPct != 75 {
		t.Errorf("oee_pct = %v, want 75", m.OEEPct)
	}
}

// Bulan tanpa unit sama sekali tidak menghasilkan OEE 0% -- itu klaim bahwa
// mesinnya tidak menghasilkan apa-apa. Yang benar adalah "belum ada angkanya".
func TestProductionOEEMonthlySummary_NullWhenNoUnits(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	rows := []ch.ProductionOEERow{oeeRow(companyID, "2026-10-01", "CLOSED", 240, 0, 0, 0, 2.0, t)}
	if err := chClient.InsertProductionOEE(ctx, rows, time.Now()); err != nil {
		t.Fatalf("InsertProductionOEE: %v", err)
	}

	summary, err := chClient.ProductionOEEMonthlySummary(ctx, companyID)
	if err != nil {
		t.Fatalf("ProductionOEEMonthlySummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected 1 bulan, got %d", len(summary))
	}
	m := summary[0]
	if m.QualityPct != nil || m.OEEPct != nil {
		t.Errorf("quality/oee = %v/%v, want nil keduanya", m.QualityPct, m.OEEPct)
	}
	// Availability tetap bisa dihitung: waktunya memang ada.
	if m.AvailabilityPct == nil || *m.AvailabilityPct != 100 {
		t.Errorf("availability_pct = %v, want 100", m.AvailabilityPct)
	}
}
