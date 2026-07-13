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

func TestCreateRequisition_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing pr_date": {
			"company_id": companyID,
			"lines":      []map[string]any{{"product_name": "Item A", "quantity": 1, "estimated_price": 100}},
		},
		"empty lines": {
			"company_id": companyID, "pr_date": today(), "lines": []map[string]any{},
		},
		"line missing product_name": {
			"company_id": companyID, "pr_date": today(),
			"lines": []map[string]any{{"product_name": "", "quantity": 1, "estimated_price": 100}},
		},
		"bad pr_date format": {
			"company_id": companyID, "pr_date": "01-07-2026",
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1, "estimated_price": 100}},
		},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/requisitions", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

type requisitionView struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	PRNumber       string  `json:"pr_number"`
	SubtotalAmount float64 `json:"subtotal_amount"`
}

func mustCreateRequisitionOn(t *testing.T, srvURL, companyID string) requisitionView {
	t.Helper()
	resp := postJSON(t, srvURL+"/requisitions", map[string]any{
		"company_id": companyID, "pr_date": today(), "requested_by": "Gudang A",
		"lines": []map[string]any{{"product_name": "Bahan Baku Uji", "quantity": 4, "estimated_price": 250}},
	})
	requireStatus(t, resp, http.StatusCreated)
	var pr requisitionView
	resp.decode(t, &pr)
	return pr
}

func TestCreateRequisition_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	pr := mustCreateRequisitionOn(t, srv.URL, companyID)
	if pr.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", pr.Status)
	}
	if pr.SubtotalAmount != 1000 {
		t.Errorf("subtotal_amount = %.2f, want 1000.00", pr.SubtotalAmount)
	}

	getResp := getJSON(t, srv.URL+"/requisitions/"+pr.ID)
	requireStatus(t, getResp, http.StatusOK)
}

func TestRequisitionLifecycle_SubmitApproveConvert(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	pr := mustCreateRequisitionOn(t, srv.URL, companyID)

	submitResp := postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/submit", nil)
	requireStatus(t, submitResp, http.StatusOK)
	var submitted requisitionView
	submitResp.decode(t, &submitted)
	if submitted.Status != "SUBMITTED" {
		t.Fatalf("status = %q, want SUBMITTED", submitted.Status)
	}

	// Submitting twice must fail.
	requireStatus(t, postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/submit", nil), http.StatusConflict)

	approveResp := postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/approve", nil)
	requireStatus(t, approveResp, http.StatusOK)
	var approved requisitionView
	approveResp.decode(t, &approved)
	if approved.Status != "APPROVED" {
		t.Fatalf("status = %q, want APPROVED", approved.Status)
	}

	convertResp := postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/convert", map[string]any{"supplier_id": supplier.ID})
	requireStatus(t, convertResp, http.StatusCreated)
	var po struct {
		ID             string  `json:"id"`
		Status         string  `json:"status"`
		SupplierID     string  `json:"supplier_id"`
		RequisitionID  *string `json:"requisition_id"`
		SubtotalAmount float64 `json:"subtotal_amount"`
	}
	convertResp.decode(t, &po)
	if po.Status != "DRAFT" {
		t.Errorf("converted purchase order status = %q, want DRAFT", po.Status)
	}
	if po.SupplierID != supplier.ID {
		t.Errorf("converted purchase order supplier_id = %q, want %q", po.SupplierID, supplier.ID)
	}
	if po.RequisitionID == nil || *po.RequisitionID != pr.ID {
		t.Errorf("converted purchase order requisition_id = %v, want %q", po.RequisitionID, pr.ID)
	}
	if po.SubtotalAmount != approved.SubtotalAmount {
		t.Errorf("converted purchase order subtotal = %.2f, want requisition subtotal %.2f", po.SubtotalAmount, approved.SubtotalAmount)
	}

	// Requisition must now be CONVERTED and can't be converted again.
	getResp := getJSON(t, srv.URL+"/requisitions/"+pr.ID)
	requireStatus(t, getResp, http.StatusOK)
	var reloaded requisitionView
	getResp.decode(t, &reloaded)
	if reloaded.Status != "CONVERTED" {
		t.Errorf("status = %q, want CONVERTED", reloaded.Status)
	}
	requireStatus(t, postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/convert", map[string]any{"supplier_id": supplier.ID}), http.StatusConflict)
}

func TestRejectRequisition(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	pr := mustCreateRequisitionOn(t, srv.URL, companyID)

	requireStatus(t, postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/submit", nil), http.StatusOK)
	rejectResp := postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/reject", nil)
	requireStatus(t, rejectResp, http.StatusOK)
	var rejected requisitionView
	rejectResp.decode(t, &rejected)
	if rejected.Status != "REJECTED" {
		t.Errorf("status = %q, want REJECTED", rejected.Status)
	}

	// A REJECTED requisition can't be converted.
	requireStatus(t, postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/convert", map[string]any{"supplier_id": supplier.ID}), http.StatusConflict)
}

func TestConvertRequisition_RequiresApproved(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	supplier := mustSeedSupplier(t, srv, companyID)
	pr := mustCreateRequisitionOn(t, srv.URL, companyID) // still DRAFT

	resp := postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/convert", map[string]any{"supplier_id": supplier.ID})
	requireStatus(t, resp, http.StatusConflict)
}

func TestConvertRequisition_MissingSupplierID(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	pr := mustCreateRequisitionOn(t, srv.URL, companyID)
	requireStatus(t, postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/submit", nil), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/approve", nil), http.StatusOK)

	resp := postJSON(t, srv.URL+"/requisitions/"+pr.ID+"/convert", map[string]any{})
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestGetRequisition_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/requisitions/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

// TestListRequisitions_FilteredByBranch confirms branch_id filtering is
// NULL-inclusive (see TestListSuppliers_FilteredByBranch for rationale).
func TestListRequisitions_FilteredByBranch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchA := uuid.NewString()
	branchB := uuid.NewString()

	mkRequisition := func(branchID *string) {
		requireStatus(t, postJSON(t, srv.URL+"/requisitions", map[string]any{
			"company_id": companyID, "branch_id": branchID, "pr_date": today(),
			"lines": []map[string]any{{"product_name": "Item A", "quantity": 1, "estimated_price": 100}},
		}), http.StatusCreated)
	}
	mkRequisition(&branchA)
	mkRequisition(nil)
	mkRequisition(&branchB)

	resp := getJSON(t, srv.URL+"/requisitions?company_id="+companyID+"&branch_id="+branchA)
	requireStatus(t, resp, http.StatusOK)
	var requisitions []struct {
		BranchID *string `json:"branch_id"`
	}
	resp.decode(t, &requisitions)
	if len(requisitions) != 2 {
		t.Fatalf("expected 2 requisitions (branchA + NULL), got %d: %+v", len(requisitions), requisitions)
	}
	for _, pr := range requisitions {
		if pr.BranchID != nil && *pr.BranchID == branchB {
			t.Errorf("branchB requisition leaked into branchA-filtered results: %+v", requisitions)
		}
	}
}
