package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Dashboard per peran (Fase 9) hanya meminta bagian yang dipakainya. Yang
// diuji di sini bukan sekadar bentuk JSON-nya, tapi bahwa service yang tidak
// diminta memang TIDAK dihubungi sama sekali -- itu keseluruhan alasan
// parameternya ada.
func TestDashboardSummary_SectionsLimitFanOut(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)

	resp := getJSON(t, srv.URL+"/dashboards/summary?company_id="+companyID+"&sections=sales,finance")
	requireStatus(t, resp, http.StatusOK)

	var body map[string]json.RawMessage
	resp.decode(t, &body)
	for _, present := range []string{"sales", "finance"} {
		if _, ok := body[present]; !ok {
			t.Errorf("bagian %q hilang dari respons", present)
		}
	}
	// Bagian yang tidak diminta HILANG, bukan terkirim berisi nol -- nol
	// adalah jawaban yang sah dan tidak boleh dikira jawaban.
	for _, absent := range []string{"purchasing", "warehouse", "production", "qc", "hr", "asset"} {
		if _, ok := body[absent]; ok {
			t.Errorf("bagian %q seharusnya tidak ada di respons", absent)
		}
	}

	if backends.sales.callCount() == 0 || backends.finance.callCount() == 0 {
		t.Error("sales-service & finance-service seharusnya dihubungi")
	}
	for name, fs := range map[string]*fakeService{
		"purchasing": backends.purchasing, "warehouse": backends.warehouse,
		"production": backends.production, "qc": backends.qc, "hr": backends.hr, "asset": backends.asset,
	} {
		if n := fs.callCount(); n != 0 {
			t.Errorf("%s-service dihubungi %d kali padahal tidak diminta", name, n)
		}
	}
}

func TestDashboardSummary_DefaultsToEverySection(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)

	resp := getJSON(t, srv.URL+"/dashboards/summary?company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)

	var body struct {
		Sections []string `json:"sections"`
	}
	resp.decode(t, &body)
	if len(body.Sections) != 8 {
		t.Fatalf("sections = %v, want 8 bagian saat parameternya tidak diisi", body.Sections)
	}

	var full map[string]json.RawMessage
	resp.decode(t, &full)
	for _, name := range body.Sections {
		if _, ok := full[name]; !ok {
			t.Errorf("bagian %q disebut di sections tapi tidak ada isinya", name)
		}
	}
	for name, fs := range map[string]*fakeService{
		"sales": backends.sales, "purchasing": backends.purchasing, "finance": backends.finance,
		"warehouse": backends.warehouse, "production": backends.production, "qc": backends.qc,
		"hr": backends.hr, "asset": backends.asset,
	} {
		if fs.callCount() == 0 {
			t.Errorf("%s-service tidak dihubungi pada permintaan tanpa filter", name)
		}
	}
}

// Salah ketik nama bagian ditolak, bukan diabaikan: bagian yang diam-diam
// hilang dari dashboard jauh lebih sulit dilacak daripada 400 yang menyebut
// namanya.
func TestDashboardSummary_UnknownSectionRejected(t *testing.T) {
	srv, _ := newServer(t)
	companyID := newCompanyID(t)

	resp := getJSON(t, srv.URL+"/dashboards/summary?company_id="+companyID+"&sections=penjualan")
	requireStatus(t, resp, http.StatusBadRequest)
	if msg := resp.errorMessage(); msg == "" {
		t.Error("pesan galat seharusnya menyebut nama bagian yang tidak dikenal")
	}

	requireStatus(t, getJSON(t, srv.URL+"/dashboards/summary?company_id="+companyID+"&sections=,"), http.StatusBadRequest)
}

// Service yang mati di bagian yang tidak diminta tidak boleh muncul sebagai
// galat di dashboard yang tidak memakainya.
func TestDashboardSummary_ErrorsOnlyFromRequestedSections(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)
	backends.asset.fail("/assets")

	var scoped struct {
		Errors []struct {
			Source string `json:"source"`
		} `json:"errors"`
	}
	getJSON(t, srv.URL+"/dashboards/summary?company_id="+companyID+"&sections=sales").decode(t, &scoped)
	if len(scoped.Errors) != 0 {
		t.Errorf("errors = %+v, want kosong -- asset-service tidak diminta", scoped.Errors)
	}

	var withAsset struct {
		Errors []struct {
			Source string `json:"source"`
		} `json:"errors"`
	}
	getJSON(t, srv.URL+"/dashboards/summary?company_id="+companyID+"&sections=asset").decode(t, &withAsset)
	if len(withAsset.Errors) != 1 || withAsset.Errors[0].Source != "asset-service" {
		t.Errorf("errors = %+v, want satu galat dari asset-service", withAsset.Errors)
	}
}
