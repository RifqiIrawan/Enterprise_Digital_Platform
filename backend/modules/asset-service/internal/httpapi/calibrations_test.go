package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type calibrationView struct {
	ID                string  `json:"id"`
	AssetID           string  `json:"asset_id"`
	ScheduledDate     string  `json:"scheduled_date"`
	PerformedDate     *string `json:"performed_date"`
	IntervalMonths    *int    `json:"interval_months"`
	NextDueDate       *string `json:"next_due_date"`
	Result            *string `json:"result"`
	CertificateNumber *string `json:"certificate_number"`
	PerformedBy       *string `json:"performed_by"`
	Status            string  `json:"status"`
	Notes             string  `json:"notes"`
}

type completedCalibrationView struct {
	calibrationView
	NextCalibration *calibrationView `json:"next_calibration"`
}

func mustCreateCalibration(t *testing.T, srv *httptest.Server, companyID, assetID, scheduledDate string, intervalMonths *int) calibrationView {
	t.Helper()
	payload := map[string]any{
		"company_id": companyID, "asset_id": assetID, "scheduled_date": scheduledDate,
	}
	if intervalMonths != nil {
		payload["interval_months"] = *intervalMonths
	}
	resp := postJSON(t, srv.URL+"/calibrations", payload)
	requireStatus(t, resp, http.StatusCreated)
	var c calibrationView
	resp.decode(t, &c)
	return c
}

func intPtr(v int) *int { return &v }

func TestCreateCalibration_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)

	cases := map[string]map[string]any{
		"tanpa asset_id":       {"company_id": companyID, "scheduled_date": today()},
		"tanpa scheduled_date": {"company_id": companyID, "asset_id": asset.ID},
		"tanpa company_id":     {"asset_id": asset.ID, "scheduled_date": today()},
		"tanggal tidak valid":  {"company_id": companyID, "asset_id": asset.ID, "scheduled_date": "besok"},
		"interval nol":         {"company_id": companyID, "asset_id": asset.ID, "scheduled_date": today(), "interval_months": 0},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/calibrations", payload), http.StatusBadRequest)
		})
	}

	t.Run("aset tidak ada", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/calibrations", map[string]any{
			"company_id": companyID, "asset_id": uuid.NewString(), "scheduled_date": today(),
		}), http.StatusNotFound)
	})
	t.Run("aset milik company lain", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/calibrations", map[string]any{
			"company_id": newCompanyID(t), "asset_id": asset.ID, "scheduled_date": today(),
		}), http.StatusNotFound)
	})
	t.Run("aset sudah dilepas", func(t *testing.T) {
		disposed := mustSeedAsset(t, srv, companyID)
		mustSetAssetStatus(t, srv, disposed.ID, "DISPOSED")
		requireStatus(t, postJSON(t, srv.URL+"/calibrations", map[string]any{
			"company_id": companyID, "asset_id": disposed.ID, "scheduled_date": today(),
		}), http.StatusConflict)
	})
}

// Kalibrasi adalah kewajiban berulang: menyelesaikan satu dengan hasil lolos
// harus langsung memunculkan jadwal berikutnya.
func TestCompleteCalibration_PassSchedulesNext(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)
	c := mustCreateCalibration(t, srv, companyID, asset.ID, "2026-08-01", intPtr(6))

	resp := postJSON(t, srv.URL+"/calibrations/"+c.ID+"/complete", map[string]any{
		"performed_date": "2026-08-05", "result": "PASS",
		"certificate_number": "CAL-2026-001", "performed_by": "PT Kalibrasi Nusantara",
	})
	requireStatus(t, resp, http.StatusOK)
	var done completedCalibrationView
	resp.decode(t, &done)

	if done.Status != "COMPLETED" || done.Result == nil || *done.Result != "PASS" {
		t.Fatalf("kalibrasi selesai = %+v", done.calibrationView)
	}
	if done.CertificateNumber == nil || *done.CertificateNumber != "CAL-2026-001" {
		t.Errorf("certificate_number = %v", done.CertificateNumber)
	}
	if done.NextDueDate == nil || (*done.NextDueDate)[:10] != "2027-02-05" {
		t.Fatalf("next_due_date = %v, want 2027-02-05 (performed + 6 bulan)", done.NextDueDate)
	}
	if done.NextCalibration == nil {
		t.Fatal("kalibrasi berikutnya seharusnya dijadwalkan otomatis")
	}
	if done.NextCalibration.Status != "SCHEDULED" || done.NextCalibration.ScheduledDate[:10] != "2027-02-05" {
		t.Errorf("jadwal berikutnya = %+v", done.NextCalibration)
	}
	if done.NextCalibration.IntervalMonths == nil || *done.NextCalibration.IntervalMonths != 6 {
		t.Errorf("interval jadwal berikutnya = %v, want ikut terbawa", done.NextCalibration.IntervalMonths)
	}

	var all []calibrationView
	getJSON(t, srv.URL+"/calibrations?company_id="+companyID).decode(t, &all)
	if len(all) != 2 {
		t.Fatalf("daftar kalibrasi = %d, want 2 (yang selesai + yang otomatis)", len(all))
	}

	// Asetnya tidak disentuh: lolos kalibrasi bukan kejadian yang mengubah status.
	if fetchAsset(t, srv, companyID, asset.ID).Status != "ACTIVE" {
		t.Error("status aset seharusnya tetap ACTIVE setelah kalibrasi PASS")
	}
}

// FAIL: asetnya ditarik ke MAINTENANCE, dan justru TIDAK dijadwalkan ulang di
// interval normal -- alat yang gagal kalibrasi butuh perbaikan dulu.
func TestCompleteCalibration_FailFlagsAssetAndSkipsRescheduling(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)
	c := mustCreateCalibration(t, srv, companyID, asset.ID, "2026-08-01", intPtr(6))

	resp := postJSON(t, srv.URL+"/calibrations/"+c.ID+"/complete", map[string]any{
		"performed_date": "2026-08-05", "result": "FAIL", "notes": "penyimpangan di luar batas",
	})
	requireStatus(t, resp, http.StatusOK)
	var done completedCalibrationView
	resp.decode(t, &done)

	if done.NextCalibration != nil {
		t.Errorf("kalibrasi gagal tidak boleh dijadwalkan ulang otomatis, dapat %+v", done.NextCalibration)
	}
	if done.NextDueDate != nil {
		t.Errorf("next_due_date = %v, want kosong untuk hasil FAIL", done.NextDueDate)
	}
	if fetchAsset(t, srv, companyID, asset.ID).Status != "MAINTENANCE" {
		t.Error("aset yang gagal kalibrasi seharusnya berstatus MAINTENANCE")
	}

	var all []calibrationView
	getJSON(t, srv.URL+"/calibrations?company_id="+companyID).decode(t, &all)
	if len(all) != 1 {
		t.Errorf("daftar kalibrasi = %d, want tetap 1", len(all))
	}
}

func TestCompleteCalibration_WithoutIntervalDoesNotReschedule(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)
	c := mustCreateCalibration(t, srv, companyID, asset.ID, "2026-08-01", nil)

	resp := postJSON(t, srv.URL+"/calibrations/"+c.ID+"/complete", map[string]any{"result": "ADJUSTED"})
	requireStatus(t, resp, http.StatusOK)
	var done completedCalibrationView
	resp.decode(t, &done)

	if done.NextCalibration != nil || done.NextDueDate != nil {
		t.Errorf("kalibrasi sekali-jalan tidak boleh menjadwalkan ulang: %+v", done)
	}
	// performed_date yang tidak diisi jatuh ke hari ini.
	if done.PerformedDate == nil || (*done.PerformedDate)[:10] != today() {
		t.Errorf("performed_date = %v, want hari ini", done.PerformedDate)
	}
}

func TestCompleteCalibration_ValidationAndStatus(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)
	c := mustCreateCalibration(t, srv, companyID, asset.ID, "2026-08-01", intPtr(12))

	requireStatus(t, postJSON(t, srv.URL+"/calibrations/"+c.ID+"/complete", map[string]any{"result": "LULUS"}), http.StatusBadRequest)
	requireStatus(t, postJSON(t, srv.URL+"/calibrations/"+c.ID+"/complete", map[string]any{}), http.StatusBadRequest)
	requireStatus(t, postJSON(t, srv.URL+"/calibrations/"+c.ID+"/complete", map[string]any{
		"result": "PASS", "performed_date": "kemarin",
	}), http.StatusBadRequest)
	requireStatus(t, postJSON(t, srv.URL+"/calibrations/"+uuid.NewString()+"/complete", map[string]any{"result": "PASS"}), http.StatusNotFound)

	requireStatus(t, postJSON(t, srv.URL+"/calibrations/"+c.ID+"/complete", map[string]any{"result": "PASS"}), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/calibrations/"+c.ID+"/complete", map[string]any{"result": "PASS"}), http.StatusConflict)
}

func TestCancelCalibration(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	asset := mustSeedAsset(t, srv, companyID)
	c := mustCreateCalibration(t, srv, companyID, asset.ID, "2026-08-01", nil)

	resp := postJSON(t, srv.URL+"/calibrations/"+c.ID+"/cancel", nil)
	requireStatus(t, resp, http.StatusOK)
	var cancelled calibrationView
	resp.decode(t, &cancelled)
	if cancelled.Status != "CANCELLED" {
		t.Errorf("status = %q, want CANCELLED", cancelled.Status)
	}

	requireStatus(t, postJSON(t, srv.URL+"/calibrations/"+c.ID+"/cancel", nil), http.StatusConflict)
	requireStatus(t, postJSON(t, srv.URL+"/calibrations/"+c.ID+"/complete", map[string]any{"result": "PASS"}), http.StatusConflict)
	requireStatus(t, postJSON(t, srv.URL+"/calibrations/"+uuid.NewString()+"/cancel", nil), http.StatusConflict)
}

func TestListCalibrations_Filters(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	assetA := mustSeedAsset(t, srv, companyID)
	assetB := mustSeedAsset(t, srv, companyID)
	first := mustCreateCalibration(t, srv, companyID, assetA.ID, "2026-01-15", nil)
	mustCreateCalibration(t, srv, companyID, assetB.ID, "2026-09-15", nil)
	requireStatus(t, postJSON(t, srv.URL+"/calibrations/"+first.ID+"/cancel", nil), http.StatusOK)

	var byAsset []calibrationView
	getJSON(t, srv.URL+"/calibrations?company_id="+companyID+"&asset_id="+assetA.ID).decode(t, &byAsset)
	if len(byAsset) != 1 || byAsset[0].AssetID != assetA.ID {
		t.Fatalf("filter aset = %+v", byAsset)
	}

	var scheduled []calibrationView
	getJSON(t, srv.URL+"/calibrations?company_id="+companyID+"&status=SCHEDULED").decode(t, &scheduled)
	if len(scheduled) != 1 || scheduled[0].AssetID != assetB.ID {
		t.Fatalf("filter status = %+v", scheduled)
	}

	var due []calibrationView
	getJSON(t, srv.URL+"/calibrations?company_id="+companyID+"&due_before=2026-06-30").decode(t, &due)
	if len(due) != 1 || due[0].ID != first.ID {
		t.Fatalf("filter due_before = %+v", due)
	}

	var otherCompany []calibrationView
	getJSON(t, srv.URL+"/calibrations?company_id="+newCompanyID(t)).decode(t, &otherCompany)
	if len(otherCompany) != 0 {
		t.Errorf("kalibrasi company lain ikut terbawa: %+v", otherCompany)
	}

	requireStatus(t, getJSON(t, srv.URL+"/calibrations"), http.StatusBadRequest)
}
