package httpapi

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	ch "github.com/enterprise-digital-platform/dw-service/internal/clickhouse"
	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
	"github.com/enterprise-digital-platform/dw-service/internal/sourcedb"
)

// RunLakeCycle adalah satu putaran perawatan data lake yang dijalankan berkala
// setelah sync: bangun ulang Silver dan Gold, lalu bandingkan Silver dengan
// ClickHouse. Ia mengembalikan baris log yang layak dicetak dan status per fact
// untuk putaran berikutnya.
//
// Yang dicatat sengaja HANYA yang berubah sejak putaran sebelumnya (prev):
// putaran ini jalan tiap jam, dan fact yang selamanya EXTRA_IN_LAKE (baris
// yang dihapus di sumber, lihat dokumentasi/06) tidak boleh mengisi log
// selamanya. Galat build dicatat SETIAP putaran -- itu bukan "keadaan" yang
// bisa dianggap sudah diketahui.
//
// Backfill TIDAK dijalankan di sini. Ia membaca seluruh tabel sumber, dan
// menjalankannya otomatis akan menyembunyikan penyebab selisih alih-alih
// memperlihatkannya; log hanya menunjuk ke endpoint-nya.
func RunLakeCycle(ctx context.Context, sources *sourcedb.Pools, dest *ch.Client, lake *datalake.Client, prev map[string]string) ([]string, map[string]string) {
	build := lake.BuildAll(ctx, liveKeyFuncs(sources))
	consistent, facts := reconcileAll(ctx, dest, lake)
	return describeLakeCycle(build, consistent, facts, prev)
}

func describeLakeCycle(build datalake.BuildResult, consistent bool, facts []reconcileFact, prev map[string]string) ([]string, map[string]string) {
	var lines []string
	for _, e := range build.Errors {
		lines = append(lines, "lake build: "+e)
	}

	next := make(map[string]string, len(facts))
	matching := 0
	var changed []string

	// Baris yang dipangkas dari Silver karena sudah dihapus di sumber. Angkanya
	// sama tiap putaran (Bronze tidak pernah melupakannya), jadi dicatat hanya
	// saat berubah, dengan kunci "pruned:<fact>" di state yang sama.
	for _, s := range build.Silver {
		key := "pruned:" + s.Fact
		next[key] = strconv.Itoa(s.Pruned)
		if s.Pruned > 0 && prev[key] != next[key] {
			changed = append(changed, fmt.Sprintf("lake build: %s pruned %d rows from Silver that were deleted at the source (kept in Bronze as history)", s.Fact, s.Pruned))
		}
	}
	for _, f := range facts {
		next[f.Fact] = f.Status
		if f.Status == reconcileMatch {
			matching++
		}
		before, seen := prev[f.Fact]
		if seen && before == f.Status {
			continue
		}
		if !seen && f.Status == reconcileMatch {
			continue // first look and healthy: nothing to say
		}
		changed = append(changed, describeFact(f, before))
	}
	sort.Strings(changed)
	lines = append(lines, changed...)

	// A summary only when something changed, so a steady state stays silent.
	if len(changed) > 0 || len(build.Errors) > 0 {
		lines = append(lines, fmt.Sprintf("lake reconcile: %d/%d facts match (consistent=%t)", matching, len(facts), consistent))
	}
	return lines, next
}

func describeFact(f reconcileFact, before string) string {
	switch f.Status {
	case reconcileMatch:
		return fmt.Sprintf("lake reconcile: %s now matches ClickHouse (was %s)", f.Fact, before)
	case reconcileMissing:
		return fmt.Sprintf("lake reconcile: %s is missing %d rows from the lake (silver %d, clickhouse %d); run POST /api/dw/lake/backfill, unless the rows were deleted at the source and ClickHouse (which never deletes) still holds them", f.Fact, f.Difference, f.SilverRows, f.ClickHouseRows)
	case reconcileExtra:
		return fmt.Sprintf("lake reconcile: %s has %d rows in the lake that ClickHouse lacks (silver %d, clickhouse %d); ClickHouse was probably cleared, since Silver already drops rows deleted at the source", f.Fact, -f.Difference, f.SilverRows, f.ClickHouseRows)
	default:
		return fmt.Sprintf("lake reconcile: %s could not be checked: %s", f.Fact, f.Error)
	}
}
