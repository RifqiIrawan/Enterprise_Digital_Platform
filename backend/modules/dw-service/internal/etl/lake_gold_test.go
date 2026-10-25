package etl

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

func testLake(t *testing.T) *datalake.Client {
	t.Helper()
	lake, err := datalake.Connect(context.Background(), getEnv("DW_TEST_MINIO_ENDPOINT", "localhost:9004"),
		getEnv("DW_TEST_MINIO_ACCESS_KEY", "minioadmin"),
		getEnv("DW_TEST_MINIO_SECRET_KEY", "minioadmin"),
		getEnv("DW_TEST_MINIO_BUCKET", "dw-lake-test"), false)
	if err != nil {
		t.Skipf("SKIP: lake tests need a local MinIO: %v", err)
	}
	return lake
}

// TestSilverFactsCoverEveryFact mengunci daftar fact Silver ke daftar fact
// ETL: fact baru yang lupa didaftarkan ke Silver akan diam-diam tidak pernah
// dibersihkan, dan Gold yang dibangun di atasnya tidak akan menyadarinya.
func TestSilverFactsCoverEveryFact(t *testing.T) {
	for _, f := range []string{
		assetSourceTable, crmSourceTable, ecommerceSourceTable, financeSourceTable,
		fleetSourceTable, hrSourceTable, hrLeaveSourceTable, hrKPISourceTable,
		inventorySourceTable, iotSourceTable, productionSourceTable,
		productionOEESourceTable, productionDowntimeSourceTable, projectSourceTable, purchasingSourceTable,
		qcSourceTable, salesSourceTable, ticketingSourceTable,
	} {
		if _, ok := datalake.SilverFacts[f]; !ok {
			t.Errorf("fact ETL %q belum terdaftar di datalake.SilverFacts", f)
		}
	}
	if len(datalake.SilverFacts) != 18 {
		t.Errorf("SilverFacts punya %d fact, ETL punya 18", len(datalake.SilverFacts))
	}
}

// TestGold_AgreesWithClickHouse adalah bukti bahwa dua jalur ke angka yang
// sama tidak berselisih: Bronze -> Silver -> Gold di MinIO, dan
// fact table -> query ClickHouse. Datanya sengaja memuat semua hal yang
// membuat keduanya mudah berbeda: satu baris ditulis dua kali dengan status
// berubah (DRAFT -> POSTED, CONFIRMED -> CANCELLED), baris DRAFT yang tidak
// boleh dihitung, dan akun non-REVENUE/EXPENSE.
func TestGold_AgreesWithClickHouse(t *testing.T) {
	ctx := context.Background()
	lake := testLake(t)

	companyID := uuid.New()
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	v1 := time.Now().Add(-time.Hour)
	v2 := time.Now()

	fin := func(status, typ string, id uuid.UUID, debit, credit float64) ch.FinanceJournalLineRow {
		return ch.FinanceJournalLineRow{
			LineID: id, JournalID: uuid.New(), CompanyID: companyID, EntryNumber: "JE-GOLD",
			EntryDate: date, Period: "2026-06", ReferenceType: "MANUAL", EntryStatus: status,
			AccountID: uuid.New(), AccountCode: "4000", AccountName: "x", AccountType: typ,
			DebitAmount: debit, CreditAmount: credit,
		}
	}
	rev, exp, asset := uuid.New(), uuid.New(), uuid.New()
	// versi 1: baris revenue masih DRAFT (9999) -- di versi 2 jadi POSTED (250,25).
	finV1 := []ch.FinanceJournalLineRow{fin("DRAFT", "REVENUE", rev, 0, 9999)}
	finV2 := []ch.FinanceJournalLineRow{
		fin("POSTED", "REVENUE", rev, 0, 250.25),
		fin("POSTED", "EXPENSE", exp, 80.10, 0),
		fin("POSTED", "ASSET", asset, 330.35, 0),
	}
	for _, batch := range []struct {
		rows []ch.FinanceJournalLineRow
		at   time.Time
	}{{finV1, v1}, {finV2, v2}} {
		if err := chClient.InsertFinanceJournalLines(ctx, batch.rows, batch.at); err != nil {
			t.Fatal(err)
		}
		if err := lake.WriteJSONLines(ctx, financeSourceTable, batch.rows, batch.at); err != nil {
			t.Fatal(err)
		}
	}

	sale := func(status string, id uuid.UUID, amount float64) ch.SalesOrderLineRow {
		return ch.SalesOrderLineRow{
			LineID: id, SalesOrderID: uuid.New(), CompanyID: companyID, SONumber: "SO-GOLD",
			OrderDate: date, OrderStatus: status, CustomerID: uuid.New(), CustomerCode: "C", CustomerName: "C",
			ProductName: "P", Quantity: 1, UnitPrice: amount, Amount: amount, UpdatedAt: v2,
		}
	}
	kept, flipped, draft := uuid.New(), uuid.New(), uuid.New()
	salesV1 := []ch.SalesOrderLineRow{sale("CONFIRMED", flipped, 1000)}
	salesV2 := []ch.SalesOrderLineRow{
		sale("CANCELLED", flipped, 1000), // dibatalkan sesudah dicatat
		sale("INVOICED", kept, 400.40),
		sale("DRAFT", draft, 7777),
	}
	for _, batch := range []struct {
		rows []ch.SalesOrderLineRow
		at   time.Time
	}{{salesV1, v1}, {salesV2, v2}} {
		if err := chClient.InsertSalesOrderLines(ctx, batch.rows, batch.at); err != nil {
			t.Fatal(err)
		}
		if err := lake.WriteJSONLines(ctx, salesSourceTable, batch.rows, batch.at); err != nil {
			t.Fatal(err)
		}
	}

	mv := func(typ string, id uuid.UUID, qty float64) ch.InventoryMovementRow {
		return ch.InventoryMovementRow{
			MovementID: id, CompanyID: companyID, WarehouseID: uuid.New(), WarehouseCode: "W", WarehouseName: "W",
			ProductID: uuid.New(), ProductSKU: "S", ProductName: "P", MovementType: typ,
			Quantity: qty, ReferenceType: "MANUAL", MovementDate: date,
		}
	}
	inA, outA, adj := uuid.New(), uuid.New(), uuid.New()
	// versi 1 mencatat kuantitas IN yang salah (500); versi 2 mengoreksinya.
	invV1 := []ch.InventoryMovementRow{mv("IN", inA, 500)}
	invV2 := []ch.InventoryMovementRow{mv("IN", inA, 120.5), mv("OUT", outA, 30.25), mv("ADJUST", adj, 9999)}
	for _, batch := range []struct {
		rows []ch.InventoryMovementRow
		at   time.Time
	}{{invV1, v1}, {invV2, v2}} {
		if err := chClient.InsertInventoryMovements(ctx, batch.rows, batch.at); err != nil {
			t.Fatal(err)
		}
		if err := lake.WriteJSONLines(ctx, inventorySourceTable, batch.rows, batch.at); err != nil {
			t.Fatal(err)
		}
	}

	res := lake.BuildAll(ctx, nil, true)
	for _, e := range res.Errors {
		t.Errorf("BuildAll: %s", e)
	}

	// --- finance ---
	wantFin, err := chClient.MonthlyFinanceSummary(ctx, companyID)
	if err != nil {
		t.Fatal(err)
	}
	gotFin := readGold[datalake.FinanceMonthlyRow](t, lake, datalake.GoldFinanceMonthly, companyID)
	if len(wantFin) != 1 || len(gotFin) != 1 {
		t.Fatalf("finance months: clickhouse=%d gold=%d, want 1 each", len(wantFin), len(gotFin))
	}
	if !gotFin[0].Revenue.Equal(wantFin[0].Revenue) || !gotFin[0].Expense.Equal(wantFin[0].Expense) {
		t.Errorf("finance gold revenue/expense = %s/%s, clickhouse = %s/%s",
			gotFin[0].Revenue, gotFin[0].Expense, wantFin[0].Revenue, wantFin[0].Expense)
	}
	if !gotFin[0].Revenue.Equal(decimal.RequireFromString("250.25")) || !gotFin[0].Expense.Equal(decimal.RequireFromString("80.10")) {
		t.Errorf("finance gold = %s/%s, want 250.25/80.10 (DRAFT 9999 must not count)", gotFin[0].Revenue, gotFin[0].Expense)
	}
	if gotFin[0].Month != wantFin[0].Month {
		t.Errorf("finance month gold=%s clickhouse=%s", gotFin[0].Month, wantFin[0].Month)
	}

	// --- sales ---
	wantSales, err := chClient.MonthlySalesSummary(ctx, companyID)
	if err != nil {
		t.Fatal(err)
	}
	gotSales := readGold[datalake.SalesMonthlyRow](t, lake, datalake.GoldSalesMonthly, companyID)
	if len(wantSales) != 1 || len(gotSales) != 1 {
		t.Fatalf("sales months: clickhouse=%d gold=%d, want 1 each", len(wantSales), len(gotSales))
	}
	if !gotSales[0].SalesValue.Equal(wantSales[0].SalesValue) {
		t.Errorf("sales gold = %s, clickhouse = %s", gotSales[0].SalesValue, wantSales[0].SalesValue)
	}
	if !gotSales[0].SalesValue.Equal(decimal.RequireFromString("400.40")) {
		t.Errorf("sales gold = %s, want 400.40 (CANCELLED and DRAFT must not count)", gotSales[0].SalesValue)
	}

	// --- inventory ---
	wantInv, err := chClient.MonthlyStockMovementSummary(ctx, companyID)
	if err != nil {
		t.Fatal(err)
	}
	gotInv := readGold[datalake.InventoryMonthlyRow](t, lake, datalake.GoldInventoryMonthly, companyID)
	if len(wantInv) != 1 || len(gotInv) != 1 {
		t.Fatalf("inventory months: clickhouse=%d gold=%d, want 1 each", len(wantInv), len(gotInv))
	}
	if !gotInv[0].StockIn.Equal(wantInv[0].StockIn) || !gotInv[0].StockOut.Equal(wantInv[0].StockOut) {
		t.Errorf("inventory gold in/out = %s/%s, clickhouse = %s/%s",
			gotInv[0].StockIn, gotInv[0].StockOut, wantInv[0].StockIn, wantInv[0].StockOut)
	}
	if !gotInv[0].StockIn.Equal(decimal.RequireFromString("120.5")) || !gotInv[0].StockOut.Equal(decimal.RequireFromString("30.25")) {
		t.Errorf("inventory gold = %s/%s, want 120.5/30.25 (corrected version wins, ADJUST must not count)", gotInv[0].StockIn, gotInv[0].StockOut)
	}
	if gotInv[0].Month != wantInv[0].Month {
		t.Errorf("inventory month gold=%s clickhouse=%s", gotInv[0].Month, wantInv[0].Month)
	}
}

func readGold[T any](t *testing.T, lake *datalake.Client, dataset string, companyID uuid.UUID) []T {
	t.Helper()
	lines, err := lake.ReadGold(context.Background(), dataset)
	if err != nil {
		t.Fatal(err)
	}
	var out []T
	for _, l := range lines {
		var probe struct {
			CompanyID string `json:"company_id"`
		}
		if err := json.Unmarshal(l, &probe); err != nil {
			t.Fatal(err)
		}
		if probe.CompanyID != companyID.String() {
			continue
		}
		var row T
		if err := json.Unmarshal(l, &row); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	return out
}

// TestGold_DowntimeAgreesWithClickHouse: Gold per (bulan, alasan) harus sama
// dengan Pareto ClickHouse untuk rentang bulan itu, termasuk catatan yang
// dikoreksi sesudah ditulis pertama kali.
func TestGold_DowntimeAgreesWithClickHouse(t *testing.T) {
	ctx := context.Background()
	lake := testLake(t)
	companyID, other := uuid.New(), uuid.New()
	v1, v2 := time.Now().Add(-time.Hour), time.Now()

	row := func(c uuid.UUID, id uuid.UUID, date, reason string, minutes int32) ch.ProductionDowntimeRow {
		r := downtimeRow(c, date, reason, minutes, t)
		r.DowntimeID = id
		return r
	}
	fixed := uuid.New()
	batches := []struct {
		rows []ch.ProductionDowntimeRow
		at   time.Time
	}{
		{[]ch.ProductionDowntimeRow{row(companyID, fixed, "2026-08-03", "BREAKDOWN", 500)}, v1},
		{[]ch.ProductionDowntimeRow{
			row(companyID, fixed, "2026-08-03", "BREAKDOWN", 40), // dikoreksi dari 500
			row(companyID, uuid.New(), "2026-08-20", "BREAKDOWN", 20),
			row(companyID, uuid.New(), "2026-08-04", "SETUP", 30),
			row(companyID, uuid.New(), "2026-09-02", "OTHER", 10),
			row(other, uuid.New(), "2026-08-03", "NO_OPERATOR", 999),
		}, v2},
	}
	for _, b := range batches {
		if err := chClient.InsertProductionDowntime(ctx, b.rows, b.at); err != nil {
			t.Fatal(err)
		}
		if err := lake.WriteJSONLines(ctx, productionDowntimeSourceTable, b.rows, b.at); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range lake.BuildAll(ctx, nil, true).Errors {
		t.Errorf("BuildAll: %s", e)
	}

	got := readGold[datalake.DowntimeMonthlyRow](t, lake, datalake.GoldDowntimeMonthly, companyID)
	if len(got) != 3 {
		t.Fatalf("downtime gold rows = %d, want 3 (Aug BREAKDOWN, Aug SETUP, Sep OTHER): %+v", len(got), got)
	}
	for _, m := range []struct{ from, to, month string }{
		{"2026-08-01", "2026-08-31", "2026-08-01"},
		{"2026-09-01", "2026-09-30", "2026-09-01"},
	} {
		from, to := mustDate(t, m.from), mustDate(t, m.to)
		want, err := chClient.ProductionDowntimePareto(ctx, companyID, &from, &to)
		if err != nil {
			t.Fatal(err)
		}
		gotBy := map[string]int64{}
		for _, g := range got {
			if g.Month == m.month {
				gotBy[g.ReasonCode] = g.Minutes
			}
		}
		if len(gotBy) != len(want) {
			t.Errorf("%s: gold %d alasan, clickhouse %d", m.month, len(gotBy), len(want))
		}
		for _, w := range want {
			if gotBy[w.ReasonCode] != w.Minutes {
				t.Errorf("%s %s: gold %d menit, clickhouse %d", m.month, w.ReasonCode, gotBy[w.ReasonCode], w.Minutes)
			}
		}
	}
	for _, g := range got {
		if g.Month == "2026-08-01" && g.ReasonCode == "BREAKDOWN" && g.Minutes != 60 {
			t.Errorf("Aug BREAKDOWN = %d, want 60 (40 dikoreksi + 20)", g.Minutes)
		}
	}
}

// TestGold_OEEAgreesWithClickHouse: tiga bulan yang masing-masing menguji satu
// cabang rumus (normal, Performance dipotong, tanpa unit => null), ditambah run
// OPEN yang tidak boleh ikut dan run yang dikoreksi sesudah ditulis pertama.
func TestGold_OEEAgreesWithClickHouse(t *testing.T) {
	ctx := context.Background()
	lake := testLake(t)
	companyID := uuid.New()
	v1, v2 := time.Now().Add(-time.Hour), time.Now()

	fixed := oeeRow(companyID, "2026-08-03", "CLOSED", 420, 60, 150, 10, 2.0, t)
	stale := fixed
	stale.PlannedMinutes, stale.QuantityGood = 9999, 1 // versi lama, harus tertimpa
	batches := []struct {
		rows []ch.ProductionOEERow
		at   time.Time
	}{
		{[]ch.ProductionOEERow{stale}, v1},
		{[]ch.ProductionOEERow{
			fixed,
			oeeRow(companyID, "2026-08-04", "OPEN", 480, 0, 0, 0, 2.0, t),
			oeeRow(companyID, "2026-09-02", "CLOSED", 100, 0, 100, 0, 2.0, t), // ideal 200 > run 100
			oeeRow(companyID, "2026-10-05", "CLOSED", 300, 30, 0, 0, 2.0, t),  // tanpa unit
			oeeRow(uuid.New(), "2026-08-03", "CLOSED", 111, 0, 5, 5, 1.0, t),  // company lain
		}, v2},
	}
	for _, b := range batches {
		if err := chClient.InsertProductionOEE(ctx, b.rows, b.at); err != nil {
			t.Fatal(err)
		}
		if err := lake.WriteJSONLines(ctx, productionOEESourceTable, b.rows, b.at); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range lake.BuildAll(ctx, nil, true).Errors {
		t.Errorf("BuildAll: %s", e)
	}

	want, err := chClient.ProductionOEEMonthlySummary(ctx, companyID)
	if err != nil {
		t.Fatal(err)
	}
	got := readGold[datalake.OEEMonthlyRow](t, lake, datalake.GoldOEEMonthly, companyID)
	if len(want) != 3 || len(got) != 3 {
		t.Fatalf("oee months: clickhouse=%d gold=%d, want 3 each", len(want), len(got))
	}
	same := func(label string, g, w *float64) {
		t.Helper()
		switch {
		case g == nil && w == nil:
		case g == nil || w == nil:
			t.Errorf("%s gold=%v clickhouse=%v (nil harus sama-sama nil)", label, g, w)
		case math.Abs(*g-*w) > 0.011:
			t.Errorf("%s gold=%.2f clickhouse=%.2f", label, *g, *w)
		}
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Month != w.Month || uint64(g.RunCount) != w.RunCount ||
			g.PlannedMinutes != w.PlannedMinutes || g.DowntimeMinutes != w.DowntimeMinutes || g.RunTimeMinutes != w.RunTimeMinutes {
			t.Errorf("%s: gold %+v != clickhouse %+v", w.Month, g, w)
		}
		same(w.Month+" availability", g.AvailabilityPct, w.AvailabilityPct)
		same(w.Month+" performance", g.PerformancePct, w.PerformancePct)
		same(w.Month+" quality", g.QualityPct, w.QualityPct)
		same(w.Month+" oee", g.OEEPct, w.OEEPct)
		if g.PerformanceCapped != w.PerformanceCapped {
			t.Errorf("%s capped gold=%v clickhouse=%v", w.Month, g.PerformanceCapped, w.PerformanceCapped)
		}
	}
	if got[0].OEEPct == nil || *got[0].OEEPct != 71.43 {
		t.Errorf("Aug OEE = %v, want 71.43", got[0].OEEPct)
	}
	if !got[1].PerformanceCapped || got[2].OEEPct != nil {
		t.Errorf("Sep capped=%v (want true), Oct OEE=%v (want nil)", got[1].PerformanceCapped, got[2].OEEPct)
	}
}
