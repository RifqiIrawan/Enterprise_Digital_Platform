package etl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func silverHasLine(t *testing.T, lineID uuid.UUID) (found bool, total int) {
	t.Helper()
	lines, err := testLake(t).ReadSilver(context.Background(), financeSourceTable)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		var r struct{ LineID uuid.UUID }
		if err := json.Unmarshal(l, &r); err != nil {
			t.Fatal(err)
		}
		if r.LineID == lineID {
			found = true
		}
	}
	return found, len(lines)
}

// TestBackfill_ClosesRowsThatPredateTheLake reproduces, with the real ETL, the
// gap found on the dev stack: a row copied to ClickHouse while no lake was
// attached is never read again, because the watermark has moved past it. A
// normal sync cannot fix that; Backfill must, without moving the watermark.
func TestBackfill_ClosesRowsThatPredateTheLake(t *testing.T) {
	ctx := context.Background()
	lake := testLake(t)

	companyID := uuid.New()
	lineID, _ := mustSeedJournalEntryWithLine(t, companyID, "POSTED")

	// Synced with no lake attached: ClickHouse has the row, Bronze never will.
	if _, err := SyncFinance(ctx, sourcePool, chClient, nil); err != nil {
		t.Fatalf("SyncFinance without lake: %v", err)
	}
	// The extract is ">= watermark", so the NEWEST row is re-read on every sync
	// (which is why Bronze accumulates duplicates). The gap therefore only hits
	// rows strictly below the watermark: push it past our line with a newer one,
	// still without a lake. The watermark is stored as a ClickHouse DateTime
	// (whole seconds), so everything inside its second is re-read too: the newer
	// row has to land at least a full second later.
	time.Sleep(1100 * time.Millisecond)
	mustSeedJournalEntryWithLine(t, companyID, "POSTED")
	if _, err := SyncFinance(ctx, sourcePool, chClient, nil); err != nil {
		t.Fatalf("second SyncFinance without lake: %v", err)
	}
	// Now a sync WITH the lake attached still does not write our line.
	if _, err := SyncFinance(ctx, sourcePool, chClient, lake); err != nil {
		t.Fatalf("SyncFinance with lake: %v", err)
	}
	if _, err := lake.BuildSilver(ctx, financeSourceTable, nil); err != nil {
		t.Fatal(err)
	}
	if found, _ := silverHasLine(t, lineID); found {
		t.Fatal("premise broken: the line is already in Silver before any backfill")
	}

	wmBefore, err := chClient.GetWatermark(ctx, financeSourceTable)
	if err != nil {
		t.Fatal(err)
	}

	n, err := BackfillFinance(ctx, sourcePool, lake)
	if err != nil {
		t.Fatalf("BackfillFinance: %v", err)
	}
	if n < 1 {
		t.Fatalf("BackfillFinance wrote %d rows, want at least the seeded one", n)
	}
	if _, err := lake.BuildSilver(ctx, financeSourceTable, nil); err != nil {
		t.Fatal(err)
	}
	found, total := silverHasLine(t, lineID)
	if !found {
		t.Fatal("line still missing from Silver after backfill")
	}

	// Backfill only writes to the lake: it must not move the ClickHouse watermark.
	wmAfter, err := chClient.GetWatermark(ctx, financeSourceTable)
	if err != nil {
		t.Fatal(err)
	}
	if !wmAfter.Equal(wmBefore) {
		t.Errorf("backfill moved the watermark: %s -> %s", wmBefore, wmAfter)
	}

	// Running it again overlaps the first Bronze file; Silver must not grow.
	if _, err := BackfillFinance(ctx, sourcePool, lake); err != nil {
		t.Fatal(err)
	}
	if _, err := lake.BuildSilver(ctx, financeSourceTable, nil); err != nil {
		t.Fatal(err)
	}
	if _, total2 := silverHasLine(t, lineID); total2 != total {
		t.Errorf("Silver grew from %d to %d rows on a repeated backfill", total, total2)
	}

}

func TestBackfill_NeedsALake(t *testing.T) {
	if _, err := BackfillFinance(context.Background(), sourcePool, nil); err == nil {
		t.Fatal("backfill with no lake must be an error: writing to the lake is its whole purpose")
	}
}

// TestFactsRegistryIsConsistent keeps the one list every consumer reads in
// step with Silver's list of key fields.
func TestFactsRegistryIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range Facts {
		if seen[f.Name] {
			t.Errorf("fact %q listed twice", f.Name)
		}
		seen[f.Name] = true
		if f.Table == "" || f.Source == nil || f.Backfill == nil {
			t.Errorf("fact %q has an incomplete registry entry", f.Name)
		}
	}
}

// TestSilver_PrunesRowsDeletedFromPostgres is the dev-stack case (hr_kpi_reviews:
// lake 2, Postgres 0) reproduced with the real ETL and the real source database.
// It also proves the other half, which is what a key-format mismatch would
// break: a row that still exists must SURVIVE pruning.
func TestSilver_PrunesRowsDeletedFromPostgres(t *testing.T) {
	ctx := context.Background()
	lake := testLake(t)
	live := func(ctx context.Context) (map[string]struct{}, error) {
		return LiveKeysFinance(ctx, sourcePool)
	}

	keep, _ := mustSeedJournalEntryWithLine(t, uuid.New(), "POSTED")
	gone, _ := mustSeedJournalEntryWithLine(t, uuid.New(), "POSTED")
	if _, err := SyncFinance(ctx, sourcePool, chClient, lake); err != nil {
		t.Fatal(err)
	}

	st, err := lake.BuildSilver(ctx, financeSourceTable, live)
	if err != nil {
		t.Fatal(err)
	}
	if found, _ := silverHasLine(t, keep); !found {
		t.Fatal("a row that still exists at the source was pruned: the live-key format differs from Silver's")
	}
	if found, _ := silverHasLine(t, gone); !found {
		t.Fatal("premise broken: the row is missing from Silver before it was deleted")
	}

	if _, err := sourcePool.Exec(ctx, `DELETE FROM journal_lines WHERE id = $1`, gone); err != nil {
		t.Fatal(err)
	}
	st, err = lake.BuildSilver(ctx, financeSourceTable, live)
	if err != nil {
		t.Fatal(err)
	}
	if st.Pruned < 1 {
		t.Errorf("Pruned = %d, want at least the deleted line", st.Pruned)
	}
	if st.BronzeRows != st.SilverRows+st.DuplicatesDropped+st.Rejected+st.Pruned {
		t.Errorf("invariant broken: %+v", st)
	}
	if found, _ := silverHasLine(t, gone); found {
		t.Error("the deleted line is still in Silver")
	}
	if found, _ := silverHasLine(t, keep); !found {
		t.Error("the surviving line was lost")
	}

	// Bronze keeps the history: an unpruned rebuild brings the row back.
	if _, err := lake.BuildSilver(ctx, financeSourceTable, nil); err != nil {
		t.Fatal(err)
	}
	if found, _ := silverHasLine(t, gone); !found {
		t.Error("Bronze must still hold the deleted row as history")
	}
}
