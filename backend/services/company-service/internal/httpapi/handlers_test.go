package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestUpdateBranchChangesNameAddressAndStatus(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)
	branch := mustSeedBranch(t, srv, company.ID)

	resp := putJSON(t, srv.URL+"/companies/"+company.ID+"/branches/"+branch.ID, map[string]any{
		"name":    "Cabang Bandung",
		"address": "Jl. Asia Afrika 10",
		"status":  "inactive",
	})
	requireStatus(t, resp, http.StatusOK)

	var updated branchFixture
	resp.decode(t, &updated)
	if updated.Name != "Cabang Bandung" {
		t.Errorf("expected name to change, got %q", updated.Name)
	}
	if updated.Address != "Jl. Asia Afrika 10" {
		t.Errorf("expected address to change, got %q", updated.Address)
	}
	if updated.Status != "inactive" {
		t.Errorf("expected status inactive, got %q", updated.Status)
	}
	// Code sengaja immutable (tidak ada di updateBranchRequest), sama seperti
	// code company: dia identitas master data yang dirujuk service lain.
	if updated.Code != branch.Code {
		t.Errorf("expected code to stay %q, got %q", branch.Code, updated.Code)
	}
}

func TestUpdateBranchRejectsEmptyName(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)
	branch := mustSeedBranch(t, srv, company.ID)

	resp := putJSON(t, srv.URL+"/companies/"+company.ID+"/branches/"+branch.ID, map[string]any{
		"name": "   ",
	})
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateBranchRejectsUnknownStatus(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)
	branch := mustSeedBranch(t, srv, company.ID)

	resp := putJSON(t, srv.URL+"/companies/"+company.ID+"/branches/"+branch.ID, map[string]any{
		"name":   "Cabang Bandung",
		"status": "archived",
	})
	requireStatus(t, resp, http.StatusBadRequest)
}

// Branch milik company lain tidak boleh bisa diubah lewat URL company yang
// salah. Ini yang dijaga predikat company_id di klausa WHERE, bukan sekadar id.
func TestUpdateBranchIsScopedToItsCompany(t *testing.T) {
	srv := newServer(t)
	owner := mustSeedCompany(t, srv)
	other := mustSeedCompany(t, srv)
	branch := mustSeedBranch(t, srv, owner.ID)

	resp := putJSON(t, srv.URL+"/companies/"+other.ID+"/branches/"+branch.ID, map[string]any{
		"name": "Diambil alih",
	})
	requireStatus(t, resp, http.StatusNotFound)

	after := fetchBranches(t, srv, owner.ID)
	if len(after) != 1 || after[0].Name != branch.Name {
		t.Fatalf("branch milik company lain ikut berubah: %+v", after)
	}
}

func TestDeleteBranchRemovesIt(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)
	branch := mustSeedBranch(t, srv, company.ID)

	requireStatus(t, deleteJSON(t, srv.URL+"/companies/"+company.ID+"/branches/"+branch.ID), http.StatusOK)

	if remaining := fetchBranches(t, srv, company.ID); len(remaining) != 0 {
		t.Fatalf("expected branch to be gone, got %+v", remaining)
	}
	// Penghapusan kedua harus 404, bukan 200 -- membuktikan responsnya benar-
	// benar mencerminkan baris yang terhapus, bukan sekadar "perintah diterima".
	requireStatus(t, deleteJSON(t, srv.URL+"/companies/"+company.ID+"/branches/"+branch.ID), http.StatusNotFound)
}

// Ini test terpenting dari kumpulan ini: departments.branch_id memakai
// ON DELETE CASCADE, jadi tanpa guard di handler, satu klik hapus branch akan
// menghapus department di bawahnya tanpa peringatan.
func TestDeleteBranchRefusedWhileDepartmentsStillAttached(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)
	branch := mustSeedBranch(t, srv, company.ID)
	department := mustSeedDepartment(t, srv, company.ID, &branch.ID)

	resp := deleteJSON(t, srv.URL+"/companies/"+company.ID+"/branches/"+branch.ID)
	requireStatus(t, resp, http.StatusConflict)
	if msg := resp.errorMessage(); msg == "" {
		t.Error("expected an explanatory error message")
	}

	if branches := fetchBranches(t, srv, company.ID); len(branches) != 1 {
		t.Fatalf("branch seharusnya masih ada, got %+v", branches)
	}
	departments := fetchDepartments(t, srv, company.ID)
	if len(departments) != 1 || departments[0].ID != department.ID {
		t.Fatalf("department ikut terhapus oleh CASCADE: %+v", departments)
	}

	// Setelah department dilepas dari branch, penghapusan boleh jalan.
	requireStatus(t, putJSON(t, srv.URL+"/companies/"+company.ID+"/departments/"+department.ID, map[string]any{
		"name":      departments[0].Name,
		"branch_id": nil,
	}), http.StatusOK)
	requireStatus(t, deleteJSON(t, srv.URL+"/companies/"+company.ID+"/branches/"+branch.ID), http.StatusOK)
}

func TestUpdateDepartmentMovesBranchAndClearsIt(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)
	first := mustSeedBranch(t, srv, company.ID)
	second := mustSeedBranch(t, srv, company.ID)
	department := mustSeedDepartment(t, srv, company.ID, &first.ID)

	resp := putJSON(t, srv.URL+"/companies/"+company.ID+"/departments/"+department.ID, map[string]any{
		"name":      "Keuangan & Pajak",
		"branch_id": second.ID,
	})
	requireStatus(t, resp, http.StatusOK)
	var moved departmentFixture
	resp.decode(t, &moved)
	if moved.BranchID == nil || *moved.BranchID != second.ID {
		t.Fatalf("expected branch_id %s, got %v", second.ID, moved.BranchID)
	}
	if moved.Name != "Keuangan & Pajak" {
		t.Errorf("expected name to change, got %q", moved.Name)
	}

	// branch_id null berarti department berlaku company-wide.
	resp = putJSON(t, srv.URL+"/companies/"+company.ID+"/departments/"+department.ID, map[string]any{
		"name":      "Keuangan & Pajak",
		"branch_id": nil,
	})
	requireStatus(t, resp, http.StatusOK)
	resp.decode(t, &moved)
	if moved.BranchID != nil {
		t.Fatalf("expected branch_id to be cleared, got %v", *moved.BranchID)
	}
}

// String kosong datang dari <select> HTML yang opsinya "(company-wide)".
// Diperlakukan sama dengan null, bukan diteruskan ke Postgres sebagai UUID
// tidak valid yang berakhir 500.
func TestUpdateDepartmentTreatsEmptyBranchIDAsCompanyWide(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)
	branch := mustSeedBranch(t, srv, company.ID)
	department := mustSeedDepartment(t, srv, company.ID, &branch.ID)

	resp := putJSON(t, srv.URL+"/companies/"+company.ID+"/departments/"+department.ID, map[string]any{
		"name":      "Keuangan",
		"branch_id": "",
	})
	requireStatus(t, resp, http.StatusOK)
	var updated departmentFixture
	resp.decode(t, &updated)
	if updated.BranchID != nil {
		t.Fatalf("expected empty branch_id to become NULL, got %v", *updated.BranchID)
	}
}

// FK di skema hanya menjamin branch-nya ADA, bukan bahwa dia milik company
// yang sama -- tanpa cek eksplisit, department bisa dipindahkan ke branch
// milik company lain dan tetap diterima Postgres.
func TestUpdateDepartmentRejectsBranchFromAnotherCompany(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)
	foreign := mustSeedCompany(t, srv)
	foreignBranch := mustSeedBranch(t, srv, foreign.ID)
	department := mustSeedDepartment(t, srv, company.ID, nil)

	resp := putJSON(t, srv.URL+"/companies/"+company.ID+"/departments/"+department.ID, map[string]any{
		"name":      "Keuangan",
		"branch_id": foreignBranch.ID,
	})
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestCreateDepartmentRejectsBranchFromAnotherCompany(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)
	foreign := mustSeedCompany(t, srv)
	foreignBranch := mustSeedBranch(t, srv, foreign.ID)

	resp := postJSON(t, srv.URL+"/companies/"+company.ID+"/departments", map[string]any{
		"code":      shortCode("DEP"),
		"name":      "Keuangan",
		"branch_id": foreignBranch.ID,
	})
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateDepartmentUnknownIDReturns404(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)

	resp := putJSON(t, srv.URL+"/companies/"+company.ID+"/departments/"+uuid.NewString(), map[string]any{
		"name": "Tidak ada",
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestDeleteDepartmentRemovesIt(t *testing.T) {
	srv := newServer(t)
	company := mustSeedCompany(t, srv)
	department := mustSeedDepartment(t, srv, company.ID, nil)

	requireStatus(t, deleteJSON(t, srv.URL+"/companies/"+company.ID+"/departments/"+department.ID), http.StatusOK)
	if remaining := fetchDepartments(t, srv, company.ID); len(remaining) != 0 {
		t.Fatalf("expected department to be gone, got %+v", remaining)
	}
	requireStatus(t, deleteJSON(t, srv.URL+"/companies/"+company.ID+"/departments/"+department.ID), http.StatusNotFound)
}

func TestDeleteDepartmentIsScopedToItsCompany(t *testing.T) {
	srv := newServer(t)
	owner := mustSeedCompany(t, srv)
	other := mustSeedCompany(t, srv)
	department := mustSeedDepartment(t, srv, owner.ID, nil)

	requireStatus(t, deleteJSON(t, srv.URL+"/companies/"+other.ID+"/departments/"+department.ID), http.StatusNotFound)
	if remaining := fetchDepartments(t, srv, owner.ID); len(remaining) != 1 {
		t.Fatalf("department milik company lain ikut terhapus: %+v", remaining)
	}
}
