package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateDevice_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id":  {"device_code": "DEV-001", "device_type": "TEMPERATURE", "name": "Sensor 1"},
		"missing device_code": {"company_id": companyID, "device_type": "TEMPERATURE", "name": "Sensor 1"},
		"missing device_type": {"company_id": companyID, "device_code": "DEV-001", "name": "Sensor 1"},
		"missing name":        {"company_id": companyID, "device_code": "DEV-001", "device_type": "TEMPERATURE"},
		"invalid device_type": {"company_id": companyID, "device_code": "DEV-001", "device_type": "LASER", "name": "Sensor 1"},
		"threshold on non-numeric type": {
			"company_id": companyID, "device_code": "DEV-001", "device_type": "RFID", "name": "Reader 1",
			"threshold_min": 1, "threshold_max": 10,
		},
		"threshold_min >= threshold_max": {
			"company_id": companyID, "device_code": "DEV-001", "device_type": "TEMPERATURE", "name": "Sensor 1",
			"threshold_min": 30, "threshold_max": 20,
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/devices", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateDevice_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	warehouseID := uuid.NewString()

	resp := postJSON(t, srv.URL+"/devices", map[string]any{
		"company_id": companyID, "device_code": "DEV-100", "device_type": "TEMPERATURE", "name": "Sensor Gudang A",
		"warehouse_id": warehouseID, "threshold_min": 15, "threshold_max": 30,
	})
	requireStatus(t, resp, http.StatusCreated)

	var d struct {
		CompanyID    string   `json:"company_id"`
		WarehouseID  *string  `json:"warehouse_id"`
		Status       string   `json:"status"`
		ThresholdMin *float64 `json:"threshold_min"`
		ThresholdMax *float64 `json:"threshold_max"`
	}
	resp.decode(t, &d)
	if d.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", d.CompanyID, companyID)
	}
	if d.Status != "ACTIVE" {
		t.Errorf("status = %q, want default ACTIVE", d.Status)
	}
	if d.WarehouseID == nil || *d.WarehouseID != warehouseID {
		t.Errorf("warehouse_id = %v, want %q", d.WarehouseID, warehouseID)
	}
	if d.ThresholdMin == nil || *d.ThresholdMin != 15 || d.ThresholdMax == nil || *d.ThresholdMax != 30 {
		t.Errorf("threshold = [%v, %v], want [15, 30]", d.ThresholdMin, d.ThresholdMax)
	}
}

func TestCreateDevice_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{"company_id": companyID, "device_code": "DEV-DUP", "device_type": "RFID", "name": "Reader A"}
	requireStatus(t, postJSON(t, srv.URL+"/devices", payload), http.StatusCreated)
	conflict := postJSON(t, srv.URL+"/devices", payload)
	requireStatus(t, conflict, http.StatusConflict)
	if conflict.errorMessage() == "" {
		t.Error("expected a non-empty error message on conflict")
	}
}

func TestGetDevice_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/devices/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateDevice_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/devices/"+uuid.NewString(), map[string]any{
		"name": "Updated", "status": "ACTIVE",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateDevice_InvalidStatusRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	d := mustSeedDevice(t, srv, companyID, "GPS")

	resp := doRequest(t, http.MethodPut, srv.URL+"/devices/"+d.ID, map[string]any{
		"name": "Renamed", "status": "BROKEN",
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateDevice_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	d := mustSeedDevice(t, srv, companyID, "HUMIDITY")

	resp := doRequest(t, http.MethodPut, srv.URL+"/devices/"+d.ID, map[string]any{
		"name": "Sensor Renamed", "status": "MAINTENANCE", "threshold_min": 30, "threshold_max": 70,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name         string   `json:"name"`
		Status       string   `json:"status"`
		ThresholdMin *float64 `json:"threshold_min"`
		ThresholdMax *float64 `json:"threshold_max"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Sensor Renamed" || updated.Status != "MAINTENANCE" {
		t.Errorf("unexpected update result: %+v", updated)
	}
	if updated.ThresholdMin == nil || *updated.ThresholdMin != 30 || updated.ThresholdMax == nil || *updated.ThresholdMax != 70 {
		t.Errorf("threshold = [%v, %v], want [30, 70]", updated.ThresholdMin, updated.ThresholdMax)
	}
}

func TestListDevices_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedDevice(t, srv, companyA, "BARCODE")
	mustSeedDevice(t, srv, companyB, "BARCODE")

	resp := getJSON(t, srv.URL+"/devices?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 device scoped to companyA, got %+v", list)
	}
}

func TestListDevices_FilteredByDeviceType(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	mustSeedDevice(t, srv, companyID, "TEMPERATURE")
	mustSeedDevice(t, srv, companyID, "VIBRATION")

	resp := getJSON(t, srv.URL+"/devices?company_id="+companyID+"&device_type=VIBRATION")
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		DeviceType string `json:"device_type"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].DeviceType != "VIBRATION" {
		t.Fatalf("expected exactly 1 VIBRATION device, got %+v", list)
	}
}

func TestListDevices_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/devices")
	requireStatus(t, resp, http.StatusBadRequest)
}
