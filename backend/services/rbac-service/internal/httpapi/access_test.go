package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type accessPayload struct {
	UserID      string `json:"user_id"`
	CompanyID   string `json:"company_id"`
	Member      bool   `json:"member"`
	Permissions map[string]struct {
		CanView    bool `json:"can_view"`
		CanCreate  bool `json:"can_create"`
		CanUpdate  bool `json:"can_update"`
		CanDelete  bool `json:"can_delete"`
		CanApprove bool `json:"can_approve"`
		CanExport  bool `json:"can_export"`
	} `json:"permissions"`
}

func fetchAccess(t *testing.T, srv *httptest.Server, userID, companyID string) accessPayload {
	t.Helper()
	resp := getJSON(t, srv.URL+"/access?user_id="+userID+"&company_id="+companyID)
	requireStatus(t, resp, http.StatusOK)
	var payload accessPayload
	resp.decode(t, &payload)
	return payload
}

func TestAccessRequiresUserAndCompany(t *testing.T) {
	srv := newServer(t)
	user := uuid.NewString()
	company := uuid.NewString()

	for _, url := range []string{
		srv.URL + "/access",
		srv.URL + "/access?user_id=" + user,
		srv.URL + "/access?company_id=" + company,
	} {
		resp := getJSON(t, url)
		requireStatus(t, resp, http.StatusBadRequest)
	}
}

// Inilah yang membuat header company dari client tidak bisa dipakai untuk
// menghindari override "cabut akses": company yang tidak pernah ditugaskan ke
// user tidak menghasilkan hak apa pun, walau role-nya sendiri punya hak.
func TestAccessDeniesEverythingInACompanyTheUserIsNotAssignedTo(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	assigned := uuid.NewString()
	other := uuid.NewString()
	menu := menuIDByCode(t, "finance", "invoices")

	grantRoleAndAssign(t, srv, role.ID, menu, user, assigned)

	inAssigned := fetchAccess(t, srv, user, assigned)
	if !inAssigned.Member {
		t.Fatal("user seharusnya member di company tempat role-nya ditugaskan")
	}
	if !inAssigned.Permissions["/finance/invoices"].CanView {
		t.Fatalf("hak view dari role tidak terbaca: %+v", inAssigned.Permissions)
	}

	inOther := fetchAccess(t, srv, user, other)
	if inOther.Member {
		t.Fatal("user bukan member company lain, tapi member = true")
	}
	if len(inOther.Permissions) != 0 {
		t.Fatalf("company yang tidak ditugaskan seharusnya kosong, dapat %+v", inOther.Permissions)
	}
}

func TestAccessIsKeyedByMenuPathAndOmitsMenusWithoutAnyRight(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	company := uuid.NewString()
	menu := menuIDByCode(t, "warehouse", "products")

	grantRoleAndAssign(t, srv, role.ID, menu, user, company)

	payload := fetchAccess(t, srv, user, company)
	got, ok := payload.Permissions["/warehouse/products"]
	if !ok {
		t.Fatalf("menu yang diberi hak tidak muncul dengan kunci path: %+v", payload.Permissions)
	}
	if !got.CanView || !got.CanUpdate || got.CanCreate {
		t.Fatalf("hak tidak sesuai yang diberikan role: %+v", got)
	}
	// Seluruh menu lain hasil seed tidak boleh ikut: gateway memperlakukan
	// "tidak ada di map" sebagai tidak boleh, jadi mengirim baris serba-false
	// hanya memperbesar jawaban tanpa mengubah artinya.
	if len(payload.Permissions) != 1 {
		t.Fatalf("hanya menu yang punya hak yang boleh ikut, dapat %d: %+v", len(payload.Permissions), payload.Permissions)
	}
}

func TestAccessOverrideBeatsRoleIncludingRevocation(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	company := uuid.NewString()
	menu := menuIDByCode(t, "finance", "invoices")

	grantRoleAndAssign(t, srv, role.ID, menu, user, company)

	// Cabut total lewat override: role tetap memberi view+update, tapi hasil
	// akhirnya harus hilang sama sekali dari map.
	requireStatus(t, putOverride(t, srv, map[string]any{
		"user_id": user, "company_id": company, "menu_id": menu,
		"can_view": false,
	}, nil), http.StatusOK)

	payload := fetchAccess(t, srv, user, company)
	if _, ok := payload.Permissions["/finance/invoices"]; ok {
		t.Fatalf("override cabut akses tidak dihormati: %+v", payload.Permissions)
	}
	if !payload.Member {
		t.Fatal("mencabut hak satu menu tidak boleh membuat user berhenti jadi member company")
	}
}

// Penugasan role yang masa berlakunya sudah lewat tidak boleh menghasilkan hak
// apa pun -- termasuk tidak membuat user tetap terhitung member.
func TestAccessIgnoresExpiredRoleAssignments(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()
	company := uuid.NewString()
	menu := menuIDByCode(t, "hr", "payroll")

	grantRoleAndAssign(t, srv, role.ID, menu, user, company)
	if !fetchAccess(t, srv, user, company).Member {
		t.Fatal("prasyarat: user harus member sebelum penugasannya dikedaluwarsakan")
	}

	// valid_to belum bisa diisi lewat API mana pun, jadi diset langsung di
	// database -- yang diuji di sini adalah query-nya, bukan jalur HTTP-nya.
	if _, err := pool.Exec(context.Background(),
		`UPDATE user_roles SET valid_to = now() - interval '1 day' WHERE user_id = $1`, user); err != nil {
		t.Fatalf("kedaluwarsakan penugasan role: %v", err)
	}

	payload := fetchAccess(t, srv, user, company)
	if payload.Member {
		t.Fatal("penugasan yang sudah kedaluwarsa masih dianggap keanggotaan aktif")
	}
	if len(payload.Permissions) != 0 {
		t.Fatalf("penugasan kedaluwarsa masih memberi hak: %+v", payload.Permissions)
	}
}
