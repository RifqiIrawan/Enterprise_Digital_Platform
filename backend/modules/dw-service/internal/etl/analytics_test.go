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
