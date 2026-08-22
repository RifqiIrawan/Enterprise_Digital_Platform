package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestCreateUserDerivesUsernameAndHashesThePassword(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)

	local, _, _ := strings.Cut(email, "@")
	if user.Username != local {
		t.Errorf("expected username %q (bagian sebelum @), got %q", local, user.Username)
	}
	if user.Status != "active" {
		t.Errorf("expected status active, got %q", user.Status)
	}
	// Hak akses ada di rbac-service; user baru tidak pernah lahir sebagai
	// super admin lewat endpoint ini.
	if user.IsSuperAdmin {
		t.Error("user baru tidak boleh is_super_admin")
	}

	var hash string
	if err := pool.QueryRow(context.Background(),
		`SELECT password_hash FROM users WHERE id = $1`, user.ID).Scan(&hash); err != nil {
		t.Fatalf("baca password_hash: %v", err)
	}
	if hash == testPassword {
		t.Fatal("password tersimpan apa adanya")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(testPassword)); err != nil {
		t.Fatalf("password_hash bukan bcrypt dari password yang dikirim: %v", err)
	}
}

func TestCreateUserNormalizesEmailAndTrimsName(t *testing.T) {
	srv := newServer(t)
	email := testEmail()

	resp := postJSON(t, srv.URL+"/users", map[string]any{
		"email":     "  " + strings.ToUpper(email) + " ",
		"full_name": "  Budi Santoso  ",
		"password":  testPassword,
	})
	requireStatus(t, resp, http.StatusCreated)

	var user userFixture
	resp.decode(t, &user)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})

	if user.Email != email {
		t.Errorf("expected email %q, got %q", email, user.Email)
	}
	if user.FullName != "Budi Santoso" {
		t.Errorf("expected trimmed full_name, got %q", user.FullName)
	}
	// Login pakai email versi ternormalisasi harus langsung jalan -- kalau
	// penyimpanan dan pencarian tidak dinormalkan dengan cara yang sama,
	// user yang baru dibuat justru tidak bisa masuk.
	mustLogin(t, srv, email, testPassword)
}

func TestCreateUserRejectsIncompletePayload(t *testing.T) {
	srv := newServer(t)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"tanpa email", map[string]any{"full_name": "Tanpa Email", "password": testPassword}},
		{"tanpa nama", map[string]any{"email": testEmail(), "full_name": "   ", "password": testPassword}},
		{"password 7 karakter", map[string]any{"email": testEmail(), "full_name": "Pendek", "password": "Ab@1234"}},
		{"tanpa password", map[string]any{"email": testEmail(), "full_name": "Tanpa Password"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/users", tc.payload), http.StatusBadRequest)
		})
	}
}

func TestCreateUserRejectsMalformedPayload(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, postRawJSON(t, srv.URL+"/users", "{"), http.StatusBadRequest)
}

func TestCreateUserRejectsDuplicateEmail(t *testing.T) {
	srv := newServer(t)
	_, email := mustCreateUser(t, srv)

	resp := postJSON(t, srv.URL+"/users", map[string]any{
		"email":     strings.ToUpper(email),
		"full_name": "Email Kembar",
		"password":  testPassword,
	})
	requireStatus(t, resp, http.StatusConflict)
}

// Password (dan hash-nya) tidak boleh pernah ikut keluar lewat HTTP -- itulah
// gunanya userResponse yang terpisah dari model.User.
func TestUserResponsesNeverExposeCredentials(t *testing.T) {
	srv := newServer(t)
	_, email := mustCreateUser(t, srv)
	created := postJSON(t, srv.URL+"/users", map[string]any{
		"email":     testEmail(),
		"full_name": "Sekali Pakai",
		"password":  testPassword,
	})
	requireStatus(t, created, http.StatusCreated)
	var throwaway userFixture
	created.decode(t, &throwaway)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, throwaway.ID)
	})

	bodies := map[string]string{
		"POST /users": string(created.body),
		"GET /users":  string(getJSON(t, srv.URL+"/users").body),
		"POST /login": string(postJSON(t, srv.URL+"/login", map[string]any{"email": email, "password": testPassword}).body),
	}
	// Pemeriksaannya menyasar KREDENSIAL, bukan kata "password": ada field sah
	// yang mengandung kata itu (must_change_password) dan sempat membuat versi
	// pertama test ini gagal palsu. Yang benar-benar tidak boleh muncul adalah
	// nilai password-nya, hash bcrypt-nya, dan key yang membawanya.
	forbidden := []string{`"password"`, `"password_hash"`, testPassword, "$2a$", "$2b$"}
	for label, body := range bodies {
		for _, needle := range forbidden {
			if strings.Contains(body, needle) {
				t.Errorf("%s membocorkan kredensial (%q): %s", label, needle, body)
			}
		}
	}
}

func TestListUsersIsOrderedByFullName(t *testing.T) {
	srv := newServer(t)
	mustCreateUser(t, srv)

	resp := getJSON(t, srv.URL+"/users")
	requireStatus(t, resp, http.StatusOK)

	var users []userFixture
	resp.decode(t, &users)
	if len(users) < 2 {
		t.Fatalf("expected at least the seeded admin plus the test user, got %d", len(users))
	}
	for i := 1; i < len(users); i++ {
		if users[i].FullName < users[i-1].FullName {
			t.Fatalf("urutan nama tidak menaik: %q setelah %q", users[i].FullName, users[i-1].FullName)
		}
	}
}

// Seed 002 menyediakan satu Super Admin supaya login bisa langsung dicoba;
// dia yang dipakai di seluruh verifikasi end-to-end di browser.
func TestSeededSuperAdminCanLogInAndCarriesTheSuperAdminClaim(t *testing.T) {
	srv := newServer(t)

	out := mustLogin(t, srv, "admin@edp.local", "Admin@12345")
	if !out.User.IsSuperAdmin {
		t.Fatal("expected the seeded admin to be a super admin")
	}
	requireStatus(t, postJSON(t, srv.URL+"/logout", map[string]any{"refresh_token": out.RefreshToken}), http.StatusOK)
}

// Kolom username juga UNIQUE padahal isinya cuma bagian sebelum "@": dua orang
// bernama sama di domain berbeda (budi@perusahaan-a.com & budi@perusahaan-b.com)
// bentrok di username, bukan di email. Pendaftaran kedua harus tetap berhasil
// dengan username yang dibedakan, bukan ditolak 409 "Email sudah terdaftar".
func TestCreateUserSurvivesAUsernameCollisionFromAnotherDomain(t *testing.T) {
	srv := newServer(t)
	local := "budi-" + strings.ToLower(uuid.NewString()[:8])

	first := postJSON(t, srv.URL+"/users", map[string]any{
		"email": local + "@perusahaan-a.test", "full_name": "Budi A", "password": testPassword,
	})
	requireStatus(t, first, http.StatusCreated)
	var userA userFixture
	first.decode(t, &userA)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userA.ID)
	})

	second := postJSON(t, srv.URL+"/users", map[string]any{
		"email": local + "@perusahaan-b.test", "full_name": "Budi B", "password": testPassword,
	})
	requireStatus(t, second, http.StatusCreated)
	var userB userFixture
	second.decode(t, &userB)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userB.ID)
	})

	if userA.Username != local {
		t.Errorf("user pertama seharusnya dapat username %q, got %q", local, userA.Username)
	}
	if userB.Username == userA.Username {
		t.Errorf("username user kedua tidak dibedakan: %q", userB.Username)
	}
	if !strings.HasPrefix(userB.Username, local+"-") {
		t.Errorf("expected username %q dengan akhiran pembeda, got %q", local, userB.Username)
	}
	// Yang penting: keduanya benar-benar bisa login dengan emailnya sendiri.
	mustLogin(t, srv, local+"@perusahaan-a.test", testPassword)
	mustLogin(t, srv, local+"@perusahaan-b.test", testPassword)
}
