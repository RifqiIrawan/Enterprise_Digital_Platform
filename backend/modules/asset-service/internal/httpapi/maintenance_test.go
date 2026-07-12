package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func today() string {
	return time.Now().Format("2006-01-02")
}

func TestCreateMaintenanceSchedule_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing asset_id": {
			"company_id": companyID, "maintenance_type": "Servis Rutin", "scheduled_date": today(),
		},
		"missing maintenance_type": {
			"company_id": companyID, "asset_id": asset.ID, "scheduled_date": today(),
		},
		"missing scheduled_date": {
			"company_id": companyID, "asset_id": asset.ID, "maintenance_type": "Servis Rutin",
		},
		"bad scheduled_date format": {
			"company_id": companyID, "asset_id": asset.ID, "maintenance_type": "Servis Rutin", "scheduled_date": "01-07-2026",
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/maintenance-schedules", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateMaintenanceSchedule_AssetNotFound(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	resp := postJSON(t, srv.URL+"/maintenance-schedules", map[string]any{
		"company_id": companyID, "asset_id": uuid.NewString(), "maintenance_type": "Servis Rutin", "scheduled_date": today(),
	})
	requireStatus(t, resp, http.StatusNotFound)
}

type maintenanceView struct {
	ID            string  `json:"id"`
	AssetID       string  `json:"asset_id"`
	Status        string  `json:"status"`
	CompletedDate *string `json:"completed_date"`
}

func mustCreateMaintenanceSchedule(t *testing.T, srvURL, companyID, assetID string) maintenanceView {
	t.Helper()
	resp := postJSON(t, srvURL+"/maintenance-schedules", map[string]any{
		"company_id": companyID, "asset_id": assetID, "maintenance_type": "Servis Rutin", "scheduled_date": today(),
	})
	requireStatus(t, resp, http.StatusCreated)
	var m maintenanceView
	resp.decode(t, &m)
	return m
}

func TestCreateMaintenanceSchedule_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)

	m := mustCreateMaintenanceSchedule(t, srv.URL, companyID, asset.ID)
	if m.Status != "SCHEDULED" {
		t.Errorf("status = %q, want SCHEDULED", m.Status)
	}
	if m.CompletedDate != nil {
		t.Errorf("completed_date = %v, want nil", m.CompletedDate)
	}
}

func TestCompleteMaintenanceSchedule_RevertsAssetFromMaintenance(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)
	mustSetAssetStatus(t, srv, asset.ID, "MAINTENANCE")
	m := mustCreateMaintenanceSchedule(t, srv.URL, companyID, asset.ID)

	resp := postJSON(t, srv.URL+"/maintenance-schedules/"+m.ID+"/complete", nil)
	requireStatus(t, resp, http.StatusOK)
	var completed maintenanceView
	resp.decode(t, &completed)
	if completed.Status != "COMPLETED" {
		t.Errorf("status = %q, want COMPLETED", completed.Status)
	}
	if completed.CompletedDate == nil {
		t.Error("expected completed_date to be set")
	}

	getResp := getJSON(t, srv.URL+"/assets?company_id="+companyID)
	requireStatus(t, getResp, http.StatusOK)
	var assets []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	getResp.decode(t, &assets)
	if len(assets) != 1 || assets[0].Status != "ACTIVE" {
		t.Fatalf("expected asset to revert to ACTIVE after completing maintenance, got %+v", assets)
	}

	// Completing an already-COMPLETED schedule must be rejected.
	requireStatus(t, postJSON(t, srv.URL+"/maintenance-schedules/"+m.ID+"/complete", nil), http.StatusConflict)
}

func TestCompleteMaintenanceSchedule_NoOpWhenAssetNotInMaintenance(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID) // stays ACTIVE, never set to MAINTENANCE
	m := mustCreateMaintenanceSchedule(t, srv.URL, companyID, asset.ID)

	requireStatus(t, postJSON(t, srv.URL+"/maintenance-schedules/"+m.ID+"/complete", nil), http.StatusOK)

	getResp := getJSON(t, srv.URL+"/assets?company_id="+companyID)
	requireStatus(t, getResp, http.StatusOK)
	var assets []struct {
		Status string `json:"status"`
	}
	getResp.decode(t, &assets)
	if len(assets) != 1 || assets[0].Status != "ACTIVE" {
		t.Errorf("expected asset to remain ACTIVE (no-op), got %+v", assets)
	}
}

func TestCompleteMaintenanceSchedule_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv.URL+"/maintenance-schedules/"+uuid.NewString()+"/complete", nil)
	requireStatus(t, resp, http.StatusConflict)
}

func TestCancelMaintenanceSchedule_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)
	m := mustCreateMaintenanceSchedule(t, srv.URL, companyID, asset.ID)

	resp := postJSON(t, srv.URL+"/maintenance-schedules/"+m.ID+"/cancel", nil)
	requireStatus(t, resp, http.StatusOK)
	var cancelled maintenanceView
	resp.decode(t, &cancelled)
	if cancelled.Status != "CANCELLED" {
		t.Errorf("status = %q, want CANCELLED", cancelled.Status)
	}

	// Cancelling an already-CANCELLED schedule must be rejected.
	requireStatus(t, postJSON(t, srv.URL+"/maintenance-schedules/"+m.ID+"/cancel", nil), http.StatusConflict)
}

func TestCancelMaintenanceSchedule_AfterCompleteConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)
	m := mustCreateMaintenanceSchedule(t, srv.URL, companyID, asset.ID)

	requireStatus(t, postJSON(t, srv.URL+"/maintenance-schedules/"+m.ID+"/complete", nil), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/maintenance-schedules/"+m.ID+"/cancel", nil), http.StatusConflict)
}

func TestCancelMaintenanceSchedule_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := postJSON(t, srv.URL+"/maintenance-schedules/"+uuid.NewString()+"/cancel", nil)
	requireStatus(t, resp, http.StatusConflict)
}

func TestListMaintenanceSchedules_FilteredByAsset(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	assetA := mustSeedAsset(t, srv, companyID)
	assetB := mustSeedAsset(t, srv, companyID)
	mustCreateMaintenanceSchedule(t, srv.URL, companyID, assetA.ID)
	mustCreateMaintenanceSchedule(t, srv.URL, companyID, assetB.ID)

	resp := getJSON(t, srv.URL+"/maintenance-schedules?company_id="+companyID+"&asset_id="+assetA.ID)
	requireStatus(t, resp, http.StatusOK)
	var schedules []struct {
		AssetID string `json:"asset_id"`
	}
	resp.decode(t, &schedules)
	if len(schedules) != 1 || schedules[0].AssetID != assetA.ID {
		t.Fatalf("expected exactly 1 schedule scoped to assetA, got %+v", schedules)
	}
}

func TestListMaintenanceSchedules_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/maintenance-schedules")
	requireStatus(t, resp, http.StatusBadRequest)
}
