package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

func TestCreateProjectValidation(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	cases := []struct {
		name    string
		payload map[string]any
		want    int
	}{
		{"tanpa company_id", map[string]any{"project_code": "PRJ-1", "name": "X"}, http.StatusBadRequest},
		{"tanpa project_code", map[string]any{"company_id": companyID, "name": "X"}, http.StatusBadRequest},
		{"tanpa name", map[string]any{"company_id": companyID, "project_code": "PRJ-1"}, http.StatusBadRequest},
		{"budget negatif", map[string]any{"company_id": companyID, "project_code": "PRJ-1", "name": "X", "budget_amount": -1}, http.StatusBadRequest},
		{"start_date bukan tanggal", map[string]any{"company_id": companyID, "project_code": "PRJ-1", "name": "X", "start_date": "12 Agustus"}, http.StatusBadRequest},
		{"end_date sebelum start_date", map[string]any{"company_id": companyID, "project_code": "PRJ-1", "name": "X", "start_date": "2026-08-10", "end_date": "2026-08-01"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/projects", tc.payload), tc.want)
		})
	}
}

func TestCreateProjectDuplicateCode(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	code := "PRJ-" + uuid.NewString()[:8]

	payload := map[string]any{"company_id": companyID, "project_code": code, "name": "Proyek A"}
	requireStatus(t, postJSON(t, srv.URL+"/projects", payload), http.StatusCreated)

	resp := postJSON(t, srv.URL+"/projects", payload)
	requireStatus(t, resp, http.StatusConflict)

	// Kode yang sama di company LAIN harus tetap boleh -- unique-nya per company,
	// bukan global.
	other := map[string]any{"company_id": newCompanyID(t), "project_code": code, "name": "Proyek A"}
	requireStatus(t, postJSON(t, srv.URL+"/projects", other), http.StatusCreated)
}

func TestListProjectsIsCompanyScoped(t *testing.T) {
	srv := newServer(t)
	companyA, companyB := newCompanyID(t), newCompanyID(t)
	mustSeedProject(t, srv, companyA)
	mustSeedProject(t, srv, companyB)

	resp := getJSON(t, srv.URL+"/projects?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var projects []projectFixture
	resp.decode(t, &projects)
	if len(projects) != 1 {
		t.Fatalf("expected exactly 1 project for company A, got %d", len(projects))
	}

	requireStatus(t, getJSON(t, srv.URL+"/projects"), http.StatusBadRequest)
}

// Filter branch_id NULL-inclusive, pola yang sama seperti 21 endpoint list
// lain di platform ini: baris tanpa branch (company-wide) harus tetap ikut
// terbawa saat memfilter per branch.
func TestListProjectsBranchFilterIsNullInclusive(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	branchID := uuid.NewString()

	mustSeedProject(t, srv, companyID) // branch_id NULL
	withBranch := postJSON(t, srv.URL+"/projects", map[string]any{
		"company_id":   companyID,
		"branch_id":    branchID,
		"project_code": "PRJ-" + uuid.NewString()[:8],
		"name":         "Proyek Cabang",
	})
	requireStatus(t, withBranch, http.StatusCreated)

	resp := getJSON(t, srv.URL+"/projects?company_id="+companyID+"&branch_id="+branchID)
	requireStatus(t, resp, http.StatusOK)
	var projects []projectFixture
	resp.decode(t, &projects)
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects (branch + company-wide), got %d", len(projects))
	}
}

func TestProjectStatusTransitions(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	t.Run("PLANNING tidak bisa langsung complete", func(t *testing.T) {
		p := mustSeedProject(t, srv, companyID)
		requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/complete", nil), http.StatusConflict)
	})

	t.Run("activate lalu hold lalu activate lagi", func(t *testing.T) {
		p := mustSeedProject(t, srv, companyID)
		requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/activate", nil), http.StatusOK)
		if got := fetchProject(t, srv, p.ID).Status; got != "ACTIVE" {
			t.Fatalf("expected ACTIVE, got %s", got)
		}
		requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/hold", nil), http.StatusOK)
		if got := fetchProject(t, srv, p.ID).Status; got != "ON_HOLD" {
			t.Fatalf("expected ON_HOLD, got %s", got)
		}
		// ON_HOLD -> ACTIVE diizinkan (proyek dilanjutkan kembali).
		requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/activate", nil), http.StatusOK)
		// Tapi hold dua kali tidak.
		requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/hold", nil), http.StatusOK)
		requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/hold", nil), http.StatusConflict)
	})

	t.Run("cancel lalu tidak bisa diapa-apakan lagi", func(t *testing.T) {
		p := mustSeedProject(t, srv, companyID)
		requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/cancel", nil), http.StatusOK)
		requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/activate", nil), http.StatusConflict)
		requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/cancel", nil), http.StatusConflict)
		// PUT pada proyek yang sudah batal juga ditolak.
		requireStatus(t, putJSON(t, srv.URL+"/projects/"+p.ID, map[string]any{
			"company_id": companyID, "name": "Ganti Nama",
		}), http.StatusConflict)
	})

	t.Run("proyek tidak ada", func(t *testing.T) {
		requireStatus(t, postJSON(t, srv.URL+"/projects/"+uuid.NewString()+"/activate", nil), http.StatusNotFound)
	})
}

// Guard lintas-entitas: proyek tidak bisa ditutup selagi masih ada tugas
// terbuka. Ini aturan yang membedakan modul ini dari CRUD biasa, jadi diuji
// sampai ke titik "setelah tugasnya diselesaikan, complete berhasil".
func TestCompleteProjectBlockedByOpenTasks(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	p := mustSeedActiveProject(t, srv, companyID)
	task := mustSeedTask(t, srv, companyID, p.ID)

	resp := postJSON(t, srv.URL+"/projects/"+p.ID+"/complete", nil)
	requireStatus(t, resp, http.StatusConflict)
	if msg := resp.errorMessage(); msg == "" {
		t.Fatal("expected an explanatory error message about open tasks")
	}

	requireStatus(t, putJSON(t, srv.URL+"/tasks/"+task.ID, map[string]any{
		"company_id": companyID, "title": task.Title, "status": "DONE",
	}), http.StatusOK)

	requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/complete", nil), http.StatusOK)
	completed := fetchProject(t, srv, p.ID)
	if completed.Status != "COMPLETED" {
		t.Fatalf("expected COMPLETED, got %s", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

// Tugas yang DIBATALKAN tidak boleh menahan penutupan proyek -- hanya
// TODO/IN_PROGRESS yang dihitung terbuka.
func TestCompleteProjectIgnoresCancelledTasks(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	p := mustSeedActiveProject(t, srv, companyID)
	task := mustSeedTask(t, srv, companyID, p.ID)

	requireStatus(t, putJSON(t, srv.URL+"/tasks/"+task.ID, map[string]any{
		"company_id": companyID, "title": task.Title, "status": "CANCELLED",
	}), http.StatusOK)

	requireStatus(t, postJSON(t, srv.URL+"/projects/"+p.ID+"/complete", nil), http.StatusOK)
}

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

func TestCreateTaskValidation(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	p := mustSeedActiveProject(t, srv, companyID)

	cases := []struct {
		name    string
		payload map[string]any
		want    int
	}{
		{"tanpa project_id", map[string]any{"company_id": companyID, "title": "X"}, http.StatusBadRequest},
		{"tanpa title", map[string]any{"company_id": companyID, "project_id": p.ID}, http.StatusBadRequest},
		{"priority ngawur", map[string]any{"company_id": companyID, "project_id": p.ID, "title": "X", "priority": "URGENT"}, http.StatusBadRequest},
		{"estimated_hours negatif", map[string]any{"company_id": companyID, "project_id": p.ID, "title": "X", "estimated_hours": -3}, http.StatusBadRequest},
		{"due_date bukan tanggal", map[string]any{"company_id": companyID, "project_id": p.ID, "title": "X", "due_date": "besok"}, http.StatusBadRequest},
		{"proyek tidak ada", map[string]any{"company_id": companyID, "project_id": uuid.NewString(), "title": "X"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/tasks", tc.payload), tc.want)
		})
	}
}

// Guard lintas-company: proyek milik company lain tidak boleh dipakai walau
// UUID-nya benar.
func TestCreateTaskRejectsProjectFromAnotherCompany(t *testing.T) {
	srv := newServer(t)
	companyA, companyB := newCompanyID(t), newCompanyID(t)
	projectA := mustSeedActiveProject(t, srv, companyA)

	resp := postJSON(t, srv.URL+"/tasks", map[string]any{
		"company_id": companyB, "project_id": projectA.ID, "title": "Nyelonong",
	})
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestTaskNumberIsSequentialPerCompany(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	p := mustSeedActiveProject(t, srv, companyID)

	first := mustSeedTask(t, srv, companyID, p.ID)
	second := mustSeedTask(t, srv, companyID, p.ID)
	if first.TaskNumber == second.TaskNumber {
		t.Fatalf("task numbers must differ, both were %s", first.TaskNumber)
	}
	if first.TaskNumber[:4] != "TSK-" {
		t.Fatalf("expected TSK- prefix, got %s", first.TaskNumber)
	}
}

// completed_at otomatis: terisi saat DONE, dikosongkan lagi saat tugas dibuka
// kembali. Tanpa yang kedua, tugas yang di-reopen membawa tanggal selesai yang
// sudah tidak benar.
func TestTaskCompletedAtFollowsStatus(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	p := mustSeedActiveProject(t, srv, companyID)
	task := mustSeedTask(t, srv, companyID, p.ID)

	if task.CompletedAt != nil {
		t.Fatal("a fresh task must not have completed_at")
	}

	done := putJSON(t, srv.URL+"/tasks/"+task.ID, map[string]any{
		"company_id": companyID, "title": task.Title, "status": "DONE",
	})
	requireStatus(t, done, http.StatusOK)
	var doneTask taskFixture
	done.decode(t, &doneTask)
	if doneTask.CompletedAt == nil {
		t.Fatal("expected completed_at after moving to DONE")
	}

	reopened := putJSON(t, srv.URL+"/tasks/"+task.ID, map[string]any{
		"company_id": companyID, "title": task.Title, "status": "IN_PROGRESS",
	})
	requireStatus(t, reopened, http.StatusOK)
	var reopenedTask taskFixture
	reopened.decode(t, &reopenedTask)
	if reopenedTask.CompletedAt != nil {
		t.Fatalf("expected completed_at to be cleared on reopen, got %v", *reopenedTask.CompletedAt)
	}
}

func TestTaskUpdateRejectsUnknownStatus(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	p := mustSeedActiveProject(t, srv, companyID)
	task := mustSeedTask(t, srv, companyID, p.ID)

	requireStatus(t, putJSON(t, srv.URL+"/tasks/"+task.ID, map[string]any{
		"company_id": companyID, "title": task.Title, "status": "ALMOST_DONE",
	}), http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// Integrasi hr-service (snapshot nama + guard status karyawan)
// ---------------------------------------------------------------------------

func TestAssignManagerSnapshotsEmployeeName(t *testing.T) {
	companyID := newCompanyID(t)
	employeeID := uuid.NewString()
	srv, hrCalls, _ := newServerWithStubs(t, hrStubConfig{
		employeeID: employeeID, companyID: companyID,
		firstName: "Budi", lastName: "Santoso", status: "ACTIVE", basicSalary: 17300000,
	}, financeStubConfig{journalEntryID: uuid.NewString()})

	resp := postJSON(t, srv.URL+"/projects", map[string]any{
		"company_id":          companyID,
		"project_code":        "PRJ-" + uuid.NewString()[:8],
		"name":                "Proyek dengan manajer",
		"manager_employee_id": employeeID,
	})
	requireStatus(t, resp, http.StatusCreated)
	var p projectFixture
	resp.decode(t, &p)

	if p.ManagerName == nil || *p.ManagerName != "Budi Santoso" {
		t.Fatalf("expected manager_name snapshot %q, got %v", "Budi Santoso", p.ManagerName)
	}
	if len(*hrCalls) != 1 {
		t.Fatalf("expected exactly 1 call to hr-service, got %d", len(*hrCalls))
	}
}

func TestAssignEmployeeGuards(t *testing.T) {
	companyID := newCompanyID(t)
	employeeID := uuid.NewString()

	t.Run("karyawan non-ACTIVE ditolak", func(t *testing.T) {
		srv, _, _ := newServerWithStubs(t, hrStubConfig{
			employeeID: employeeID, companyID: companyID,
			firstName: "Rina", lastName: "Wijaya", status: "TERMINATED",
		}, financeStubConfig{})
		resp := postJSON(t, srv.URL+"/projects", map[string]any{
			"company_id": companyID, "project_code": "PRJ-" + uuid.NewString()[:8],
			"name": "X", "manager_employee_id": employeeID,
		})
		requireStatus(t, resp, http.StatusConflict)
	})

	t.Run("karyawan milik company lain ditolak", func(t *testing.T) {
		srv, _, _ := newServerWithStubs(t, hrStubConfig{
			employeeID: employeeID, companyID: newCompanyID(t),
			firstName: "Rina", lastName: "Wijaya", status: "ACTIVE",
		}, financeStubConfig{})
		resp := postJSON(t, srv.URL+"/projects", map[string]any{
			"company_id": companyID, "project_code": "PRJ-" + uuid.NewString()[:8],
			"name": "X", "manager_employee_id": employeeID,
		})
		requireStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("hr-service mati menghasilkan 502", func(t *testing.T) {
		srv, _, _ := newServerWithStubs(t, hrStubConfig{fails: true}, financeStubConfig{})
		resp := postJSON(t, srv.URL+"/projects", map[string]any{
			"company_id": companyID, "project_code": "PRJ-" + uuid.NewString()[:8],
			"name": "X", "manager_employee_id": employeeID,
		})
		requireStatus(t, resp, http.StatusBadGateway)
	})

	t.Run("tanpa manager, hr-service tidak dipanggil sama sekali", func(t *testing.T) {
		srv, hrCalls, _ := newServerWithStubs(t, hrStubConfig{
			employeeID: employeeID, companyID: companyID, status: "ACTIVE",
		}, financeStubConfig{})
		mustSeedProject(t, srv, companyID)
		if len(*hrCalls) != 0 {
			t.Fatalf("expected no hr-service calls, got %d", len(*hrCalls))
		}
	})
}

// ---------------------------------------------------------------------------
// Timesheets
// ---------------------------------------------------------------------------

func newTimesheetServer(t *testing.T, companyID, employeeID string, salary float64) (*testServerBundle, string) {
	t.Helper()
	journalID := uuid.NewString()
	srv, hrCalls, financeCalls := newServerWithStubs(t, hrStubConfig{
		employeeID: employeeID, companyID: companyID,
		firstName: "Dewi", lastName: "Lestari", status: "ACTIVE", basicSalary: salary,
	}, financeStubConfig{journalEntryID: journalID})
	return &testServerBundle{srv: srv, hrCalls: hrCalls, financeCalls: financeCalls}, journalID
}

func TestCreateTimesheetRequiresActiveProject(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()
	b, _ := newTimesheetServer(t, companyID, employeeID, 17300000)

	planning := mustSeedProject(t, b.srv, companyID)
	resp := postJSON(t, b.srv.URL+"/timesheets", map[string]any{
		"company_id": companyID, "project_id": planning.ID, "employee_id": employeeID, "hours": 4,
	})
	requireStatus(t, resp, http.StatusConflict)

	requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+planning.ID+"/activate", nil), http.StatusOK)
	requireStatus(t, postJSON(t, b.srv.URL+"/timesheets", map[string]any{
		"company_id": companyID, "project_id": planning.ID, "employee_id": employeeID, "hours": 4,
	}), http.StatusCreated)
}

// Tarif default diturunkan dari gaji pokok hr-service dengan pembagi 173.
// Angka sengaja dipilih supaya hasilnya bulat dan bisa dihitung tangan:
// 17.300.000 / 173 = 100.000 per jam, 4 jam = 400.000.
func TestTimesheetDerivesHourlyRateFromSalary(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()
	b, _ := newTimesheetServer(t, companyID, employeeID, 17300000)
	p := mustSeedActiveProject(t, b.srv, companyID)

	resp := postJSON(t, b.srv.URL+"/timesheets", map[string]any{
		"company_id": companyID, "project_id": p.ID, "employee_id": employeeID, "hours": 4,
	})
	requireStatus(t, resp, http.StatusCreated)
	var ts timesheetFixture
	resp.decode(t, &ts)

	if ts.HourlyRate != 100000 {
		t.Fatalf("expected derived hourly_rate 100000, got %v", ts.HourlyRate)
	}
	if ts.Amount != 400000 {
		t.Fatalf("expected amount 400000, got %v", ts.Amount)
	}
	if ts.EmployeeName != "Dewi Lestari" {
		t.Fatalf("expected employee_name snapshot, got %q", ts.EmployeeName)
	}
	if ts.Status != "DRAFT" {
		t.Fatalf("expected new timesheet to be DRAFT, got %s", ts.Status)
	}
}

// Tarif eksplisit harus MENANG atas tarif turunan -- tarif billing proyek
// sering berbeda dari gaji karyawan.
func TestExplicitHourlyRateWinsOverDerived(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()
	b, _ := newTimesheetServer(t, companyID, employeeID, 17300000)
	p := mustSeedActiveProject(t, b.srv, companyID)

	resp := postJSON(t, b.srv.URL+"/timesheets", map[string]any{
		"company_id": companyID, "project_id": p.ID, "employee_id": employeeID,
		"hours": 2, "hourly_rate": 250000,
	})
	requireStatus(t, resp, http.StatusCreated)
	var ts timesheetFixture
	resp.decode(t, &ts)
	if ts.HourlyRate != 250000 || ts.Amount != 500000 {
		t.Fatalf("expected rate 250000 and amount 500000, got %v / %v", ts.HourlyRate, ts.Amount)
	}
}

func TestTimesheetValidation(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()
	b, _ := newTimesheetServer(t, companyID, employeeID, 17300000)
	p := mustSeedActiveProject(t, b.srv, companyID)

	cases := []struct {
		name    string
		payload map[string]any
		want    int
	}{
		{"hours nol", map[string]any{"company_id": companyID, "project_id": p.ID, "employee_id": employeeID, "hours": 0}, http.StatusBadRequest},
		{"hours lebih dari 24", map[string]any{"company_id": companyID, "project_id": p.ID, "employee_id": employeeID, "hours": 25}, http.StatusBadRequest},
		{"hours negatif", map[string]any{"company_id": companyID, "project_id": p.ID, "employee_id": employeeID, "hours": -2}, http.StatusBadRequest},
		{"tarif negatif", map[string]any{"company_id": companyID, "project_id": p.ID, "employee_id": employeeID, "hours": 2, "hourly_rate": -1}, http.StatusBadRequest},
		{"tanpa employee_id", map[string]any{"company_id": companyID, "project_id": p.ID, "hours": 2}, http.StatusBadRequest},
		{"work_date bukan tanggal", map[string]any{"company_id": companyID, "project_id": p.ID, "employee_id": employeeID, "hours": 2, "work_date": "kemarin"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, postJSON(t, b.srv.URL+"/timesheets", tc.payload), tc.want)
		})
	}
}

// task_id opsional, tapi kalau diisi harus milik proyek yang sama.
func TestTimesheetRejectsTaskFromAnotherProject(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()
	b, _ := newTimesheetServer(t, companyID, employeeID, 17300000)
	projectA := mustSeedActiveProject(t, b.srv, companyID)
	projectB := mustSeedActiveProject(t, b.srv, companyID)
	taskInB := mustSeedTask(t, b.srv, companyID, projectB.ID)

	resp := postJSON(t, b.srv.URL+"/timesheets", map[string]any{
		"company_id": companyID, "project_id": projectA.ID, "employee_id": employeeID,
		"hours": 2, "task_id": taskInB.ID,
	})
	requireStatus(t, resp, http.StatusBadRequest)

	taskInA := mustSeedTask(t, b.srv, companyID, projectA.ID)
	requireStatus(t, postJSON(t, b.srv.URL+"/timesheets", map[string]any{
		"company_id": companyID, "project_id": projectA.ID, "employee_id": employeeID,
		"hours": 2, "task_id": taskInA.ID,
	}), http.StatusCreated)
}

func TestTimesheetApprovalFlow(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()
	b, _ := newTimesheetServer(t, companyID, employeeID, 17300000)
	p := mustSeedActiveProject(t, b.srv, companyID)

	ts := mustSeedTimesheet(t, b.srv, companyID, p.ID, employeeID, 4)
	requireStatus(t, postJSON(t, b.srv.URL+"/timesheets/"+ts.ID+"/approve", nil), http.StatusOK)
	// Approve dua kali ditolak.
	requireStatus(t, postJSON(t, b.srv.URL+"/timesheets/"+ts.ID+"/approve", nil), http.StatusConflict)
	// Reject dari APPROVED masih boleh (persetujuan bisa ditarik sebelum posting).
	requireStatus(t, postJSON(t, b.srv.URL+"/timesheets/"+ts.ID+"/reject", nil), http.StatusOK)
}

// ---------------------------------------------------------------------------
// Integrasi finance-service (posting biaya proyek ke GL)
// ---------------------------------------------------------------------------

func TestPostProjectCostHappyPath(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()
	b, journalID := newTimesheetServer(t, companyID, employeeID, 17300000)
	p := mustSeedActiveProject(t, b.srv, companyID)

	// Dua timesheet APPROVED (4 jam + 2 jam @100.000 = 600.000) dan satu yang
	// sengaja dibiarkan DRAFT supaya terbukti TIDAK ikut terposting.
	first := mustSeedTimesheet(t, b.srv, companyID, p.ID, employeeID, 4)
	second := mustSeedTimesheet(t, b.srv, companyID, p.ID, employeeID, 2)
	draft := mustSeedTimesheet(t, b.srv, companyID, p.ID, employeeID, 3)
	requireStatus(t, postJSON(t, b.srv.URL+"/timesheets/"+first.ID+"/approve", nil), http.StatusOK)
	requireStatus(t, postJSON(t, b.srv.URL+"/timesheets/"+second.ID+"/approve", nil), http.StatusOK)

	expenseAccount, payableAccount := uuid.NewString(), uuid.NewString()
	resp := postJSON(t, b.srv.URL+"/projects/"+p.ID+"/post-cost", map[string]any{
		"company_id":         companyID,
		"expense_account_id": expenseAccount,
		"payable_account_id": payableAccount,
	})
	requireStatus(t, resp, http.StatusOK)
	var out postCostFixture
	resp.decode(t, &out)

	if out.PostedCount != 2 {
		t.Fatalf("expected 2 timesheets posted, got %d", out.PostedCount)
	}
	if out.PostedAmount != 600000 {
		t.Fatalf("expected posted amount 600000, got %v", out.PostedAmount)
	}
	if out.JournalEntryID != journalID {
		t.Fatalf("expected journal entry id from finance-service, got %q", out.JournalEntryID)
	}
	if out.Project.ActualCost != 600000 {
		t.Fatalf("expected project actual_cost 600000, got %v", out.Project.ActualCost)
	}

	// Jurnal yang dikirim harus BALANCE dan memakai akun yang diminta -- kalau
	// tidak, finance-service asli akan menolaknya dengan 400.
	if len(*b.financeCalls) != 2 {
		t.Fatalf("expected create + post calls to finance-service, got %d", len(*b.financeCalls))
	}
	body := (*b.financeCalls)[0].body
	if body["reference_type"] != "project_cost" {
		t.Fatalf("expected reference_type project_cost, got %v", body["reference_type"])
	}
	lines, ok := body["lines"].([]any)
	if !ok || len(lines) != 2 {
		t.Fatalf("expected 2 journal lines, got %v", body["lines"])
	}
	debit := lines[0].(map[string]any)
	credit := lines[1].(map[string]any)
	if debit["account_id"] != expenseAccount || debit["debit_amount"].(float64) != 600000 {
		t.Fatalf("unexpected debit line: %v", debit)
	}
	if credit["account_id"] != payableAccount || credit["credit_amount"].(float64) != 600000 {
		t.Fatalf("unexpected credit line: %v", credit)
	}

	// Status akhir tiap timesheet: dua POSTED dengan journal_entry_id, satu
	// tetap DRAFT.
	byID := map[string]timesheetFixture{}
	for _, ts := range fetchTimesheets(t, b.srv, companyID, p.ID) {
		byID[ts.ID] = ts
	}
	for _, id := range []string{first.ID, second.ID} {
		if byID[id].Status != "POSTED" {
			t.Fatalf("expected timesheet %s to be POSTED, got %s", id, byID[id].Status)
		}
		if byID[id].JournalEntryID == nil || *byID[id].JournalEntryID != journalID {
			t.Fatalf("expected timesheet %s to carry the journal entry id", id)
		}
	}
	if byID[draft.ID].Status != "DRAFT" {
		t.Fatalf("expected untouched timesheet to stay DRAFT, got %s", byID[draft.ID].Status)
	}
}

// Posting kedua kali tanpa persetujuan baru harus ditolak -- inilah yang
// mencegah biaya yang sama masuk GL dua kali.
func TestPostProjectCostTwiceIsRejected(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()
	b, _ := newTimesheetServer(t, companyID, employeeID, 17300000)
	p := mustSeedActiveProject(t, b.srv, companyID)
	ts := mustSeedTimesheet(t, b.srv, companyID, p.ID, employeeID, 4)
	requireStatus(t, postJSON(t, b.srv.URL+"/timesheets/"+ts.ID+"/approve", nil), http.StatusOK)

	payload := map[string]any{
		"company_id": companyID, "expense_account_id": uuid.NewString(), "payable_account_id": uuid.NewString(),
	}
	requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+p.ID+"/post-cost", payload), http.StatusOK)
	requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+p.ID+"/post-cost", payload), http.StatusConflict)

	// actual_cost tidak boleh ikut bertambah dua kali.
	if got := fetchProject(t, b.srv, p.ID).ActualCost; got != 400000 {
		t.Fatalf("expected actual_cost to stay 400000, got %v", got)
	}
}

// Kegagalan finance-service TIDAK boleh meninggalkan state setengah jadi:
// timesheet harus tetap APPROVED (bisa dicoba lagi) dan actual_cost tetap 0.
func TestPostProjectCostRollsBackWhenFinanceFails(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()

	for _, tc := range []struct {
		name string
		financeStubConfig
	}{
		{"gagal saat membuat jurnal", financeStubConfig{journalEntryID: uuid.NewString(), createFails: true}},
		{"gagal saat memposting jurnal", financeStubConfig{journalEntryID: uuid.NewString(), postFails: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _, _ := newServerWithStubs(t, hrStubConfig{
				employeeID: employeeID, companyID: companyID,
				firstName: "Dewi", lastName: "Lestari", status: "ACTIVE", basicSalary: 17300000,
			}, tc.financeStubConfig)

			p := mustSeedActiveProject(t, srv, companyID)
			ts := mustSeedTimesheet(t, srv, companyID, p.ID, employeeID, 4)
			requireStatus(t, postJSON(t, srv.URL+"/timesheets/"+ts.ID+"/approve", nil), http.StatusOK)

			resp := postJSON(t, srv.URL+"/projects/"+p.ID+"/post-cost", map[string]any{
				"company_id": companyID, "expense_account_id": uuid.NewString(), "payable_account_id": uuid.NewString(),
			})
			requireStatus(t, resp, http.StatusBadGateway)

			if got := fetchProject(t, srv, p.ID).ActualCost; got != 0 {
				t.Fatalf("expected actual_cost to stay 0 after a failed posting, got %v", got)
			}
			all := fetchTimesheets(t, srv, companyID, p.ID)
			if len(all) != 1 || all[0].Status != "APPROVED" {
				t.Fatalf("expected the timesheet to stay APPROVED and retryable, got %+v", all)
			}
			if all[0].JournalEntryID != nil {
				t.Fatal("expected no journal_entry_id after a failed posting")
			}
		})
	}
}

func TestPostProjectCostValidation(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()
	b, _ := newTimesheetServer(t, companyID, employeeID, 17300000)
	p := mustSeedActiveProject(t, b.srv, companyID)

	t.Run("tanpa akun", func(t *testing.T) {
		requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+p.ID+"/post-cost", map[string]any{
			"company_id": companyID,
		}), http.StatusBadRequest)
	})

	t.Run("tidak ada timesheet APPROVED", func(t *testing.T) {
		requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+p.ID+"/post-cost", map[string]any{
			"company_id": companyID, "expense_account_id": uuid.NewString(), "payable_account_id": uuid.NewString(),
		}), http.StatusConflict)
	})

	t.Run("proyek tidak ACTIVE", func(t *testing.T) {
		planning := mustSeedProject(t, b.srv, companyID)
		requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+planning.ID+"/post-cost", map[string]any{
			"company_id": companyID, "expense_account_id": uuid.NewString(), "payable_account_id": uuid.NewString(),
		}), http.StatusConflict)
	})

	t.Run("proyek company lain", func(t *testing.T) {
		requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+p.ID+"/post-cost", map[string]any{
			"company_id": newCompanyID(t), "expense_account_id": uuid.NewString(), "payable_account_id": uuid.NewString(),
		}), http.StatusNotFound)
	})
}

// Guard akuntansi pada penutupan proyek: timesheet APPROVED yang belum
// diposting adalah biaya yang sudah diakui tapi belum masuk GL. Kalau proyek
// keburu ditutup, actual_cost-nya permanen understated.
func TestCompleteProjectBlockedByUnpostedTimesheets(t *testing.T) {
	companyID, employeeID := newCompanyID(t), uuid.NewString()
	b, _ := newTimesheetServer(t, companyID, employeeID, 17300000)
	p := mustSeedActiveProject(t, b.srv, companyID)
	ts := mustSeedTimesheet(t, b.srv, companyID, p.ID, employeeID, 4)

	// DRAFT menahan.
	requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+p.ID+"/complete", nil), http.StatusConflict)

	// APPROVED tapi belum diposting juga menahan.
	requireStatus(t, postJSON(t, b.srv.URL+"/timesheets/"+ts.ID+"/approve", nil), http.StatusOK)
	requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+p.ID+"/complete", nil), http.StatusConflict)

	// Setelah diposting ke GL, penutupan berhasil.
	requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+p.ID+"/post-cost", map[string]any{
		"company_id": companyID, "expense_account_id": uuid.NewString(), "payable_account_id": uuid.NewString(),
	}), http.StatusOK)
	requireStatus(t, postJSON(t, b.srv.URL+"/projects/"+p.ID+"/complete", nil), http.StatusOK)
}
