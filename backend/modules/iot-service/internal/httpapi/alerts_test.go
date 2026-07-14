package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestAcknowledgeAlert_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPost, srv.URL+"/alerts/"+uuid.NewString()+"/acknowledge", nil, "")
	requireStatus(t, resp, http.StatusConflict)
}

func TestAcknowledgeAlert_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	dev := mustSeedDevice(t, srv, companyID, "TEMPERATURE")
	alertID := mustSeedAlert(t, dev.ID, companyID)

	resp := doRequest(t, http.MethodPost, srv.URL+"/alerts/"+alertID+"/acknowledge", nil, "")
	requireStatus(t, resp, http.StatusOK)

	var a struct {
		Status         string  `json:"status"`
		AcknowledgedAt *string `json:"acknowledged_at"`
	}
	resp.decode(t, &a)
	if a.Status != "ACKNOWLEDGED" || a.AcknowledgedAt == nil {
		t.Errorf("unexpected acknowledge result: %+v", a)
	}
}

func TestAcknowledgeAlert_AlreadyAcknowledgedConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	dev := mustSeedDevice(t, srv, companyID, "TEMPERATURE")
	alertID := mustSeedAlert(t, dev.ID, companyID)

	requireStatus(t, doRequest(t, http.MethodPost, srv.URL+"/alerts/"+alertID+"/acknowledge", nil, ""), http.StatusOK)
	requireStatus(t, doRequest(t, http.MethodPost, srv.URL+"/alerts/"+alertID+"/acknowledge", nil, ""), http.StatusConflict)
}

func TestResolveAlert_FromOpen(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	dev := mustSeedDevice(t, srv, companyID, "TEMPERATURE")
	alertID := mustSeedAlert(t, dev.ID, companyID)

	resp := doRequest(t, http.MethodPost, srv.URL+"/alerts/"+alertID+"/resolve", nil, "")
	requireStatus(t, resp, http.StatusOK)

	var a struct {
		Status     string  `json:"status"`
		ResolvedAt *string `json:"resolved_at"`
	}
	resp.decode(t, &a)
	if a.Status != "RESOLVED" || a.ResolvedAt == nil {
		t.Errorf("unexpected resolve result: %+v", a)
	}
}

func TestResolveAlert_FromAcknowledged(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	dev := mustSeedDevice(t, srv, companyID, "TEMPERATURE")
	alertID := mustSeedAlert(t, dev.ID, companyID)

	requireStatus(t, doRequest(t, http.MethodPost, srv.URL+"/alerts/"+alertID+"/acknowledge", nil, ""), http.StatusOK)
	requireStatus(t, doRequest(t, http.MethodPost, srv.URL+"/alerts/"+alertID+"/resolve", nil, ""), http.StatusOK)
}

func TestResolveAlert_AlreadyResolvedConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	dev := mustSeedDevice(t, srv, companyID, "TEMPERATURE")
	alertID := mustSeedAlert(t, dev.ID, companyID)

	requireStatus(t, doRequest(t, http.MethodPost, srv.URL+"/alerts/"+alertID+"/resolve", nil, ""), http.StatusOK)
	requireStatus(t, doRequest(t, http.MethodPost, srv.URL+"/alerts/"+alertID+"/resolve", nil, ""), http.StatusConflict)
}

func TestListAlerts_ScopedAndFilteredByStatus(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	devA := mustSeedDevice(t, srv, companyA, "TEMPERATURE")
	devB := mustSeedDevice(t, srv, companyB, "TEMPERATURE")
	openID := mustSeedAlert(t, devA.ID, companyA)
	resolvedID := mustSeedAlert(t, devA.ID, companyA)
	mustSeedAlert(t, devB.ID, companyB)

	requireStatus(t, doRequest(t, http.MethodPost, srv.URL+"/alerts/"+resolvedID+"/resolve", nil, ""), http.StatusOK)

	resp := getJSON(t, srv.URL+"/alerts?company_id="+companyA+"&status=OPEN")
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].ID != openID {
		t.Fatalf("expected exactly 1 OPEN alert (companyA's open one), got %+v", list)
	}
}

func TestListAlerts_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/alerts")
	requireStatus(t, resp, http.StatusBadRequest)
}
