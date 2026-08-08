package etl

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
)

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("decimal.NewFromString(%q): %v", s, err)
	}
	return d
}

// TestMonthlyFinanceSummary_AggregatesRevenueAndExpense menguji query
// analitik ClickHouse langsung (bukan lewat SyncFinance) dengan angka bersih
// yang bisa dihitung tangan -- pola yang sama dengan dataset regresi
// linear/z-score di ai-bi-service: hasil agregasi HARUS persis, bukan cuma
// "ada hasil".
func TestMonthlyFinanceSummary_AggregatesRevenueAndExpense(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	entryDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	syncedAt := time.Now()

	rows := []ch.FinanceJournalLineRow{
		// REVENUE, POSTED -- dihitung (2 baris, total 300).
		{
			LineID: uuid.New(), JournalID: uuid.New(), CompanyID: companyID,
			EntryNumber: "JE-TEST-1", EntryDate: entryDate, Period: "2026-06",
			ReferenceType: "MANUAL", EntryStatus: "POSTED",
			AccountID: uuid.New(), AccountCode: "4000", AccountName: "Revenue",
			AccountType: "REVENUE", DebitAmount: 0, CreditAmount: 100,
		},
		{
			LineID: uuid.New(), JournalID: uuid.New(), CompanyID: companyID,
			EntryNumber: "JE-TEST-1", EntryDate: entryDate, Period: "2026-06",
			ReferenceType: "MANUAL", EntryStatus: "POSTED",
			AccountID: uuid.New(), AccountCode: "4000", AccountName: "Revenue",
			AccountType: "REVENUE", DebitAmount: 0, CreditAmount: 200,
		},
		// EXPENSE, POSTED -- dihitung (1 baris, total 80).
		{
			LineID: uuid.New(), JournalID: uuid.New(), CompanyID: companyID,
			EntryNumber: "JE-TEST-1", EntryDate: entryDate, Period: "2026-06",
			ReferenceType: "MANUAL", EntryStatus: "POSTED",
			AccountID: uuid.New(), AccountCode: "5000", AccountName: "Expense",
			AccountType: "EXPENSE", DebitAmount: 80, CreditAmount: 0,
		},
		// ASSET, POSTED -- TIDAK dihitung ke revenue maupun expense.
		{
			LineID: uuid.New(), JournalID: uuid.New(), CompanyID: companyID,
			EntryNumber: "JE-TEST-1", EntryDate: entryDate, Period: "2026-06",
			ReferenceType: "MANUAL", EntryStatus: "POSTED",
			AccountID: uuid.New(), AccountCode: "1000", AccountName: "Cash",
			AccountType: "ASSET", DebitAmount: 220, CreditAmount: 0,
		},
		// REVENUE, DRAFT -- TIDAK dihitung karena belum POSTED.
		{
			LineID: uuid.New(), JournalID: uuid.New(), CompanyID: companyID,
			EntryNumber: "JE-TEST-2", EntryDate: entryDate, Period: "2026-06",
			ReferenceType: "MANUAL", EntryStatus: "DRAFT",
			AccountID: uuid.New(), AccountCode: "4000", AccountName: "Revenue",
			AccountType: "REVENUE", DebitAmount: 0, CreditAmount: 9999,
		},
	}

	if err := chClient.InsertFinanceJournalLines(ctx, rows, syncedAt); err != nil {
		t.Fatalf("InsertFinanceJournalLines: %v", err)
	}

	summary, err := chClient.MonthlyFinanceSummary(ctx, companyID)
	if err != nil {
		t.Fatalf("MonthlyFinanceSummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected exactly 1 month in summary, got %d: %+v", len(summary), summary)
	}

	got := summary[0]
	if got.Month != "2026-06-01" {
		t.Errorf("month = %q, want 2026-06-01", got.Month)
	}
	if !got.Revenue.Equal(mustDecimal(t, "300")) {
		t.Errorf("revenue = %s, want 300 (DRAFT row must be excluded)", got.Revenue)
	}
	if !got.Expense.Equal(mustDecimal(t, "80")) {
		t.Errorf("expense = %s, want 80", got.Expense)
	}
}

// TestMonthlyFinanceSummary_NoDataReturnsEmpty memverifikasi company tanpa
// journal line sama sekali mengembalikan slice kosong, bukan error atau baris
// dengan nilai nol -- konsisten dengan pola "company baru, belum ada data"
// yang dipakai di ai-bi-service.
func TestMonthlyFinanceSummary_NoDataReturnsEmpty(t *testing.T) {
	summary, err := chClient.MonthlyFinanceSummary(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("MonthlyFinanceSummary: %v", err)
	}
	if len(summary) != 0 {
		t.Errorf("expected empty summary for company with no data, got %+v", summary)
	}
}

// TestMonthlyFinanceSummary_DualWriteDoesNotDoubleCount adalah test regresi
// UNTUK Materialized View (mv_finance_monthly_line_state, lihat komentar
// MonthlyFinanceSummary di clickhouse.go) -- dw-service dual-write ke
// fact_finance_journal_lines lewat DUA jalur terpisah yang benar-benar
// berjalan sebagai proses/waktu berbeda: batch ETL (internal/etl, tiap 5
// menit) dan Kafka Streaming ETL (internal/streaming, event-triggered). Line
// yang sama bisa masuk dua kali dengan synced_at berbeda. MV memproses
// setiap event INSERT independen (bukan menunggu ReplacingMergeTree merge di
// background) -- kalau desainnya SummingMergeTree/sum() naif per baris,
// dual-write ini akan membuat revenue-nya terhitung DOBEL. Test ini meniru
// itu secara eksplisit: DUA panggilan InsertFinanceJournalLines TERPISAH
// (bukan satu panggilan dengan slice 2 baris -- itu akan diproses MV sebagai
// SATU block dan ke-GROUP BY sebelum sempat jadi bug) untuk line_id yang
// SAMA, synced_at kedua LEBIH BARU dari yang pertama.
func TestMonthlyFinanceSummary_DualWriteDoesNotDoubleCount(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	lineID := uuid.New()
	entryDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	firstSync := time.Now()
	secondSync := firstSync.Add(5 * time.Second)

	row := ch.FinanceJournalLineRow{
		LineID: lineID, JournalID: uuid.New(), CompanyID: companyID,
		EntryNumber: "JE-DUALWRITE", EntryDate: entryDate, Period: "2026-06",
		ReferenceType: "MANUAL", EntryStatus: "POSTED",
		AccountID: uuid.New(), AccountCode: "4000", AccountName: "Revenue",
		AccountType: "REVENUE", DebitAmount: 0, CreditAmount: 500,
	}

	// Jalur 1: seolah batch ETL mensync line ini.
	if err := chClient.InsertFinanceJournalLines(ctx, []ch.FinanceJournalLineRow{row}, firstSync); err != nil {
		t.Fatalf("InsertFinanceJournalLines (batch): %v", err)
	}
	// Jalur 2: seolah streaming ETL mensync line yang SAMA lagi (mis. dipicu
	// event finance.journal.posted yang tiba nyaris bersamaan), synced_at
	// lebih baru -- INSERT terpisah, bukan bagian dari slice yang sama.
	if err := chClient.InsertFinanceJournalLines(ctx, []ch.FinanceJournalLineRow{row}, secondSync); err != nil {
		t.Fatalf("InsertFinanceJournalLines (streaming): %v", err)
	}

	summary, err := chClient.MonthlyFinanceSummary(ctx, companyID)
	if err != nil {
		t.Fatalf("MonthlyFinanceSummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected exactly 1 month, got %d: %+v", len(summary), summary)
	}
	got := summary[0]
	if !got.Revenue.Equal(mustDecimal(t, "500")) {
		t.Errorf("revenue = %s, want 500 (dual-write of the same line_id must NOT be double-counted to 1000)", got.Revenue)
	}
	if !got.Expense.Equal(mustDecimal(t, "0")) {
		t.Errorf("expense = %s, want 0", got.Expense)
	}
}

// TestMonthlyStockMovementSummary_AggregatesInAndOut menguji query analitik
// kedua di dw-service (setelah MonthlyFinanceSummary) dengan pola yang sama:
// angka bersih yang bisa dihitung tangan, bukan cuma "ada hasil".
func TestMonthlyStockMovementSummary_AggregatesInAndOut(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	movementDate := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	syncedAt := time.Now()

	rows := []ch.InventoryMovementRow{
		// IN -- dihitung (2 baris, total 150).
		{
			MovementID: uuid.New(), CompanyID: companyID, WarehouseID: uuid.New(),
			WarehouseCode: "WH1", WarehouseName: "Gudang Utama", ProductID: uuid.New(),
			ProductSKU: "SKU-1", ProductName: "Produk 1", MovementType: "IN",
			Quantity: 100, ReferenceType: "PURCHASE_ORDER", MovementDate: movementDate,
		},
		{
			MovementID: uuid.New(), CompanyID: companyID, WarehouseID: uuid.New(),
			WarehouseCode: "WH1", WarehouseName: "Gudang Utama", ProductID: uuid.New(),
			ProductSKU: "SKU-2", ProductName: "Produk 2", MovementType: "IN",
			Quantity: 50, ReferenceType: "STOCK_TRANSFER", MovementDate: movementDate,
		},
		// OUT -- dihitung (1 baris, total 30).
		{
			MovementID: uuid.New(), CompanyID: companyID, WarehouseID: uuid.New(),
			WarehouseCode: "WH1", WarehouseName: "Gudang Utama", ProductID: uuid.New(),
			ProductSKU: "SKU-1", ProductName: "Produk 1", MovementType: "OUT",
			Quantity: 30, ReferenceType: "SALES_ORDER", MovementDate: movementDate,
		},
	}

	if err := chClient.InsertInventoryMovements(ctx, rows, syncedAt); err != nil {
		t.Fatalf("InsertInventoryMovements: %v", err)
	}

	summary, err := chClient.MonthlyStockMovementSummary(ctx, companyID)
	if err != nil {
		t.Fatalf("MonthlyStockMovementSummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected exactly 1 month, got %d: %+v", len(summary), summary)
	}
	got := summary[0]
	if got.Month != "2026-06-01" {
		t.Errorf("month = %q, want 2026-06-01", got.Month)
	}
	if !got.StockIn.Equal(mustDecimal(t, "150")) {
		t.Errorf("stock_in = %s, want 150", got.StockIn)
	}
	if !got.StockOut.Equal(mustDecimal(t, "30")) {
		t.Errorf("stock_out = %s, want 30", got.StockOut)
	}
}

// TestMonthlyStockMovementSummary_NoDataReturnsEmpty memverifikasi company
// tanpa pergerakan stok sama sekali mengembalikan slice kosong, bukan error
// atau baris dengan nilai nol -- konsisten dengan MonthlyFinanceSummary.
func TestMonthlyStockMovementSummary_NoDataReturnsEmpty(t *testing.T) {
	summary, err := chClient.MonthlyStockMovementSummary(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("MonthlyStockMovementSummary: %v", err)
	}
	if len(summary) != 0 {
		t.Errorf("expected empty summary for company with no data, got %+v", summary)
	}
}

// TestMonthlySalesSummary_AggregatesCommittedOrdersOnly menguji query
// analitik ketiga di dw-service dengan pola yang sama: angka bersih yang
// bisa dihitung tangan, DRAFT dan CANCELLED sengaja disertakan di dataset
// untuk membuktikan keduanya benar-benar dikecualikan (bukan cuma diasumsikan
// dari komentar kode).
func TestMonthlySalesSummary_AggregatesCommittedOrdersOnly(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	orderDate := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	syncedAt := time.Now()

	rows := []ch.SalesOrderLineRow{
		// CONFIRMED, FULFILLED, INVOICED -- dihitung (total 100+200+300=600).
		{
			LineID: uuid.New(), SalesOrderID: uuid.New(), CompanyID: companyID,
			SONumber: "SO-1", OrderDate: orderDate, OrderStatus: "CONFIRMED",
			CustomerID: uuid.New(), CustomerCode: "CUST-1", CustomerName: "Customer 1",
			ProductName: "Produk A", Quantity: 1, UnitPrice: 100, Amount: 100, UpdatedAt: orderDate,
		},
		{
			LineID: uuid.New(), SalesOrderID: uuid.New(), CompanyID: companyID,
			SONumber: "SO-2", OrderDate: orderDate, OrderStatus: "FULFILLED",
			CustomerID: uuid.New(), CustomerCode: "CUST-1", CustomerName: "Customer 1",
			ProductName: "Produk B", Quantity: 1, UnitPrice: 200, Amount: 200, UpdatedAt: orderDate,
		},
		{
			LineID: uuid.New(), SalesOrderID: uuid.New(), CompanyID: companyID,
			SONumber: "SO-3", OrderDate: orderDate, OrderStatus: "INVOICED",
			CustomerID: uuid.New(), CustomerCode: "CUST-1", CustomerName: "Customer 1",
			ProductName: "Produk C", Quantity: 1, UnitPrice: 300, Amount: 300, UpdatedAt: orderDate,
		},
		// DRAFT -- TIDAK dihitung (belum komitmen).
		{
			LineID: uuid.New(), SalesOrderID: uuid.New(), CompanyID: companyID,
			SONumber: "SO-4", OrderDate: orderDate, OrderStatus: "DRAFT",
			CustomerID: uuid.New(), CustomerCode: "CUST-1", CustomerName: "Customer 1",
			ProductName: "Produk D", Quantity: 1, UnitPrice: 9999, Amount: 9999, UpdatedAt: orderDate,
		},
		// CANCELLED -- TIDAK dihitung (dibatalkan).
		{
			LineID: uuid.New(), SalesOrderID: uuid.New(), CompanyID: companyID,
			SONumber: "SO-5", OrderDate: orderDate, OrderStatus: "CANCELLED",
			CustomerID: uuid.New(), CustomerCode: "CUST-1", CustomerName: "Customer 1",
			ProductName: "Produk E", Quantity: 1, UnitPrice: 8888, Amount: 8888, UpdatedAt: orderDate,
		},
	}

	if err := chClient.InsertSalesOrderLines(ctx, rows, syncedAt); err != nil {
		t.Fatalf("InsertSalesOrderLines: %v", err)
	}

	summary, err := chClient.MonthlySalesSummary(ctx, companyID)
	if err != nil {
		t.Fatalf("MonthlySalesSummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected exactly 1 month, got %d: %+v", len(summary), summary)
	}
	got := summary[0]
	if got.Month != "2026-06-01" {
		t.Errorf("month = %q, want 2026-06-01", got.Month)
	}
	if !got.SalesValue.Equal(mustDecimal(t, "600")) {
		t.Errorf("sales_value = %s, want 600 (DRAFT and CANCELLED rows must be excluded)", got.SalesValue)
	}
}

// TestMonthlySalesSummary_NoDataReturnsEmpty memverifikasi company tanpa
// sales order sama sekali mengembalikan slice kosong, konsisten dengan
// endpoint analitik lain.
func TestMonthlySalesSummary_NoDataReturnsEmpty(t *testing.T) {
	summary, err := chClient.MonthlySalesSummary(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("MonthlySalesSummary: %v", err)
	}
	if len(summary) != 0 {
		t.Errorf("expected empty summary for company with no data, got %+v", summary)
	}
}
