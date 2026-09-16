package httpapi_test

import (
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Formula (BOM berbasis persentase/batch) -- potongan terakhir Fase 3. Yang
// dijaga di berkas ini bukan sekadar "field baru tersimpan", melainkan tiga
// aturan yang membuatnya bukan CRUD: neraca massa >= 100%, penolakan setengah
// batch, dan ketelitian snapshot work_order_lines yang dihitung dari
// percentage, bukan dari quantity_per_unit turunannya.

type bomLineView struct {
	ComponentProductID string   `json:"component_product_id"`
	QuantityPerUnit    float64  `json:"quantity_per_unit"`
	Percentage         *float64 `json:"percentage"`
}

type bomView struct {
	ID              string        `json:"id"`
	BOMType         string        `json:"bom_type"`
	BatchSize       *float64      `json:"batch_size"`
	Lines           []bomLineView `json:"lines"`
	TotalPercentage *float64      `json:"total_percentage"`
	YieldPercent    *float64      `json:"yield_percent"`
}

type formulaFixture struct {
	bomView
	ComponentA string
	ComponentB string
}

// mustSeedFormulaBOM membuat formula dua komponen dengan batch_size 1.000.
// percentageA + percentageB menentukan yield-nya: 60+40 berarti tidak ada
// susut, 63+42 berarti 105% input untuk 100% output.
func mustSeedFormulaBOM(t *testing.T, srv *httptest.Server, companyID string, batchSize, percentageA, percentageB float64) formulaFixture {
	t.Helper()
	code := "FRM-" + uuid.NewString()[:8]
	componentA := uuid.NewString()
	componentB := uuid.NewString()
	resp := postJSON(t, srv.URL+"/boms", map[string]any{
		"company_id": companyID, "bom_code": code, "name": "Formula " + code,
		"product_id": uuid.NewString(), "bom_type": "BATCH", "batch_size": batchSize,
		"lines": []map[string]any{
			{"component_product_id": componentA, "percentage": percentageA},
			{"component_product_id": componentB, "percentage": percentageB},
		},
	})
	requireStatus(t, resp, http.StatusCreated)
	var b bomView
	resp.decode(t, &b)
	return formulaFixture{bomView: b, ComponentA: componentA, ComponentB: componentB}
}

func TestCreateBOM_Formula_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	componentID := uuid.NewString()
	base := func(extra map[string]any) map[string]any {
		payload := map[string]any{
			"company_id": companyID, "bom_code": "FRM-" + uuid.NewString()[:8],
			"name": "Formula uji", "product_id": uuid.NewString(),
		}
		maps.Copy(payload, extra)
		return payload
	}

	cases := map[string]map[string]any{
		"bom_type tidak dikenal": base(map[string]any{
			"bom_type": "RESEP",
			"lines":    []map[string]any{{"component_product_id": componentID, "quantity_per_unit": 1}},
		}),
		"BATCH tanpa batch_size": base(map[string]any{
			"bom_type": "BATCH",
			"lines":    []map[string]any{{"component_product_id": componentID, "percentage": 100}},
		}),
		"BATCH dengan batch_size nol": base(map[string]any{
			"bom_type": "BATCH", "batch_size": 0,
			"lines": []map[string]any{{"component_product_id": componentID, "percentage": 100}},
		}),
		"BATCH tanpa percentage": base(map[string]any{
			"bom_type": "BATCH", "batch_size": 1000,
			"lines": []map[string]any{{"component_product_id": componentID, "quantity_per_unit": 1}},
		}),
		"BATCH dengan percentage di bawah minimum": base(map[string]any{
			"bom_type": "BATCH", "batch_size": 1000,
			"lines": []map[string]any{
				{"component_product_id": componentID, "percentage": 0.005},
				{"component_product_id": uuid.NewString(), "percentage": 100},
			},
		}),
		"UNIT dengan batch_size": base(map[string]any{
			"batch_size": 1000,
			"lines":      []map[string]any{{"component_product_id": componentID, "quantity_per_unit": 1}},
		}),
		"UNIT dengan percentage": base(map[string]any{
			"lines": []map[string]any{{"component_product_id": componentID, "quantity_per_unit": 1, "percentage": 50}},
		}),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/boms", payload), http.StatusBadRequest)
		})
	}
}

// Neraca massa: satu batch 1.000 kg tidak bisa keluar dari 90% input. Pesannya
// harus menyebut jumlah persentasenya, bukan sekadar "tidak valid" -- yang
// perlu dikoreksi orangnya adalah angka itu.
func TestCreateBOM_Formula_RejectsBelowHundredPercent(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	resp := postJSON(t, srv.URL+"/boms", map[string]any{
		"company_id": companyID, "bom_code": "FRM-" + uuid.NewString()[:8],
		"name": "Formula kurang", "product_id": uuid.NewString(),
		"bom_type": "BATCH", "batch_size": 1000,
		"lines": []map[string]any{
			{"component_product_id": uuid.NewString(), "percentage": 60},
			{"component_product_id": uuid.NewString(), "percentage": 30},
		},
	})
	requireStatus(t, resp, http.StatusBadRequest)
	if msg := resp.errorMessage(); !strings.Contains(msg, "90") {
		t.Errorf("pesan galat harus menyebut jumlah persentasenya, dapat: %q", msg)
	}
}

func TestCreateBOM_Formula_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	f := mustSeedFormulaBOM(t, srv, companyID, 1000, 60, 40)

	if f.BOMType != "BATCH" {
		t.Errorf("bom_type = %q, mau BATCH", f.BOMType)
	}
	if f.BatchSize == nil || *f.BatchSize != 1000 {
		t.Fatalf("batch_size = %v, mau 1000", f.BatchSize)
	}
	if len(f.Lines) != 2 {
		t.Fatalf("mau 2 baris, dapat %d", len(f.Lines))
	}
	// quantity_per_unit baris formula adalah turunan percentage / 100 --
	// kolom lama tetap terisi supaya pembaca lama tidak melihat nol.
	if f.Lines[0].Percentage == nil || *f.Lines[0].Percentage != 60 || f.Lines[0].QuantityPerUnit != 0.6 {
		t.Errorf("baris 1 = %+v, mau percentage 60 & quantity_per_unit 0,6", f.Lines[0])
	}
	if f.TotalPercentage == nil || *f.TotalPercentage != 100 {
		t.Errorf("total_percentage = %v, mau 100", f.TotalPercentage)
	}
	if f.YieldPercent == nil || math.Abs(*f.YieldPercent-100) > 0.0001 {
		t.Errorf("yield_percent = %v, mau 100", f.YieldPercent)
	}

	// GET mengembalikan angka turunan yang sama dengan POST.
	getResp := getJSON(t, srv.URL+"/boms/"+f.ID)
	requireStatus(t, getResp, http.StatusOK)
	var fetched bomView
	getResp.decode(t, &fetched)
	if fetched.TotalPercentage == nil || *fetched.TotalPercentage != 100 || fetched.BOMType != "BATCH" {
		t.Errorf("GET /boms/{id} = %+v, mau BATCH dengan total 100%%", fetched)
	}
}

// Susut proses: 105% input untuk satu batch penuh berarti yield 95,238%.
// Angkanya dihitung service, bukan UI, supaya dua layar tidak menyimpang.
func TestCreateBOM_Formula_YieldBelowHundred(t *testing.T) {
	srv := newServer(t)
	f := mustSeedFormulaBOM(t, srv, newCompanyID(t), 1000, 63, 42)

	if f.TotalPercentage == nil || math.Abs(*f.TotalPercentage-105) > 0.0001 {
		t.Fatalf("total_percentage = %v, mau 105", f.TotalPercentage)
	}
	if f.YieldPercent == nil || math.Abs(*f.YieldPercent-95.2381) > 0.0001 {
		t.Errorf("yield_percent = %v, mau 95,2381", f.YieldPercent)
	}
}

// BOM tanpa bom_type tetap UNIT: seluruh BOM yang sudah tersimpan dan seluruh
// pemanggil lama ada di jalur ini, dan tidak boleh berubah artinya.
func TestCreateBOM_DefaultsToUnitType(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	b := mustSeedBOM(t, srv, companyID)

	resp := getJSON(t, srv.URL+"/boms/"+b.ID)
	requireStatus(t, resp, http.StatusOK)
	var fetched bomView
	resp.decode(t, &fetched)
	if fetched.BOMType != "UNIT" {
		t.Errorf("bom_type = %q, mau UNIT", fetched.BOMType)
	}
	if fetched.BatchSize != nil || fetched.TotalPercentage != nil || fetched.YieldPercent != nil {
		t.Errorf("BOM UNIT tidak boleh punya angka batch/persentase: %+v", fetched)
	}
	if fetched.Lines[0].Percentage != nil {
		t.Errorf("baris BOM UNIT tidak boleh punya percentage: %+v", fetched.Lines[0])
	}
}

type workOrderFormulaView struct {
	ID              string `json:"id"`
	QuantityPlanned float64
	BatchCount      *int `json:"batch_count"`
	Lines           []struct {
		ComponentProductID string  `json:"component_product_id"`
		QuantityRequired   float64 `json:"quantity_required"`
	} `json:"lines"`
}

// Setengah batch tidak bisa dijalankan di lantai produksi. Yang dijaga di sini
// bukan cuma status 400-nya, tapi bahwa pesannya menyebut dua angka yang sah --
// tanpa itu orangnya hanya tahu dia salah, bukan harus mengetik apa.
func TestCreateWorkOrder_Formula_RejectsPartialBatch(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	f := mustSeedFormulaBOM(t, srv, companyID, 1000, 60, 40)

	resp := postJSON(t, srv.URL+"/work-orders", map[string]any{
		"company_id": companyID, "bom_id": f.ID, "warehouse_id": uuid.NewString(),
		"quantity_planned": 1500, "planned_start_date": today(),
	})
	requireStatus(t, resp, http.StatusBadRequest)
	msg := resp.errorMessage()
	if !strings.Contains(msg, "1000") || !strings.Contains(msg, "2000") {
		t.Errorf("pesan galat harus menyebut 1000 dan 2000 sebagai angka yang sah, dapat: %q", msg)
	}

	// Di bawah satu batch juga ditolak: batch tidak bisa dijalankan separuh.
	below := postJSON(t, srv.URL+"/work-orders", map[string]any{
		"company_id": companyID, "bom_id": f.ID, "warehouse_id": uuid.NewString(),
		"quantity_planned": 400, "planned_start_date": today(),
	})
	requireStatus(t, below, http.StatusBadRequest)
}

func TestCreateWorkOrder_Formula_SnapshotsWholeBatches(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	f := mustSeedFormulaBOM(t, srv, companyID, 1000, 60, 40)

	resp := postJSON(t, srv.URL+"/work-orders", map[string]any{
		"company_id": companyID, "bom_id": f.ID, "warehouse_id": uuid.NewString(),
		"quantity_planned": 2000, "planned_start_date": today(),
	})
	requireStatus(t, resp, http.StatusCreated)
	var wo workOrderFormulaView
	resp.decode(t, &wo)

	if wo.BatchCount == nil || *wo.BatchCount != 2 {
		t.Fatalf("batch_count = %v, mau 2", wo.BatchCount)
	}
	if len(wo.Lines) != 2 {
		t.Fatalf("mau 2 baris kebutuhan, dapat %d", len(wo.Lines))
	}
	want := map[string]float64{f.ComponentA: 1200, f.ComponentB: 800}
	for _, l := range wo.Lines {
		if math.Abs(l.QuantityRequired-want[l.ComponentProductID]) > 0.0001 {
			t.Errorf("kebutuhan komponen %s = %g, mau %g", l.ComponentProductID, l.QuantityRequired, want[l.ComponentProductID])
		}
	}
}

// Persentase pecahan adalah alasan work_order_lines dihitung dari percentage,
// bukan dari quantity_per_unit turunannya: 33,3333% dari batch 1.000 x 3 batch
// = 999,999 kg. Lewat quantity_per_unit yang sudah dibulatkan ke NUMERIC(15,4)
// (0,3333) angkanya jadi 999,9 -- meleset 0,099 kg yang tidak pernah muncul
// sebagai galat, hanya sebagai stok yang pelan-pelan tidak cocok.
func TestCreateWorkOrder_Formula_KeepsFractionalPercentPrecision(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	f := mustSeedFormulaBOM(t, srv, companyID, 1000, 33.3333, 66.6667)

	resp := postJSON(t, srv.URL+"/work-orders", map[string]any{
		"company_id": companyID, "bom_id": f.ID, "warehouse_id": uuid.NewString(),
		"quantity_planned": 3000, "planned_start_date": today(),
	})
	requireStatus(t, resp, http.StatusCreated)
	var wo workOrderFormulaView
	resp.decode(t, &wo)

	var got float64
	for _, l := range wo.Lines {
		if l.ComponentProductID == f.ComponentA {
			got = l.QuantityRequired
		}
	}
	if math.Abs(got-999.999) > 0.0001 {
		t.Errorf("kebutuhan komponen A = %g, mau 999,999 (bukan 999,9 lewat quantity_per_unit yang sudah dibulatkan)", got)
	}
}

// Work order dari BOM UNIT tidak punya batch: kolomnya tetap kosong, dan
// angka kebutuhannya tetap quantity_per_unit x quantity_planned.
func TestCreateWorkOrder_UnitBOM_HasNoBatchCount(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	bom := mustSeedBOM(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/work-orders", map[string]any{
		"company_id": companyID, "bom_id": bom.ID, "warehouse_id": uuid.NewString(),
		"quantity_planned": 7, "planned_start_date": today(),
	})
	requireStatus(t, resp, http.StatusCreated)
	var wo workOrderFormulaView
	resp.decode(t, &wo)

	if wo.BatchCount != nil {
		t.Errorf("batch_count = %v, mau kosong untuk BOM UNIT", *wo.BatchCount)
	}
	if len(wo.Lines) != 1 || math.Abs(wo.Lines[0].QuantityRequired-14) > 0.0001 {
		t.Errorf("kebutuhan komponen = %+v, mau 14 (2 per unit x 7)", wo.Lines)
	}
}
