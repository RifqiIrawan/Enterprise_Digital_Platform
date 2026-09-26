package datalake

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	co1 = "11111111-1111-1111-1111-111111111111"
	co2 = "22222222-2222-2222-2222-222222222222"
	id1 = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	id2 = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

func uniqueFact(t *testing.T) string {
	t.Helper()
	return "test_silver_" + time.Now().Format("150405.000000000")
}

func bronze(t *testing.T, fact string, at time.Time, lines ...string) {
	t.Helper()
	key := fact + "/" + at.Format("2006/01/02/") + at.Format("150405.000000000") + ".jsonl"
	if err := testClient.Put(context.Background(), key, []byte(strings.Join(lines, "\n")+"\n")); err != nil {
		t.Fatal(err)
	}
}

func cleanup(t *testing.T, fact string) {
	t.Helper()
	ctx := context.Background()
	for _, prefix := range []string{fact + "/", silverPrefix + fact + "/"} {
		keys, _ := testClient.ListKeys(ctx, prefix)
		for _, k := range keys {
			_ = testClient.Remove(ctx, k)
		}
	}
}

func TestBuildSilver_LatestVersionWinsAndDuplicatesDropped(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	t0 := time.Now()
	// v1 dan v2 baris yang sama (id1, company 1): v2 harus menang. id1 di
	// company 2 adalah baris LAIN -- kunci mencakup company.
	bronze(t, fact, t0, `{"LineID":"`+id1+`","CompanyID":"`+co1+`","Status":"DRAFT"}`,
		`{"LineID":"`+id2+`","CompanyID":"`+co1+`","Status":"DRAFT"}`)
	bronze(t, fact, t0.Add(time.Second), `{"LineID":"`+id1+`","CompanyID":"`+co1+`","Status":"POSTED"}`,
		`{"LineID":"`+id1+`","CompanyID":"`+co2+`","Status":"DRAFT"}`)

	st, err := testClient.buildSilver(ctx, fact, "LineID", nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.BronzeRows != 4 || st.SilverRows != 3 || st.DuplicatesDropped != 1 || st.Rejected != 0 {
		t.Fatalf("stats = %+v, want bronze 4 / silver 3 / dup 1 / rejected 0", st)
	}

	data, err := testClient.Get(ctx, silverPrefix+fact+"/current.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"CompanyID":"`+co1+`","Status":"POSTED"`) {
		t.Errorf("expected POSTED version of company-1 %s to win:\n%s", id1, data)
	}
	if strings.Contains(string(data), `"LineID":"`+id1+`","CompanyID":"`+co1+`","Status":"DRAFT"`) {
		t.Errorf("stale DRAFT version of company-1 %s leaked into Silver:\n%s", id1, data)
	}
}

func TestBuildSilver_RejectsBadRowsAndKeepsInvariant(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	bronze(t, fact, time.Now(),
		`{"LineID":"`+id1+`","CompanyID":"`+co1+`"}`,
		`not json at all`,
		`{"CompanyID":"`+co1+`"}`, // tanpa id
		`{"LineID":"00000000-0000-0000-0000-000000000000","CompanyID":"`+co1+`"}`, // id kosong
		`{"LineID":"`+id2+`"}`) // tanpa company

	st, err := testClient.buildSilver(ctx, fact, "LineID", nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.SilverRows != 1 || st.Rejected != 4 {
		t.Fatalf("stats = %+v, want silver 1 / rejected 4", st)
	}
	if st.BronzeRows != st.SilverRows+st.DuplicatesDropped+st.Rejected+st.Pruned {
		t.Errorf("invarian bronze = silver + dup + rejected rusak: %+v", st)
	}
	rej, err := testClient.Get(ctx, silverRejectKey(fact))
	if err != nil || bytes.Count(rej, []byte("\n")) != 4 {
		t.Errorf("rejected.jsonl should hold 4 lines, err=%v:\n%s", err, rej)
	}
}

func TestBuildSilver_IdempotentAndClearsStaleRejects(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	bronze(t, fact, time.Now(), `{"LineID":"`+id1+`","CompanyID":"`+co1+`"}`, `garbage`)
	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil); err != nil {
		t.Fatal(err)
	}
	first, _ := testClient.Get(ctx, silverKey(fact))
	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil); err != nil {
		t.Fatal(err)
	}
	second, _ := testClient.Get(ctx, silverKey(fact))
	if !bytes.Equal(first, second) {
		t.Error("dua build berturut-turut harus menghasilkan berkas identik")
	}

	// Setelah Bronze diperbaiki (di sini: dihapus lalu diganti yang bersih),
	// rejected.jsonl lama tidak boleh tersisa.
	keys, _ := testClient.ListKeys(ctx, fact+"/")
	for _, k := range keys {
		_ = testClient.Remove(ctx, k)
	}
	bronze(t, fact, time.Now(), `{"LineID":"`+id1+`","CompanyID":"`+co1+`"}`)
	st, err := testClient.buildSilver(ctx, fact, "LineID", nil)
	if err != nil || st.Rejected != 0 {
		t.Fatalf("stats=%+v err=%v", st, err)
	}
	if left, _ := testClient.ListKeys(ctx, silverRejectKey(fact)); len(left) != 0 {
		t.Errorf("rejected.jsonl basi masih ada: %v", left)
	}
}

func TestBuildSilver_UnknownFact(t *testing.T) {
	if _, err := testClient.BuildSilver(context.Background(), "nope", nil); err == nil {
		t.Fatal("fact tak dikenal harus galat")
	}
}

func TestReadSilver_MissingIsEmptyNotError(t *testing.T) {
	rows, err := testClient.ReadSilver(context.Background(), "never_built_"+time.Now().Format("150405.000000000"))
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%d err=%v, want empty and nil", len(rows), err)
	}
}

func liveOf(keys ...string) LiveKeys {
	return func(context.Context) (map[string]struct{}, error) {
		m := map[string]struct{}{}
		for _, k := range keys {
			m[k] = struct{}{}
		}
		return m, nil
	}
}

// A row deleted at the source stays in Bronze (history) but must leave Silver
// (current state). Bronze rows: id1 twice (an update), id2 once. Only id1 is
// still alive at the source.
func TestBuildSilver_PrunesRowsDeletedAtTheSource(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	t0 := time.Now()
	bronze(t, fact, t0, `{"LineID":"`+id1+`","CompanyID":"`+co1+`","V":1}`, `{"LineID":"`+id2+`","CompanyID":"`+co1+`"}`)
	bronze(t, fact, t0.Add(time.Second), `{"LineID":"`+id1+`","CompanyID":"`+co1+`","V":2}`)

	st, err := testClient.buildSilver(ctx, fact, "LineID", liveOf(co1+"/"+id1))
	if err != nil {
		t.Fatal(err)
	}
	if st.BronzeRows != 3 || st.SilverRows != 1 || st.DuplicatesDropped != 1 || st.Pruned != 1 || st.Rejected != 0 {
		t.Fatalf("stats = %+v, want bronze 3 / silver 1 / dup 1 / pruned 1", st)
	}
	if st.BronzeRows != st.SilverRows+st.DuplicatesDropped+st.Rejected+st.Pruned {
		t.Errorf("invariant broken: %+v", st)
	}
	data, _ := testClient.Get(ctx, silverKey(fact))
	if strings.Contains(string(data), id2) || !strings.Contains(string(data), `"V":2`) {
		t.Errorf("silver should hold only the live, newest id1 row:\n%s", data)
	}
	// History is untouched.
	if keys, _ := testClient.ListKeys(ctx, fact+"/"); len(keys) != 2 {
		t.Errorf("Bronze must keep both objects, has %d", len(keys))
	}
}

// If the source cannot be asked, the build fails and the previous Silver stays:
// publishing unpruned rows would resurrect deleted data, and pruning against an
// empty answer would wipe rows that still exist.
func TestBuildSilver_SourceUnreachableKeepsPreviousSilver(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	bronze(t, fact, time.Now(), `{"LineID":"`+id1+`","CompanyID":"`+co1+`"}`)
	if _, err := testClient.buildSilver(ctx, fact, "LineID", nil); err != nil {
		t.Fatal(err)
	}
	before, _ := testClient.Get(ctx, silverKey(fact))

	down := func(context.Context) (map[string]struct{}, error) { return nil, errors.New("postgres down") }
	if _, err := testClient.buildSilver(ctx, fact, "LineID", down); err == nil || !strings.Contains(err.Error(), "postgres down") {
		t.Fatalf("want the source error, got %v", err)
	}
	after, _ := testClient.Get(ctx, silverKey(fact))
	if !bytes.Equal(before, after) {
		t.Error("a failed build must leave the previous Silver untouched")
	}
}

func TestBuildSilver_NilLiveKeysDoesNotPrune(t *testing.T) {
	ctx := context.Background()
	fact := uniqueFact(t)
	t.Cleanup(func() { cleanup(t, fact) })

	bronze(t, fact, time.Now(), `{"LineID":"`+id1+`","CompanyID":"`+co1+`"}`)
	st, err := testClient.buildSilver(ctx, fact, "LineID", nil)
	if err != nil || st.Pruned != 0 || st.SilverRows != 1 {
		t.Fatalf("stats=%+v err=%v", st, err)
	}
}
