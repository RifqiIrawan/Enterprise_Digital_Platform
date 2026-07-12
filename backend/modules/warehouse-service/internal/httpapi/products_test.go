package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateProduct_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id": {"sku": "SKU-001", "name": "Produk Uji"},
		"missing sku":        {"company_id": companyID, "name": "Produk Uji"},
		"missing name":       {"company_id": companyID, "sku": "SKU-001"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/products", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateProduct_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/products", map[string]any{
		"company_id": companyID, "sku": "SKU-100", "name": "Produk Uji Coba", "cost_price": 5000,
	})
	requireStatus(t, resp, http.StatusCreated)

	var p struct {
		CompanyID string  `json:"company_id"`
		Unit      string  `json:"unit"`
		IsActive  bool    `json:"is_active"`
		CostPrice float64 `json:"cost_price"`
	}
	resp.decode(t, &p)
	if p.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", p.CompanyID, companyID)
	}
	if p.Unit != "pcs" {
		t.Errorf("unit = %q, want default pcs", p.Unit)
	}
	if !p.IsActive {
		t.Error("expected is_active to default true")
	}
	if p.CostPrice != 5000 {
		t.Errorf("cost_price = %.2f, want 5000.00", p.CostPrice)
	}
}

func TestCreateProduct_DuplicateSKUConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{"company_id": companyID, "sku": "SKU-DUP", "name": "Produk A"}
	requireStatus(t, postJSON(t, srv.URL+"/products", payload), http.StatusCreated)

	dup := map[string]any{"company_id": companyID, "sku": "SKU-DUP", "name": "Produk B"} // same sku, different name
	requireStatus(t, postJSON(t, srv.URL+"/products", dup), http.StatusConflict)
}

func TestCreateProduct_DuplicateNameConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{"company_id": companyID, "sku": "SKU-A", "name": "Nama Sama"}
	requireStatus(t, postJSON(t, srv.URL+"/products", payload), http.StatusCreated)

	dup := map[string]any{"company_id": companyID, "sku": "SKU-B", "name": "Nama Sama"} // different sku, same name
	requireStatus(t, postJSON(t, srv.URL+"/products", dup), http.StatusConflict)
}

func TestUpdateProduct_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/products/"+uuid.NewString(), map[string]any{
		"name": "Updated", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateProduct_BlankNameRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	p := mustSeedProduct(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/products/"+p.ID, map[string]any{
		"name": "   ", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateProduct_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	p := mustSeedProduct(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/products/"+p.ID, map[string]any{
		"name": "Renamed Product", "unit": "box", "cost_price": 1234.5, "is_active": false,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name      string  `json:"name"`
		Unit      string  `json:"unit"`
		CostPrice float64 `json:"cost_price"`
		IsActive  bool    `json:"is_active"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Renamed Product" || updated.Unit != "box" || updated.CostPrice != 1234.5 {
		t.Errorf("unexpected update result: %+v", updated)
	}
	if updated.IsActive {
		t.Error("expected is_active to be false after update")
	}
}

func TestListProducts_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedProduct(t, srv, companyA)
	mustSeedProduct(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/products?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 product scoped to companyA, got %+v", list)
	}
}

func TestListProducts_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/products")
	requireStatus(t, resp, http.StatusBadRequest)
}
