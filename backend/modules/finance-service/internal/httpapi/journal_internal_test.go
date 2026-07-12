package httpapi

import "testing"

func TestAmountsEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b float64
		want bool
	}{
		{"identical", 100.00, 100.00, true},
		{"within epsilon", 100.005, 100.00, true},
		{"just outside epsilon", 100.02, 100.00, false},
		{"negative diff within epsilon", 100.00, 100.005, true},
		{"zero vs zero", 0, 0, true},
		{"far apart", 100.00, 50.00, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := amountsEqual(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("amountsEqual(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
