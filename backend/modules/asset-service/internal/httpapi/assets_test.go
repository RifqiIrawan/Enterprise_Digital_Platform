package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateAsset_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id": {"asset_code": "AST-001", "name": "Forklift"},
		"missing asset_code": {"company_id": companyID, "name": "Forklift"},
		"missing name":       {"company_id": companyID, "asset_code": "AST-001"},
		"bad acquisition_date format": {
			"company_id": companyID, "asset_code": "AST-001", "name": "Forklift", "acquisition_date": "01-07-2026",
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/assets", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateAsset_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	warehouseID := uuid.NewString()

	resp := postJSON(t, srv.URL+"/assets", map[string]any{
		"company_id": companyID, "asset_code": "AST-100", "name": "Forklift Toyota 3 Ton",
		"category": "Alat Berat", "acquisition_date": "2024-01-15", "acquisition_cost": 250_000_000,
		"warehouse_id": warehouseID,
	})
	requireStatus(t, resp, http.StatusCreated)

	var a struct {
		CompanyID   string  `json:"company_id"`
		WarehouseID *string `json:"warehouse_id"`
		Status      string  `json:"status"`
	}
	resp.decode(t, &a)
	if a.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", a.CompanyID, companyID)
	}
	if a.Status != "ACTIVE" {
		t.Errorf("status = %q, want default ACTIVE", a.Status)
	}
	if a.WarehouseID == nil || *a.WarehouseID != warehouseID {
		t.Errorf("warehouse_id = %v, want %q", a.WarehouseID, warehouseID)
	}
}

func TestCreateAsset_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{"company_id": companyID, "asset_code": "AST-DUP", "name": "Aset A"}
	requireStatus(t, postJSON(t, srv.URL+"/assets", payload), http.StatusCreated)
	conflict := postJSON(t, srv.URL+"/assets", payload)
	requireStatus(t, conflict, http.StatusConflict)
	if conflict.errorMessage() == "" {
		t.Error("expected a non-empty error message on conflict")
	}
}

func TestUpdateAsset_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/assets/"+uuid.NewString(), map[string]any{
		"name": "Updated", "status": "ACTIVE",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateAsset_BlankNameRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAsset(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/assets/"+a.ID, map[string]any{
		"name": "   ", "status": "ACTIVE",
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateAsset_MissingStatusRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAsset(t, srv, companyID)

	// status has no default/omit-empty handling in updateAsset -- it's
	// validated against an explicit allow-list and an empty string isn't in
	// it, so callers MUST always resend the current status on every update.
	resp := doRequest(t, http.MethodPut, srv.URL+"/assets/"+a.ID, map[string]any{
		"name": "Renamed",
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateAsset_InvalidStatusRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAsset(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/assets/"+a.ID, map[string]any{
		"name": "Renamed", "status": "RETIRED",
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateAsset_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	a := mustSeedAsset(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/assets/"+a.ID, map[string]any{
		"name": "Renamed Asset", "category": "Kendaraan", "status": "DISPOSED", "notes": "Sudah tidak dipakai",
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Status   string `json:"status"`
		Notes    string `json:"notes"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Renamed Asset" || updated.Category != "Kendaraan" || updated.Status != "DISPOSED" || updated.Notes != "Sudah tidak dipakai" {
		t.Errorf("unexpected update result: %+v", updated)
	}
}

func TestListAssets_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedAsset(t, srv, companyA)
	mustSeedAsset(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/assets?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 asset scoped to companyA, got %+v", list)
	}
}

// TestListAssets_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive: a branch filter must still surface unassigned (NULL
// branch_id) rows alongside that branch's own rows.
func TestListAssets_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkAsset := func(branchID *string) {
		code := "AST-" + uuid.NewString()[:8]
		requireStatus(t, postJSON(t, srv.URL+"/assets", map[string]any{
			"company_id": companyID, "branch_id": branchID, "asset_code": code, "name": "Test Asset " + code,
		}), http.StatusCreated)
	}
	mkAsset(&branchA)
	mkAsset(nil)
	mkAsset(&branchB)

	resp := getJSON(t, srv.URL+"/assets?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var assets []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &assets)
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets (branchA + NULL), got %d: %+v", len(assets), assets)
	}
	for _, a := range assets {
		if a.BranchID != nil && *a.BranchID == branchB {
			t.Errorf("branchB asset leaked into branchA-filtered results: %+v", assets)
		}
	}
}

func TestListAssets_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/assets")
	requireStatus(t, resp, http.StatusBadRequest)
}
