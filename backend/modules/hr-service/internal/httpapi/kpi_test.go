package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type indicatorView struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Unit        string  `json:"unit"`
	TargetValue float64 `json:"target_value"`
	Weight      float64 `json:"weight"`
	IsActive    bool    `json:"is_active"`
}

type reviewItemView struct {
	IndicatorID   string  `json:"indicator_id"`
	IndicatorName string  `json:"indicator_name"`
	TargetValue   float64 `json:"target_value"`
	Weight        float64 `json:"weight"`
	ActualValue   float64 `json:"actual_value"`
	Achievement   float64 `json:"achievement"`
	Score         float64 `json:"score"`
}

type reviewView struct {
	ID              string           `json:"id"`
	EmployeeID      string           `json:"employee_id"`
	EmployeeName    string           `json:"employee_name"`
	Period          string           `json:"period"`
	Status          string           `json:"status"`
	TotalScore      float64          `json:"total_score"`
	Rating          string           `json:"rating"`
	Notes           string           `json:"notes"`
	RejectionReason string           `json:"rejection_reason"`
	Items           []reviewItemView `json:"items"`
	TotalWeight     float64          `json:"total_weight"`
}

func mustCreateIndicator(t *testing.T, srv *httptest.Server, companyID, code string, target, weight float64) indicatorView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/kpi-indicators", map[string]any{
		"company_id": companyID, "code": code, "name": "Indikator " + code,
		"unit": "poin", "target_value": target, "weight": weight,
	})
	requireStatus(t, resp, http.StatusCreated)
	var i indicatorView
	resp.decode(t, &i)
	return i
}

func mustCreateReview(t *testing.T, srv *httptest.Server, companyID, employeeID, period string) reviewView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/kpi-reviews", map[string]any{
		"company_id": companyID, "employee_id": employeeID, "period": period,
	})
	requireStatus(t, resp, http.StatusCreated)
	var v reviewView
	resp.decode(t, &v)
	return v
}

func reviewDetail(t *testing.T, srv *httptest.Server, id string) reviewView {
	t.Helper()
	resp := getJSON(t, srv.URL+"/kpi-reviews/"+id)
	requireStatus(t, resp, http.StatusOK)
	var v reviewView
	resp.decode(t, &v)
	return v
}

func TestCreateKPIIndicator_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"tanpa company", map[string]any{"code": "A", "name": "A", "target_value": 10, "weight": 50}},
		{"tanpa code", map[string]any{"company_id": companyID, "name": "A", "target_value": 10, "weight": 50}},
		{"tanpa nama", map[string]any{"company_id": companyID, "code": "A", "target_value": 10, "weight": 50}},
		{"target 0", map[string]any{"company_id": companyID, "code": "A", "name": "A", "target_value": 0, "weight": 50}},
		{"bobot 0", map[string]any{"company_id": companyID, "code": "A", "name": "A", "target_value": 10, "weight": 0}},
		{"bobot > 100", map[string]any{"company_id": companyID, "code": "A", "name": "A", "target_value": 10, "weight": 101}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/kpi-indicators", tc.payload), http.StatusBadRequest)
		})
	}
}

// Code dinormalkan (huruf besar, spasi jadi underscore) dan unik per company.
func TestCreateKPIIndicator_NormalizesCodeAndRejectsDuplicate(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	resp := postJSON(t, srv.URL+"/kpi-indicators", map[string]any{
		"company_id": companyA, "code": "  omzet bulanan ", "name": "Omzet Bulanan",
		"target_value": 100000000, "weight": 40,
	})
	requireStatus(t, resp, http.StatusCreated)
	var i indicatorView
	resp.decode(t, &i)
	if i.Code != "OMZET_BULANAN" {
		t.Fatalf("expected code OMZET_BULANAN, got %q", i.Code)
	}
	if i.Unit != "poin" {
		t.Errorf("expected unit default 'poin', got %q", i.Unit)
	}

	dup := postJSON(t, srv.URL+"/kpi-indicators", map[string]any{
		"company_id": companyA, "code": "OMZET_BULANAN", "name": "Duplikat",
		"target_value": 1, "weight": 1,
	})
	requireStatus(t, dup, http.StatusConflict)

	// Company lain bebas memakai code yang sama.
	mustCreateIndicator(t, srv, companyB, "OMZET_BULANAN", 1, 1)
}

func TestListKPIIndicators_ActiveOnlyFilter(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	active := mustCreateIndicator(t, srv, companyID, "AKTIF", 10, 50)
	inactive := mustCreateIndicator(t, srv, companyID, "NONAKTIF", 10, 50)

	requireStatus(t, putJSON(t, srv.URL+"/kpi-indicators/"+inactive.ID, map[string]any{
		"name": "Indikator NONAKTIF", "unit": "poin", "target_value": 10, "weight": 50, "is_active": false,
	}), http.StatusOK)

	var all []indicatorView
	getJSON(t, srv.URL+"/kpi-indicators?company_id="+companyID).decode(t, &all)
	if len(all) != 2 {
		t.Fatalf("expected 2 indikator tanpa filter, got %d", len(all))
	}

	var onlyActive []indicatorView
	getJSON(t, srv.URL+"/kpi-indicators?company_id="+companyID+"&active_only=true").decode(t, &onlyActive)
	if len(onlyActive) != 1 || onlyActive[0].ID != active.ID {
		t.Fatalf("expected hanya indikator aktif, got %+v", onlyActive)
	}
}

func TestListKPIIndicators_RequiresCompany(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, getJSON(t, srv.URL+"/kpi-indicators"), http.StatusBadRequest)
}

// Indikator yang sudah dipakai penilaian tidak boleh dihapus -- penilaian lama
// harus tetap bisa dibaca utuh.
func TestDeleteKPIIndicator_BlockedWhenUsed(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	ind := mustCreateIndicator(t, srv, companyID, "DIPAKAI", 10, 100)

	unused := mustCreateIndicator(t, srv, companyID, "BELUM_DIPAKAI", 10, 10)
	requireStatus(t, doRequest(t, http.MethodDelete, srv.URL+"/kpi-indicators/"+unused.ID, nil, ""), http.StatusOK)

	mustCreateReview(t, srv, companyID, emp.ID, "2026-08")

	resp := doRequest(t, http.MethodDelete, srv.URL+"/kpi-indicators/"+ind.ID, nil, "")
	requireStatus(t, resp, http.StatusConflict)

	requireStatus(t, doRequest(t, http.MethodDelete, srv.URL+"/kpi-indicators/"+uuid.NewString(), nil, ""), http.StatusNotFound)
}

// Penilaian baru menyalin seluruh indikator AKTIF sebagai rinciannya.
func TestCreateKPIReview_SnapshotsActiveIndicators(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	mustCreateIndicator(t, srv, companyID, "OMZET", 100, 60)
	mustCreateIndicator(t, srv, companyID, "KEHADIRAN", 20, 40)

	v := mustCreateReview(t, srv, companyID, emp.ID, "2026-08")
	if v.Status != "DRAFT" || v.EmployeeName == "" {
		t.Fatalf("penilaian baru tidak sesuai: %+v", v)
	}

	detail := reviewDetail(t, srv, v.ID)
	if len(detail.Items) != 2 {
		t.Fatalf("expected 2 rincian indikator, got %d", len(detail.Items))
	}
	if detail.TotalWeight != 100 {
		t.Errorf("expected total bobot 100, got %v", detail.TotalWeight)
	}
}

// Salinan itulah yang membuat penilaian lama tidak berubah saat master
// indikator disesuaikan.
func TestKPIReview_ItemsKeepSnapshotWhenIndicatorChanges(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	ind := mustCreateIndicator(t, srv, companyID, "OMZET", 100, 100)

	v := mustCreateReview(t, srv, companyID, emp.ID, "2026-08")

	requireStatus(t, putJSON(t, srv.URL+"/kpi-indicators/"+ind.ID, map[string]any{
		"name": "Omzet (target baru)", "unit": "poin", "target_value": 999, "weight": 10,
	}), http.StatusOK)

	detail := reviewDetail(t, srv, v.ID)
	if detail.Items[0].TargetValue != 100 || detail.Items[0].Weight != 100 {
		t.Fatalf("rincian ikut berubah saat master diubah: %+v", detail.Items[0])
	}
}

func TestCreateKPIReview_ValidationAndDuplicates(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)

	// Belum ada indikator sama sekali.
	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "period": "2026-08",
	}), http.StatusBadRequest)

	mustCreateIndicator(t, srv, companyID, "OMZET", 100, 100)

	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "period": "2026-8",
	}), http.StatusBadRequest)
	// 400, bukan 404: activeEmployee memperlakukan karyawan tak dikenal sebagai
	// payload yang salah, sama seperti di cuti & lembur.
	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews", map[string]any{
		"company_id": companyID, "employee_id": uuid.NewString(), "period": "2026-08",
	}), http.StatusBadRequest)

	mustCreateReview(t, srv, companyID, emp.ID, "2026-08")
	// Satu penilaian per karyawan per periode.
	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews", map[string]any{
		"company_id": companyID, "employee_id": emp.ID, "period": "2026-08",
	}), http.StatusConflict)
}

// Inti perhitungannya: achievement = actual/target*100, score = achievement *
// bobot/100, total = jumlah score, rating mengikuti total.
func TestPutKPIScores_ComputesAchievementScoreAndRating(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	omzet := mustCreateIndicator(t, srv, companyID, "OMZET", 100, 60)
	hadir := mustCreateIndicator(t, srv, companyID, "KEHADIRAN", 20, 40)

	v := mustCreateReview(t, srv, companyID, emp.ID, "2026-08")

	// Omzet 80/100 = 80% x bobot 60 = 48; kehadiran 20/20 = 100% x 40 = 40.
	// Total 88 -> rating BAIK (>= 75, < 90).
	resp := putJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/scores", map[string]any{
		"items": []map[string]any{
			{"indicator_id": omzet.ID, "actual_value": 80},
			{"indicator_id": hadir.ID, "actual_value": 20},
		},
		"notes": "penilaian triwulan",
	})
	requireStatus(t, resp, http.StatusOK)

	var after reviewView
	resp.decode(t, &after)
	if after.TotalScore != 88 {
		t.Fatalf("expected total 88, got %v", after.TotalScore)
	}
	if after.Rating != "BAIK" {
		t.Fatalf("expected rating BAIK, got %q", after.Rating)
	}
	if after.Notes != "penilaian triwulan" {
		t.Errorf("catatan tidak tersimpan: %q", after.Notes)
	}

	detail := reviewDetail(t, srv, v.ID)
	for _, it := range detail.Items {
		if it.IndicatorID == omzet.ID && (it.Achievement != 80 || it.Score != 48) {
			t.Errorf("omzet: expected 80%% & score 48, got %v & %v", it.Achievement, it.Score)
		}
		if it.IndicatorID == hadir.ID && (it.Achievement != 100 || it.Score != 40) {
			t.Errorf("kehadiran: expected 100%% & score 40, got %v & %v", it.Achievement, it.Score)
		}
	}
}

// Pencapaian di atas target tetap dihargai, tapi dibatasi supaya satu indikator
// yang meledak tidak menutupi indikator lain yang gagal total.
func TestPutKPIScores_AchievementIsCapped(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	ind := mustCreateIndicator(t, srv, companyID, "OMZET", 100, 100)

	v := mustCreateReview(t, srv, companyID, emp.ID, "2026-08")
	requireStatus(t, putJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/scores", map[string]any{
		"items": []map[string]any{{"indicator_id": ind.ID, "actual_value": 500}},
	}), http.StatusOK)

	detail := reviewDetail(t, srv, v.ID)
	if detail.Items[0].Achievement != 150 {
		t.Fatalf("expected pencapaian dibatasi 150, got %v", detail.Items[0].Achievement)
	}
	if detail.TotalScore != 150 {
		t.Fatalf("expected total 150, got %v", detail.TotalScore)
	}
	if detail.Rating != "SANGAT BAIK" {
		t.Errorf("expected rating SANGAT BAIK, got %q", detail.Rating)
	}
}

func TestPutKPIScores_RejectsForeignIndicatorAndNegativeValue(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	ind := mustCreateIndicator(t, srv, companyID, "OMZET", 100, 100)
	v := mustCreateReview(t, srv, companyID, emp.ID, "2026-08")

	requireStatus(t, putJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/scores", map[string]any{
		"items": []map[string]any{{"indicator_id": uuid.NewString(), "actual_value": 10}},
	}), http.StatusBadRequest)

	requireStatus(t, putJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/scores", map[string]any{
		"items": []map[string]any{{"indicator_id": ind.ID, "actual_value": -1}},
	}), http.StatusBadRequest)

	requireStatus(t, putJSON(t, srv.URL+"/kpi-reviews/"+uuid.NewString()+"/scores", map[string]any{
		"items": []map[string]any{},
	}), http.StatusNotFound)
}

// Alur status lengkap, termasuk pagar "nilai dikunci setelah diajukan".
func TestKPIReviewWorkflow_StatusTransitions(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	ind := mustCreateIndicator(t, srv, companyID, "OMZET", 100, 100)
	v := mustCreateReview(t, srv, companyID, emp.ID, "2026-08")

	requireStatus(t, putJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/scores", map[string]any{
		"items": []map[string]any{{"indicator_id": ind.ID, "actual_value": 90}},
	}), http.StatusOK)

	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/submit", nil), http.StatusOK)

	// Setelah diajukan, nilainya terkunci.
	requireStatus(t, putJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/scores", map[string]any{
		"items": []map[string]any{{"indicator_id": ind.ID, "actual_value": 100}},
	}), http.StatusConflict)

	// Menolak wajib menyertakan alasan.
	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/reject", map[string]any{}), http.StatusBadRequest)
	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/reject", map[string]any{
		"rejection_reason": "angka omzet belum diverifikasi",
	}), http.StatusOK)

	// Yang ditolak boleh diperbaiki lalu diajukan lagi.
	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/submit", nil), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/approve", nil), http.StatusOK)

	final := reviewDetail(t, srv, v.ID)
	if final.Status != "APPROVED" {
		t.Fatalf("expected APPROVED, got %q", final.Status)
	}
	// Alasan penolakan lama dibersihkan saat diajukan ulang.
	if final.RejectionReason != "" {
		t.Errorf("expected rejection_reason kosong setelah disetujui, got %q", final.RejectionReason)
	}
	// Yang sudah disetujui tidak bisa disetujui atau diajukan lagi.
	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/approve", nil), http.StatusConflict)
	requireStatus(t, postJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/submit", nil), http.StatusConflict)
}

// Bobot yang tidak genap 100% membuat nilai akhir tidak bisa dibandingkan
// antar karyawan, jadi pengajuannya ditahan.
func TestSubmitKPIReview_BlockedWhenWeightsDoNotSumTo100(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	mustCreateIndicator(t, srv, companyID, "OMZET", 100, 60)

	v := mustCreateReview(t, srv, companyID, emp.ID, "2026-08")
	resp := postJSON(t, srv.URL+"/kpi-reviews/"+v.ID+"/submit", nil)
	requireStatus(t, resp, http.StatusConflict)
	if msg := resp.errorMessage(); msg == "" {
		t.Error("expected pesan yang menyebut total bobot")
	}
}

func TestListKPIReviews_FilterByPeriodAndEmployee(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	empA := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	empB := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	mustCreateIndicator(t, srv, companyID, "OMZET", 100, 100)

	mustCreateReview(t, srv, companyID, empA.ID, "2026-07")
	mustCreateReview(t, srv, companyID, empA.ID, "2026-08")
	mustCreateReview(t, srv, companyID, empB.ID, "2026-08")

	var all []reviewView
	getJSON(t, srv.URL+"/kpi-reviews?company_id="+companyID).decode(t, &all)
	if len(all) != 3 {
		t.Fatalf("expected 3 penilaian, got %d", len(all))
	}
	// Urutan: periode terbaru dulu.
	if all[0].Period != "2026-08" {
		t.Errorf("expected periode terbaru di atas, got %q", all[0].Period)
	}

	var byPeriod []reviewView
	getJSON(t, srv.URL+"/kpi-reviews?company_id="+companyID+"&period=2026-07").decode(t, &byPeriod)
	if len(byPeriod) != 1 {
		t.Fatalf("expected 1 penilaian di 2026-07, got %d", len(byPeriod))
	}

	var byEmployee []reviewView
	getJSON(t, srv.URL+"/kpi-reviews?company_id="+companyID+"&employee_id="+empB.ID).decode(t, &byEmployee)
	if len(byEmployee) != 1 || byEmployee[0].EmployeeID != empB.ID {
		t.Fatalf("filter karyawan tidak bekerja: %+v", byEmployee)
	}

	requireStatus(t, getJSON(t, srv.URL+"/kpi-reviews"), http.StatusBadRequest)
	requireStatus(t, getJSON(t, srv.URL+"/kpi-reviews?company_id="+companyID+"&period=agustus"), http.StatusBadRequest)
}

func TestGetKPIReview_NotFound(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, getJSON(t, srv.URL+"/kpi-reviews/"+uuid.NewString()), http.StatusNotFound)
}
