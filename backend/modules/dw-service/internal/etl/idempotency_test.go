package etl

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestSyncFinance_RerunIsIdempotent proves the ReplacingMergeTree design
// actually works, not just that data lands once: the watermark is
// inclusive (">="), so a rerun with no new data still re-processes the
// last-seen boundary row -- FINAL must still report exactly one row per
// line_id, not two, after two Sync calls in a row.
func TestSyncFinance_RerunIsIdempotent(t *testing.T) {
	companyID := uuid.New()
	lineID, _ := mustSeedJournalEntryWithLine(t, companyID, "POSTED")

	if _, err := SyncFinance(context.Background(), sourcePool, chClient, nil); err != nil {
		t.Fatalf("first SyncFinance: %v", err)
	}
	if _, err := SyncFinance(context.Background(), sourcePool, chClient, nil); err != nil {
		t.Fatalf("second SyncFinance: %v", err)
	}

	var count uint64
	row := chClient.QueryRow(context.Background(),
		"SELECT count(*) FROM fact_finance_journal_lines FINAL WHERE line_id = ?", lineID)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count synced rows: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 row for line_id %s after 2 syncs, got %d (ReplacingMergeTree dedup not working as expected)", lineID, count)
	}
}
