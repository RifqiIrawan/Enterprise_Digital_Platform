package httpapi_test

import (
	"math"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// Penyuntingan komponen BOM lewat PUT /boms/{id}. Yang dijaga: aturan formula
// yang sama dengan saat membuat (neraca massa >= 100%), tipe BOM tidak bisa
// diganti, penggantian komponen atomik, dan work order yang sudah ada tidak
// ikut berubah karena kebutuhannya sudah di-snapshot.

func putBOM(t *testing.T, srvURL, id string, payload map[string]any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPut, srvURL+"/boms/"+id, payload, "")
}

func TestUpdateBOM_ReplacesFormulaLinesAndBatchSize(t *testing.T) {
	srv := newServer(t)
	f := mustSeedFormulaBOM(t, srv, newCompanyID(t), 1000, 60, 40)
	newComponent := uuid.NewString()

	resp := putBOM(t, srv.URL, f.ID, map[string]any{
		"name": "Formula v2", "is_active": true, "batch_size": 500,
		"lines": []map[string]any{
			{"component_product_id": f.ComponentA, "percentage": 50},
			{"component_product_id": newComponent, "percentage": 55},
		},
	})
	requireStatus(t, resp, http.StatusOK)
	var got bomView
	resp.decode(t, &got)
	if got.BatchSize == nil || *got.BatchSize != 500 {
		t.Errorf("batch_size = %v, mau 500", got.BatchSize)
	}
	if len(got.Lines) != 2 || got.Lines[1].ComponentProductID != newComponent {
		t.Fatalf("lines tidak terganti: %+v", got.Lines)
	}
	if got.TotalPercentage == nil || math.Abs(*got.TotalPercentage-105) > 0.0001 {
		t.Errorf("total_percentage = %v, mau 105", got.TotalPercentage)
	}
	if got.YieldPercent == nil || math.Abs(*got.YieldPercent-95.238) > 0.01 {
		t.Errorf("yield_percent = %v, mau ~95,24", got.YieldPercent)
	}

	// Yang tersimpan sama dengan yang dikembalikan, dan baris lama benar-benar hilang.
	check := doRequest(t, http.MethodGet, srv.URL+"/boms/"+f.ID, nil, "")
	requireStatus(t, check, http.StatusOK)
	var stored bomView
	check.decode(t, &stored)
	if len(stored.Lines) != 2 || stored.Lines[0].Percentage == nil || *stored.Lines[0].Percentage != 50 {
		t.Errorf("tersimpan: %+v", stored.Lines)
	}
}

func TestUpdateBOM_ReplacesUnitLines(t *testing.T) {
	srv := newServer(t)
	b := mustSeedBOM(t, srv, newCompanyID(t))
	resp := putBOM(t, srv.URL, b.ID, map[string]any{
		"name": "Test BOM", "is_active": true,
		"lines": []map[string]any{{"component_product_id": b.ComponentProductID, "quantity_per_unit": 5}},
	})
	requireStatus(t, resp, http.StatusOK)
	var got bomView
	resp.decode(t, &got)
	if len(got.Lines) != 1 || got.Lines[0].QuantityPerUnit != 5 {
		t.Errorf("lines = %+v", got.Lines)
	}
}

func TestUpdateBOM_RejectsInvalidEditsAndLeavesBOMUntouched(t *testing.T) {
	srv := newServer(t)
	f := mustSeedFormulaBOM(t, srv, newCompanyID(t), 1000, 60, 40)
	unit := mustSeedBOM(t, srv, newCompanyID(t))
	comp := uuid.NewString()

	cases := map[string]struct {
		id      string
		payload map[string]any
	}{
		"formula di bawah 100%":             {f.ID, map[string]any{"name": "x", "lines": []map[string]any{{"component_product_id": comp, "percentage": 99}}}},
		"lines kosong":                      {f.ID, map[string]any{"name": "x", "lines": []map[string]any{}}},
		"formula memakai quantity_per_unit": {f.ID, map[string]any{"name": "x", "lines": []map[string]any{{"component_product_id": comp, "quantity_per_unit": 1, "percentage": 100}}}},
		"batch_size nol":                    {f.ID, map[string]any{"name": "x", "batch_size": 0}},
		"batch_size pada BOM UNIT":          {unit.ID, map[string]any{"name": "x", "batch_size": 100}},
		"percentage pada BOM UNIT":          {unit.ID, map[string]any{"name": "x", "lines": []map[string]any{{"component_product_id": comp, "quantity_per_unit": 1, "percentage": 50}}}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, putBOM(t, srv.URL, c.id, c.payload), http.StatusBadRequest)
		})
	}

	check := doRequest(t, http.MethodGet, srv.URL+"/boms/"+f.ID, nil, "")
	var stored bomView
	check.decode(t, &stored)
	if len(stored.Lines) != 2 || *stored.Lines[0].Percentage != 60 || *stored.BatchSize != 1000 {
		t.Errorf("BOM berubah setelah edit yang ditolak: %+v", stored)
	}
}

func TestUpdateBOM_NameOnlyKeepsLines(t *testing.T) {
	srv := newServer(t)
	f := mustSeedFormulaBOM(t, srv, newCompanyID(t), 1000, 60, 40)
	requireStatus(t, putBOM(t, srv.URL, f.ID, map[string]any{"name": "Renamed", "is_active": true}), http.StatusOK)

	check := doRequest(t, http.MethodGet, srv.URL+"/boms/"+f.ID, nil, "")
	var stored bomView
	check.decode(t, &stored)
	if len(stored.Lines) != 2 || stored.BatchSize == nil || *stored.BatchSize != 1000 {
		t.Errorf("name-only update mengubah komponen/batch: %+v", stored)
	}
}

func TestUpdateBOM_ExistingWorkOrderKeepsItsSnapshot(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	f := mustSeedFormulaBOM(t, srv, companyID, 1000, 60, 40)

	resp := postJSON(t, srv.URL+"/work-orders", map[string]any{
		"company_id": companyID, "bom_id": f.ID, "warehouse_id": uuid.NewString(),
		"quantity_planned": 1000, "planned_start_date": today(),
	})
	requireStatus(t, resp, http.StatusCreated)
	var wo workOrderFormulaView
	resp.decode(t, &wo)

	requireStatus(t, putBOM(t, srv.URL, f.ID, map[string]any{
		"name": "v2", "is_active": true,
		"lines": []map[string]any{{"component_product_id": uuid.NewString(), "percentage": 100}},
	}), http.StatusOK)

	after := doRequest(t, http.MethodGet, srv.URL+"/work-orders/"+wo.ID, nil, "")
	requireStatus(t, after, http.StatusOK)
	var got workOrderFormulaView
	after.decode(t, &got)
	want := map[string]float64{f.ComponentA: 600, f.ComponentB: 400}
	if len(got.Lines) != 2 {
		t.Fatalf("snapshot berubah: %+v", got.Lines)
	}
	for _, l := range got.Lines {
		if math.Abs(l.QuantityRequired-want[l.ComponentProductID]) > 0.0001 {
			t.Errorf("komponen %s = %g, mau %g", l.ComponentProductID, l.QuantityRequired, want[l.ComponentProductID])
		}
	}
}
