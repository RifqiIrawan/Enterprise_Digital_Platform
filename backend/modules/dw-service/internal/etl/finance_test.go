package etl

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func mustSeedAccount(t *testing.T) (id uuid.UUID, code, accType string) {
	t.Helper()
	code = "ACC-" + uuid.NewString()[:8]
	accType = "EXPENSE"
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO accounts (account_code, account_name, account_type) VALUES ($1, $2, $3) RETURNING id`,
		code, "Test Account "+code, accType,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return id, code, accType
}

func mustSeedJournalEntryWithLine(t *testing.T, companyID uuid.UUID, status string) (lineID uuid.UUID, accountCode string) {
	t.Helper()
	accountID, code, _ := mustSeedAccount(t)

	var journalID uuid.UUID
	var postedAt *time.Time
	if status == "POSTED" {
		now := time.Now()
		postedAt = &now
	}
	err := sourcePool.QueryRow(context.Background(), `
		INSERT INTO journal_entries (company_id, entry_number, entry_date, period, status, posted_at)
		VALUES ($1, $2, CURRENT_DATE, '2026-07', $3, $4)
		RETURNING id`,
		companyID, "JE-"+uuid.NewString()[:8], status, postedAt,
	).Scan(&journalID)
	if err != nil {
		t.Fatalf("seed journal entry: %v", err)
	}

	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO journal_lines (journal_id, account_id, debit_amount, credit_amount)
		VALUES ($1, $2, 100.50, 0)
		RETURNING id`,
		journalID, accountID,
	).Scan(&lineID)
	if err != nil {
		t.Fatalf("seed journal line: %v", err)
	}
	return lineID, code
}

func TestSyncFinance_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	lineID, accountCode := mustSeedJournalEntryWithLine(t, companyID, "POSTED")

	n, err := SyncFinance(context.Background(), sourcePool, chClient)
	if err != nil {
		t.Fatalf("SyncFinance: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotCompanyID, gotAccountCode string
	var gotDebit decimal.Decimal
	row := chClient.QueryRow(context.Background(),
		"SELECT company_id, account_code, debit_amount FROM fact_finance_journal_lines FINAL WHERE line_id = ?", lineID)
	if err := row.Scan(&gotCompanyID, &gotAccountCode, &gotDebit); err != nil {
		t.Fatalf("query synced finance row: %v", err)
	}
	if gotCompanyID != companyID.String() {
		t.Errorf("company_id = %q, want %q", gotCompanyID, companyID.String())
	}
	if gotAccountCode != accountCode {
		t.Errorf("account_code = %q, want %q", gotAccountCode, accountCode)
	}
	if !gotDebit.Equal(decimal.NewFromFloat(100.50)) {
		t.Errorf("debit_amount = %v, want 100.50", gotDebit)
	}
}

// TestSyncFinance_DraftEntriesExcludedFromWatermarkButStillSynced confirms a
// DRAFT entry (no posted_at) still gets synced using created_at as the
// watermark fallback -- the warehouse should reflect current state
// (including DRAFT) rather than only POSTED entries, per the plan's design
// decision to sync all statuses.
func TestSyncFinance_DraftEntrySynced(t *testing.T) {
	companyID := uuid.New()
	lineID, _ := mustSeedJournalEntryWithLine(t, companyID, "DRAFT")

	if _, err := SyncFinance(context.Background(), sourcePool, chClient); err != nil {
		t.Fatalf("SyncFinance: %v", err)
	}

	var status string
	row := chClient.QueryRow(context.Background(),
		"SELECT entry_status FROM fact_finance_journal_lines FINAL WHERE line_id = ?", lineID)
	if err := row.Scan(&status); err != nil {
		t.Fatalf("query synced draft row: %v", err)
	}
	if status != "DRAFT" {
		t.Errorf("entry_status = %q, want DRAFT", status)
	}
}
