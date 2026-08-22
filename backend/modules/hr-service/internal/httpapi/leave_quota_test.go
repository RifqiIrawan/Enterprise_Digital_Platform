package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
)

type leaveQuotaView struct {
	EmployeeID    string `json:"employee_id"`
	EmployeeName  string `json:"employee_name"`
	Year          int    `json:"year"`
	TotalDays     int    `json:"total_days"`
	CarriedOver   int    `json:"carried_over"`
	UsedDays      int    `json:"used_days"`
	RemainingDays int    `json:"remaining_days"`
	Note          string `json:"note"`
	HasQuotaRow   bool   `json:"has_quota_row"`
}

func putJSON(t *testing.T, url string, payload any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPut, url, payload, uuid.NewString())
}

func quotaOf(t *testing.T, srv *httptest.Server, companyID, employeeID string, year int) leaveQuotaView {
	t.Helper()
	resp := getJSON(t, srv.URL+"/leave-quotas?company_id="+companyID+"&employee_id="+employeeID+"&year="+strconv.Itoa(year))
	requireStatus(t, resp, http.StatusOK)
	var rows []leaveQuotaView
	resp.decode(t, &rows)
	if len(rows) != 1 {
		t.Fatalf("expected 1 baris kuota untuk karyawan ini, got %d", len(rows))
	}
	return rows[0]
}

func mustSetQuota(t *testing.T, srv *httptest.Server, employeeID string, year, totalDays, carriedOver int) leaveQuotaView {
	t.Helper()
	resp := putJSON(t, srv.URL+"/leave-quotas", map[string]any{
		"employee_id": employeeID, "year": year, "total_days": totalDays, "carried_over": carriedOver,
	})
	requireStatus(t, resp, http.StatusOK)
	var q leaveQuotaView
	resp.decode(t, &q)
	return q
}

// Karyawan yang belum pernah diatur kuotanya tetap muncul dengan default 12
// hari (batas minimum UU Ketenagakerjaan), ditandai has_quota_row = false.
func TestListLeaveQuotas_DefaultsForEmployeesWithoutRow(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)

	q := quotaOf(t, srv, companyID, emp.ID, 2026)
	if q.TotalDays != 12 || q.CarriedOver != 0 {
		t.Fatalf("expected default 12+0, got %d+%d", q.TotalDays, q.CarriedOver)
	}
	if q.HasQuotaRow {
		t.Error("expected has_quota_row false untuk karyawan yang belum diatur")
	}
	if q.UsedDays != 0 || q.RemainingDays != 12 {
		t.Errorf("expected 0 terpakai & 12 sisa, got %d & %d", q.UsedDays, q.RemainingDays)
	}
	if q.EmployeeName == "" {
		t.Error("expected nama karyawan ikut terbawa")
	}
}

func TestPutLeaveQuota_UpsertsSingleRow(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)

	mustSetQuota(t, srv, emp.ID, 2026, 15, 3)
	second := mustSetQuota(t, srv, emp.ID, 2026, 18, 0)

	if second.TotalDays != 18 || second.CarriedOver != 0 {
		t.Fatalf("PUT kedua seharusnya menggantikan yang pertama, got %d+%d", second.TotalDays, second.CarriedOver)
	}
	q := quotaOf(t, srv, companyID, emp.ID, 2026)
	if !q.HasQuotaRow || q.TotalDays != 18 || q.RemainingDays != 18 {
		t.Fatalf("kuota tersimpan tidak sesuai: %+v", q)
	}
}

func TestPutLeaveQuota_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)

	cases := []struct {
		name    string
		payload map[string]any
		status  int
	}{
		{"tanpa employee_id", map[string]any{"year": 2026, "total_days": 12}, http.StatusBadRequest},
		{"tanpa tahun", map[string]any{"employee_id": emp.ID, "total_days": 12}, http.StatusBadRequest},
		{"tahun di luar akal", map[string]any{"employee_id": emp.ID, "year": 1800}, http.StatusBadRequest},
		{"jatah negatif", map[string]any{"employee_id": emp.ID, "year": 2026, "total_days": -1}, http.StatusBadRequest},
		{"karyawan tidak ada", map[string]any{"employee_id": uuid.NewString(), "year": 2026}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, putJSON(t, srv.URL+"/leave-quotas", tc.payload), tc.status)
		})
	}
	_ = companyID
}

// Hari terpakai dihitung ulang dari cuti APPROVED, bukan disimpan: pembatalan
// cuti otomatis mengembalikan jatahnya tanpa ada angka yang perlu dikoreksi.
func TestLeaveQuota_UsedDaysFollowApprovedLeave(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)

	// Sen 17 s/d Rab 19 Agustus 2026 = 3 hari kerja.
	l := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-08-17", "2026-08-19")
	if q := quotaOf(t, srv, companyID, emp.ID, 2026); q.UsedDays != 0 {
		t.Fatalf("cuti DRAFT belum boleh memakan jatah, got %d", q.UsedDays)
	}

	mustApproveLeave(t, srv, l.ID)
	q := quotaOf(t, srv, companyID, emp.ID, 2026)
	if q.UsedDays != 3 || q.RemainingDays != 9 {
		t.Fatalf("expected 3 terpakai & 9 sisa, got %d & %d", q.UsedDays, q.RemainingDays)
	}

	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/cancel", nil), http.StatusOK)
	q = quotaOf(t, srv, companyID, emp.ID, 2026)
	if q.UsedDays != 0 || q.RemainingDays != 12 {
		t.Fatalf("jatah seharusnya kembali setelah cuti dibatalkan, got %d & %d", q.UsedDays, q.RemainingDays)
	}
}

// Hanya cuti ANNUAL yang memakan jatah; sakit & melahirkan punya aturannya
// sendiri di UU.
func TestLeaveQuota_OnlyAnnualLeaveConsumesQuota(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)

	sick := mustCreateLeave(t, srv, companyID, emp.ID, "SICK", "2026-08-17", "2026-08-19")
	mustApproveLeave(t, srv, sick.ID)

	if q := quotaOf(t, srv, companyID, emp.ID, 2026); q.UsedDays != 0 || q.RemainingDays != 12 {
		t.Fatalf("cuti sakit tidak boleh memotong jatah tahunan, got %d & %d", q.UsedDays, q.RemainingDays)
	}
}

// Hari libur di tengah rentang cuti tidak ikut memakan jatah.
func TestLeaveQuota_HolidaysDoNotConsumeQuota(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	mustCreateHoliday(t, srv, companyID, "2026-08-17", "HUT RI")

	l := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-08-17", "2026-08-19")
	mustApproveLeave(t, srv, l.ID)

	q := quotaOf(t, srv, companyID, emp.ID, 2026)
	if q.UsedDays != 2 || q.RemainingDays != 10 {
		t.Fatalf("expected 2 terpakai (17 Agustus libur), got %d terpakai & %d sisa", q.UsedDays, q.RemainingDays)
	}
}

// Pagar utamanya: persetujuan ditolak kalau jatahnya tidak cukup, dengan pesan
// yang menyebut kebutuhan vs sisa.
func TestApproveLeave_BlockedWhenQuotaExhausted(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	mustSetQuota(t, srv, emp.ID, 2026, 2, 0)

	// Sen 17 s/d Rab 19 Agustus = 3 hari kerja, jatah cuma 2.
	l := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-08-17", "2026-08-19")
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/submit", nil), http.StatusOK)

	resp := postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/approve", nil)
	requireStatus(t, resp, http.StatusConflict)
	if msg := resp.errorMessage(); msg == "" {
		t.Error("expected pesan yang menjelaskan sisa jatah")
	}

	// Pengajuannya TIDAK berubah status -- masih bisa diproses setelah jatahnya
	// ditambah, bukan hangus.
	var after leaveView
	var list []leaveView
	getJSON(t, srv.URL+"/leave-requests?company_id="+companyID).decode(t, &list)
	for _, item := range list {
		if item.ID == l.ID {
			after = item
		}
	}
	if after.Status != "SUBMITTED" {
		t.Fatalf("expected status tetap SUBMITTED, got %q", after.Status)
	}

	mustSetQuota(t, srv, emp.ID, 2026, 5, 0)
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/approve", nil), http.StatusOK)
}

// carried_over ikut menambah jatah yang tersedia.
func TestApproveLeave_CarriedOverCountsTowardsQuota(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	mustSetQuota(t, srv, emp.ID, 2026, 2, 1) // 2 + 1 sisa tahun lalu = 3

	l := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-08-17", "2026-08-19")
	mustApproveLeave(t, srv, l.ID)

	q := quotaOf(t, srv, companyID, emp.ID, 2026)
	if q.RemainingDays != 0 {
		t.Fatalf("expected sisa 0 setelah memakai 3 dari 2+1, got %d", q.RemainingDays)
	}
}

// Cuti yang menyeberang tahun membebani jatah masing-masing tahun, bukan
// menumpuk di salah satunya. 28 Des 2026 (Senin) s/d 1 Jan 2027 (Jumat):
// 4 hari kerja di 2026 (28-31 Des) dan 1 hari di 2027 (1 Jan).
func TestApproveLeave_CrossYearSplitsQuotaPerYear(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	mustSetQuota(t, srv, emp.ID, 2026, 10, 0)
	mustSetQuota(t, srv, emp.ID, 2027, 10, 0)

	l := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-12-28", "2027-01-01")
	mustApproveLeave(t, srv, l.ID)

	q2026 := quotaOf(t, srv, companyID, emp.ID, 2026)
	q2027 := quotaOf(t, srv, companyID, emp.ID, 2027)
	if q2026.UsedDays != 4 {
		t.Errorf("expected 4 hari terpakai di 2026, got %d", q2026.UsedDays)
	}
	if q2027.UsedDays != 1 {
		t.Errorf("expected 1 hari terpakai di 2027, got %d", q2027.UsedDays)
	}
}

// Jatah tahun berikutnya yang habis harus menahan persetujuan, walau tahun
// pertamanya masih longgar.
func TestApproveLeave_CrossYearBlockedByTheYearThatRunsOut(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)
	mustSetQuota(t, srv, emp.ID, 2026, 10, 0)
	mustSetQuota(t, srv, emp.ID, 2027, 0, 0)

	l := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-12-28", "2027-01-01")
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/submit", nil), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+l.ID+"/approve", nil), http.StatusConflict)
}

func TestListLeaveQuotas_RequiresCompany(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, getJSON(t, srv.URL+"/leave-quotas"), http.StatusBadRequest)
	requireStatus(t, getJSON(t, srv.URL+"/leave-quotas?company_id="+newCompanyID(t)+"&year=abc"), http.StatusBadRequest)
}
