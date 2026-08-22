package httpapi_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/enterprise-digital-platform/auth-service/internal/jwtutil"
)

func TestLoginReturnsTokensAndUserProfile(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)

	out := mustLogin(t, srv, email, testPassword)

	if out.TokenType != "Bearer" {
		t.Errorf("expected token_type Bearer, got %q", out.TokenType)
	}
	if out.ExpiresIn != int(testAccessTTL.Seconds()) {
		t.Errorf("expected expires_in %d, got %d", int(testAccessTTL.Seconds()), out.ExpiresIn)
	}
	if out.RefreshToken == "" {
		t.Error("expected a refresh token")
	}
	if out.User.ID != user.ID || out.User.Email != email {
		t.Errorf("expected the logged-in user in the body, got %+v", out.User)
	}

	// Klaim di access token inilah yang dibaca api-gateway untuk mengisi
	// X-User-Id & X-Is-Super-Admin ke service di belakangnya.
	claims, err := jwtutil.Parse(testJWTSecret, out.AccessToken)
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	if claims.Subject != user.ID {
		t.Errorf("expected sub %s, got %s", user.ID, claims.Subject)
	}
	if claims.Email != email || claims.FullName != user.FullName {
		t.Errorf("expected email/full_name in the claims, got %+v", claims)
	}
	if claims.IsSuperAdmin {
		t.Error("user biasa tidak boleh dapat klaim is_super_admin")
	}
	if claims.Issuer != "auth-service" {
		t.Errorf("expected issuer auth-service, got %q", claims.Issuer)
	}
}

// Refresh token disimpan sebagai SHA-256, bukan apa adanya: bocornya isi tabel
// refresh_tokens tidak boleh langsung bisa dipakai login ulang.
func TestLoginStoresOnlyTheHashOfTheRefreshToken(t *testing.T) {
	srv := newServer(t)
	_, email := mustCreateUser(t, srv)

	out := mustLogin(t, srv, email, testPassword)
	sum := sha256.Sum256([]byte(out.RefreshToken))
	wantHash := hex.EncodeToString(sum[:])

	var storedHash string
	if err := pool.QueryRow(context.Background(),
		`SELECT token_hash FROM refresh_tokens WHERE token_hash = $1`, wantHash).Scan(&storedHash); err != nil {
		t.Fatalf("refresh token tidak tersimpan sebagai hash: %v", err)
	}

	var plaintextRows int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`, out.RefreshToken).Scan(&plaintextRows); err != nil {
		t.Fatalf("cek plaintext: %v", err)
	}
	if plaintextRows != 0 {
		t.Fatal("refresh token tersimpan apa adanya, bukan hash")
	}
}

func TestLoginRecordsLastLoginAt(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)
	if user.LastLoginAt != nil {
		t.Fatalf("user baru seharusnya belum punya last_login_at, got %v", user.LastLoginAt)
	}

	before := time.Now().Add(-time.Minute)
	out := mustLogin(t, srv, email, testPassword)

	if out.User.LastLoginAt == nil {
		t.Fatal("expected last_login_at in the login response")
	}
	var stored *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT last_login_at FROM users WHERE id = $1`, user.ID).Scan(&stored); err != nil {
		t.Fatalf("baca last_login_at: %v", err)
	}
	if stored == nil || stored.Before(before) {
		t.Fatalf("expected last_login_at to be updated, got %v", stored)
	}
}

// Email di-lowercase & di-trim sebelum dicari, jadi huruf besar atau spasi
// nyasar dari form login tidak bikin akun terasa "hilang".
func TestLoginNormalizesEmailBeforeLookup(t *testing.T) {
	srv := newServer(t)
	_, email := mustCreateUser(t, srv)

	resp := postJSON(t, srv.URL+"/login", map[string]any{
		"email":    "  " + strings.ToUpper(email) + "  ",
		"password": testPassword,
	})
	requireStatus(t, resp, http.StatusOK)
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	srv := newServer(t)
	_, email := mustCreateUser(t, srv)

	resp := postJSON(t, srv.URL+"/login", map[string]any{"email": email, "password": "SalahBanget123"})
	requireStatus(t, resp, http.StatusUnauthorized)
}

// Email tak dikenal dan password salah harus tidak bisa dibedakan dari luar --
// kalau beda, endpoint login jadi alat pengecek "email ini terdaftar atau tidak".
func TestLoginDoesNotRevealWhetherAnEmailExists(t *testing.T) {
	srv := newServer(t)
	_, email := mustCreateUser(t, srv)

	wrongPassword := postJSON(t, srv.URL+"/login", map[string]any{"email": email, "password": "SalahBanget123"})
	unknownEmail := postJSON(t, srv.URL+"/login", map[string]any{"email": testEmail(), "password": testPassword})

	if wrongPassword.status != unknownEmail.status {
		t.Fatalf("status berbeda: password salah %d, email tak dikenal %d", wrongPassword.status, unknownEmail.status)
	}
	if wrongPassword.errorMessage() != unknownEmail.errorMessage() {
		t.Fatalf("pesan berbeda: %q vs %q", wrongPassword.errorMessage(), unknownEmail.errorMessage())
	}
}

// Akun nonaktif ditolak SEBELUM password dicek, dengan 403 (bukan 401) supaya
// frontend bisa membedakan "kredensial salah" dari "akun dimatikan".
func TestLoginRejectsInactiveAccount(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)

	for _, status := range []string{"inactive", "locked"} {
		t.Run(status, func(t *testing.T) {
			setUserStatus(t, user.ID, status)
			resp := postJSON(t, srv.URL+"/login", map[string]any{"email": email, "password": testPassword})
			requireStatus(t, resp, http.StatusForbidden)
		})
	}
}

func TestLoginRequiresEmailAndPassword(t *testing.T) {
	srv := newServer(t)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"tanpa email", map[string]any{"password": testPassword}},
		{"email hanya spasi", map[string]any{"email": "   ", "password": testPassword}},
		{"tanpa password", map[string]any{"email": testEmail()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/login", tc.payload), http.StatusBadRequest)
		})
	}
}

func TestLoginRejectsMalformedPayload(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, postRawJSON(t, srv.URL+"/login", "{bukan json"), http.StatusBadRequest)
}

func TestRefreshIssuesANewAccessTokenWithoutRotatingTheRefreshToken(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)
	out := mustLogin(t, srv, email, testPassword)

	resp := postJSON(t, srv.URL+"/refresh", map[string]any{"refresh_token": out.RefreshToken})
	requireStatus(t, resp, http.StatusOK)

	var refreshed loginResponse
	resp.decode(t, &refreshed)
	if refreshed.AccessToken == "" {
		t.Fatal("expected a new access token")
	}
	claims, err := jwtutil.Parse(testJWTSecret, refreshed.AccessToken)
	if err != nil {
		t.Fatalf("parse access token hasil refresh: %v", err)
	}
	if claims.Subject != user.ID {
		t.Errorf("expected sub %s, got %s", user.ID, claims.Subject)
	}
	// Refresh token TIDAK dirotasi: yang lama tetap berlaku dan tidak ada
	// refresh_token baru di body, jadi frontend tetap memakai yang dipegangnya.
	if refreshed.RefreshToken != "" {
		t.Errorf("expected no new refresh token, got %q", refreshed.RefreshToken)
	}
	requireStatus(t, postJSON(t, srv.URL+"/refresh", map[string]any{"refresh_token": out.RefreshToken}), http.StatusOK)
}

func TestRefreshRejectsUnknownToken(t *testing.T) {
	srv := newServer(t)

	resp := postJSON(t, srv.URL+"/refresh", map[string]any{"refresh_token": "token-yang-tidak-pernah-ada"})
	requireStatus(t, resp, http.StatusUnauthorized)
}

func TestRefreshRejectsExpiredToken(t *testing.T) {
	srv := newServer(t)
	_, email := mustCreateUser(t, srv)
	out := mustLogin(t, srv, email, testPassword)

	// TTL-nya 24 jam; dimundurkan lewat SQL supaya test tidak perlu menunggu.
	if _, err := pool.Exec(context.Background(),
		`UPDATE refresh_tokens SET expires_at = now() - interval '1 minute' WHERE token_hash = $1`,
		sha256Hex(out.RefreshToken)); err != nil {
		t.Fatalf("mundurkan expires_at: %v", err)
	}

	resp := postJSON(t, srv.URL+"/refresh", map[string]any{"refresh_token": out.RefreshToken})
	requireStatus(t, resp, http.StatusUnauthorized)
}

func TestRefreshRejectsTokenOfAnInactiveUser(t *testing.T) {
	srv := newServer(t)
	user, email := mustCreateUser(t, srv)
	out := mustLogin(t, srv, email, testPassword)

	// Menonaktifkan akun harus langsung memutus sesi yang masih berjalan pada
	// perpanjangan berikutnya, bukan menunggu refresh token-nya kedaluwarsa.
	setUserStatus(t, user.ID, "inactive")

	resp := postJSON(t, srv.URL+"/refresh", map[string]any{"refresh_token": out.RefreshToken})
	requireStatus(t, resp, http.StatusUnauthorized)
}

func TestRefreshRequiresARefreshToken(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, postJSON(t, srv.URL+"/refresh", map[string]any{}), http.StatusBadRequest)
	requireStatus(t, postRawJSON(t, srv.URL+"/refresh", "["), http.StatusBadRequest)
}

func TestLogoutRevokesTheRefreshToken(t *testing.T) {
	srv := newServer(t)
	_, email := mustCreateUser(t, srv)
	out := mustLogin(t, srv, email, testPassword)

	requireStatus(t, postJSON(t, srv.URL+"/logout", map[string]any{"refresh_token": out.RefreshToken}), http.StatusOK)

	var revokedAt *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT revoked_at FROM refresh_tokens WHERE token_hash = $1`, sha256Hex(out.RefreshToken)).Scan(&revokedAt); err != nil {
		t.Fatalf("baca revoked_at: %v", err)
	}
	if revokedAt == nil {
		t.Fatal("expected revoked_at to be set")
	}

	requireStatus(t, postJSON(t, srv.URL+"/refresh", map[string]any{"refresh_token": out.RefreshToken}), http.StatusUnauthorized)
}

// Logout dua kali (mis. dua tab browser) tetap 200: klausa `revoked_at IS NULL`
// membuat yang kedua tidak mengubah apa pun, bukan error.
func TestLogoutIsIdempotent(t *testing.T) {
	srv := newServer(t)
	_, email := mustCreateUser(t, srv)
	out := mustLogin(t, srv, email, testPassword)

	requireStatus(t, postJSON(t, srv.URL+"/logout", map[string]any{"refresh_token": out.RefreshToken}), http.StatusOK)

	var firstRevokedAt time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT revoked_at FROM refresh_tokens WHERE token_hash = $1`, sha256Hex(out.RefreshToken)).Scan(&firstRevokedAt); err != nil {
		t.Fatalf("baca revoked_at: %v", err)
	}

	requireStatus(t, postJSON(t, srv.URL+"/logout", map[string]any{"refresh_token": out.RefreshToken}), http.StatusOK)

	var secondRevokedAt time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT revoked_at FROM refresh_tokens WHERE token_hash = $1`, sha256Hex(out.RefreshToken)).Scan(&secondRevokedAt); err != nil {
		t.Fatalf("baca revoked_at kedua: %v", err)
	}
	if !secondRevokedAt.Equal(firstRevokedAt) {
		t.Errorf("logout kedua mengubah revoked_at: %v -> %v", firstRevokedAt, secondRevokedAt)
	}
}

// Logout token yang tidak dikenal tetap 200: memberi tahu "token ini tidak ada"
// pun sudah membocorkan informasi, dan tidak ada yang bisa dilakukan pemanggil.
func TestLogoutOfAnUnknownTokenStillSucceeds(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, postJSON(t, srv.URL+"/logout", map[string]any{"refresh_token": "entah-apa"}), http.StatusOK)
}

func TestLogoutRequiresARefreshToken(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, postJSON(t, srv.URL+"/logout", map[string]any{}), http.StatusBadRequest)
}

func sha256Hex(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
