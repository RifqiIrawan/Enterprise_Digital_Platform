package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const newPassword = "PasswordBaru@2026"

func changePassword(t *testing.T, srv *httptest.Server, userID string, payload map[string]any) apiResponse {
	t.Helper()
	headers := map[string]string{}
	if userID != "" {
		headers["X-User-Id"] = userID
	}
	return postJSONWithHeaders(t, srv.URL+"/change-password", payload, headers)
}

func TestChangePasswordReplacesTheHashAndLetsTheUserLogInWithTheNewOne(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)

	resp := changePassword(t, srv, user.ID, map[string]any{
		"current_password": testPassword,
		"new_password":     newPassword,
	})
	requireStatus(t, resp, http.StatusOK)

	var hash string
	if err := pool.QueryRow(context.Background(),
		`SELECT password_hash FROM users WHERE id = $1`, user.ID).Scan(&hash); err != nil {
		t.Fatalf("baca password_hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(newPassword)); err != nil {
		t.Fatalf("hash tersimpan bukan dari password baru: %v", err)
	}

	mustLogin(t, srv, email, newPassword)
	requireStatus(t, postJSON(t, srv.URL+"/login", map[string]any{
		"email": email, "password": testPassword,
	}), http.StatusUnauthorized)
}

// Ganti password mengusir SELURUH sesi, termasuk milik pemanggil sendiri:
// alasan paling umum orang menggantinya adalah karena curiga ada yang lain
// memakai akunnya, dan sesi itu yang harus putus.
func TestChangePasswordRevokesEveryRefreshToken(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)
	sessionA := mustLogin(t, srv, email, testPassword)
	sessionB := mustLogin(t, srv, email, testPassword)

	requireStatus(t, changePassword(t, srv, user.ID, map[string]any{
		"current_password": testPassword,
		"new_password":     newPassword,
	}), http.StatusOK)

	for name, session := range map[string]loginResponse{"sesi A": sessionA, "sesi B": sessionB} {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/refresh", map[string]any{
				"refresh_token": session.RefreshToken,
			}), http.StatusUnauthorized)
		})
	}

	var active int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL`, user.ID).Scan(&active); err != nil {
		t.Fatalf("hitung refresh token aktif: %v", err)
	}
	if active != 0 {
		t.Fatalf("expected every refresh token to be revoked, %d masih aktif", active)
	}
}

func TestChangePasswordRejectsTheWrongCurrentPassword(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)

	resp := changePassword(t, srv, user.ID, map[string]any{
		"current_password": "BukanPasswordnya123",
		"new_password":     newPassword,
	})
	requireStatus(t, resp, http.StatusUnauthorized)

	// Password lama harus tetap berlaku setelah percobaan yang gagal.
	mustLogin(t, srv, email, testPassword)
}

// Identitas diambil dari X-User-Id yang di-inject api-gateway dari klaim JWT,
// bukan dari body -- tanpa header itu tidak ada yang bisa diganti.
func TestChangePasswordNeedsAnIdentityHeader(t *testing.T) {
	srv := newServer(t)
	mustCreateUser(t, srv)

	resp := changePassword(t, srv, "", map[string]any{
		"current_password": testPassword,
		"new_password":     newPassword,
	})
	requireStatus(t, resp, http.StatusUnauthorized)
}

// Password orang lain tidak bisa diganti dengan menebak password sendiri:
// yang dicek adalah password milik user di header.
func TestChangePasswordOfAnotherUserFailsOnTheCurrentPasswordCheck(t *testing.T) {
	srv := newServer(t)
	victim, victimEmail := mustCreateUser(t, srv)
	_, _ = mustCreateUser(t, srv)

	resp := changePassword(t, srv, victim.ID, map[string]any{
		"current_password": "TebakanNgawur123",
		"new_password":     newPassword,
	})
	requireStatus(t, resp, http.StatusUnauthorized)
	mustLogin(t, srv, victimEmail, testPassword)
}

func TestChangePasswordValidatesThePayload(t *testing.T) {
	srv := newServer(t)
	user, _ := mustCreateUser(t, srv)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"tanpa password lama", map[string]any{"new_password": newPassword}},
		{"password baru 7 karakter", map[string]any{"current_password": testPassword, "new_password": "Ab@1234"}},
		{"password baru sama dengan yang lama", map[string]any{"current_password": testPassword, "new_password": testPassword}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, changePassword(t, srv, user.ID, tc.payload), http.StatusBadRequest)
		})
	}
}

func TestChangePasswordRejectsAnInactiveAccount(t *testing.T) {
	srv := newServer(t)
	user, _ := mustCreateUser(t, srv)
	setUserStatus(t, user.ID, "inactive")

	resp := changePassword(t, srv, user.ID, map[string]any{
		"current_password": testPassword,
		"new_password":     newPassword,
	})
	requireStatus(t, resp, http.StatusForbidden)
}

func TestChangePasswordUnknownUserReturns401(t *testing.T) {
	srv := newServer(t)

	resp := changePassword(t, srv, uuid.NewString(), map[string]any{
		"current_password": testPassword,
		"new_password":     newPassword,
	})
	requireStatus(t, resp, http.StatusUnauthorized)
}

func TestUpdateUserChangesNameAndPhone(t *testing.T) {
	srv := newServer(t)
	user, _ := mustCreateUser(t, srv)

	resp := putJSON(t, srv.URL+"/users/"+user.ID, map[string]any{
		"full_name": "  Nama Diperbarui  ",
		"phone":     "0811000111",
	})
	requireStatus(t, resp, http.StatusOK)

	var updated userFixture
	resp.decode(t, &updated)
	if updated.FullName != "Nama Diperbarui" {
		t.Errorf("expected trimmed full_name, got %q", updated.FullName)
	}
	if updated.Phone != "0811000111" {
		t.Errorf("expected phone to change, got %q", updated.Phone)
	}
	// Email & username adalah identitas login/tampilan yang sengaja tidak ada
	// di updateUserRequest.
	if updated.Email != user.Email || updated.Username != user.Username {
		t.Errorf("email/username seharusnya tidak berubah, got %s/%s", updated.Email, updated.Username)
	}
}

// Status kosong berarti TIDAK DIUBAH: menyunting nama tidak boleh diam-diam
// mengaktifkan kembali akun yang sengaja dinonaktifkan.
func TestUpdateUserWithoutStatusKeepsTheExistingOne(t *testing.T) {
	srv := newServer(t)
	user, _ := mustCreateUser(t, srv)
	setUserStatus(t, user.ID, "locked")

	resp := putJSON(t, srv.URL+"/users/"+user.ID, map[string]any{"full_name": "Masih Terkunci"})
	requireStatus(t, resp, http.StatusOK)

	var updated userFixture
	resp.decode(t, &updated)
	if updated.Status != "locked" {
		t.Fatalf("expected status to stay locked, got %q", updated.Status)
	}
}

func TestUpdateUserDeactivationBlocksLoginAndRevokesSessions(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)
	session := mustLogin(t, srv, email, testPassword)

	requireStatus(t, putJSON(t, srv.URL+"/users/"+user.ID, map[string]any{
		"full_name": user.FullName,
		"status":    "inactive",
	}), http.StatusOK)

	// Login baru ditolak 403 (akun tidak aktif)...
	requireStatus(t, postJSON(t, srv.URL+"/login", map[string]any{
		"email": email, "password": testPassword,
	}), http.StatusForbidden)
	// ...dan sesi yang sudah berjalan tidak bisa diperpanjang lagi.
	requireStatus(t, postJSON(t, srv.URL+"/refresh", map[string]any{
		"refresh_token": session.RefreshToken,
	}), http.StatusUnauthorized)
}

func TestUpdateUserCanReactivateAnAccount(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)
	setUserStatus(t, user.ID, "inactive")

	requireStatus(t, putJSON(t, srv.URL+"/users/"+user.ID, map[string]any{
		"full_name": user.FullName,
		"status":    "active",
	}), http.StatusOK)

	mustLogin(t, srv, email, testPassword)
}

func TestUpdateUserRejectsBadPayload(t *testing.T) {
	srv := newServer(t)
	user, _ := mustCreateUser(t, srv)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"nama kosong", map[string]any{"full_name": "   ", "status": "active"}},
		{"status tidak dikenal", map[string]any{"full_name": "Nama", "status": "archived"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, putJSON(t, srv.URL+"/users/"+user.ID, tc.payload), http.StatusBadRequest)
		})
	}
}

func TestUpdateUserUnknownIDReturns404(t *testing.T) {
	srv := newServer(t)

	resp := putJSON(t, srv.URL+"/users/"+uuid.NewString(), map[string]any{"full_name": "Siapa Saja"})
	requireStatus(t, resp, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Reset password oleh admin
//
// Jalur "lupa password" di platform ini: admin yang menetapkan, karena tidak
// ada infrastruktur email sama sekali di stack ini.
// ---------------------------------------------------------------------------

type resetPasswordView struct {
	Status            string `json:"status"`
	TemporaryPassword string `json:"temporary_password"`
}

func mustResetPassword(t *testing.T, srv *httptest.Server, userID string, payload map[string]any) resetPasswordView {
	t.Helper()
	resp := postJSON(t, srv.URL+"/users/"+userID+"/reset-password", payload)
	requireStatus(t, resp, http.StatusOK)
	var out resetPasswordView
	resp.decode(t, &out)
	return out
}

func TestResetPassword_GeneratesUsableTemporaryPassword(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)

	out := mustResetPassword(t, srv, user.ID, nil)
	if len(out.TemporaryPassword) < 8 {
		t.Fatalf("password sementara terlalu pendek: %q", out.TemporaryPassword)
	}

	// Password lama langsung tidak berlaku, yang sementara bisa dipakai.
	requireStatus(t, postJSON(t, srv.URL+"/login", map[string]any{
		"email": email, "password": testPassword,
	}), http.StatusUnauthorized)
	session := mustLogin(t, srv, email, out.TemporaryPassword)

	// Dan user ditandai wajib mengganti sendiri.
	if !session.User.MustChangePassword {
		t.Error("expected must_change_password true setelah direset admin")
	}
}

// Admin boleh menetapkan password sendiri; kalau begitu tidak ada nilai yang
// dikembalikan (dia sudah tahu).
func TestResetPassword_AcceptsExplicitPassword(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)

	out := mustResetPassword(t, srv, user.ID, map[string]any{"new_password": "Sementara@2026"})
	if out.TemporaryPassword != "" {
		t.Errorf("password yang ditetapkan admin tidak perlu dikembalikan, got %q", out.TemporaryPassword)
	}
	mustLogin(t, srv, email, "Sementara@2026")
}

func TestResetPassword_ValidationAndNotFound(t *testing.T) {
	srv := newServer(t)
	user, _ := mustCreateUser(t, srv)

	requireStatus(t, postJSON(t, srv.URL+"/users/"+user.ID+"/reset-password",
		map[string]any{"new_password": "pendek"}), http.StatusBadRequest)
	requireStatus(t, postJSON(t, srv.URL+"/users/"+uuid.NewString()+"/reset-password",
		map[string]any{}), http.StatusNotFound)
}

// Alasan paling umum sebuah reset diminta adalah akun yang dipakai orang lain,
// jadi sesi yang sedang berjalan harus putus saat itu juga.
func TestResetPassword_RevokesExistingSessions(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)
	session := mustLogin(t, srv, email, testPassword)

	mustResetPassword(t, srv, user.ID, nil)

	requireStatus(t, postJSON(t, srv.URL+"/refresh", map[string]any{
		"refresh_token": session.RefreshToken,
	}), http.StatusUnauthorized)
}

// Penanda wajib-ganti mati begitu user benar-benar menggantinya sendiri --
// setelah itu password yang berlaku hanya diketahui dia.
func TestChangePassword_ClearsMustChangeFlag(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)
	out := mustResetPassword(t, srv, user.ID, nil)

	before := mustLogin(t, srv, email, out.TemporaryPassword)
	if !before.User.MustChangePassword {
		t.Fatal("prasyarat gagal: penanda seharusnya menyala")
	}

	requireStatus(t, changePassword(t, srv, user.ID, map[string]any{
		"current_password": out.TemporaryPassword,
		"new_password":     newPassword,
	}), http.StatusOK)

	after := mustLogin(t, srv, email, newPassword)
	if after.User.MustChangePassword {
		t.Error("penanda seharusnya mati setelah user mengganti password sendiri")
	}
}

// User biasa (belum pernah direset) tidak boleh ikut ditandai.
func TestUser_MustChangePasswordDefaultsFalse(t *testing.T) {
	srv := newServer(t)
	_, email := mustCreateUser(t, srv)

	session := mustLogin(t, srv, email, testPassword)
	if session.User.MustChangePassword {
		t.Error("user baru tidak boleh ditandai wajib ganti password")
	}
}
