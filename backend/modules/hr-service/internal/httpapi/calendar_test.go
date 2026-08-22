package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type holidayView struct {
	ID          string `json:"id"`
	CompanyID   string `json:"company_id"`
	HolidayDate string `json:"holiday_date"`
	Name        string `json:"name"`
	IsNational  bool   `json:"is_national"`
}

func mustCreateHoliday(t *testing.T, srv *httptest.Server, companyID, date, name string) holidayView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/holidays", map[string]any{
		"company_id": companyID, "holiday_date": date, "name": name,
	})
	requireStatus(t, resp, http.StatusCreated)
	var h holidayView
	resp.decode(t, &h)
	return h
}

func TestCreateHoliday_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"tanpa company_id", map[string]any{"holiday_date": "2026-08-17", "name": "HUT RI"}},
		{"tanpa tanggal", map[string]any{"company_id": companyID, "name": "HUT RI"}},
		{"tanpa nama", map[string]any{"company_id": companyID, "holiday_date": "2026-08-17", "name": "  "}},
		{"format tanggal salah", map[string]any{"company_id": companyID, "holiday_date": "17-08-2026", "name": "HUT RI"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/holidays", tc.payload), http.StatusBadRequest)
		})
	}
}

// Kalender di-scope per company: tanggal yang sama boleh ada di dua company,
// tapi tidak boleh dobel di company yang sama.
func TestCreateHoliday_DuplicateDatePerCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustCreateHoliday(t, srv, companyA, "2026-08-17", "HUT RI")

	dup := postJSON(t, srv.URL+"/holidays", map[string]any{
		"company_id": companyA, "holiday_date": "2026-08-17", "name": "Duplikat",
	})
	requireStatus(t, dup, http.StatusConflict)

	// Company lain tidak terpengaruh.
	mustCreateHoliday(t, srv, companyB, "2026-08-17", "HUT RI")
}

func TestListHolidays_ScopedToCompanyAndYear(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)

	mustCreateHoliday(t, srv, companyA, "2026-08-17", "HUT RI")
	mustCreateHoliday(t, srv, companyA, "2027-01-01", "Tahun Baru")
	mustCreateHoliday(t, srv, companyB, "2026-12-25", "Natal")

	var all []holidayView
	getJSON(t, srv.URL+"/holidays?company_id="+companyA).decode(t, &all)
	if len(all) != 2 {
		t.Fatalf("expected 2 hari libur company A, got %d", len(all))
	}
	// Diurutkan menaik supaya bisa langsung dipakai UI kalender.
	if all[0].HolidayDate > all[1].HolidayDate {
		t.Errorf("urutan tanggal tidak menaik: %+v", all)
	}

	var only2026 []holidayView
	getJSON(t, srv.URL+"/holidays?company_id="+companyA+"&year=2026").decode(t, &only2026)
	if len(only2026) != 1 || only2026[0].Name != "HUT RI" {
		t.Fatalf("filter tahun tidak bekerja: %+v", only2026)
	}
}

func TestListHolidays_RequiresCompanyAndValidYear(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, getJSON(t, srv.URL+"/holidays"), http.StatusBadRequest)
	requireStatus(t, getJSON(t, srv.URL+"/holidays?company_id="+newCompanyID(t)+"&year=dua-ribu"), http.StatusBadRequest)
}

func TestDeleteHoliday(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	h := mustCreateHoliday(t, srv, companyID, "2026-08-17", "HUT RI")

	requireStatus(t, doRequest(t, http.MethodDelete, srv.URL+"/holidays/"+h.ID, nil, ""), http.StatusOK)

	var after []holidayView
	getJSON(t, srv.URL+"/holidays?company_id="+companyID).decode(t, &after)
	if len(after) != 0 {
		t.Fatalf("expected kalender kosong setelah dihapus, got %+v", after)
	}

	requireStatus(t, doRequest(t, http.MethodDelete, srv.URL+"/holidays/"+uuid.NewString(), nil, ""), http.StatusNotFound)
}

// Inti dari kalender: hari libur di tengah rentang cuti tidak dihitung sebagai
// hari cuti. 17 Agustus 2026 jatuh pada hari Senin.
func TestCreateLeaveRequest_SkipsHolidays(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 6000000, 0)

	// Sen 17 s/d Jum 21 Agustus 2026 = 5 hari kerja sebelum ada kalender.
	before := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-08-17", "2026-08-21")
	if before.TotalDays != 5 {
		t.Fatalf("prasyarat gagal, expected 5 hari kerja, got %d", before.TotalDays)
	}
	requireStatus(t, postJSON(t, srv.URL+"/leave-requests/"+before.ID+"/cancel", nil), http.StatusOK)

	mustCreateHoliday(t, srv, companyID, "2026-08-17", "HUT RI")

	after := mustCreateLeave(t, srv, companyID, emp.ID, "ANNUAL", "2026-08-17", "2026-08-21")
	if after.TotalDays != 4 {
		t.Fatalf("expected 4 hari kerja setelah 17 Agustus jadi hari libur, got %d", after.TotalDays)
	}
}

// Lembur di tanggal merah otomatis memakai tarif hari libur tanpa perlu
// dicentang manual -- sebelumnya bergantung pada ketelitian yang menginput.
func TestCreateOvertime_HolidayDetectedFromCalendar(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	// Gaji 8.650.000 / 173 = tarif 50.000/jam persis.
	emp := mustSeedEmployee(t, srv, companyID, 8650000, 0)
	mustCreateHoliday(t, srv, companyID, "2026-08-17", "HUT RI")

	// is_holiday sengaja dikirim false: kalender yang menentukan.
	o := mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-17", 2, false)
	if !o.IsHoliday {
		t.Fatal("lembur di tanggal merah seharusnya otomatis is_holiday = true")
	}
	// Tarif hari libur 2x untuk 2 jam pertama: 2 x 2 x 50.000 = 200.000.
	if o.Amount != 200000 {
		t.Fatalf("expected upah lembur hari libur 200000, got %v", o.Amount)
	}
}

// Akhir pekan juga hari libur menurut Kepmenaker, walau tidak ada di kalender.
// 2026-08-22 adalah hari Sabtu.
func TestCreateOvertime_WeekendCountsAsHoliday(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 8650000, 0)

	o := mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-22", 2, false)
	if !o.IsHoliday {
		t.Fatal("lembur di hari Sabtu seharusnya dihitung sebagai hari libur")
	}
}

// Hari kerja biasa tetap memakai tarif hari kerja -- kalender tidak boleh
// membuat semua lembur jadi tarif libur. 2026-08-18 adalah Selasa.
func TestCreateOvertime_WorkdayStaysWorkdayRate(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 8650000, 0)
	mustCreateHoliday(t, srv, companyID, "2026-08-17", "HUT RI")

	o := mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-18", 2, false)
	if o.IsHoliday {
		t.Fatal("lembur di hari kerja biasa seharusnya bukan hari libur")
	}
	// Hari kerja: jam pertama 1,5x, jam berikutnya 2x = (1,5 + 2) x 50.000.
	if o.Amount != 175000 {
		t.Fatalf("expected upah lembur hari kerja 175000, got %v", o.Amount)
	}
}

// Centang manual tetap dihormati: kalender bisa saja belum memuat libur
// pengganti yang baru diumumkan.
func TestCreateOvertime_ManualHolidayFlagStillHonoured(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	emp := mustSeedEmployee(t, srv, companyID, 8650000, 0)

	o := mustCreateOvertime(t, srv, companyID, emp.ID, "2026-08-18", 2, true)
	if !o.IsHoliday {
		t.Fatal("is_holiday = true dari pemanggil seharusnya tetap dipakai")
	}
	if o.Amount != 200000 {
		t.Fatalf("expected tarif hari libur 200000, got %v", o.Amount)
	}
}
