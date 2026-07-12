package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateCustomer_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing company_id":    {"customer_code": "CUST-001", "name": "PT Uji"},
		"missing customer_code": {"company_id": companyID, "name": "PT Uji"},
		"missing name":          {"company_id": companyID, "customer_code": "CUST-001"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/customers", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateCustomer_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/customers", map[string]any{
		"company_id": companyID, "customer_code": "CUST-100", "name": "PT Pelanggan Uji",
		"email": "cust@example.test", "tax_id": "01.234.567.8-901.000",
	})
	requireStatus(t, resp, http.StatusCreated)

	var c struct {
		CompanyID string `json:"company_id"`
		IsActive  bool   `json:"is_active"`
	}
	resp.decode(t, &c)
	if c.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", c.CompanyID, companyID)
	}
	if !c.IsActive {
		t.Error("expected is_active to default true")
	}
}

func TestCreateCustomer_DuplicateCodeConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	payload := map[string]any{"company_id": companyID, "customer_code": "CUST-DUP", "name": "PT A"}
	requireStatus(t, postJSON(t, srv.URL+"/customers", payload), http.StatusCreated)
	conflict := postJSON(t, srv.URL+"/customers", payload)
	requireStatus(t, conflict, http.StatusConflict)
	if conflict.errorMessage() == "" {
		t.Error("expected a non-empty error message on conflict")
	}
}

func TestUpdateCustomer_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/customers/"+uuid.NewString(), map[string]any{
		"name": "Updated", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateCustomer_BlankNameRejected(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCustomer(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/customers/"+c.ID, map[string]any{
		"name": "   ", "is_active": true,
	}, "")
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateCustomer_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	c := mustSeedCustomer(t, srv, companyID)

	resp := doRequest(t, http.MethodPut, srv.URL+"/customers/"+c.ID, map[string]any{
		"name": "Renamed Customer", "email": "new@example.test", "is_active": false,
	}, "")
	requireStatus(t, resp, http.StatusOK)

	var updated struct {
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	}
	resp.decode(t, &updated)
	if updated.Name != "Renamed Customer" {
		t.Errorf("name = %q, want %q", updated.Name, "Renamed Customer")
	}
	if updated.IsActive {
		t.Error("expected is_active to be false after update")
	}
}

func TestListCustomers_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustSeedCustomer(t, srv, companyA)
	mustSeedCustomer(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/customers?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 customer scoped to companyA, got %+v", list)
	}
}

func TestListCustomers_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/customers")
	requireStatus(t, resp, http.StatusBadRequest)
}
