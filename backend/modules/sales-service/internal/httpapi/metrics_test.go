package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestMetrics_Endpoint(t *testing.T) {
	srv := newServer(t)

	resp := getJSON(t, srv.URL+"/metrics")
	requireStatus(t, resp, http.StatusOK)

	if !strings.Contains(string(resp.body), "go_goroutines") {
		t.Fatalf("expected /metrics body to contain go_goroutines, got: %s", resp.body)
	}
}
