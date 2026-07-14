package httpapi_test

import (
	"net/http"
	"testing"
)

func TestListReadings_ScopedAndFiltered(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	devA := mustSeedDevice(t, srv, companyA, "TEMPERATURE")
	devB := mustSeedDevice(t, srv, companyB, "TEMPERATURE")

	val := 25.5
	mustSeedReading(t, devA.ID, companyA, "TEMPERATURE", &val, nil)
	mustSeedReading(t, devB.ID, companyB, "TEMPERATURE", &val, nil)

	resp := getJSON(t, srv.URL+"/readings?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
		DeviceID  string `json:"device_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA || list[0].DeviceID != devA.ID {
		t.Fatalf("expected exactly 1 reading scoped to companyA, got %+v", list)
	}
}

func TestListReadings_FilteredByDeviceID(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	dev1 := mustSeedDevice(t, srv, companyID, "HUMIDITY")
	dev2 := mustSeedDevice(t, srv, companyID, "HUMIDITY")

	v1, v2 := 40.0, 60.0
	mustSeedReading(t, dev1.ID, companyID, "HUMIDITY", &v1, nil)
	mustSeedReading(t, dev2.ID, companyID, "HUMIDITY", &v2, nil)

	resp := getJSON(t, srv.URL+"/readings?company_id="+companyID+"&device_id="+dev1.ID)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		DeviceID string `json:"device_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].DeviceID != dev1.ID {
		t.Fatalf("expected exactly 1 reading for dev1, got %+v", list)
	}
}

// TestListReadings_LimitRespected confirms pagination actually caps rows
// returned (unlike warehouse-service's stock-movements endpoint, which has
// a hardcoded 200-row window with no way to fetch the rest -- see the
// implementation plan's rationale for adding real limit/offset here).
func TestListReadings_LimitRespected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	dev := mustSeedDevice(t, srv, companyID, "VIBRATION")

	for i := range 5 {
		v := float64(i)
		mustSeedReading(t, dev.ID, companyID, "VIBRATION", &v, nil)
	}

	resp := getJSON(t, srv.URL+"/readings?company_id="+companyID+"&limit=2")
	requireStatus(t, resp, http.StatusOK)
	var list []map[string]any
	resp.decode(t, &list)
	if len(list) != 2 {
		t.Fatalf("expected exactly 2 readings with limit=2, got %d", len(list))
	}
}

func TestListReadings_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/readings")
	requireStatus(t, resp, http.StatusBadRequest)
}
