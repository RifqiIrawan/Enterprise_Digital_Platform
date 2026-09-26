package httpapi

import (
	"strings"
	"testing"

	"github.com/enterprise-digital-platform/dw-service/internal/datalake"
)

func fact(name, status string, silver int, chRows uint64) reconcileFact {
	d, _ := classifyReconcile(silver, chRows)
	return reconcileFact{Fact: name, SilverRows: silver, ClickHouseRows: chRows, Difference: d, Status: status}
}

func TestLakeCycle_HealthyFirstLookIsSilent(t *testing.T) {
	facts := []reconcileFact{fact("a", reconcileMatch, 3, 3), fact("b", reconcileMatch, 1, 1)}
	lines, next := describeLakeCycle(datalake.BuildResult{}, true, facts, nil)
	if len(lines) != 0 {
		t.Errorf("a healthy first cycle must log nothing, got %v", lines)
	}
	if next["a"] != reconcileMatch || len(next) != 2 {
		t.Errorf("next state = %v", next)
	}
}

func TestLakeCycle_LogsGapOnceThenStaysQuiet(t *testing.T) {
	facts := []reconcileFact{fact("finance", reconcileMissing, 8, 36), fact("kpi", reconcileExtra, 2, 0)}

	lines, next := describeLakeCycle(datalake.BuildResult{}, false, facts, nil)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "finance is missing 28 rows") || !strings.Contains(joined, "backfill") {
		t.Errorf("missing-rows line absent or without the backfill hint:\n%s", joined)
	}
	if !strings.Contains(joined, "kpi has 2 rows in the lake") || !strings.Contains(joined, "0/2 facts match") {
		t.Errorf("extra-rows line or summary absent:\n%s", joined)
	}

	// Same state next hour: a permanent EXTRA_IN_LAKE must not fill the log.
	if again, _ := describeLakeCycle(datalake.BuildResult{}, false, facts, next); len(again) != 0 {
		t.Errorf("unchanged state logged again: %v", again)
	}
}

func TestLakeCycle_ReportsRecovery(t *testing.T) {
	prev := map[string]string{"finance": reconcileMissing}
	lines, _ := describeLakeCycle(datalake.BuildResult{}, true, []reconcileFact{fact("finance", reconcileMatch, 36, 36)}, prev)
	if len(lines) == 0 || !strings.Contains(lines[0], "finance now matches ClickHouse (was MISSING_FROM_LAKE)") {
		t.Errorf("recovery not reported: %v", lines)
	}
}

func TestLakeCycle_BuildErrorsAreLoggedEveryCycle(t *testing.T) {
	facts := []reconcileFact{fact("a", reconcileMatch, 1, 1)}
	build := datalake.BuildResult{Errors: []string{"silver a: boom"}}
	for i := 0; i < 2; i++ {
		lines, _ := describeLakeCycle(build, true, facts, map[string]string{"a": reconcileMatch})
		if len(lines) == 0 || lines[0] != "lake build: silver a: boom" {
			t.Fatalf("cycle %d: build error not logged: %v", i, lines)
		}
	}
}

func TestLakeCycle_UncheckableFactIsReported(t *testing.T) {
	f := reconcileFact{Fact: "a", Status: reconcileError, Error: "clickhouse down"}
	lines, _ := describeLakeCycle(datalake.BuildResult{}, false, []reconcileFact{f}, nil)
	if !strings.Contains(strings.Join(lines, "\n"), "a could not be checked: clickhouse down") {
		t.Errorf("error status not described: %v", lines)
	}
}
