package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateWarehouse_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id": {"code": "WH-001", "name": "Gudang Uji"},
		"missing code":       {"company_id": companyID, "name": "Gudang Uji"},
		"missing name":       {"company_id": companyID, "code": "WH-001"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/warehouses", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateWarehouse_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/warehouses", map[string]any{
		"company_id": companyID, "code": "WH-100", "name": "Gudang Uji Coba", "address": "Jl. Uji No. 1",
	})
	requireStatus(t, resp, http.StatusCreated)

	var wh struct {
		CompanyID string `json:"company_id"`
		IsActive  bool   `json:"is_active"`
	}
	resp.decode(t, &wh)
	if wh.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", wh.CompanyID, companyID)
	}
	if !wh.IsActive {
		t.Error("expected is_active to default true")
	}
}

func TestCreateWarehouse_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{"company_id": companyID, "code": "WH-DUP", "name": "Gudang A"}
	requireStatus(t, postJSON(t, srv.URL+"/warehouses", payload), http.StatusCreated)
	conflict := postJSON(t, srv.URL+"/warehouses", payload)
	requireStatus(t, conflict, http.StatusConflict)
	if conflict.errorMessage() == "" {
		t.Error("expected a non-empty error message on conflict")
	}
}

func TestUpdateWarehouse_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/warehouses/"+uuid.NewString(), map[string]any{
		"name": "Updated", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateWarehouse_BlankNameRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/warehouses/"+wh.ID, map[string]any{
		"name": "   ", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateWarehouse_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	wh := mustSeedWarehouse(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/warehouses/"+wh.ID, map[string]any{
		"name": "Renamed Warehouse", "address": "New Address", "is_active": false,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Renamed Warehouse" {
		t.Errorf("name = %q, want %q", updated.Name, "Renamed Warehouse")
	}
	if updated.IsActive {
		t.Error("expected is_active to be false after update")
	}
}

func TestListWarehouses_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedWarehouse(t, srv, companyA)
	mustSeedWarehouse(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/warehouses?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 warehouse scoped to companyA, got %+v", list)
	}
}

func TestListWarehouses_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/warehouses")
	requireStatus(t, resp, http.StatusBadRequest)
}
