package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type overrideFixture struct {
	ID         string  `json:"id"`
	UserID     string  `json:"user_id"`
	CompanyID  string  `json:"company_id"`
	MenuID     string  `json:"menu_id"`
	MenuName   string  `json:"menu_name"`
	MenuPath   string  `json:"menu_path"`
	ModuleID   string  `json:"module_id"`
	ModuleName string  `json:"module_name"`
	CanView    bool    `json:"can_view"`
	CanCreate  bool    `json:"can_create"`
	CanUpdate  bool    `json:"can_update"`
	CanDelete  bool    `json:"can_delete"`
	CanApprove bool    `json:"can_approve"`
	CanExport  bool    `json:"can_export"`
	CreatedBy  *string `json:"created_by"`
}

type effectivePermission struct {
	MenuID      string `json:"menu_id"`
	MenuName    string `json:"menu_name"`
	ModuleID    string `json:"module_id"`
	CanView     bool   `json:"can_view"`
	CanCreate   bool   `json:"can_create"`
	CanUpdate   bool   `json:"can_update"`
	CanDelete   bool   `json:"can_delete"`
	CanApprove  bool   `json:"can_approve"`
	CanExport   bool   `json:"can_export"`
	Source      string `json:"source"`
	RoleActions struct {
		CanView   bool `json:"can_view"`
		CanUpdate bool `json:"can_update"`
	} `json:"role_actions"`
}

// putOverride mengirim override untuk satu menu dan mengembalikan hasilnya.
func putOverride(t *testing.T, srv *httptest.Server, payload map[string]any, headers map[string]string) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPut, srv.URL+"/user-overrides", payload, headers)
}

func effectivePermissions(t *testing.T, srv *httptest.Server, userID, companyID string) map[string]effectivePermission {
	t.Helper()
	resp := getJSON(t, srv.URL+"/user-permissions?user_id="+userID+"&company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var rows []effectivePermission
	resp.decode(t, &rows)
	byMenu := make(map[string]effectivePermission, len(rows))
	for _, row := range rows {
		byMenu[row.MenuID] = row
	}
	return byMenu
}

// grantRoleAndAssign memberi role hak view+update pada satu menu lalu
// menugaskannya ke user pada company tertentu.
func grantRoleAndAssign(t *testing.T, srv *httptest.Server, roleID, menuID, userID, companyID string) {
	t.Helper()
	requireStatus(t, putJSON(t, srv.URL+"/roles/"+roleID+"/permissions", []map[string]any{
		{"menu_id": menuID, "can_view": true, "can_update": true},
	}), http.StatusOK)
	requireStatus(t, postJSON(t, srv.URL+"/user-roles", map[string]any{
		"user_id": userID, "role_id": roleID, "company_id": companyID,
	}), http.StatusCreated)
}

func TestPutUserOverrideGrantsAMenuTheRoleDoesNotHave(t *testing.T) {
	srv := newServer(t)
	user := uuid.NewString()
	company := uuid.NewString()
	menu := menuIDByCode(t, "hr", "leave")
	actor := uuid.NewString()

	resp := putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": menu,
		"can_view": true, "can_export": true,
	}, map[string]string{"X-User-Id": actor})
	requireStatus(t, resp, http.StatusOK)

	var override overrideFixture
	resp.decode(t, &override)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_menu_permission_overrides WHERE id = $1`, override.ID)
	})

	if !override.CanView || !override.CanExport || override.CanDelete {
		t.Errorf("hak override tidak sesuai payload: %+v", override)
	}
	// Nama menu & modul ikut di response supaya UI tidak perlu join sendiri.
	if override.MenuName != "Cuti" || override.ModuleName != "HR" {
		t.Errorf("expected nama menu & modul ikut terbawa, got %q / %q", override.MenuName, override.ModuleName)
	}
	// created_by = jejak siapa yang memberi akses khusus ini, dari header yang
	// di-inject api-gateway.
	if override.CreatedBy == nil || *override.CreatedBy != actor {
		t.Errorf("expected created_by %s, got %v", actor, override.CreatedBy)
	}

	perms := effectivePermissions(t, srv, user, company)
	if got := perms[menu]; !got.CanView || !got.CanExport || got.Source != "override" {
		t.Fatalf("permission efektif tidak mengikuti override: %+v", got)
	}
}

// Override MENANG UTUH atas role, bukan digabung: menu yang diberikan role bisa
// dicabut untuk satu user tertentu dengan override semua-false.
func TestPutUserOverrideCanRevokeWhatTheRoleGrants(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	company := uuid.NewString()
	menu := menuIDByCode(t, "hr", "leave")
	grantRoleAndAssign(t, srv, role.ID, menu, user, company)

	before := effectivePermissions(t, srv, user, company)
	if got := before[menu]; !got.CanView || got.Source != "role" {
		t.Fatalf("prasyarat gagal, role seharusnya memberi akses: %+v", got)
	}

	resp := putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": menu, "can_view": false,
	}, nil)
	requireStatus(t, resp, http.StatusOK)
	var override overrideFixture
	resp.decode(t, &override)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_menu_permission_overrides WHERE id = $1`, override.ID)
	})

	after := effectivePermissions(t, srv, user, company)
	got := after[menu]
	if got.CanView || got.CanUpdate || got.Source != "override" {
		t.Fatalf("override pencabutan tidak berlaku: %+v", got)
	}
	// Hak asli dari role tetap dilaporkan terpisah supaya UI bisa menunjukkan
	// "aslinya boleh, tapi dicabut untuk user ini".
	if !got.RoleActions.CanView || !got.RoleActions.CanUpdate {
		t.Errorf("expected role_actions tetap menunjukkan hak bawaan role, got %+v", got.RoleActions)
	}
}

func TestPutUserOverrideIsIdempotentPerScope(t *testing.T) {
	srv := newServer(t)
	user := uuid.NewString()
	company := uuid.NewString()
	menu := menuIDByCode(t, "hr", "overtime")

	first := putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": menu, "can_view": true,
	}, nil)
	requireStatus(t, first, http.StatusOK)
	second := putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": menu,
		"can_view": true, "can_approve": true,
	}, nil)
	requireStatus(t, second, http.StatusOK)

	var latest overrideFixture
	second.decode(t, &latest)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_menu_permission_overrides WHERE user_id = $1`, user)
	})

	var stored int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM user_menu_permission_overrides WHERE user_id = $1 AND company_id = $2 AND menu_id = $3`,
		user, company, menu).Scan(&stored); err != nil {
		t.Fatalf("hitung override: %v", err)
	}
	if stored != 1 {
		t.Fatalf("expected satu baris override per scope, got %d", stored)
	}
	if !latest.CanApprove {
		t.Error("PUT kedua seharusnya menggantikan hak yang lama")
	}
}

func TestPutUserOverrideValidatesThePayload(t *testing.T) {
	srv := newServer(t)
	menu := menuIDByCode(t, "hr", "leave")

	cases := []struct {
		name    string
		payload map[string]any
		status  int
	}{
		{"tanpa user_id", map[string]any{"company_id": uuid.NewString(), "menu_id": menu, "can_view": true}, http.StatusBadRequest},
		{"tanpa company_id", map[string]any{"user_id": uuid.NewString(), "menu_id": menu, "can_view": true}, http.StatusBadRequest},
		{"tanpa menu_id", map[string]any{"user_id": uuid.NewString(), "company_id": uuid.NewString(), "can_view": true}, http.StatusBadRequest},
		// Scope branch/department ditolak eksplisit, bukan disimpan sebagai
		// baris yang diam-diam tidak berpengaruh.
		{"scope branch", map[string]any{"user_id": uuid.NewString(), "company_id": uuid.NewString(), "menu_id": menu, "branch_id": uuid.NewString(), "can_view": true}, http.StatusBadRequest},
		{"scope department", map[string]any{"user_id": uuid.NewString(), "company_id": uuid.NewString(), "menu_id": menu, "department_id": uuid.NewString(), "can_view": true}, http.StatusBadRequest},
		// Hak turunan tanpa can_view tidak punya arti.
		{"can_update tanpa can_view", map[string]any{"user_id": uuid.NewString(), "company_id": uuid.NewString(), "menu_id": menu, "can_update": true}, http.StatusBadRequest},
		{"menu tidak dikenal", map[string]any{"user_id": uuid.NewString(), "company_id": uuid.NewString(), "menu_id": uuid.NewString(), "can_view": true}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, putOverride(t, srv, tc.payload, nil), tc.status)
		})
	}
}

func TestDeleteUserOverrideRestoresTheRoleDefault(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	company := uuid.NewString()
	menu := menuIDByCode(t, "hr", "leave")
	grantRoleAndAssign(t, srv, role.ID, menu, user, company)

	resp := putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": menu, "can_view": false,
	}, nil)
	requireStatus(t, resp, http.StatusOK)
	var override overrideFixture
	resp.decode(t, &override)

	requireStatus(t, deleteJSON(t, srv.URL+"/user-overrides/"+override.ID), http.StatusOK)

	after := effectivePermissions(t, srv, user, company)
	if got := after[menu]; !got.CanView || !got.CanUpdate || got.Source != "role" {
		t.Fatalf("hak bawaan role tidak kembali setelah override dihapus: %+v", got)
	}
}

func TestDeleteUserOverrideUnknownIDReturns404(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, deleteJSON(t, srv.URL+"/user-overrides/"+uuid.NewString()), http.StatusNotFound)
}

func TestListUserOverridesIsScopedToUserAndOptionallyCompany(t *testing.T) {
	srv := newServer(t)
	user := uuid.NewString()
	otherUser := uuid.NewString()
	companyA := uuid.NewString()
	companyB := uuid.NewString()
	leave := menuIDByCode(t, "hr", "leave")
	overtime := menuIDByCode(t, "hr", "overtime")

	for _, o := range []struct{ user, company, menu string }{
		{user, companyA, leave},
		{user, companyB, overtime},
		{otherUser, companyA, leave},
	} {
		requireStatus(t, putOverride(t, srv, map[string]any{
			"user_id": o.user, "company_id": o.company, "menu_id": o.menu, "can_view": true,
		}, nil), http.StatusOK)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM user_menu_permission_overrides WHERE user_id = ANY($1)`, []string{user, otherUser})
	})

	var all []overrideFixture
	getJSON(t, srv.URL+"/user-overrides?user_id="+user).decode(t, &all)
	if len(all) != 2 {
		t.Fatalf("expected 2 override untuk user ini, got %d", len(all))
	}

	var onlyA []overrideFixture
	getJSON(t, srv.URL+"/user-overrides?user_id="+user+"&company_id="+companyA).decode(t, &onlyA)
	if len(onlyA) != 1 || onlyA[0].MenuID != leave {
		t.Fatalf("expected hanya override company A, got %+v", onlyA)
	}
}

func TestListUserOverridesRequiresUserID(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, getJSON(t, srv.URL+"/user-overrides"), http.StatusBadRequest)
}

func TestUserPermissionsRequiresUserAndCompany(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, getJSON(t, srv.URL+"/user-permissions?company_id="+uuid.NewString()), http.StatusBadRequest)
	requireStatus(t, getJSON(t, srv.URL+"/user-permissions?user_id="+uuid.NewString()), http.StatusBadRequest)
}

// Satu menu yang diberikan dua role dengan hak berbeda digabung per kolom
// (model.MenuActions.Or) -- bukan role terakhir yang menang.
func TestUserPermissionsMergesActionsFromSeveralRoles(t *testing.T) {
	srv := newServer(t)
	roleA := mustCreateRole(t, srv)
	roleB := mustCreateRole(t, srv)
	user := uuid.NewString()
	company := uuid.NewString()
	menu := menuIDByCode(t, "hr", "leave")

	requireStatus(t, putJSON(t, srv.URL+"/roles/"+roleA.ID+"/permissions", []map[string]any{
		{"menu_id": menu, "can_view": true, "can_create": true},
	}), http.StatusOK)
	requireStatus(t, putJSON(t, srv.URL+"/roles/"+roleB.ID+"/permissions", []map[string]any{
		{"menu_id": menu, "can_view": true, "can_approve": true},
	}), http.StatusOK)
	for _, roleID := range []string{roleA.ID, roleB.ID} {
		requireStatus(t, postJSON(t, srv.URL+"/user-roles", map[string]any{
			"user_id": user, "role_id": roleID, "company_id": company,
		}), http.StatusCreated)
	}

	got := effectivePermissions(t, srv, user, company)[menu]
	if !got.CanView || !got.CanCreate || !got.CanApprove {
		t.Fatalf("hak dari dua role tidak digabung: %+v", got)
	}
	if got.CanDelete {
		t.Errorf("hak yang tidak diberikan role mana pun ikut menyala: %+v", got)
	}
}

func TestUserPermissionsCoversEveryActiveMenu(t *testing.T) {
	srv := newServer(t)

	rows := effectivePermissions(t, srv, uuid.NewString(), uuid.NewString())

	var activeMenus int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM menus WHERE is_active = true`).Scan(&activeMenus); err != nil {
		t.Fatalf("hitung menu aktif: %v", err)
	}
	if len(rows) != activeMenus {
		t.Fatalf("expected %d baris, got %d", activeMenus, len(rows))
	}
	for _, row := range rows {
		if row.Source != "none" || row.CanView {
			t.Fatalf("user tanpa role & override seharusnya tidak punya akses: %+v", row)
		}
	}
}

// menu-tree tanpa company_id = perilaku lama persis (murni hak role), supaya
// pemanggil yang belum mengirim company_id tidak berubah hasilnya.
func TestMenuTreeIgnoresOverridesWhenNoCompanyIsGiven(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	company := uuid.NewString()
	leave := menuIDByCode(t, "hr", "leave")
	overtime := menuIDByCode(t, "hr", "overtime")
	grantRoleAndAssign(t, srv, role.ID, leave, user, company)

	requireStatus(t, putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": overtime, "can_view": true,
	}, nil), http.StatusOK)
	requireStatus(t, putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": leave, "can_view": false,
	}, nil), http.StatusOK)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_menu_permission_overrides WHERE user_id = $1`, user)
	})

	ids := menuIDsInTree(fetchMenuTree(t, srv, user, false))
	if !ids[leave] || ids[overtime] || len(ids) != 1 {
		t.Fatalf("tanpa company_id, menu-tree harus murni dari role: %v", ids)
	}
}

func TestMenuTreeAppliesOverridesWhenCompanyIsGiven(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	company := uuid.NewString()
	leave := menuIDByCode(t, "hr", "leave")
	overtime := menuIDByCode(t, "hr", "overtime")
	grantRoleAndAssign(t, srv, role.ID, leave, user, company)

	// Override menambah menu yang role-nya tidak punya...
	requireStatus(t, putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": overtime, "can_view": true,
	}, nil), http.StatusOK)
	// ...dan mencabut menu yang role-nya punya.
	requireStatus(t, putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": leave, "can_view": false,
	}, nil), http.StatusOK)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_menu_permission_overrides WHERE user_id = $1`, user)
	})

	ids := menuIDsInTree(fetchMenuTreeForCompany(t, srv, user, company))
	if ids[leave] {
		t.Error("menu yang dicabut override masih muncul di sidebar")
	}
	if !ids[overtime] {
		t.Error("menu yang ditambahkan override tidak muncul di sidebar")
	}
	if len(ids) != 1 {
		t.Fatalf("expected tepat satu menu, got %v", ids)
	}
}

// Override milik company lain tidak boleh ikut terpakai: hak khusus di satu
// perusahaan tidak berlaku saat user membuka perusahaan lain.
func TestMenuTreeIgnoresOverridesOfAnotherCompany(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	companyA := uuid.NewString()
	companyB := uuid.NewString()
	leave := menuIDByCode(t, "hr", "leave")
	overtime := menuIDByCode(t, "hr", "overtime")
	grantRoleAndAssign(t, srv, role.ID, leave, user, companyA)

	requireStatus(t, putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": companyA, "menu_id": overtime, "can_view": true,
	}, nil), http.StatusOK)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_menu_permission_overrides WHERE user_id = $1`, user)
	})

	ids := menuIDsInTree(fetchMenuTreeForCompany(t, srv, user, companyB))
	if ids[overtime] {
		t.Error("override company A ikut terpakai saat membuka company B")
	}
	if !ids[leave] {
		t.Error("menu dari role hilang saat membuka company B")
	}
}

// Super admin tidak lewat jalur role/override sama sekali -- dia selalu melihat
// seluruh menu aktif, termasuk yang punya override pencabutan.
func TestMenuTreeForSuperAdminIgnoresOverrides(t *testing.T) {
	srv := newServer(t)
	user := uuid.NewString()
	company := uuid.NewString()
	leave := menuIDByCode(t, "hr", "leave")

	requireStatus(t, putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": leave, "can_view": false,
	}, nil), http.StatusOK)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_menu_permission_overrides WHERE user_id = $1`, user)
	})

	ids := menuIDsInTree(fetchMenuTree(t, srv, user, true))
	if !ids[leave] {
		t.Fatal("super admin seharusnya tetap melihat menu yang di-override cabut")
	}
}
