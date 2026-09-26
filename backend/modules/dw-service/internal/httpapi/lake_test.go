package httpapi

import "testing"

func TestClassifyReconcile(t *testing.T) {
	cases := []struct {
		silver   int
		ch       uint64
		wantDiff int64
		want     string
	}{
		{36, 36, 0, reconcileMatch},
		{0, 0, 0, reconcileMatch},
		{8, 36, 28, reconcileMissing}, // the dev-stack gap: rows in ClickHouse, absent from the lake
		{0, 5, 5, reconcileMissing},   // Silver never built
		{4, 3, -1, reconcileExtra},
	}
	for _, c := range cases {
		diff, status := classifyReconcile(c.silver, c.ch)
		if diff != c.wantDiff || status != c.want {
			t.Errorf("classifyReconcile(%d, %d) = %d, %s; want %d, %s", c.silver, c.ch, diff, status, c.wantDiff, c.want)
		}
	}
}
