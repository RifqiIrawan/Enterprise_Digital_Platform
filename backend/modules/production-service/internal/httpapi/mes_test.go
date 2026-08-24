package httpapi_test

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// ---------- views & helpers ----------

type machineView struct {
	ID                    string  `json:"id"`
	Code                  string  `json:"code"`
	Name                  string  `json:"name"`
	MachineType           string  `json:"machine_type"`
	Location              *string `json:"location"`
	IdealCycleTimeMinutes float64 `json:"ideal_cycle_time_minutes"`
	Status                string  `json:"status"`
}

type shiftView struct {
	ID             string `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
	BreakMinutes   int    `json:"break_minutes"`
	PlannedMinutes int    `json:"planned_minutes"`
	IsActive       bool   `json:"is_active"`
}

type downtimeView struct {
	ID         string `json:"id"`
	ReasonCode string `json:"reason_code"`
	Minutes    int    `json:"minutes"`
}

type oeeView struct {
	PlannedMinutes    int     `json:"planned_minutes"`
	DowntimeMinutes   int     `json:"downtime_minutes"`
	RunTimeMinutes    int     `json:"run_time_minutes"`
	QuantityGood      float64 `json:"quantity_good"`
	QuantityReject    float64 `json:"quantity_reject"`
	Availability      float64 `json:"availability"`
	Performance       float64 `json:"performance"`
	Quality           float64 `json:"quality"`
	OEE               float64 `json:"oee"`
	PerformanceCapped bool    `json:"performance_capped"`
}

type runView struct {
	ID              string         `json:"id"`
	RunNumber       string         `json:"run_number"`
	Status          string         `json:"status"`
	PlannedMinutes  int            `json:"planned_minutes"`
	QuantityGood    *float64       `json:"quantity_good"`
	QuantityReject  *float64       `json:"quantity_reject"`
	DowntimeMinutes int            `json:"downtime_minutes"`
	DowntimeLogs    []downtimeView `json:"downtime_logs"`
	OEE             *oeeView       `json:"oee"`
	Machine         *machineView   `json:"machine"`
}

type oeeSummaryView struct {
	Overall  oeeView `json:"overall"`
	Machines []struct {
		MachineID   string `json:"machine_id"`
		MachineCode string `json:"machine_code"`
		RunCount    int    `json:"run_count"`
		oeeView
	} `json:"machines"`
	DowntimeByReason []struct {
		ReasonCode string  `json:"reason_code"`
		Minutes    int     `json:"minutes"`
		Share      float64 `json:"share"`
	} `json:"downtime_by_reason"`
}

func putJSON(t *testing.T, url string, payload any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPut, url, payload, uuid.NewString())
}

func deleteJSON(t *testing.T, url string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodDelete, url, nil, uuid.NewString())
}

// requireClose membandingkan angka pecahan dengan toleransi: nilai OEE dibulatkan
// ke 4 desimal di server, dan membandingkannya persis dengan literal float di
// test hanya menghasilkan kegagalan palsu dari sisa representasi biner.
func requireClose(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.00005 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func mustSeedMachine(t *testing.T, srv *httptest.Server, companyID string, idealCycleMinutes float64) machineView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/machines", map[string]any{
		"company_id": companyID, "code": "MC-" + uuid.NewString()[:8], "name": "Mesin Uji",
		"machine_type": "PACKING", "ideal_cycle_time_minutes": idealCycleMinutes,
	})
	requireStatus(t, resp, http.StatusCreated)
	var m machineView
	resp.decode(t, &m)
	return m
}

func mustSeedShift(t *testing.T, srv *httptest.Server, companyID, start, end string, breakMinutes int) shiftView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/shifts", map[string]any{
		"company_id": companyID, "code": "SH-" + uuid.NewString()[:8], "name": "Shift Uji",
		"start_time": start, "end_time": end, "break_minutes": breakMinutes,
	})
	requireStatus(t, resp, http.StatusCreated)
	var s shiftView
	resp.decode(t, &s)
	return s
}

// mustSeedStartedWorkOrder menyiapkan work order yang sudah IN_PROGRESS --
// satu-satunya keadaan di mana production run boleh dibuat.
func mustSeedStartedWorkOrder(t *testing.T, srv *httptest.Server, companyID string) (workOrderView, bomFixture, string) {
	t.Helper()
	bom := mustSeedBOM(t, srv, companyID)
	warehouseID := uuid.NewString()
	wo := mustCreateWorkOrder(t, srv.URL, companyID, bom.ID, warehouseID, 200)
	mustStartWorkOrder(t, srv.URL, wo.ID)
	return wo, bom, warehouseID
}

func mustOpenRun(t *testing.T, srv *httptest.Server, companyID, workOrderID, machineID, shiftID string) runView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/production-runs", map[string]any{
		"company_id": companyID, "work_order_id": workOrderID, "machine_id": machineID,
		"shift_id": shiftID, "run_date": today(),
	})
	requireStatus(t, resp, http.StatusCreated)
	var run runView
	resp.decode(t, &run)
	return run
}

// ---------- machines ----------

func TestCreateMachine_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing code":              {"company_id": companyID, "name": "Mesin", "ideal_cycle_time_minutes": 1},
		"missing name":              {"company_id": companyID, "code": "MC-1", "ideal_cycle_time_minutes": 1},
		"missing company_id":        {"code": "MC-1", "name": "Mesin", "ideal_cycle_time_minutes": 1},
		"zero ideal cycle time":     {"company_id": companyID, "code": "MC-1", "name": "Mesin", "ideal_cycle_time_minutes": 0},
		"negative ideal cycle time": {"company_id": companyID, "code": "MC-1", "name": "Mesin", "ideal_cycle_time_minutes": -3},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/machines", payload), http.StatusBadRequest)
		})
	}
}

func TestCreateMachine_DefaultsAndDuplicateCode(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	code := "MC-" + uuid.NewString()[:8]

	resp := postJSON(t, srv.URL+"/machines", map[string]any{
		"company_id": companyID, "code": code, "name": "Mesin Cetak", "ideal_cycle_time_minutes": 1.5,
	})
	requireStatus(t, resp, http.StatusCreated)
	var m machineView
	resp.decode(t, &m)
	if m.Status != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE by default", m.Status)
	}
	if m.MachineType != "GENERAL" {
		t.Errorf("machine_type = %q, want GENERAL by default", m.MachineType)
	}
	if m.Location != nil {
		t.Errorf("location = %v, want null when not supplied", *m.Location)
	}

	dup := postJSON(t, srv.URL+"/machines", map[string]any{
		"company_id": companyID, "code": code, "name": "Mesin lain", "ideal_cycle_time_minutes": 2,
	})
	requireStatus(t, dup, http.StatusConflict)

	// Kode yang sama di company lain harus tetap boleh.
	other := postJSON(t, srv.URL+"/machines", map[string]any{
		"company_id": newCompanyID(t), "code": code, "name": "Mesin company lain", "ideal_cycle_time_minutes": 2,
	})
	requireStatus(t, other, http.StatusCreated)
}

func TestListMachines_ScopedByCompanyAndStatus(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	active := mustSeedMachine(t, srv, companyID, 1)
	idle := mustSeedMachine(t, srv, companyID, 1)
	mustSeedMachine(t, srv, newCompanyID(t), 1)

	requireStatus(t, putJSON(t, srv.URL+"/machines/"+idle.ID, map[string]any{
		"name": "Mesin Uji", "ideal_cycle_time_minutes": 1, "status": "MAINTENANCE",
	}), http.StatusOK)

	var all []machineView
	getJSON(t, srv.URL+"/machines?company_id="+companyID).decode(t, &all)
	if len(all) != 2 {
		t.Fatalf("expected 2 machines for this company, got %d", len(all))
	}

	var onlyActive []machineView
	getJSON(t, srv.URL+"/machines?company_id="+companyID+"&status=ACTIVE").decode(t, &onlyActive)
	if len(onlyActive) != 1 || onlyActive[0].ID != active.ID {
		t.Fatalf("status filter returned %+v, want only %s", onlyActive, active.ID)
	}

	requireStatus(t, getJSON(t, srv.URL+"/machines"), http.StatusBadRequest)
	requireStatus(t, getJSON(t, srv.URL+"/machines/"+uuid.NewString()), http.StatusNotFound)
}

func TestUpdateMachine_RejectsUnknownStatus(t *testing.T) {
	srv := newServer(t)
	m := mustSeedMachine(t, srv, newCompanyID(t), 1)
	resp := putJSON(t, srv.URL+"/machines/"+m.ID, map[string]any{
		"name": "Mesin", "ideal_cycle_time_minutes": 1, "status": "RUSAK",
	})
	requireStatus(t, resp, http.StatusBadRequest)
	requireStatus(t, putJSON(t, srv.URL+"/machines/"+uuid.NewString(), map[string]any{
		"name": "Mesin", "ideal_cycle_time_minutes": 1,
	}), http.StatusNotFound)
}

// Menarik mesin ke MAINTENANCE saat run-nya masih OPEN akan meninggalkan run
// yang tidak bisa ditutup lewat UI dan merusak Availability mesin itu.
func TestUpdateMachine_BlockedWhileRunIsOpen(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 1)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)
	run := mustOpenRun(t, srv, companyID, wo.ID, machine.ID, shift.ID)

	blocked := putJSON(t, srv.URL+"/machines/"+machine.ID, map[string]any{
		"name": "Mesin Uji", "ideal_cycle_time_minutes": 1, "status": "MAINTENANCE",
	})
	requireStatus(t, blocked, http.StatusConflict)

	// Setelah run ditutup, perubahan status boleh.
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+run.ID+"/close", map[string]any{
		"quantity_good": 10, "quantity_reject": 0,
	}), http.StatusOK)
	requireStatus(t, putJSON(t, srv.URL+"/machines/"+machine.ID, map[string]any{
		"name": "Mesin Uji", "ideal_cycle_time_minutes": 1, "status": "MAINTENANCE",
	}), http.StatusOK)
}

// ---------- shifts ----------

func TestCreateShift_PlannedMinutes(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := []struct {
		name        string
		start, end  string
		breakMin    int
		wantMinutes int
	}{
		{"shift pagi 8 jam dengan istirahat 1 jam", "08:00", "16:00", 60, 420},
		{"shift tanpa istirahat", "08:00", "16:00", 0, 480},
		{"shift malam melewati tengah malam", "22:00", "06:00", 60, 420},
		{"shift pendek", "13:15", "17:45", 15, 255},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := mustSeedShift(t, srv, companyID, c.start, c.end, c.breakMin)
			if s.PlannedMinutes != c.wantMinutes {
				t.Errorf("planned_minutes = %d, want %d", s.PlannedMinutes, c.wantMinutes)
			}
			if s.StartTime != c.start || s.EndTime != c.end {
				t.Errorf("jam shift terbaca %q-%q, want %q-%q", s.StartTime, s.EndTime, c.start, c.end)
			}
			if !s.IsActive {
				t.Error("shift baru seharusnya aktif")
			}
		})
	}
}

func TestCreateShift_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := map[string]map[string]any{
		"missing code":                    {"company_id": companyID, "name": "Shift", "start_time": "08:00", "end_time": "16:00"},
		"missing company_id":              {"code": "S1", "name": "Shift", "start_time": "08:00", "end_time": "16:00"},
		"start_time bukan jam":            {"company_id": companyID, "code": "S1", "name": "Shift", "start_time": "8 pagi", "end_time": "16:00"},
		"end_time bukan jam":              {"company_id": companyID, "code": "S1", "name": "Shift", "start_time": "08:00", "end_time": ""},
		"mulai dan selesai sama":          {"company_id": companyID, "code": "S1", "name": "Shift", "start_time": "08:00", "end_time": "08:00"},
		"istirahat menghabiskan shift":    {"company_id": companyID, "code": "S1", "name": "Shift", "start_time": "08:00", "end_time": "12:00", "break_minutes": 240},
		"istirahat melebihi durasi shift": {"company_id": companyID, "code": "S1", "name": "Shift", "start_time": "08:00", "end_time": "12:00", "break_minutes": 300},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/shifts", payload), http.StatusBadRequest)
		})
	}
}

func TestShift_DuplicateCodeAndUpdate(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	code := "SH-" + uuid.NewString()[:8]
	base := map[string]any{"company_id": companyID, "code": code, "name": "Pagi", "start_time": "07:00", "end_time": "15:00", "break_minutes": 30}
	requireStatus(t, postJSON(t, srv.URL+"/shifts", base), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/shifts", base), http.StatusConflict)

	var shifts []shiftView
	getJSON(t, srv.URL+"/shifts?company_id="+companyID).decode(t, &shifts)
	if len(shifts) != 1 {
		t.Fatalf("expected 1 shift, got %d", len(shifts))
	}

	resp := putJSON(t, srv.URL+"/shifts/"+shifts[0].ID, map[string]any{
		"name": "Pagi (revisi)", "start_time": "07:00", "end_time": "15:00", "break_minutes": 60, "is_active": false,
	})
	requireStatus(t, resp, http.StatusOK)
	var updated shiftView
	resp.decode(t, &updated)
	if updated.PlannedMinutes != 420 || updated.IsActive {
		t.Errorf("setelah update: planned_minutes = %d (want 420), is_active = %v (want false)", updated.PlannedMinutes, updated.IsActive)
	}

	requireStatus(t, putJSON(t, srv.URL+"/shifts/"+uuid.NewString(), map[string]any{
		"name": "X", "start_time": "07:00", "end_time": "15:00",
	}), http.StatusNotFound)
	requireStatus(t, getJSON(t, srv.URL+"/shifts"), http.StatusBadRequest)
}

// ---------- production runs ----------

func TestCreateProductionRun_RequiresInProgressWorkOrder(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 1)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	bom := mustSeedBOM(t, srv, companyID)
	draft := mustCreateWorkOrder(t, srv.URL, companyID, bom.ID, uuid.NewString(), 100)

	payload := map[string]any{
		"company_id": companyID, "work_order_id": draft.ID, "machine_id": machine.ID,
		"shift_id": shift.ID, "run_date": today(),
	}
	resp := postJSON(t, srv.URL+"/production-runs", payload)
	requireStatus(t, resp, http.StatusConflict)

	mustStartWorkOrder(t, srv.URL, draft.ID)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs", payload), http.StatusCreated)
}

func TestCreateProductionRun_ValidationAndMissingReferences(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 1)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)

	t.Run("field wajib", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/production-runs", map[string]any{"company_id": companyID}), http.StatusBadRequest)
	})
	t.Run("run_date bukan tanggal", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/production-runs", map[string]any{
			"company_id": companyID, "work_order_id": wo.ID, "machine_id": machine.ID, "shift_id": shift.ID, "run_date": "kemarin",
		}), http.StatusBadRequest)
	})
	t.Run("mesin tidak ada", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/production-runs", map[string]any{
			"company_id": companyID, "work_order_id": wo.ID, "machine_id": uuid.NewString(), "shift_id": shift.ID, "run_date": today(),
		}), http.StatusNotFound)
	})
	t.Run("shift tidak ada", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/production-runs", map[string]any{
			"company_id": companyID, "work_order_id": wo.ID, "machine_id": machine.ID, "shift_id": uuid.NewString(), "run_date": today(),
		}), http.StatusNotFound)
	})
	t.Run("work order milik company lain", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/production-runs", map[string]any{
			"company_id": newCompanyID(t), "work_order_id": wo.ID, "machine_id": machine.ID, "shift_id": shift.ID, "run_date": today(),
		}), http.StatusNotFound)
	})
}

func TestCreateProductionRun_MachineMustBeActiveAndShiftMustBeLive(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 1)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)

	requireStatus(t, putJSON(t, srv.URL+"/machines/"+machine.ID, map[string]any{
		"name": "Mesin Uji", "ideal_cycle_time_minutes": 1, "status": "MAINTENANCE",
	}), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs", map[string]any{
		"company_id": companyID, "work_order_id": wo.ID, "machine_id": machine.ID, "shift_id": shift.ID, "run_date": today(),
	}), http.StatusConflict)

	requireStatus(t, putJSON(t, srv.URL+"/machines/"+machine.ID, map[string]any{
		"name": "Mesin Uji", "ideal_cycle_time_minutes": 1, "status": "ACTIVE",
	}), http.StatusOK)
	requireStatus(t, putJSON(t, srv.URL+"/shifts/"+shift.ID, map[string]any{
		"name": "Shift Uji", "start_time": "08:00", "end_time": "16:00", "break_minutes": 60, "is_active": false,
	}), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs", map[string]any{
		"company_id": companyID, "work_order_id": wo.ID, "machine_id": machine.ID, "shift_id": shift.ID, "run_date": today(),
	}), http.StatusConflict)
}

// Satu mesin hanya mengerjakan satu hal pada satu waktu.
func TestCreateProductionRun_OneOpenRunPerMachine(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 1)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)
	run := mustOpenRun(t, srv, companyID, wo.ID, machine.ID, shift.ID)

	second := postJSON(t, srv.URL+"/production-runs", map[string]any{
		"company_id": companyID, "work_order_id": wo.ID, "machine_id": machine.ID, "shift_id": shift.ID, "run_date": today(),
	})
	requireStatus(t, second, http.StatusConflict)

	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+run.ID+"/close", map[string]any{
		"quantity_good": 5, "quantity_reject": 0,
	}), http.StatusOK)
	// Setelah run pertama ditutup, mesin bebas lagi.
	requireStatus(t, postJSON(t, srv.URL+"/production-runs", map[string]any{
		"company_id": companyID, "work_order_id": wo.ID, "machine_id": machine.ID, "shift_id": shift.ID, "run_date": today(),
	}), http.StatusCreated)
}

func TestProductionRun_SnapshotsShiftPlannedMinutes(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 1)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)
	run := mustOpenRun(t, srv, companyID, wo.ID, machine.ID, shift.ID)
	if run.PlannedMinutes != 420 {
		t.Fatalf("planned_minutes = %d, want 420 (snapshot dari shift)", run.PlannedMinutes)
	}

	// Jam shift diperpendek SETELAH run dibuat; run yang sudah ada tidak
	// boleh ikut berubah, kalau tidak OEE periode lampau ditulis ulang.
	requireStatus(t, putJSON(t, srv.URL+"/shifts/"+shift.ID, map[string]any{
		"name": "Shift Uji", "start_time": "08:00", "end_time": "12:00", "break_minutes": 0, "is_active": true,
	}), http.StatusOK)

	var reloaded runView
	getJSON(t, srv.URL+"/production-runs/"+run.ID).decode(t, &reloaded)
	if reloaded.PlannedMinutes != 420 {
		t.Errorf("planned_minutes setelah shift diubah = %d, want tetap 420", reloaded.PlannedMinutes)
	}
}

func TestListProductionRuns_Filters(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	machineA := mustSeedMachine(t, srv, companyID, 1)
	machineB := mustSeedMachine(t, srv, companyID, 1)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)
	runA := mustOpenRun(t, srv, companyID, wo.ID, machineA.ID, shift.ID)
	mustOpenRun(t, srv, companyID, wo.ID, machineB.ID, shift.ID)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+runA.ID+"/close", map[string]any{
		"quantity_good": 10, "quantity_reject": 1,
	}), http.StatusOK)

	var all []runView
	getJSON(t, srv.URL+"/production-runs?company_id="+companyID).decode(t, &all)
	if len(all) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(all))
	}

	var open []runView
	getJSON(t, srv.URL+"/production-runs?company_id="+companyID+"&status=OPEN").decode(t, &open)
	if len(open) != 1 || open[0].Status != "OPEN" {
		t.Fatalf("status filter returned %+v", open)
	}

	var byMachine []runView
	getJSON(t, srv.URL+"/production-runs?company_id="+companyID+"&machine_id="+machineA.ID).decode(t, &byMachine)
	if len(byMachine) != 1 || byMachine[0].ID != runA.ID {
		t.Fatalf("machine filter returned %+v", byMachine)
	}

	var byWorkOrder []runView
	getJSON(t, srv.URL+"/production-runs?company_id="+companyID+"&work_order_id="+wo.ID).decode(t, &byWorkOrder)
	if len(byWorkOrder) != 2 {
		t.Fatalf("work_order filter returned %d runs, want 2", len(byWorkOrder))
	}

	var none []runView
	getJSON(t, srv.URL+"/production-runs?company_id="+companyID+"&from=2000-01-01&to=2000-01-31").decode(t, &none)
	if len(none) != 0 {
		t.Fatalf("date filter returned %d runs, want 0", len(none))
	}

	requireStatus(t, getJSON(t, srv.URL+"/production-runs"), http.StatusBadRequest)
	requireStatus(t, getJSON(t, srv.URL+"/production-runs/"+uuid.NewString()), http.StatusNotFound)
}

// ---------- downtime ----------

func TestDowntime_ValidationLimitAndDeletion(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 1)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60) // 420 menit
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)
	run := mustOpenRun(t, srv, companyID, wo.ID, machine.ID, shift.ID)
	url := srv.URL + "/production-runs/" + run.ID + "/downtime"

	requireStatus(t, postJSON(t, url, map[string]any{"reason_code": "KEHABISAN_KOPI", "minutes": 10}), http.StatusBadRequest)
	requireStatus(t, postJSON(t, url, map[string]any{"reason_code": "BREAKDOWN", "minutes": 0}), http.StatusBadRequest)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+uuid.NewString()+"/downtime", map[string]any{
		"reason_code": "BREAKDOWN", "minutes": 10,
	}), http.StatusNotFound)

	first := postJSON(t, url, map[string]any{"reason_code": "BREAKDOWN", "minutes": 400, "notes": "motor terbakar"})
	requireStatus(t, first, http.StatusCreated)
	var log downtimeView
	first.decode(t, &log)

	// 400 + 30 > 420 menit shift: run time tidak boleh negatif.
	requireStatus(t, postJSON(t, url, map[string]any{"reason_code": "SETUP", "minutes": 30}), http.StatusConflict)
	// Tepat sampai batas boleh.
	requireStatus(t, postJSON(t, url, map[string]any{"reason_code": "SETUP", "minutes": 20}), http.StatusCreated)

	var detail runView
	getJSON(t, srv.URL+"/production-runs/"+run.ID).decode(t, &detail)
	if detail.DowntimeMinutes != 420 || len(detail.DowntimeLogs) != 2 {
		t.Fatalf("downtime = %d menit dalam %d catatan, want 420 dalam 2", detail.DowntimeMinutes, len(detail.DowntimeLogs))
	}

	requireStatus(t, deleteJSON(t, url+"/"+uuid.NewString()), http.StatusNotFound)
	requireStatus(t, deleteJSON(t, url+"/"+log.ID), http.StatusNoContent)

	getJSON(t, srv.URL+"/production-runs/"+run.ID).decode(t, &detail)
	if detail.DowntimeMinutes != 20 || len(detail.DowntimeLogs) != 1 {
		t.Fatalf("setelah hapus: downtime = %d menit dalam %d catatan, want 20 dalam 1", detail.DowntimeMinutes, len(detail.DowntimeLogs))
	}

	// Run yang sudah ditutup tidak menerima perubahan downtime lagi.
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+run.ID+"/close", map[string]any{
		"quantity_good": 100, "quantity_reject": 0,
	}), http.StatusOK)
	requireStatus(t, postJSON(t, url, map[string]any{"reason_code": "SETUP", "minutes": 5}), http.StatusConflict)
	requireStatus(t, deleteJSON(t, url+"/"+detail.DowntimeLogs[0].ID), http.StatusConflict)
}

// ---------- OEE ----------

func TestCloseProductionRun_ComputesOEE(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 2) // 2 menit per unit
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)
	run := mustOpenRun(t, srv, companyID, wo.ID, machine.ID, shift.ID)

	// OEE belum ada selama run masih berjalan.
	var open runView
	getJSON(t, srv.URL+"/production-runs/"+run.ID).decode(t, &open)
	if open.OEE != nil {
		t.Errorf("run OPEN seharusnya belum punya OEE, dapat %+v", open.OEE)
	}

	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+run.ID+"/downtime", map[string]any{"reason_code": "BREAKDOWN", "minutes": 45}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+run.ID+"/downtime", map[string]any{"reason_code": "SETUP", "minutes": 15}), http.StatusCreated)

	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+run.ID+"/close", map[string]any{"quantity_good": -1, "quantity_reject": 0}), http.StatusBadRequest)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+run.ID+"/close", map[string]any{"quantity_good": 0, "quantity_reject": 0}), http.StatusBadRequest)

	resp := postJSON(t, srv.URL+"/production-runs/"+run.ID+"/close", map[string]any{"quantity_good": 150, "quantity_reject": 10})
	requireStatus(t, resp, http.StatusOK)
	var closed runView
	resp.decode(t, &closed)

	if closed.Status != "CLOSED" || closed.OEE == nil {
		t.Fatalf("status = %q, oee = %+v", closed.Status, closed.OEE)
	}
	// planned 420, downtime 60 -> run time 360
	// A = 360/420, P = 2 menit x 160 unit / 360, Q = 150/160
	if closed.OEE.RunTimeMinutes != 360 || closed.OEE.DowntimeMinutes != 60 {
		t.Errorf("run_time = %d, downtime = %d; want 360 & 60", closed.OEE.RunTimeMinutes, closed.OEE.DowntimeMinutes)
	}
	requireClose(t, "availability", closed.OEE.Availability, 0.8571)
	requireClose(t, "performance", closed.OEE.Performance, 0.8889)
	requireClose(t, "quality", closed.OEE.Quality, 0.9375)
	requireClose(t, "oee", closed.OEE.OEE, 0.7143)
	if closed.OEE.PerformanceCapped {
		t.Error("performance_capped = true, want false untuk performance di bawah 100%")
	}

	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+run.ID+"/close", map[string]any{"quantity_good": 1, "quantity_reject": 0}), http.StatusConflict)
}

// Ideal cycle time yang disetel terlalu lambat menghasilkan Performance di atas
// 100%; dipotong ke 100% dan ditandai, bukan diam-diam dibiarkan menaikkan OEE.
func TestCloseProductionRun_PerformanceCapped(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 5) // jelas terlalu lambat
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)
	run := mustOpenRun(t, srv, companyID, wo.ID, machine.ID, shift.ID)

	resp := postJSON(t, srv.URL+"/production-runs/"+run.ID+"/close", map[string]any{"quantity_good": 160, "quantity_reject": 0})
	requireStatus(t, resp, http.StatusOK)
	var closed runView
	resp.decode(t, &closed)

	if !closed.OEE.PerformanceCapped {
		t.Error("performance_capped = false, want true")
	}
	requireClose(t, "performance", closed.OEE.Performance, 1)
	requireClose(t, "oee", closed.OEE.OEE, 1) // A=1 (tanpa downtime), Q=1
}

func TestOEESummary_AggregatesPerMachineAndByReason(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60) // 420 menit
	machineA := mustSeedMachine(t, srv, companyID, 2)
	machineB := mustSeedMachine(t, srv, companyID, 3)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)

	runA := mustOpenRun(t, srv, companyID, wo.ID, machineA.ID, shift.ID)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+runA.ID+"/downtime", map[string]any{"reason_code": "BREAKDOWN", "minutes": 45}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+runA.ID+"/downtime", map[string]any{"reason_code": "SETUP", "minutes": 15}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+runA.ID+"/close", map[string]any{"quantity_good": 150, "quantity_reject": 10}), http.StatusOK)

	runB := mustOpenRun(t, srv, companyID, wo.ID, machineB.ID, shift.ID)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+runB.ID+"/close", map[string]any{"quantity_good": 100, "quantity_reject": 0}), http.StatusOK)

	// Run yang masih OPEN tidak boleh ikut terhitung.
	machineC := mustSeedMachine(t, srv, companyID, 1)
	mustOpenRun(t, srv, companyID, wo.ID, machineC.ID, shift.ID)

	var summary oeeSummaryView
	getJSON(t, srv.URL+"/oee?company_id="+companyID).decode(t, &summary)

	if len(summary.Machines) != 2 {
		t.Fatalf("expected 2 machines with closed runs, got %d (%+v)", len(summary.Machines), summary.Machines)
	}
	byID := map[string]oeeView{}
	runCounts := map[string]int{}
	for _, m := range summary.Machines {
		byID[m.MachineID] = m.oeeView
		runCounts[m.MachineID] = m.RunCount
	}
	a, okA := byID[machineA.ID]
	b, okB := byID[machineB.ID]
	if !okA || !okB {
		t.Fatalf("ringkasan tidak memuat kedua mesin: %+v", summary.Machines)
	}
	if runCounts[machineA.ID] != 1 || runCounts[machineB.ID] != 1 {
		t.Errorf("run_count = %v, want 1 untuk tiap mesin", runCounts)
	}
	requireClose(t, "mesin A availability", a.Availability, 0.8571)
	requireClose(t, "mesin A oee", a.OEE, 0.7143)
	requireClose(t, "mesin B availability", b.Availability, 1)
	requireClose(t, "mesin B performance", b.Performance, 0.7143) // 3 menit x 100 unit / 420

	// Overall: planned 840, downtime 60, run time 780, ideal 320 + 300 = 620 menit,
	// good 250 dari 260 unit. Perhatikan performance overall TIDAK memakai satu
	// ideal cycle time -- tiap mesin punya kecepatannya sendiri.
	if summary.Overall.PlannedMinutes != 840 || summary.Overall.RunTimeMinutes != 780 {
		t.Errorf("overall planned/run time = %d/%d, want 840/780", summary.Overall.PlannedMinutes, summary.Overall.RunTimeMinutes)
	}
	requireClose(t, "overall availability", summary.Overall.Availability, 0.9286)
	requireClose(t, "overall performance", summary.Overall.Performance, 0.7949)
	requireClose(t, "overall quality", summary.Overall.Quality, 0.9615)
	requireClose(t, "overall oee", summary.Overall.OEE, 0.7097)

	if len(summary.DowntimeByReason) != 2 {
		t.Fatalf("downtime_by_reason = %+v, want 2 alasan", summary.DowntimeByReason)
	}
	if summary.DowntimeByReason[0].ReasonCode != "BREAKDOWN" || summary.DowntimeByReason[0].Minutes != 45 {
		t.Errorf("alasan terbesar = %+v, want BREAKDOWN 45 menit", summary.DowntimeByReason[0])
	}
	requireClose(t, "share BREAKDOWN", summary.DowntimeByReason[0].Share, 0.75)
	requireClose(t, "share SETUP", summary.DowntimeByReason[1].Share, 0.25)

	// Filter per mesin mempersempit ringkasan sekaligus Pareto-nya.
	var onlyB oeeSummaryView
	getJSON(t, srv.URL+"/oee?company_id="+companyID+"&machine_id="+machineB.ID).decode(t, &onlyB)
	if len(onlyB.Machines) != 1 || onlyB.Machines[0].MachineID != machineB.ID {
		t.Fatalf("filter mesin mengembalikan %+v", onlyB.Machines)
	}
	if len(onlyB.DowntimeByReason) != 0 {
		t.Errorf("mesin B tidak punya downtime, dapat %+v", onlyB.DowntimeByReason)
	}

	// Periode di luar rentang run mana pun: nol, bukan error.
	var empty oeeSummaryView
	getJSON(t, srv.URL+"/oee?company_id="+companyID+"&from=2000-01-01&to=2000-01-31").decode(t, &empty)
	if len(empty.Machines) != 0 || empty.Overall.OEE != 0 {
		t.Errorf("periode kosong = %+v", empty)
	}

	requireStatus(t, getJSON(t, srv.URL+"/oee"), http.StatusBadRequest)
}

// ---------- MES <-> Work Order ----------

func TestCompleteWorkOrder_BlockedByOpenProductionRun(t *testing.T) {
	srv, calls := newServerWithWarehouseStub(t, 0)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 1)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)
	mustOpenRun(t, srv, companyID, wo.ID, machine.ID, shift.ID)

	resp := postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 100})
	requireStatus(t, resp, http.StatusConflict)
	if len(*calls) != 0 {
		t.Errorf("warehouse-service tidak boleh dipanggil saat penyelesaian ditolak, dapat %d panggilan", len(*calls))
	}
}

func TestCompleteWorkOrder_QuantityMustMatchClosedRuns(t *testing.T) {
	srv, calls := newServerWithWarehouseStub(t, 0)
	companyID := newCompanyID(t)
	machine := mustSeedMachine(t, srv, companyID, 1)
	shift := mustSeedShift(t, srv, companyID, "08:00", "16:00", 60)
	wo, bom, _ := mustSeedStartedWorkOrder(t, srv, companyID)

	run := mustOpenRun(t, srv, companyID, wo.ID, machine.ID, shift.ID)
	requireStatus(t, postJSON(t, srv.URL+"/production-runs/"+run.ID+"/close", map[string]any{
		"quantity_good": 180, "quantity_reject": 20,
	}), http.StatusOK)

	// 200 = bagus + reject; yang masuk gudang hanya yang bagus.
	mismatch := postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 200})
	requireStatus(t, mismatch, http.StatusConflict)
	if len(*calls) != 0 {
		t.Fatalf("warehouse-service dipanggil %d kali padahal penyelesaian ditolak", len(*calls))
	}

	ok := postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 180})
	requireStatus(t, ok, http.StatusOK)
	var completed workOrderView
	ok.decode(t, &completed)
	if completed.QuantityProduced == nil || *completed.QuantityProduced != 180 {
		t.Errorf("quantity_produced = %v, want 180", completed.QuantityProduced)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 warehouse calls, got %d", len(*calls))
	}
	_ = bom
}

// Work order tanpa production run sama sekali harus tetap berperilaku seperti
// sebelum MES ada -- angka hasil diketik saat menyelesaikan WO.
func TestCompleteWorkOrder_WithoutRunsKeepsTypedQuantity(t *testing.T) {
	srv, _ := newServerWithWarehouseStub(t, 0)
	companyID := newCompanyID(t)
	wo, _, _ := mustSeedStartedWorkOrder(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/work-orders/"+wo.ID+"/complete", map[string]any{"quantity_produced": 7})
	requireStatus(t, resp, http.StatusOK)
	var completed workOrderView
	resp.decode(t, &completed)
	if completed.QuantityProduced == nil || *completed.QuantityProduced != 7 {
		t.Errorf("quantity_produced = %v, want 7", completed.QuantityProduced)
	}
}
