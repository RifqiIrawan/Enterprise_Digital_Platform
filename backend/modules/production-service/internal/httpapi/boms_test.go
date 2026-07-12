package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateBOM_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	productID := uuid.NewString()
	componentID := uuid.NewString()

	cases := map[string]map[string]any{
		"missing bom_code": {
			"company_id": companyID, "name": "Resep A", "product_id": productID,
			"lines": []map[string]any{{"component_product_id": componentID, "quantity_per_unit": 1}},
		},
		"missing product_id": {
			"company_id": companyID, "bom_code": "BOM-001", "name": "Resep A",
			"lines": []map[string]any{{"component_product_id": componentID, "quantity_per_unit": 1}},
		},
		"empty lines": {
			"company_id": companyID, "bom_code": "BOM-001", "name": "Resep A", "product_id": productID,
			"lines": []map[string]any{},
		},
		"line missing component_product_id": {
			"company_id": companyID, "bom_code": "BOM-001", "name": "Resep A", "product_id": productID,
			"lines": []map[string]any{{"quantity_per_unit": 1}},
		},
		"line zero quantity_per_unit": {
			"company_id": companyID, "bom_code": "BOM-001", "name": "Resep A", "product_id": productID,
			"lines": []map[string]any{{"component_product_id": componentID, "quantity_per_unit": 0}},
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/boms", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateBOM_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	b := mustSeedBOM(t, srv, companyID)
	if b.ID == "" {
		t.Fatal("expected a generated id")
	}

	getResp := getJSON(t, srv.URL+"/boms/"+b.ID)
	requireStatus(t, getResp, http.StatusOK)
	var full struct {
		IsActive bool `json:"is_active"`
		Lines    []struct {
			ComponentProductID string  `json:"component_product_id"`
			QuantityPerUnit    float64 `json:"quantity_per_unit"`
		} `json:"lines"`
	}
	getResp.decode(t, &full)
	if !full.IsActive {
		t.Error("expected is_active to default true")
	}
	if len(full.Lines) != 1 || full.Lines[0].ComponentProductID != b.ComponentProductID || full.Lines[0].QuantityPerUnit != 2 {
		t.Errorf("unexpected lines: %+v", full.Lines)
	}
}

func TestCreateBOM_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{
		"company_id": companyID, "bom_code": "BOM-DUP", "name": "Resep A", "product_id": uuid.NewString(),
		"lines": []map[string]any{{"component_product_id": uuid.NewString(), "quantity_per_unit": 1}},
	}
	requireStatus(t, postJSON(t, srv.URL+"/boms", payload), http.StatusCreated)
	conflict := postJSON(t, srv.URL+"/boms", payload)
	requireStatus(t, conflict, http.StatusConflict)
	if conflict.errorMessage() == "" {
		t.Error("expected a non-empty error message on conflict")
	}
}

func TestUpdateBOM_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/boms/"+uuid.NewString(), map[string]any{
		"name": "Updated", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateBOM_BlankNameRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	b := mustSeedBOM(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/boms/"+b.ID, map[string]any{
		"name": "   ", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateBOM_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	b := mustSeedBOM(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/boms/"+b.ID, map[string]any{
		"name": "Renamed BOM", "is_active": false,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Renamed BOM" {
		t.Errorf("name = %q, want %q", updated.Name, "Renamed BOM")
	}
	if updated.IsActive {
		t.Error("expected is_active to be false after update")
	}
}

func TestGetBOM_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/boms/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func TestListBOMs_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedBOM(t, srv, companyA)
	mustSeedBOM(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/boms?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 BOM scoped to companyA, got %+v", list)
	}
}

func TestListBOMs_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/boms")
	requireStatus(t, resp, http.StatusBadRequest)
}
