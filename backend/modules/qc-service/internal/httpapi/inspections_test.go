package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func today() string {
	return time.Now().Format("2006-01-02")
}

func TestCreateInspection_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	standard := mustSeedStandard(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing standard_id": {
			"company_id": companyID, "inspection_date": today(), "inspected_quantity": 10,
		},
		"missing inspection_date": {
			"company_id": companyID, "standard_id": standard.ID, "inspected_quantity": 10,
		},
		"zero inspected_quantity": {
			"company_id": companyID, "standard_id": standard.ID, "inspection_date": today(), "inspected_quantity": 0,
		},
		"invalid reference_type": {
			"company_id": companyID, "standard_id": standard.ID, "inspection_date": today(), "inspected_quantity": 10,
			"reference_type": "SALES_ORDER",
		},
		"negative passed_quantity": {
			"company_id": companyID, "standard_id": standard.ID, "inspection_date": today(), "inspected_quantity": 10,
			"passed_quantity": -1,
		},
		"negative failed_quantity": {
			"company_id": companyID, "standard_id": standard.ID, "inspection_date": today(), "inspected_quantity": 10,
			"failed_quantity": -1,
		},
		"passed+failed exceeds inspected": {
			"company_id": companyID, "standard_id": standard.ID, "inspection_date": today(), "inspected_quantity": 10,
			"passed_quantity": 8, "failed_quantity": 5,
		},
		"bad inspection_date format": {
			"company_id": companyID, "standard_id": standard.ID, "inspection_date": "01-07-2026", "inspected_quantity": 10,
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/inspections", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateInspection_StandardNotFound(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	resp := postJSON(t, srv.URL+"/inspections", map[string]any{
		"company_id": companyID, "standard_id": uuid.NewString(), "inspection_date": today(), "inspected_quantity": 10,
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestCreateInspection_InactiveStandardConflict(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	standard := mustSeedStandard(t, srv, companyID)

	requireStatus(t, doRequest(t, http.MethodPut, srv.URL+"/standards/"+standard.ID, map[string]any{
		"name": "Standard", "is_active": false,
	}, ""), http.StatusOK)

	resp := postJSON(t, srv.URL+"/inspections", map[string]any{
		"company_id": companyID, "standard_id": standard.ID, "inspection_date": today(), "inspected_quantity": 10,
	})
	requireStatus(t, resp, http.StatusConflict)
}

type inspectionView struct {
	ID                string  `json:"id"`
	InspectionNumber  string  `json:"inspection_number"`
	StandardID        string  `json:"standard_id"`
	ProductID         string  `json:"product_id"`
	ReferenceType     string  `json:"reference_type"`
	ReferenceID       *string `json:"reference_id"`
	ReferenceNumber   *string `json:"reference_number"`
	InspectedQuantity float64 `json:"inspected_quantity"`
	PassedQuantity    float64 `json:"passed_quantity"`
	FailedQuantity    float64 `json:"failed_quantity"`
	Result            string  `json:"result"`
	InspectedBy       *string `json:"inspected_by"`
}

func TestCreateInspection_ResultComputation(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	standard := mustSeedStandard(t, srv, companyID)

	cases := []struct {
		name           string
		passed, failed float64
		wantResult     string
	}{
		{"all passed", 10, 0, "PASS"},
		{"all failed", 0, 10, "FAIL"},
		{"partial", 6, 4, "PARTIAL"},
		{"neither recorded yet (failed=0 wins)", 0, 0, "PASS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/inspections", map[string]any{
				"company_id": companyID, "standard_id": standard.ID, "inspection_date": today(),
				"inspected_quantity": 10, "passed_quantity": tc.passed, "failed_quantity": tc.failed,
			})
			requireStatus(t, resp, http.StatusCreated)
			var insp inspectionView
			resp.decode(t, &insp)
			if insp.Result != tc.wantResult {
				t.Errorf("result = %q, want %q (passed=%.0f failed=%.0f)", insp.Result, tc.wantResult, tc.passed, tc.failed)
			}
		})
	}
}

func TestCreateInspection_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	standard := mustSeedStandard(t, srv, companyID)
	refID := uuid.NewString()

	resp := postJSON(t, srv.URL+"/inspections", map[string]any{
		"company_id": companyID, "standard_id": standard.ID, "inspection_date": today(),
		"inspected_quantity": 20, "passed_quantity": 15, "failed_quantity": 5,
		"reference_type": "WORK_ORDER", "reference_id": refID, "reference_number": "WO-202607-0001",
	})
	requireStatus(t, resp, http.StatusCreated)

	var insp inspectionView
	resp.decode(t, &insp)
	if !strings.HasPrefix(insp.InspectionNumber, "INS-") {
		t.Errorf("inspection_number = %q, want INS- prefix", insp.InspectionNumber)
	}
	if insp.StandardID != standard.ID {
		t.Errorf("standard_id = %q, want %q", insp.StandardID, standard.ID)
	}
	if insp.ProductID != standard.ProductID {
		t.Errorf("product_id = %q, want %q (copied from standard, not sent by client)", insp.ProductID, standard.ProductID)
	}
	if insp.ReferenceType != "WORK_ORDER" || insp.ReferenceID == nil || *insp.ReferenceID != refID {
		t.Errorf("reference = %s/%v, want WORK_ORDER/%s", insp.ReferenceType, insp.ReferenceID, refID)
	}
	if insp.ReferenceNumber == nil || *insp.ReferenceNumber != "WO-202607-0001" {
		t.Errorf("reference_number = %v, want WO-202607-0001", insp.ReferenceNumber)
	}
	if insp.Result != "PARTIAL" {
		t.Errorf("result = %q, want PARTIAL", insp.Result)
	}
	if insp.InspectedBy == nil {
		t.Error("expected inspected_by to be set from X-User-Id header")
	}

	getResp := getJSON(t, srv.URL+"/inspections/"+insp.ID)
	requireStatus(t, getResp, http.StatusOK)
}

func TestCreateInspection_DefaultsReferenceTypeToManual(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	standard := mustSeedStandard(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/inspections", map[string]any{
		"company_id": companyID, "standard_id": standard.ID, "inspection_date": today(), "inspected_quantity": 5,
	})
	requireStatus(t, resp, http.StatusCreated)
	var insp inspectionView
	resp.decode(t, &insp)
	if insp.ReferenceType != "MANUAL" {
		t.Errorf("reference_type = %q, want default MANUAL", insp.ReferenceType)
	}
	if insp.ReferenceID != nil {
		t.Errorf("reference_id = %v, want nil when not provided", insp.ReferenceID)
	}
}

func TestGetInspection_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/inspections/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

func TestListInspections_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/inspections")
	requireStatus(t, resp, http.StatusBadRequest)
}
