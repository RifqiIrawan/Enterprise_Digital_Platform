package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCreateRoleNormalizesCodeAndMarksItNonSystem(t *testing.T) {
	srv := newServer(t)

	suffix := strings.ToLower(uuid.NewString()[:8])
	resp := postJSON(t, srv.URL+"/roles", map[string]any{
		"code":        "  Gudang Malam " + suffix + " ",
		"name":        "  Gudang Shift Malam  ",
		"description": "Role custom",
	})
	requireStatus(t, resp, http.StatusCreated)

	var role roleFixture
	resp.decode(t, &role)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM roles WHERE id = $1`, role.ID)
	})

	// Spasi jadi underscore + lowercase: code dipakai sebagai kunci di
	// frontend & seed permission, jadi bentuknya harus konsisten.
	if want := "gudang_malam_" + suffix; role.Code != want {
		t.Errorf("expected code %q, got %q", want, role.Code)
	}
	if role.Name != "Gudang Shift Malam" {
		t.Errorf("expected name to be trimmed, got %q", role.Name)
	}
	// Role bikinan user tidak pernah jadi role sistem -- kalau bisa, dia
	// otomatis kebal dari DELETE dan tidak bisa dibersihkan lagi.
	if role.IsSystem {
		t.Error("expected is_system false for a user-created role")
	}
	if role.CompanyID != nil {
		t.Errorf("expected company_id nil, got %v", *role.CompanyID)
	}
}

func TestCreateRoleRejectsEmptyCodeOrName(t *testing.T) {
	srv := newServer(t)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"code kosong", map[string]any{"code": "   ", "name": "Ada Nama"}},
		{"name kosong", map[string]any{"code": "ada_code", "name": "  "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/roles", tc.payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

func TestCreateRoleRejectsMalformedPayload(t *testing.T) {
	srv := newServer(t)

	resp := postRawJSON(t, srv.URL+"/roles", "{bukan json")
	requireStatus(t, resp, http.StatusBadRequest)
}

// Code role global harus unik lintas seluruh role global (uq_roles_company_code
// dengan COALESCE company_id). Duplikat harus jadi 409, bukan 500.
func TestCreateRoleRejectsDuplicateCode(t *testing.T) {
	srv := newServer(t)
	first := mustCreateRole(t, srv)

	resp := postJSON(t, srv.URL+"/roles", map[string]any{
		"code": first.Code,
		"name": "Role Kembar",
	})
	requireStatus(t, resp, http.StatusConflict)
	if msg := resp.errorMessage(); msg == "" {
		t.Error("expected an error message in the 409 body")
	}
}

func TestUpdateRoleChangesNameAndDescriptionButNotCode(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)

	resp := putJSON(t, srv.URL+"/roles/"+role.ID, map[string]any{
		"name":        "  Nama Baru  ",
		"description": "Deskripsi baru",
	})
	requireStatus(t, resp, http.StatusOK)

	var updated roleFixture
	resp.decode(t, &updated)
	if updated.Name != "Nama Baru" {
		t.Errorf("expected name %q, got %q", "Nama Baru", updated.Name)
	}
	if updated.Description != "Deskripsi baru" {
		t.Errorf("expected description to change, got %q", updated.Description)
	}
	// Code sengaja tidak ada di updateRoleRequest: dia identitas yang dirujuk
	// seed permission dan pengecekan role di frontend.
	if updated.Code != role.Code {
		t.Errorf("expected code to stay %q, got %q", role.Code, updated.Code)
	}
}

func TestUpdateRoleRejectsEmptyName(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)

	resp := putJSON(t, srv.URL+"/roles/"+role.ID, map[string]any{"name": "   "})
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestUpdateRoleUnknownIDReturns404(t *testing.T) {
	srv := newServer(t)

	resp := putJSON(t, srv.URL+"/roles/"+uuid.NewString(), map[string]any{"name": "Apa Saja"})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestDeleteRoleRemovesCustomRole(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)

	requireStatus(t, deleteJSON(t, srv.URL+"/roles/"+role.ID), http.StatusOK)

	var roles []roleFixture
	getJSON(t, srv.URL+"/roles").decode(t, &roles)
	for _, r := range roles {
		if r.ID == role.ID {
			t.Fatalf("role %s masih ada setelah dihapus", role.ID)
		}
	}
}

// Role bawaan platform (super_admin dkk) adalah acuan seluruh seed permission;
// menghapusnya akan meng-cascade role_menu_permissions & user_roles.
func TestDeleteSystemRoleIsForbidden(t *testing.T) {
	srv := newServer(t)
	id := systemRoleID(t, "super_admin")

	resp := deleteJSON(t, srv.URL+"/roles/"+id)
	requireStatus(t, resp, http.StatusForbidden)

	var stillThere bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM roles WHERE id = $1)`, id).Scan(&stillThere); err != nil {
		t.Fatalf("cek role sistem: %v", err)
	}
	if !stillThere {
		t.Fatal("role sistem terhapus padahal request ditolak")
	}
}

func TestDeleteRoleUnknownIDReturns404(t *testing.T) {
	srv := newServer(t)

	resp := deleteJSON(t, srv.URL+"/roles/"+uuid.NewString())
	requireStatus(t, resp, http.StatusNotFound)
}

// listRoles diurutkan is_system DESC, name ASC: role bawaan platform selalu di
// atas di dropdown penugasan role, role custom menyusul.
func TestListRolesPutsSystemRolesFirst(t *testing.T) {
	srv := newServer(t)
	mustCreateRole(t, srv)

	var roles []roleFixture
	getJSON(t, srv.URL+"/roles").decode(t, &roles)
	if len(roles) < 2 {
		t.Fatalf("expected at least the seeded system roles, got %d", len(roles))
	}

	seenCustom := false
	for _, r := range roles {
		if !r.IsSystem {
			seenCustom = true
			continue
		}
		if seenCustom {
			t.Fatalf("role sistem %q muncul setelah role custom", r.Code)
		}
	}
	if !seenCustom {
		t.Fatal("role custom bikinan test tidak ada di daftar")
	}
}

func TestGetRolePermissionsListsEveryActiveMenuWithNoAccessByDefault(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)

	var perms []menuPermission
	resp := getJSON(t, srv.URL+"/roles/"+role.ID+"/permissions")
	requireStatus(t, resp, http.StatusOK)
	resp.decode(t, &perms)

	var activeMenus int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM menus WHERE is_active = true`).Scan(&activeMenus); err != nil {
		t.Fatalf("hitung menu aktif: %v", err)
	}
	// Matrix permission di frontend butuh SELURUH menu, bukan hanya yang
	// sudah punya row -- itulah gunanya LEFT JOIN + COALESCE di query.
	if len(perms) != activeMenus {
		t.Fatalf("expected %d menu rows, got %d", activeMenus, len(perms))
	}
	for _, p := range perms {
		if p.CanView || p.CanCreate || p.CanUpdate || p.CanDelete || p.CanApprove || p.CanExport {
			t.Fatalf("role baru seharusnya belum punya akses apa pun, menu %q punya %+v", p.Name, p)
		}
	}
}

func TestGetRolePermissionsUnknownRoleReturns404(t *testing.T) {
	srv := newServer(t)

	resp := getJSON(t, srv.URL+"/roles/"+uuid.NewString()+"/permissions")
	requireStatus(t, resp, http.StatusNotFound)
}

// PUT mengganti seluruh set: menu yang tidak dikirim ulang harus kembali ke
// "tanpa akses". Ini perilaku yang diandalkan RolePermissionMatrixPage, yang
// hanya mengirim baris dengan can_view = true.
func TestPutRolePermissionsReplacesTheWholeSet(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	leaveMenu := menuIDByCode(t, "hr", "leave")
	overtimeMenu := menuIDByCode(t, "hr", "overtime")

	first := putJSON(t, srv.URL+"/roles/"+role.ID+"/permissions", []map[string]any{
		{"menu_id": leaveMenu, "can_view": true, "can_create": true, "can_approve": true},
		{"menu_id": overtimeMenu, "can_view": true, "can_export": true},
	})
	requireStatus(t, first, http.StatusOK)

	perms := permissionsByMenuID(t, first)
	if got := perms[leaveMenu]; !got.CanView || !got.CanCreate || !got.CanApprove {
		t.Errorf("menu cuti: expected view+create+approve, got %+v", got)
	}
	if got := perms[leaveMenu]; got.CanDelete || got.CanExport || got.CanUpdate {
		t.Errorf("menu cuti: aksi yang tidak dikirim seharusnya false, got %+v", got)
	}
	if got := perms[overtimeMenu]; !got.CanView || !got.CanExport {
		t.Errorf("menu lembur: expected view+export, got %+v", got)
	}

	second := putJSON(t, srv.URL+"/roles/"+role.ID+"/permissions", []map[string]any{
		{"menu_id": overtimeMenu, "can_view": true},
	})
	requireStatus(t, second, http.StatusOK)

	perms = permissionsByMenuID(t, second)
	if perms[leaveMenu].CanView {
		t.Error("menu cuti masih punya akses padahal tidak dikirim di PUT kedua")
	}
	if got := perms[overtimeMenu]; !got.CanView || got.CanExport {
		t.Errorf("menu lembur: expected view-only setelah PUT kedua, got %+v", got)
	}
}

// Baris dengan can_view = false tidak disimpan sama sekali: tanpa hak lihat,
// hak turunannya (create/update/...) tidak punya arti di UI.
func TestPutRolePermissionsSkipsRowsWithoutCanView(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	menuID := menuIDByCode(t, "hr", "leave")

	resp := putJSON(t, srv.URL+"/roles/"+role.ID+"/permissions", []map[string]any{
		{"menu_id": menuID, "can_view": false, "can_create": true, "can_delete": true},
	})
	requireStatus(t, resp, http.StatusOK)

	var stored int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM role_menu_permissions WHERE role_id = $1`, role.ID).Scan(&stored); err != nil {
		t.Fatalf("hitung permission tersimpan: %v", err)
	}
	if stored != 0 {
		t.Fatalf("expected no permission row, got %d", stored)
	}
}

func TestPutRolePermissionsRejectsMalformedPayload(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)

	resp := putRawJSON(t, srv.URL+"/roles/"+role.ID+"/permissions", `{"menu_id":"x"}`)
	requireStatus(t, resp, http.StatusBadRequest)
}

// Kalau satu baris gagal disimpan (mis. menu_id tidak ada -> FK violation),
// SELURUH PUT harus batal, termasuk DELETE di awal transaksi. Tanpa itu, satu
// payload cacat bisa mengosongkan seluruh akses sebuah role.
func TestPutRolePermissionsRollsBackWhenAMenuIsUnknown(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	menuID := menuIDByCode(t, "hr", "leave")

	requireStatus(t, putJSON(t, srv.URL+"/roles/"+role.ID+"/permissions", []map[string]any{
		{"menu_id": menuID, "can_view": true, "can_update": true},
	}), http.StatusOK)

	broken := putJSON(t, srv.URL+"/roles/"+role.ID+"/permissions", []map[string]any{
		{"menu_id": menuIDByCode(t, "hr", "overtime"), "can_view": true},
		{"menu_id": uuid.NewString(), "can_view": true},
	})
	requireStatus(t, broken, http.StatusInternalServerError)

	perms := permissionsByMenuID(t, getJSON(t, srv.URL+"/roles/"+role.ID+"/permissions"))
	if got := perms[menuID]; !got.CanView || !got.CanUpdate {
		t.Fatalf("permission lama hilang setelah PUT yang gagal: %+v", got)
	}
	if perms[menuIDByCode(t, "hr", "overtime")].CanView {
		t.Fatal("permission dari PUT yang gagal ikut tersimpan")
	}
}

// Menghapus role harus ikut membersihkan penugasannya (ON DELETE CASCADE di
// user_roles), supaya tidak ada baris yatim yang bikin JOIN di listUserRoles
// menghilangkan user dari daftar tanpa penjelasan.
func TestDeleteRoleCascadesToPermissionsAndAssignments(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	userID := uuid.NewString()

	requireStatus(t, putJSON(t, srv.URL+"/roles/"+role.ID+"/permissions", []map[string]any{
		{"menu_id": menuIDByCode(t, "hr", "leave"), "can_view": true},
	}), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/user-roles", map[string]any{
		"user_id":    userID,
		"role_id":    role.ID,
		"company_id": uuid.NewString(),
	}), http.StatusCreated)

	requireStatus(t, deleteJSON(t, srv.URL+"/roles/"+role.ID), http.StatusOK)

	var perms, assignments int
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM role_menu_permissions WHERE role_id = $1`, role.ID).Scan(&perms); err != nil {
		t.Fatalf("hitung permission: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_roles WHERE role_id = $1`, role.ID).Scan(&assignments); err != nil {
		t.Fatalf("hitung penugasan: %v", err)
	}
	if perms != 0 || assignments != 0 {
		t.Fatalf("expected cascade to clean up, got %d permissions and %d assignments", perms, assignments)
	}
}

func permissionsByMenuID(t *testing.T, resp apiResponse) map[string]menuPermission {
	t.Helper()
	var perms []menuPermission
	resp.decode(t, &perms)
	byID := make(map[string]menuPermission, len(perms))
	for _, p := range perms {
		byID[p.ID] = p
	}
	return byID
}
