package jwtutil_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/enterprise-digital-platform/auth-service/internal/jwtutil"
)

const secret = "secret-khusus-test"

func TestIssueAndParseRoundTrip(t *testing.T) {
	token, err := jwtutil.IssueAccessToken(secret, "user-1", "user@edp.test", "User Satu", true, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	claims, err := jwtutil.Parse(secret, token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.Subject != "user-1" || claims.Email != "user@edp.test" || claims.FullName != "User Satu" {
		t.Errorf("klaim tidak sesuai: %+v", claims)
	}
	if !claims.IsSuperAdmin {
		t.Error("expected is_super_admin true")
	}
	if claims.Issuer != "auth-service" {
		t.Errorf("expected issuer auth-service, got %q", claims.Issuer)
	}
	if claims.ExpiresAt == nil || !claims.ExpiresAt.After(time.Now()) {
		t.Errorf("expected a future exp, got %v", claims.ExpiresAt)
	}
}

// Token yang ditandatangani secret lain harus ditolak: inilah satu-satunya
// yang memisahkan token asli dari token buatan siapa pun.
func TestParseRejectsATokenSignedWithAnotherSecret(t *testing.T) {
	token, err := jwtutil.IssueAccessToken("secret-lain", "user-1", "user@edp.test", "User Satu", false, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	if _, err := jwtutil.Parse(secret, token); err == nil {
		t.Fatal("token dengan secret berbeda seharusnya ditolak")
	}
}

func TestParseRejectsAnExpiredToken(t *testing.T) {
	token, err := jwtutil.IssueAccessToken(secret, "user-1", "user@edp.test", "User Satu", false, -time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	if _, err := jwtutil.Parse(secret, token); err == nil {
		t.Fatal("token kedaluwarsa seharusnya ditolak")
	}
}

// Algorithm confusion: token ber-alg "none" tidak boleh lolos hanya karena
// tidak punya signature untuk diperiksa.
func TestParseRejectsTheNoneAlgorithm(t *testing.T) {
	claims := jwtutil.Claims{
		Email: "penyusup@edp.test",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Issuer:    "auth-service",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("bikin token alg=none: %v", err)
	}

	if _, err := jwtutil.Parse(secret, token); err == nil {
		t.Fatal("token alg=none seharusnya ditolak")
	}
}

// Varian lain dari algorithm confusion: token RS256 yang menaruh public key
// (di sini: kunci apa pun) sebagai "secret". Penjagaannya adalah pemeriksaan
// *jwt.SigningMethodHMAC di dalam Parse.
func TestParseRejectsANonHMACAlgorithm(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("bikin kunci RSA: %v", err)
	}
	claims := jwtutil.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Issuer:    "auth-service",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatalf("bikin token RS256: %v", err)
	}

	if _, err := jwtutil.Parse(secret, token); err == nil {
		t.Fatal("token RS256 seharusnya ditolak oleh pemeriksaan signing method")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "bukan.token.sama.sekali", "a.b.c"} {
		if _, err := jwtutil.Parse(secret, s); err == nil {
			t.Errorf("string %q seharusnya ditolak", s)
		}
	}
}
