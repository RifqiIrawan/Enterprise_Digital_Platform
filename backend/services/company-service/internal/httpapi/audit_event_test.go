package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Test di file ini sengaja internal (package httpapi, bukan httpapi_test)
// supaya bisa memanggil newAuditEvent/actorFromHeader langsung sebagai fungsi
// murni -- pola yang sama dengan pendingTopics di audit-service, yang dipisah
// justru supaya bisa diuji tanpa Kafka. Publisher di service ini adalah struct
// konkret (bukan interface), jadi event yang dipublikasikan tidak bisa
// ditangkap lewat test HTTP end-to-end tanpa membongkar tipenya.

func requestWithActor(userID, email string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/companies", nil)
	if userID != "" {
		r.Header.Set("X-User-Id", userID)
	}
	if email != "" {
		r.Header.Set("X-User-Email", email)
	}
	return r
}

func TestActorFromHeaderReadsGatewayHeaders(t *testing.T) {
	userID, email := actorFromHeader(requestWithActor("user-123", "admin@edp.local"))

	if userID == nil || *userID != "user-123" {
		t.Fatalf("actor user id = %v, mau user-123", userID)
	}
	if email == nil || *email != "admin@edp.local" {
		t.Fatalf("actor email = %v, mau admin@edp.local", email)
	}
}

// Dipanggil langsung (bukan lewat api-gateway) header ini memang tidak ada.
// Yang benar adalah nil -- bukan pointer ke string kosong, yang akan terbaca
// di audit log sebagai "ada aktornya, tapi kosong".
func TestActorFromHeaderNilWhenCalledWithoutGateway(t *testing.T) {
	userID, email := actorFromHeader(requestWithActor("", ""))

	if userID != nil {
		t.Fatalf("actor user id = %v, mau nil", *userID)
	}
	if email != nil {
		t.Fatalf("actor email = %v, mau nil", *email)
	}
}

func TestNewAuditEventCarriesActor(t *testing.T) {
	companyID := "company-1"
	ev := newAuditEvent(requestWithActor("user-123", "admin@edp.local"),
		"company.branch.deleted", &companyID, "delete", "branch", "branch-9", nil)

	if ev.ActorUserID == nil || *ev.ActorUserID != "user-123" {
		t.Fatalf("ActorUserID = %v, mau user-123", ev.ActorUserID)
	}
	if ev.ActorEmail == nil || *ev.ActorEmail != "admin@edp.local" {
		t.Fatalf("ActorEmail = %v, mau admin@edp.local", ev.ActorEmail)
	}
	if ev.EntityType != "branch" || ev.EntityID != "branch-9" || ev.Action != "delete" {
		t.Fatalf("amplop selain aktor ikut berubah: %+v", ev)
	}
}

// Tanpa aktor, kunci actor_* harus HILANG dari JSON (omitempty), bukan muncul
// sebagai null/"" -- audit-service memetakan langsung amplop ini ke kolomnya.
func TestAuditEventOmitsActorKeysWhenAbsent(t *testing.T) {
	ev := newAuditEvent(requestWithActor("", ""), "company.company.created", nil, "create", "company", "c-1", nil)

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := decoded["actor_user_id"]; ok {
		t.Errorf("actor_user_id tetap muncul di JSON tanpa aktor: %s", data)
	}
	if _, ok := decoded["actor_email"]; ok {
		t.Errorf("actor_email tetap muncul di JSON tanpa aktor: %s", data)
	}
}

// newAuditEvent dipanggil dari handler, dan handler selalu punya *http.Request.
// Guard nil-nya tetap diuji supaya refactor yang memanggilnya dari tempat lain
// (mis. job internal) tidak berubah jadi panic.
func TestActorFromHeaderNilRequest(t *testing.T) {
	userID, email := actorFromHeader(nil)
	if userID != nil || email != nil {
		t.Fatalf("mau nil,nil untuk request nil; dapat %v,%v", userID, email)
	}
}
