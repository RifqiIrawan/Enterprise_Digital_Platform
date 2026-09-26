package etl

import (
	"context"
	"encoding/json"
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
		productionOEESourceTable, projectSourceTable, purchasingSourceTable,
		qcSourceTable, salesSourceTable, ticketingSourceTable,
	} {
		if _, ok := datalake.SilverFacts[f]; !ok {
			t.Errorf("fact ETL %q belum terdaftar di datalake.SilverFacts", f)
		}
	}
	if len(datalake.SilverFacts) != 17 {
		t.Errorf("SilverFacts punya %d fact, ETL punya 17", len(datalake.SilverFacts))
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

	res := lake.BuildAll(ctx)
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
