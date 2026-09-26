package datalake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// bronzeAt writes a Bronze object named the way the ETL names it
// (<fact>/YYYY/MM/DD/<unix-nano>.jsonl), so the build can read its age.
func bronzeAt(t *testing.T, fact string, at time.Time, lines ...string) string {
	t.Helper()
	key := fmt.Sprintf("%s/%04d/%02d/%02d/%d.jsonl", fact, at.Year(), at.Month(), at.Day(), at.UnixNano())
	if err := testClient.Put(context.Background(), key, []byte(strings.Join(lines, "\n")+"\n")); err != nil {
		t.Fatal(err)
	}
	return key
}

func rowJSON(company, id, v string) string {
	return `{"LineID":"` + id + `","CompanyID":"` + company + `","V":"` + v + `"}`
}

func uid(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }

func silverBytes(t *testing.T, fact string) []byte {
	t.Helper()
	b, err := testClient.Get(context.Background(), silverKey(fact))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func checkInvariant(t *testing.T, st SilverStats) {
	t.Helper()
	if st.PreviousSilverRows+st.BronzeRows != st.SilverRows+st.DuplicatesDropped+st.Rejected+st.Pruned {
		t.Errorf("invariant broken: %+v", st)
	}
}

// The property that makes the incremental build trustworthy: after any
// sequence of Bronze objects (new rows, updates, rows repeated across
// objects), the incremental result is byte-identical to a forced full rebuild.
func TestIncrementalSilver_EqualsFullRebuildOverManyRounds(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	rng := rand.New(rand.NewSource(42))
	base := time.Now().Add(-3 * time.Hour) // every object is older than settleLag
	version := 0
	for round := 0; round < 25; round++ {
		for o := 0; o < 1+rng.Intn(3); o++ {
			var lines []string
			for r := 0; r < 1+rng.Intn(20); r++ {
				version++
				// 40 ids across two companies: plenty of updates to existing rows.
				lines = append(lines, rowJSON(uid(1+rng.Intn(2)), uid(1+rng.Intn(40)), fmt.Sprint(version)))
			}
			bronzeAt(t, fact, base, lines...)
			base = base.Add(time.Second)
		}

		inc, err := testClient.buildSilver(ctx, fact, "LineID", nil, false)
		if err != nil {
			t.Fatalf("round %d incremental: %v", round, err)
		}
		checkInvariant(t, inc)
		gotInc := append([]byte(nil), silverBytes(t, fact)...)

		full, err := testClient.buildSilver(ctx, fact, "LineID", nil, true)
		if err != nil {
			t.Fatalf("round %d full: %v", round, err)
		}
		checkInvariant(t, full)
		if !bytes.Equal(gotInc, silverBytes(t, fact)) {
			t.Fatalf("round %d: incremental Silver (%s) differs from a full rebuild: %d vs %d bytes", round, inc.Mode, len(gotInc), len(silverBytes(t, fact)))
		}
		if round > 0 && inc.Mode != "incremental" {
			t.Errorf("round %d ran in %s mode, want incremental", round, inc.Mode)
		}
	}
}

func TestIncrementalSilver_ReadsOnlyNewObjects(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	base := time.Now().Add(-3 * time.Hour)
	bronzeAt(t, fact, base, rowJSON(co1, id1, "1"), rowJSON(co1, id2, "1"))
	if st, err := testClient.buildSilver(ctx, fact, "LineID", nil, false); err != nil || st.Mode != "full" || st.SilverRows != 2 {
		t.Fatalf("first build = %+v, %v; want a full build of 2 rows", st, err)
	}

	bronzeAt(t, fact, base.Add(time.Minute), rowJSON(co1, id1, "2"), rowJSON(co2, id1, "1"))
	st, err := testClient.buildSilver(ctx, fact, "LineID", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != "incremental" || st.BronzeObjects != 1 || st.BronzeRows != 2 || st.PreviousSilverRows != 2 || st.SilverRows != 3 || st.DuplicatesDropped != 1 {
		t.Fatalf("stats = %+v; want incremental, 1 object / 2 rows read, previous 2, silver 3, 1 duplicate", st)
	}
	checkInvariant(t, st)
	data := string(silverBytes(t, fact))
	if !strings.Contains(data, `"V":"2"`) || strings.Contains(data, `"LineID":"`+id1+`","CompanyID":"`+co1+`","V":"1"`) {
		t.Errorf("the update did not replace the old version:\n%s", data)
	}
}

// Objects younger than settleLag are read again on every build (another writer
// may still add an object with an older key), and doing so must change nothing.
func TestIncrementalSilver_UnsettledObjectsAreReprocessedIdempotently(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	bronzeAt(t, fact, time.Now().Add(-3*time.Hour), rowJSON(co1, id1, "old"))
	bronzeAt(t, fact, time.Now().Add(-time.Minute), rowJSON(co1, id1, "young"), rowJSON(co1, id2, "young"))

	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil, false); err != nil {
		t.Fatal(err)
	}
	first := append([]byte(nil), silverBytes(t, fact)...)
	st, err := testClient.buildSilver(ctx, fact, "LineID", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.BronzeObjects != 1 {
		t.Errorf("the young object should be re-read (1 object), read %d", st.BronzeObjects)
	}
	checkInvariant(t, st)
	if !bytes.Equal(first, silverBytes(t, fact)) {
		t.Error("re-reading the unsettled object changed Silver")
	}
	if strings.Count(string(first), "\n") != 2 {
		t.Errorf("want 2 rows, got:\n%s", first)
	}
}

// A late object whose key is OLDER than the marker is invisible to incremental
// builds; that is the documented assumption, and a forced full build recovers.
func TestIncrementalSilver_ForcedFullRecoversALateObject(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	base := time.Now().Add(-3 * time.Hour)
	bronzeAt(t, fact, base.Add(time.Minute), rowJSON(co1, id1, "1"))
	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil, false); err != nil {
		t.Fatal(err)
	}
	bronzeAt(t, fact, base, rowJSON(co1, id2, "late")) // key older than the marker

	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(silverBytes(t, fact)), "late") {
		t.Fatal("premise broken: an incremental build saw an object older than its marker")
	}
	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(silverBytes(t, fact)), "late") {
		t.Error("a full rebuild must pick up the late object")
	}
}

func TestIncrementalSilver_FullRebuildEveryDay(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	bronzeAt(t, fact, time.Now().Add(-3*time.Hour), rowJSON(co1, id1, "1"))
	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil, false); err != nil {
		t.Fatal(err)
	}
	st, _ := testClient.readSilverState(ctx, fact)
	st.LastFullAt = time.Now().Add(-fullRebuildEvery - time.Hour)
	b, _ := json.Marshal(st)
	if err := testClient.Put(ctx, silverStateKey(fact), b); err != nil {
		t.Fatal(err)
	}
	got, err := testClient.buildSilver(ctx, fact, "LineID", nil, false)
	if err != nil || got.Mode != "full" {
		t.Fatalf("build = %+v, %v; want a full build once the last full one is over a day old", got, err)
	}
}

func TestIncrementalSilver_PrunesRowsFromThePreviousSilver(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	base := time.Now().Add(-3 * time.Hour)
	bronzeAt(t, fact, base, rowJSON(co1, id1, "1"), rowJSON(co1, id2, "1"))
	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil, false); err != nil {
		t.Fatal(err)
	}
	// id2 is deleted at the source; a new object touches only id1.
	bronzeAt(t, fact, base.Add(time.Minute), rowJSON(co1, id1, "2"))
	st, err := testClient.buildSilver(ctx, fact, "LineID", liveOf(co1+"/"+id1), false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != "incremental" || st.Pruned != 1 || st.SilverRows != 1 {
		t.Fatalf("stats = %+v; want incremental, 1 pruned, 1 row left", st)
	}
	checkInvariant(t, st)
	if strings.Contains(string(silverBytes(t, fact)), id2) {
		t.Error("the deleted row survived in the merged Silver")
	}
}

// A corrupt previous Silver must not wedge the ticker: the build recovers by
// running once in full mode.
func TestIncrementalSilver_CorruptPreviousSilverFallsBackToFull(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	bronzeAt(t, fact, time.Now().Add(-3*time.Hour), rowJSON(co1, id1, "1"), rowJSON(co1, id2, "1"))
	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil, false); err != nil {
		t.Fatal(err)
	}
	// Out of order: the merge cannot trust it.
	corrupt := rowJSON(co1, id2, "x") + "\n" + rowJSON(co1, id1, "x") + "\n"
	if err := testClient.Put(ctx, silverKey(fact), []byte(corrupt)); err != nil {
		t.Fatal(err)
	}
	bronzeAt(t, fact, time.Now().Add(-2*time.Hour), rowJSON(co1, id1, "2"))

	st, err := testClient.buildSilver(ctx, fact, "LineID", nil, false)
	if err != nil {
		t.Fatalf("a corrupt Silver must trigger a full rebuild, not an error: %v", err)
	}
	if st.Mode != "full" || st.SilverRows != 2 {
		t.Errorf("stats = %+v; want the full fallback with 2 rows", st)
	}
}

func TestIncrementalSilver_RejectedLinesAccumulateWithoutDuplicates(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	base := time.Now().Add(-3 * time.Hour)
	bronzeAt(t, fact, base, rowJSON(co1, id1, "1"), "not json")
	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil, false); err != nil {
		t.Fatal(err)
	}
	bronzeAt(t, fact, base.Add(time.Minute), rowJSON(co1, id2, "1"), `{"CompanyID":"`+co1+`"}`)
	st, err := testClient.buildSilver(ctx, fact, "LineID", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	checkInvariant(t, st)
	rej, err := testClient.Get(ctx, silverRejectKey(fact))
	if err != nil || bytes.Count(rej, []byte("\n")) != 2 {
		t.Errorf("rejected.jsonl should keep the old line and add the new one (2 lines), err=%v:\n%s", err, rej)
	}
}

// fastKey reads keys from trusted Silver lines without a full parse; it must
// agree with the strict rowKey on every shape a real row can take.
func TestFastKeyAgreesWithRowKey(t *testing.T) {
	type row struct {
		LineID          string
		ParentCompanyID string
		CompanyID       string
		Name            string
		Note            string
		Amount          float64
	}
	tricky := []string{
		"plain",
		`has "quotes" inside`,
		`"CompanyID":"99999999-9999-9999-9999-999999999999"`, // a value that looks like the key
		`"LineID":"88888888-8888-8888-8888-888888888888",`,
		`back\slash and "CompanyID":"`,
		"unicode \u00e9\u4e16\u754c and newline\nend",
		"",
	}
	idP, coP := []byte(`"LineID":"`), []byte(`"CompanyID":"`)
	for i, s := range tricky {
		line, err := json.Marshal(row{LineID: id1, ParentCompanyID: id2, CompanyID: co1, Name: s, Note: s, Amount: 1.5})
		if err != nil {
			t.Fatal(err)
		}
		want, wok := rowKey("LineID", line)
		got, gok := fastKey(line, idP, coP)
		if wok != gok || want != got {
			t.Errorf("case %d (%q): fastKey = %q,%v; rowKey = %q,%v", i, s, got, gok, want, wok)
		}
	}
}
