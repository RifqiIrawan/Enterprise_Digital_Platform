package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

type riskFactorView struct {
	Code         string  `json:"code"`
	Contribution float64 `json:"contribution"`
	Detail       string  `json:"detail"`
}

type riskItemView struct {
	SubjectType string           `json:"subject_type"`
	SubjectID   string           `json:"subject_id"`
	Code        string           `json:"code"`
	Name        string           `json:"name"`
	RiskScore   float64          `json:"risk_score"`
	RiskLevel   string           `json:"risk_level"`
	Factors     []riskFactorView `json:"factors"`
	Action      string           `json:"recommended_action"`
}

type predictiveView struct {
	Items  []riskItemView `json:"items"`
	Errors []struct {
		Source string `json:"source"`
	} `json:"errors"`
}

func (v predictiveView) byCode(code string) (riskItemView, bool) {
	for _, item := range v.Items {
		if item.Code == code {
			return item, true
		}
	}
	return riskItemView{}, false
}

func (item riskItemView) factor(code string) (riskFactorView, bool) {
	for _, f := range item.Factors {
		if f.Code == code {
			return f, true
		}
	}
	return riskFactorView{}, false
}

func daysAgo(n int) string {
	return time.Now().AddDate(0, 0, -n).Format("2006-01-02")
}

func TestPredictiveMaintenance_MissingCompanyID(t *testing.T) {
	srv, _ := newServer(t)
	requireStatus(t, getJSON(t, srv.URL+"/predictive-maintenance/scan"), http.StatusBadRequest)
}

// Availability rendah adalah sinyal terkuat yang dimiliki platform ini tentang
// kesehatan mesin; skornya harus bisa ditelusuri ke faktor itu, bukan muncul
// begitu saja.
func TestPredictiveMaintenance_ScoresMachineFromOEE(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)

	backends.production.json("/oee", http.StatusOK, map[string]any{
		"machines": []map[string]any{
			// availability 0,60 -> (0,85-0,60)*200 = 50, dipotong 45.
			// quality 0,90     -> (0,95-0,90)*200 = 10.
			{
				"machine_id": "mc-1", "machine_code": "MC-001", "machine_name": "Mesin Cetak",
				"run_count": 8, "availability": 0.6, "quality": 0.9, "oee": 0.5,
				"downtime_minutes": 168, "planned_minutes": 420,
			},
			// Mesin sehat: tidak boleh muncul sama sekali.
			{
				"machine_id": "mc-2", "machine_code": "MC-002", "machine_name": "Mesin Potong",
				"run_count": 10, "availability": 0.97, "quality": 0.99, "oee": 0.9,
				"downtime_minutes": 12, "planned_minutes": 420,
			},
			// Belum pernah ada run tertutup: tidak ada bukti, jadi tidak dinilai.
			{
				"machine_id": "mc-3", "machine_code": "MC-003", "machine_name": "Mesin Baru",
				"run_count": 0, "availability": 0, "quality": 0, "oee": 0,
			},
		},
	})
	backends.production.json("/machines", http.StatusOK, []map[string]any{
		{"id": "mc-1", "code": "MC-001", "name": "Mesin Cetak", "status": "ACTIVE"},
	})

	var body predictiveView
	getJSON(t, srv.URL+"/predictive-maintenance/scan?company_id="+companyID).decode(t, &body)

	if len(body.Items) != 1 {
		t.Fatalf("items = %+v, want hanya MC-001", body.Items)
	}
	item := body.Items[0]
	if item.SubjectType != "machine" || item.SubjectID != "mc-1" {
		t.Errorf("subjek = %+v", item)
	}
	if item.RiskScore != 55 {
		t.Errorf("risk_score = %v, want 55 (45 availability + 10 quality)", item.RiskScore)
	}
	if item.RiskLevel != "MEDIUM" {
		t.Errorf("risk_level = %q, want MEDIUM", item.RiskLevel)
	}
	f, ok := item.factor("LOW_AVAILABILITY")
	if !ok || f.Contribution != 45 {
		t.Errorf("faktor availability = %+v, want kontribusi 45 (dipotong dari 50)", f)
	}
	if f.Detail == "" {
		t.Error("faktor harus menjelaskan dirinya sendiri, bukan cuma angka")
	}
	if _, ok := item.factor("QUALITY_DRAG"); !ok {
		t.Error("faktor reject tinggi hilang")
	}
	if item.Action == "" {
		t.Error("recommended_action kosong")
	}
}

// Status mesin menambah bobot, dan bersamaan dengan availability rendah
// itulah yang mendorong sebuah mesin ke HIGH -- satu faktor saja tidak cukup.
func TestPredictiveMaintenance_MachineInMaintenanceReachesHigh(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)

	backends.production.json("/oee", http.StatusOK, map[string]any{
		"machines": []map[string]any{{
			"machine_id": "mc-1", "machine_code": "MC-001", "machine_name": "Mesin Cetak",
			"run_count": 5, "availability": 0.5, "quality": 0.8, "oee": 0.3,
			"downtime_minutes": 210, "planned_minutes": 420,
		}},
	})
	backends.production.json("/machines", http.StatusOK, []map[string]any{
		{"id": "mc-1", "code": "MC-001", "name": "Mesin Cetak", "status": "MAINTENANCE"},
	})

	var body predictiveView
	getJSON(t, srv.URL+"/predictive-maintenance/scan?company_id="+companyID).decode(t, &body)

	item, ok := body.byCode("MC-001")
	if !ok {
		t.Fatalf("MC-001 tidak ada di hasil: %+v", body.Items)
	}
	// 45 (availability dipotong) + 30 (quality dipotong 20) + 15 (status) = 80
	if item.RiskScore != 80 || item.RiskLevel != "HIGH" {
		t.Errorf("risk = %v (%s), want 80 HIGH", item.RiskScore, item.RiskLevel)
	}
	if _, ok := item.factor("IN_MAINTENANCE"); !ok {
		t.Error("status MAINTENANCE seharusnya jadi faktor tersendiri")
	}
}

func TestPredictiveMaintenance_ScoresAssetsFromScheduleAndLifetime(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)

	life := 24
	backends.asset.json("/assets", http.StatusOK, []map[string]any{
		// Terlambat 30 hari + belum pernah dirawat.
		{"id": "as-1", "asset_code": "AST-001", "name": "Genset", "status": "ACTIVE", "acquisition_cost": 100, "salvage_value": 0, "useful_life_months": life, "accumulated_depreciation": 10, "book_value": 90},
		// Sudah dirawat, tidak terlambat, tapi nilai bukunya tinggal 5%.
		{"id": "as-2", "asset_code": "AST-002", "name": "Forklift", "status": "ACTIVE", "acquisition_cost": 100, "salvage_value": 0, "useful_life_months": life, "accumulated_depreciation": 95, "book_value": 5},
		// Sudah dilepas: tidak dinilai sama sekali.
		{"id": "as-3", "asset_code": "AST-003", "name": "Mesin Lama", "status": "DISPOSED", "acquisition_cost": 100, "salvage_value": 0, "useful_life_months": life, "accumulated_depreciation": 100, "book_value": 0},
	})
	backends.asset.json("/maintenance-schedules", http.StatusOK, []map[string]any{
		{"id": "ms-1", "asset_id": "as-1", "scheduled_date": daysAgo(30), "status": "SCHEDULED"},
		{"id": "ms-2", "asset_id": "as-2", "scheduled_date": daysAgo(200), "status": "COMPLETED"},
	})

	var body predictiveView
	getJSON(t, srv.URL+"/predictive-maintenance/scan?company_id="+companyID).decode(t, &body)

	if _, ok := body.byCode("AST-003"); ok {
		t.Error("aset DISPOSED tidak boleh ikut dinilai")
	}

	late, ok := body.byCode("AST-001")
	if !ok {
		t.Fatalf("AST-001 tidak ada di hasil: %+v", body.Items)
	}
	f, _ := late.factor("OVERDUE_MAINTENANCE")
	// 30 dari 60 hari -> setengah dari 45 poin.
	if f.Contribution != 22.5 {
		t.Errorf("kontribusi keterlambatan = %v, want 22.5", f.Contribution)
	}
	if _, ok := late.factor("NEVER_MAINTAINED"); !ok {
		t.Error("aset tanpa riwayat maintenance seharusnya ditandai")
	}
	if late.RiskScore != 42.5 || late.RiskLevel != "MEDIUM" {
		t.Errorf("risk = %v (%s), want 42.5 MEDIUM", late.RiskScore, late.RiskLevel)
	}

	old, ok := body.byCode("AST-002")
	if !ok {
		t.Fatalf("AST-002 tidak ada di hasil: %+v", body.Items)
	}
	if _, ok := old.factor("END_OF_LIFE"); !ok {
		t.Errorf("aset dengan nilai buku 5%% seharusnya ditandai mendekati akhir umur manfaat: %+v", old.Factors)
	}
	if _, ok := old.factor("OVERDUE_MAINTENANCE"); ok {
		t.Error("jadwal yang sudah COMPLETED tidak boleh dihitung terlambat")
	}

	// Yang paling berisiko lebih dulu.
	if body.Items[0].RiskScore < body.Items[len(body.Items)-1].RiskScore {
		t.Errorf("hasil tidak terurut dari risiko tertinggi: %+v", body.Items)
	}
}

// Satu service mati tidak boleh mengosongkan hasil dari service lain -- pola
// toleransi kegagalan sebagian yang sama dengan dashboard summary.
func TestPredictiveMaintenance_PartialFailure(t *testing.T) {
	srv, backends := newServer(t)
	companyID := newCompanyID(t)

	backends.production.fail("/oee")
	backends.asset.json("/assets", http.StatusOK, []map[string]any{
		{"id": "as-1", "asset_code": "AST-001", "name": "Genset", "status": "MAINTENANCE", "acquisition_cost": 100},
	})

	var body predictiveView
	getJSON(t, srv.URL+"/predictive-maintenance/scan?company_id="+companyID).decode(t, &body)

	if len(body.Errors) == 0 || body.Errors[0].Source != "production-service" {
		t.Errorf("errors = %+v, want menyebut production-service", body.Errors)
	}
	if _, ok := body.byCode("AST-001"); !ok {
		t.Error("aset tetap harus dinilai walau production-service mati")
	}
}
