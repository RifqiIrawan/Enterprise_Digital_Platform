package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func mustSeedTicket(t *testing.T, companyID uuid.UUID) (ticketID uuid.UUID, categoryName string) {
	t.Helper()
	categoryName = "Test Category " + uuid.NewString()[:8]
	var categoryID uuid.UUID
	err := sourcePool.QueryRow(context.Background(),
		`INSERT INTO ticket_categories (category_code, name) VALUES ($1, $2) RETURNING id`,
		"CAT-"+uuid.NewString()[:8], categoryName,
	).Scan(&categoryID)
	if err != nil {
		t.Fatalf("seed ticket category: %v", err)
	}

	err = sourcePool.QueryRow(context.Background(), `
		INSERT INTO tickets (company_id, ticket_number, category_id, subject, priority, status, requester_name)
		VALUES ($1, $2, $3, 'Login gagal', 'HIGH', 'OPEN', 'Budi')
		RETURNING id`,
		companyID, "TICKET-"+uuid.NewString()[:8], categoryID,
	).Scan(&ticketID)
	if err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	return ticketID, categoryName
}

func TestSyncTicketing_ExtractsAndLoads(t *testing.T) {
	companyID := uuid.New()
	ticketID, categoryName := mustSeedTicket(t, companyID)

	n, err := SyncTicketing(context.Background(), sourcePool, chClient, nil)
	if err != nil {
		t.Fatalf("SyncTicketing: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 row synced, got %d", n)
	}

	var gotCategoryName, gotPriority, gotStatus string
	row := chClient.QueryRow(context.Background(),
		"SELECT category_name, priority, status FROM fact_ticketing_tickets FINAL WHERE ticket_id = ?", ticketID)
	if err := row.Scan(&gotCategoryName, &gotPriority, &gotStatus); err != nil {
		t.Fatalf("query synced ticketing row: %v", err)
	}
	if gotCategoryName != categoryName {
		t.Errorf("category_name = %q, want %q", gotCategoryName, categoryName)
	}
	if gotPriority != "HIGH" {
		t.Errorf("priority = %q, want HIGH", gotPriority)
	}
	if gotStatus != "OPEN" {
		t.Errorf("status = %q, want OPEN", gotStatus)
	}
}
