package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type moduleFixture struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

type menuFixture struct {
	ID        string  `json:"id"`
	ModuleID  string  `json:"module_id"`
	ParentID  *string `json:"parent_id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Path      string  `json:"path"`
	Icon      string  `json:"icon"`
	SortOrder int     `json:"sort_order"`
	IsActive  bool    `json:"is_active"`
}

func TestListModulesIsOrderedBySortOrder(t *testing.T) {
	srv := newServer(t)

	var modules []moduleFixture
	resp := getJSON(t, srv.URL+"/modules")
	requireStatus(t, resp, http.StatusOK)
	resp.decode(t, &modules)

	if len(modules) < 10 {
		t.Fatalf("expected the seeded modules, got %d", len(modules))
	}
	for i := 1; i < len(modules); i++ {
		if modules[i].SortOrder < modules[i-1].SortOrder {
			t.Fatalf("module %q (sort %d) muncul setelah %q (sort %d)",
				modules[i].Code, modules[i].SortOrder, modules[i-1].Code, modules[i-1].SortOrder)
		}
	}
	if modules[0].Code != "core" {
		t.Errorf("expected core module first, got %q", modules[0].Code)
	}
}

func TestListMenusSkipsInactiveMenus(t *testing.T) {
	srv := newServer(t)
	active := menuIDByCode(t, "hr", "leave")
	inactive := mustInsertMenu(t, moduleIDByCode(t, "hr"), nil, false)

	var menus []menuFixture
	resp := getJSON(t, srv.URL+"/menus")
	requireStatus(t, resp, http.StatusOK)
	resp.decode(t, &menus)

	seen := map[string]bool{}
	for _, m := range menus {
		seen[m.ID] = true
		if !m.IsActive {
			t.Fatalf("menu non-aktif %q ikut terbawa", m.Name)
		}
	}
	if !seen[active] {
		t.Error("menu aktif hasil seed tidak ada di daftar")
	}
	// is_active = false adalah cara menyembunyikan menu tanpa menghapus
	// permission yang sudah terlanjur diberikan ke role.
	if seen[inactive] {
		t.Error("menu dengan is_active = false seharusnya tidak muncul")
	}
}

func TestMenuTreeRequiresUserID(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, getJSON(t, srv.URL+"/menu-tree"), http.StatusBadRequest)
}

// Super Admin ditandai header X-Is-Super-Admin yang di-inject api-gateway dari
// klaim JWT: dia melihat seluruh menu aktif tanpa perlu satu pun baris di
// user_roles / role_menu_permissions.
func TestMenuTreeForSuperAdminCoversEveryActiveMenu(t *testing.T) {
	srv := newServer(t)

	tree := fetchMenuTree(t, srv, uuid.NewString(), true)

	var activeMenus int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM menus WHERE is_active = true`).Scan(&activeMenus); err != nil {
		t.Fatalf("hitung menu aktif: %v", err)
	}
	if got := countMenuNodes(tree); got != activeMenus {
		t.Fatalf("expected %d menu in the tree, got %d", activeMenus, got)
	}
}

// Regresi urutan sidebar: perakitan pohon pernah mengiterasi map Go, yang
// SENGAJA diacak, sehingga ORDER BY di query terbuang dan urutan menu berubah
// tiap request. Body yang identik antar request adalah buktinya.
func TestMenuTreeOrderIsStableAcrossRequests(t *testing.T) {
	srv := newServer(t)
	userID := uuid.NewString()

	first := getJSONWithHeaders(t, srv.URL+"/menu-tree?user_id="+userID,
		map[string]string{"X-Is-Super-Admin": "true"})
	requireStatus(t, first, http.StatusOK)

	for i := 0; i < 4; i++ {
		next := getJSONWithHeaders(t, srv.URL+"/menu-tree?user_id="+userID,
			map[string]string{"X-Is-Super-Admin": "true"})
		requireStatus(t, next, http.StatusOK)
		if string(next.body) != string(first.body) {
			t.Fatalf("urutan menu-tree berubah di request ke-%d", i+2)
		}
	}
}

// Urutannya bukan sekadar stabil, tapi memang mengikuti mod.sort_order lalu
// m.sort_order -- itu yang menentukan susunan grup & item di sidebar.
func TestMenuTreeFollowsModuleThenMenuSortOrder(t *testing.T) {
	srv := newServer(t)

	tree := fetchMenuTree(t, srv, uuid.NewString(), true)

	rows, err := pool.Query(context.Background(), `
		SELECT mod.id, m.id
		FROM menus m
		JOIN modules mod ON mod.id = m.module_id
		WHERE m.is_active = true AND m.parent_id IS NULL
		ORDER BY mod.sort_order ASC, m.sort_order ASC`)
	if err != nil {
		t.Fatalf("query urutan yang diharapkan: %v", err)
	}
	defer rows.Close()

	wantModules := []string{}
	wantMenus := map[string][]string{}
	for rows.Next() {
		var modID, menuID string
		if err := rows.Scan(&modID, &menuID); err != nil {
			t.Fatalf("scan urutan: %v", err)
		}
		if _, ok := wantMenus[modID]; !ok {
			wantModules = append(wantModules, modID)
		}
		wantMenus[modID] = append(wantMenus[modID], menuID)
	}

	if len(tree) != len(wantModules) {
		t.Fatalf("expected %d module groups, got %d", len(wantModules), len(tree))
	}
	for i, mod := range tree {
		if mod.ID != wantModules[i] {
			t.Fatalf("module ke-%d: expected %s, got %s (%s)", i, wantModules[i], mod.ID, mod.Name)
		}
		want := wantMenus[mod.ID]
		if len(mod.Menus) != len(want) {
			t.Fatalf("module %q: expected %d menu, got %d", mod.Name, len(want), len(mod.Menus))
		}
		for j, menu := range mod.Menus {
			if menu.ID != want[j] {
				t.Fatalf("module %q menu ke-%d: expected %s, got %s (%s)", mod.Name, j, want[j], menu.ID, menu.Name)
			}
		}
	}
}

func TestMenuTreeForNormalUserOnlyIncludesMenusItCanView(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	leave := menuIDByCode(t, "hr", "leave")
	overtime := menuIDByCode(t, "hr", "overtime")

	requireStatus(t, putJSON(t, srv.URL+"/roles/"+role.ID+"/permissions", []map[string]any{
		{"menu_id": leave, "can_view": true},
		{"menu_id": overtime, "can_view": true},
	}), http.StatusOK)
	mustAssignRole(t, srv, user, role.ID)

	tree := fetchMenuTree(t, srv, user, false)
	got := menuIDsInTree(tree)
	if len(got) != 2 || !got[leave] || !got[overtime] {
		t.Fatalf("expected exactly the two granted HR menus, got %d: %v", len(got), got)
	}
	if len(tree) != 1 || tree[0].Name != "HR" {
		t.Fatalf("expected only the HR module group, got %+v", tree)
	}
}

// Baris permission dengan can_view = false tidak boleh memunculkan menu:
// filter `rmp.can_view = true` di query inilah beda "punya baris" dan
// "boleh melihat".
func TestMenuTreeExcludesPermissionRowsWithoutCanView(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	leave := menuIDByCode(t, "hr", "leave")

	// Ditulis langsung ke tabel: PUT /permissions memang membuang baris
	// can_view = false, jadi keadaan ini hanya bisa dibuat lewat SQL.
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO role_menu_permissions (role_id, menu_id, can_view, can_update)
		VALUES ($1, $2, false, true)`, role.ID, leave); err != nil {
		t.Fatalf("insert permission tanpa can_view: %v", err)
	}
	mustAssignRole(t, srv, user, role.ID)

	if tree := fetchMenuTree(t, srv, user, false); len(tree) != 0 {
		t.Fatalf("expected an empty tree, got %+v", tree)
	}
}

func TestMenuTreeForUserWithoutRolesIsEmptyArray(t *testing.T) {
	srv := newServer(t)

	resp := getJSON(t, srv.URL+"/menu-tree?user_id="+uuid.NewString())
	requireStatus(t, resp, http.StatusOK)
	if got := string(resp.body); got != "[]\n" {
		t.Fatalf("expected an empty JSON array, got %q", got)
	}
}

// Satu menu yang diberikan lewat dua role sekaligus harus muncul sekali saja --
// itu tugas DISTINCT di subquery; tanpa itu sidebar menampilkan menu ganda.
func TestMenuTreeDeduplicatesMenusGrantedBySeveralRoles(t *testing.T) {
	srv := newServer(t)
	roleA := mustCreateRole(t, srv)
	roleB := mustCreateRole(t, srv)
	user := uuid.NewString()
	leave := menuIDByCode(t, "hr", "leave")

	for _, roleID := range []string{roleA.ID, roleB.ID} {
		requireStatus(t, putJSON(t, srv.URL+"/roles/"+roleID+"/permissions", []map[string]any{
			{"menu_id": leave, "can_view": true},
		}), http.StatusOK)
		mustAssignRole(t, srv, user, roleID)
	}

	tree := fetchMenuTree(t, srv, user, false)
	if n := countMenuNodes(tree); n != 1 {
		t.Fatalf("expected the menu once, got %d nodes: %+v", n, tree)
	}
}

// Menu anak (parent_id terisi) harus bersarang di bawah induknya, bukan ikut
// jadi item level teratas di grup modul.
func TestMenuTreeNestsChildMenusUnderTheirParent(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	moduleID := moduleIDByCode(t, "hr")

	parent := mustInsertMenu(t, moduleID, nil, true)
	child := mustInsertMenu(t, moduleID, &parent, true)

	requireStatus(t, putJSON(t, srv.URL+"/roles/"+role.ID+"/permissions", []map[string]any{
		{"menu_id": parent, "can_view": true},
		{"menu_id": child, "can_view": true},
	}), http.StatusOK)
	mustAssignRole(t, srv, user, role.ID)

	tree := fetchMenuTree(t, srv, user, false)
	if len(tree) != 1 {
		t.Fatalf("expected one module group, got %d", len(tree))
	}
	if len(tree[0].Menus) != 1 || tree[0].Menus[0].ID != parent {
		t.Fatalf("expected only the parent at top level, got %+v", tree[0].Menus)
	}
	if len(tree[0].Menus[0].Children) != 1 || tree[0].Menus[0].Children[0].ID != child {
		t.Fatalf("expected the child nested under its parent, got %+v", tree[0].Menus[0].Children)
	}
}

func fetchMenuTree(t *testing.T, srv *httptest.Server, userID string, superAdmin bool) []menuTreeModule {
	t.Helper()
	headers := map[string]string{}
	if superAdmin {
		headers["X-Is-Super-Admin"] = "true"
	}
	resp := getJSONWithHeaders(t, srv.URL+"/menu-tree?user_id="+userID, headers)
	requireStatus(t, resp, http.StatusOK)
	var tree []menuTreeModule
	resp.decode(t, &tree)
	return tree
}

func countMenuNodes(tree []menuTreeModule) int {
	var walk func(entries []menuTreeEntry) int
	walk = func(entries []menuTreeEntry) int {
		n := 0
		for _, e := range entries {
			n += 1 + walk(e.Children)
		}
		return n
	}
	total := 0
	for _, mod := range tree {
		total += walk(mod.Menus)
	}
	return total
}

func menuIDsInTree(tree []menuTreeModule) map[string]bool {
	ids := map[string]bool{}
	var walk func(entries []menuTreeEntry)
	walk = func(entries []menuTreeEntry) {
		for _, e := range entries {
			ids[e.ID] = true
			walk(e.Children)
		}
	}
	for _, mod := range tree {
		walk(mod.Menus)
	}
	return ids
}

func moduleIDByCode(t *testing.T, code string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM modules WHERE code = $1`, code).Scan(&id); err != nil {
		t.Fatalf("cari module %q: %v", code, err)
	}
	return id
}

// mustInsertMenu menambah menu langsung ke tabel (tidak ada endpoint CRUD menu:
// menu hanya lahir dari file migrasi seed) dan membersihkannya lagi di akhir
// test supaya tidak mengubah jumlah menu yang diperiksa test lain.
func mustInsertMenu(t *testing.T, moduleID string, parentID *string, active bool) string {
	t.Helper()
	code := "test_" + uuid.NewString()[:8]
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO menus (module_id, parent_id, code, name, path, icon, sort_order, is_active)
		VALUES ($1, $2, $3, $4, $5, 'bi-question', 9000, $6)
		RETURNING id`, moduleID, parentID, code, "Menu "+code, "/test/"+code, active).Scan(&id); err != nil {
		t.Fatalf("insert menu test: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM menus WHERE id = $1`, id)
	})
	return id
}

func mustAssignRole(t *testing.T, srv *httptest.Server, userID, roleID string) {
	t.Helper()
	requireStatus(t, postJSON(t, srv.URL+"/user-roles", map[string]any{
		"user_id":    userID,
		"role_id":    roleID,
		"company_id": uuid.NewString(),
	}), http.StatusCreated)
}
