package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// putJSON: service ini baru punya postJSON/getJSON di helper-nya.
func putJSON(t *testing.T, url string, payload any) apiResponse {
	t.Helper()
	return doRequest(t, http.MethodPut, url, payload, "")
}

// mustUpdateAccountStatus mengubah status lewat endpoint update yang sungguhan
// (bukan SQL langsung), supaya validasinya ikut teruji.
func mustUpdateAccountStatus(t *testing.T, srv *httptest.Server, acc accountFixture, status string) accountFixture {
	t.Helper()
	resp := putJSON(t, srv.URL+"/accounts/"+acc.ID, map[string]any{
		"name": acc.Name, "account_type": "PROSPECT", "status": status,
	})
	requireStatus(t, resp, http.StatusOK)
	var updated accountFixture
	resp.decode(t, &updated)
	return updated
}

// Account & Contact adalah master data terakhir di platform ini yang belum
// punya cara dinonaktifkan. Test di bawah menjaga dua hal: statusnya benar-benar
// tersimpan, dan yang nonaktif tidak bisa dipakai untuk data BARU (sementara
// data lama yang sudah menempel tetap utuh).

func TestAccount_DefaultsToActive(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	acc := mustSeedAccount(t, srv, companyID)
	if acc.Status != "ACTIVE" {
		t.Fatalf("account baru seharusnya ACTIVE, got %q", acc.Status)
	}
}

func TestUpdateAccount_DeactivateAndReactivate(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	acc := mustSeedAccount(t, srv, companyID)

	updated := mustUpdateAccountStatus(t, srv, acc, "INACTIVE")
	if updated.Status != "INACTIVE" {
		t.Fatalf("expected INACTIVE, got %q", updated.Status)
	}

	back := mustUpdateAccountStatus(t, srv, acc, "ACTIVE")
	if back.Status != "ACTIVE" {
		t.Fatalf("expected ACTIVE kembali, got %q", back.Status)
	}
}

// Status kosong berarti TIDAK DIUBAH: menyunting nama tidak boleh diam-diam
// mengaktifkan kembali account yang sengaja dinonaktifkan.
func TestUpdateAccount_EmptyStatusKeepsExisting(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	acc := mustSeedAccount(t, srv, companyID)
	mustUpdateAccountStatus(t, srv, acc, "INACTIVE")

	resp := putJSON(t, srv.URL+"/accounts/"+acc.ID, map[string]any{
		"name": "Nama Baru", "account_type": "CUSTOMER",
	})
	requireStatus(t, resp, http.StatusOK)
	var after accountFixture
	resp.decode(t, &after)
	if after.Status != "INACTIVE" {
		t.Fatalf("status seharusnya tetap INACTIVE, got %q", after.Status)
	}
	if after.Name != "Nama Baru" {
		t.Errorf("nama tidak ikut berubah: %q", after.Name)
	}
}

func TestUpdateAccount_RejectsUnknownStatus(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	acc := mustSeedAccount(t, srv, companyID)

	resp := putJSON(t, srv.URL+"/accounts/"+acc.ID, map[string]any{
		"name": acc.Name, "account_type": "PROSPECT", "status": "ARCHIVED",
	})
	requireStatus(t, resp, http.StatusBadRequest)
}

// Pagar utama: menonaktifkan account yang masih punya opportunity berjalan akan
// menyembunyikan pekerjaan yang sedang dikerjakan orang.
func TestUpdateAccount_BlockedWhenOpportunityStillOpen(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	acc := mustSeedAccount(t, srv, companyID)
	opp := mustSeedOpportunity(t, srv, companyID, acc.ID)

	resp := putJSON(t, srv.URL+"/accounts/"+acc.ID, map[string]any{
		"name": acc.Name, "account_type": "PROSPECT", "status": "INACTIVE",
	})
	requireStatus(t, resp, http.StatusConflict)
	if msg := resp.errorMessage(); msg == "" {
		t.Error("expected pesan yang menyebut jumlah opportunity terbuka")
	}

	// Setelah opportunity-nya ditutup, penonaktifan berhasil.
	requireStatus(t, postJSON(t, srv.URL+"/opportunities/"+opp.ID+"/lose", map[string]any{}), http.StatusOK)
	if got := mustUpdateAccountStatus(t, srv, acc, "INACTIVE"); got.Status != "INACTIVE" {
		t.Fatalf("expected INACTIVE setelah opportunity ditutup, got %q", got.Status)
	}
}

func TestInactiveAccount_CannotBeUsedForNewRecords(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	acc := mustSeedAccount(t, srv, companyID)
	mustUpdateAccountStatus(t, srv, acc, "INACTIVE")

	t.Run("contact baru", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/contacts", map[string]any{
			"company_id": companyID, "account_id": acc.ID, "first_name": "Budi",
		})
		requireStatus(t, resp, http.StatusConflict)
	})

	t.Run("opportunity baru", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/opportunities", map[string]any{
			"company_id": companyID, "account_id": acc.ID, "name": "Deal Baru", "amount": 1000,
		})
		requireStatus(t, resp, http.StatusConflict)
	})

	t.Run("activity baru", func(t *testing.T) {
		resp := postJSON(t, srv.URL+"/activities", map[string]any{
			"company_id": companyID, "reference_type": "ACCOUNT", "reference_id": acc.ID,
			"activity_type": "CALL", "subject": "Telepon",
		})
		requireStatus(t, resp, http.StatusConflict)
	})
}

// Data yang SUDAH menempel di account nonaktif tetap terbaca -- yang dilarang
// hanya membuat yang baru.
func TestInactiveAccount_ExistingDataStillReadable(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	acc := mustSeedAccount(t, srv, companyID)
	contact := mustSeedContact(t, srv, companyID, acc.ID)
	mustUpdateAccountStatus(t, srv, acc, "INACTIVE")

	var contacts []contactFixture
	getJSON(t, srv.URL+"/contacts?company_id="+companyID).decode(t, &contacts)
	found := false
	for _, c := range contacts {
		if c.ID == contact.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("contact milik account nonaktif ikut hilang dari daftar")
	}
}

func TestListAccounts_FilterByStatus(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	aktif := mustSeedAccount(t, srv, companyID)
	nonaktif := mustSeedAccount(t, srv, companyID)
	mustUpdateAccountStatus(t, srv, nonaktif, "INACTIVE")

	var all []accountFixture
	getJSON(t, srv.URL+"/accounts?company_id="+companyID).decode(t, &all)
	if len(all) != 2 {
		t.Fatalf("tanpa filter seharusnya 2 account, got %d", len(all))
	}

	var onlyActive []accountFixture
	getJSON(t, srv.URL+"/accounts?company_id="+companyID+"&status=ACTIVE").decode(t, &onlyActive)
	if len(onlyActive) != 1 || onlyActive[0].ID != aktif.ID {
		t.Fatalf("filter ACTIVE tidak bekerja: %+v", onlyActive)
	}

	requireStatus(t, getJSON(t, srv.URL+"/accounts?company_id="+companyID+"&status=ENTAH"), http.StatusBadRequest)
}

func TestContact_StatusLifecycle(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	acc := mustSeedAccount(t, srv, companyID)
	contact := mustSeedContact(t, srv, companyID, acc.ID)
	if contact.Status != "ACTIVE" {
		t.Fatalf("contact baru seharusnya ACTIVE, got %q", contact.Status)
	}

	resp := putJSON(t, srv.URL+"/contacts/"+contact.ID, map[string]any{
		"first_name": contact.FirstName, "account_id": acc.ID, "status": "INACTIVE",
	})
	requireStatus(t, resp, http.StatusOK)
	var after contactFixture
	resp.decode(t, &after)
	if after.Status != "INACTIVE" {
		t.Fatalf("expected INACTIVE, got %q", after.Status)
	}

	// Contact nonaktif tidak bisa dipakai untuk opportunity & activity baru.
	requireStatus(t, postJSON(t, srv.URL+"/opportunities", map[string]any{
		"company_id": companyID, "account_id": acc.ID, "contact_id": contact.ID, "name": "Deal", "amount": 1,
	}), http.StatusConflict)
	requireStatus(t, postJSON(t, srv.URL+"/activities", map[string]any{
		"company_id": companyID, "reference_type": "CONTACT", "reference_id": contact.ID,
		"activity_type": "CALL", "subject": "Telepon",
	}), http.StatusConflict)

	var onlyActive []contactFixture
	getJSON(t, srv.URL+"/contacts?company_id="+companyID+"&status=ACTIVE").decode(t, &onlyActive)
	for _, c := range onlyActive {
		if c.ID == contact.ID {
			t.Fatal("contact nonaktif masih muncul di filter ACTIVE")
		}
	}
}
