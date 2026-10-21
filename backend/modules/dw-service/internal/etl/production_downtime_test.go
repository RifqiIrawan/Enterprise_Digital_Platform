package etl

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

// fact_production_downtime: alasan downtime per catatan, dari run yang sudah
// ditutup. Tiga hal yang dijaga: satu baris per catatan (bukan per run),
// catatan dari run OPEN tidak ikut dan baru terambil setelah run ditutup
// (watermark ada di run, bukan di catatan), dan jumlah menitnya SELALU sama
// dengan fact_production_oee untuk run yang sama.

// mustSeedDowntime menanam catatan downtime dengan alasan tertentu pada sebuah run.
func mustSeedDowntime(t *testing.T, runID uuid.UUID, reason string, minutes int) {
	t.Helper()
	if _, err := sourcePool.Exec(context.Background(),
		`INSERT INTO downtime_logs (production_run_id, reason_code, minutes) VALUES ($1, $2, $3)`,
		runID, reason, minutes); err != nil {
		t.Fatalf("seed downtime %s: %v", reason, err)
	}
}

func downtimeRowsFor(t *testing.T, runID uuid.UUID) map[string]int32 {
	t.Helper()
	got := map[string]int32{}
	for _, reason := range []string{"BREAKDOWN", "SETUP", "MATERIAL_SHORTAGE", "NO_OPERATOR", "ADJUSTMENT", "PLANNED_STOP", "OTHER"} {
		var minutes int64
		if err := chClient.QueryRow(context.Background(),
			`SELECT toInt64(sum(minutes)) FROM fact_production_downtime FINAL WHERE run_id = ? AND reason_code = ?`,
			runID, reason).Scan(&minutes); err != nil {
			t.Fatalf("query downtime %s: %v", reason, err)
		}
		if minutes != 0 {
			got[reason] = int32(minutes)
		}
	}
	return got
}

func TestSyncProductionDowntime_OneRowPerLogWithReason(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	good, reject := 150.0, 10.0
	f := mustSeedProductionRun(t, companyID, "CLOSED", 420, &good, &reject, 2.0, nil)
	mustSeedDowntime(t, f.runID, "BREAKDOWN", 40)
	mustSeedDowntime(t, f.runID, "SETUP", 15)
	mustSeedDowntime(t, f.runID, "BREAKDOWN", 5)

	if _, err := SyncProductionDowntime(ctx, sourcePool, chClient, nil); err != nil {
		t.Fatalf("SyncProductionDowntime: %v", err)
	}
	// Disalin dua kali: ReplacingMergeTree + FINAL tidak boleh menggandakan.
	if _, err := SyncProductionDowntime(ctx, sourcePool, chClient, nil); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	var count uint64
	if err := chClient.QueryRow(ctx,
		`SELECT count() FROM fact_production_downtime FINAL WHERE run_id = ?`, f.runID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("baris = %d, want 3 (satu per catatan)", count)
	}
	got := downtimeRowsFor(t, f.runID)
	if got["BREAKDOWN"] != 45 || got["SETUP"] != 15 {
		t.Errorf("menit per alasan = %v, want BREAKDOWN 45, SETUP 15", got)
	}
}

func TestSyncProductionDowntime_WaitsForRunToClose(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	f := mustSeedProductionRun(t, companyID, "OPEN", 480, nil, nil, 1.5, nil)
	mustSeedDowntime(t, f.runID, "NO_OPERATOR", 25)

	if _, err := SyncProductionDowntime(ctx, sourcePool, chClient, nil); err != nil {
		t.Fatal(err)
	}
	if got := downtimeRowsFor(t, f.runID); len(got) != 0 {
		t.Fatalf("catatan dari run OPEN ikut tersalin: %v", got)
	}

	// Catatan sudah ada SEBELUM run ditutup; watermark yang memakai
	// downtime_logs.created_at tidak akan pernah mengambilnya. Menutup run
	// menaikkan production_runs.updated_at, dan itu yang dipakai.
	if _, err := sourcePool.Exec(ctx,
		`UPDATE production_runs SET status = 'CLOSED', quantity_good = 10, quantity_reject = 0, updated_at = now() WHERE id = $1`, f.runID); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncProductionDowntime(ctx, sourcePool, chClient, nil); err != nil {
		t.Fatal(err)
	}
	if got := downtimeRowsFor(t, f.runID); got["NO_OPERATOR"] != 25 {
		t.Errorf("setelah run ditutup: %v, want NO_OPERATOR 25", got)
	}
}

// Jumlah menit downtime di dua fact harus sama persis untuk run yang sama.
// Kalau menyimpang, grafik Pareto dan grafik OEE di BI saling bertentangan.
func TestSyncProductionDowntime_TotalsMatchOEEFact(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	good, reject := 100.0, 0.0
	f := mustSeedProductionRun(t, companyID, "CLOSED", 400, &good, &reject, 2.0, nil)
	mustSeedDowntime(t, f.runID, "BREAKDOWN", 30)
	mustSeedDowntime(t, f.runID, "SETUP", 20)
	mustSeedDowntime(t, f.runID, "OTHER", 7)

	if _, err := SyncProductionOEE(ctx, sourcePool, chClient, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncProductionDowntime(ctx, sourcePool, chClient, nil); err != nil {
		t.Fatal(err)
	}

	var oeeMinutes int32
	if err := chClient.QueryRow(ctx,
		`SELECT downtime_minutes FROM fact_production_oee FINAL WHERE run_id = ?`, f.runID).Scan(&oeeMinutes); err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, m := range downtimeRowsFor(t, f.runID) {
		sum += int64(m)
	}
	if int64(oeeMinutes) != sum || sum != 57 {
		t.Errorf("fact OEE %d menit, fact downtime %d menit, want keduanya 57", oeeMinutes, sum)
	}
}

func downtimeRow(companyID uuid.UUID, date, reason string, minutes int32, t *testing.T) ch.ProductionDowntimeRow {
	t.Helper()
	return ch.ProductionDowntimeRow{
		DowntimeID: uuid.New(), CompanyID: companyID, RunID: uuid.New(), RunNumber: "PRUN-" + uuid.NewString()[:6],
		MachineID: uuid.New(), MachineCode: "MC-1", MachineName: "Mesin 1",
		RunDate: mustDate(t, date), ReasonCode: reason, Minutes: minutes, UpdatedAt: time.Now(),
	}
}

func TestProductionDowntimePareto_OrdersAndAccumulates(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	other := uuid.New()
	rows := []ch.ProductionDowntimeRow{
		downtimeRow(companyID, "2026-08-03", "BREAKDOWN", 40, t),
		downtimeRow(companyID, "2026-08-04", "BREAKDOWN", 20, t),
		downtimeRow(companyID, "2026-08-04", "SETUP", 30, t),
		downtimeRow(companyID, "2026-09-02", "OTHER", 10, t),
		// Company lain tidak boleh ikut.
		downtimeRow(other, "2026-08-03", "NO_OPERATOR", 999, t),
	}
	if err := chClient.InsertProductionDowntime(ctx, rows, time.Now()); err != nil {
		t.Fatal(err)
	}

	got, err := chClient.ProductionDowntimePareto(ctx, companyID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ReasonCode != "BREAKDOWN" || got[0].Minutes != 60 || got[1].ReasonCode != "SETUP" || got[2].ReasonCode != "OTHER" {
		t.Fatalf("urutan/menit salah: %+v", got)
	}
	want := []float64{0.6, 0.9, 1}
	for i, w := range want {
		if got[i].CumulativeShare != w {
			t.Errorf("kumulatif[%d] = %v, want %v", i, got[i].CumulativeShare, w)
		}
	}

	// Rentang tanggal: hanya Agustus.
	from, to := mustDate(t, "2026-08-01"), mustDate(t, "2026-08-31")
	aug, err := chClient.ProductionDowntimePareto(ctx, companyID, &from, &to)
	if err != nil {
		t.Fatal(err)
	}
	if len(aug) != 2 || aug[0].Minutes != 60 || aug[1].Minutes != 30 {
		t.Fatalf("Agustus: %+v", aug)
	}
	if aug[1].CumulativeShare != 1 {
		t.Errorf("baris terakhir kumulatif = %v, want tepat 1", aug[1].CumulativeShare)
	}

	// Company tanpa data: daftar kosong, bukan error atau null.
	empty, err := chClient.ProductionDowntimePareto(ctx, uuid.New(), nil, nil)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("tanpa data: %v, %v (mau [] dan nil error)", empty, err)
	}
}
