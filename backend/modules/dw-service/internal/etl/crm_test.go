package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func mustSeedOpportunity(t *testing.T, companyID uuid.UUID) (opportunityID uuid.UUID, accountName string) {
	t.Helper()
	accountName = "Test Account " + uuid.NewString()[:8]
	var accountID uuid.UUID
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO accounts (account_code, name, account_type) VALUES ($1, $2, 'CUSTOMER') RETURNING id`,
		"ACC-"+uuid.NewString()[:8], accountName,
	).Scan(&accountID)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO opportunities (company_id, opportunity_number, account_id, name, stage, amount, probability)
		VALUES ($1, $2, $3, 'Test Deal', 'NEGOTIATION', 5000000, 60)
		RETURNING id`,
		companyID, "OPP-"+uuid.NewString()[:8], accountID,
	).Scan(&opportunityID)
	if err != nil {
		t.Fatalf("seed opportunity: %v", err)
	}
	return opportunityID, accountName
}

func TestSyncCRM_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	opportunityID, accountName := mustSeedOpportunity(t, companyID)

	n, err := SyncCRM(context.Background(), sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncCRM: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotAccountName, gotStage string
	var gotAmount decimal.Decimal
	row := chClient.QueryRow(context.Background(),
		"SELECT account_name, stage, amount FROM fact_crm_opportunities FINAL WHERE opportunity_id = ?", opportunityID)
	if err := row.Scan(&gotAccountName, &gotStage, &gotAmount); err != nil {
		t.Fatalf("query synced crm row: %v", err)
	}
	if gotAccountName != accountName {
		t.Errorf("account_name = %q, want %q", gotAccountName, accountName)
	}
	if gotStage != "NEGOTIATION" {
		t.Errorf("stage = %q, want NEGOTIATION", gotStage)
	}
	if !gotAmount.Equal(decimal.NewFromInt(5000000)) {
		t.Errorf("amount = %v, want 5000000", gotAmount)
	}
}
