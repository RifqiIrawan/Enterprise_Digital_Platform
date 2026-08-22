package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestAssignUserRoleJoinsRoleCodeAndName(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	userID := uuid.NewString()
	companyID := uuid.NewString()

	resp := postJSON(t, srv.URL+"/user-roles", map[string]any{
		"user_id":    userID,
		"role_id":    role.ID,
		"company_id": companyID,
	})
	requireStatus(t, resp, http.StatusCreated)

	var assigned userRoleFixture
	resp.decode(t, &assigned)
	// UserRoleAssignmentPage merender chip role dari code/name di response ini,
	// jadi POST harus sudah membawa hasil JOIN-nya, bukan hanya role_id.
	if assigned.RoleCode != role.Code || assigned.RoleName != role.Name {
		t.Errorf("expected role %s/%s, got %s/%s", role.Code, role.Name, assigned.RoleCode, assigned.RoleName)
	}
	if assigned.UserID != userID || assigned.CompanyID != companyID {
		t.Errorf("expected scope %s/%s, got %s/%s", userID, companyID, assigned.UserID, assigned.CompanyID)
	}
	// branch_id/department_id NULL = berlaku di seluruh branch & department.
	if assigned.BranchID != nil {
		t.Errorf("expected branch_id nil, got %v", *assigned.BranchID)
	}
}

// assigned_by diambil dari X-User-Id yang di-inject api-gateway dari klaim JWT:
// inilah jejak "siapa yang memberi akses" yang dipakai saat audit.
func TestAssignUserRoleRecordsAssignedByFromHeader(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	actorID := uuid.NewString()

	resp := postJSONWithHeaders(t, srv.URL+"/user-roles", map[string]any{
		"user_id":    uuid.NewString(),
		"role_id":    role.ID,
		"company_id": uuid.NewString(),
	}, map[string]string{"X-User-Id": actorID})
	requireStatus(t, resp, http.StatusCreated)

	var assigned userRoleFixture
	resp.decode(t, &assigned)

	var assignedBy *string
	if err := pool.QueryRow(context.Background(),
		`SELECT assigned_by::text FROM user_roles WHERE id = $1`, assigned.ID).Scan(&assignedBy); err != nil {
		t.Fatalf("baca assigned_by: %v", err)
	}
	if assignedBy == nil || *assignedBy != actorID {
		t.Fatalf("expected assigned_by %s, got %v", actorID, assignedBy)
	}
}

// Tanpa header (mis. dipanggil langsung, bukan lewat gateway) penugasan tetap
// jalan dengan assigned_by NULL -- bukan gagal, dan bukan string kosong yang
// akan ditolak kolom UUID.
func TestAssignUserRoleWithoutHeaderLeavesAssignedByNull(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)

	resp := postJSON(t, srv.URL+"/user-roles", map[string]any{
		"user_id":    uuid.NewString(),
		"role_id":    role.ID,
		"company_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusCreated)

	var assigned userRoleFixture
	resp.decode(t, &assigned)

	var assignedBy *string
	if err := pool.QueryRow(context.Background(),
		`SELECT assigned_by::text FROM user_roles WHERE id = $1`, assigned.ID).Scan(&assignedBy); err != nil {
		t.Fatalf("baca assigned_by: %v", err)
	}
	if assignedBy != nil {
		t.Fatalf("expected assigned_by NULL, got %q", *assignedBy)
	}
}

func TestAssignUserRoleRequiresUserRoleAndCompany(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"tanpa user_id", map[string]any{"role_id": role.ID, "company_id": uuid.NewString()}},
		{"tanpa role_id", map[string]any{"user_id": uuid.NewString(), "company_id": uuid.NewString()}},
		{"tanpa company_id", map[string]any{"user_id": uuid.NewString(), "role_id": role.ID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireStatus(t, postJSON(t, srv.URL+"/user-roles", tc.payload), http.StatusBadRequest)
		})
	}
}

func TestAssignUserRoleRejectsMalformedPayload(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, postRawJSON(t, srv.URL+"/user-roles", "["), http.StatusBadRequest)
}

// Role yang tidak ada harus jadi 404 -- bukan 500 dari pelanggaran foreign key.
func TestAssignUserRoleUnknownRoleReturns404(t *testing.T) {
	srv := newServer(t)

	resp := postJSON(t, srv.URL+"/user-roles", map[string]any{
		"user_id":    uuid.NewString(),
		"role_id":    uuid.NewString(),
		"company_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestListUserRolesRequiresUserID(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, getJSON(t, srv.URL+"/user-roles"), http.StatusBadRequest)
}

func TestListUserRolesReturnsOnlyThatUsersAssignments(t *testing.T) {
	srv := newServer(t)
	roleA := mustCreateRole(t, srv)
	roleB := mustCreateRole(t, srv)
	user := uuid.NewString()
	otherUser := uuid.NewString()

	for _, roleID := range []string{roleA.ID, roleB.ID} {
		requireStatus(t, postJSON(t, srv.URL+"/user-roles", map[string]any{
			"user_id":    user,
			"role_id":    roleID,
			"company_id": uuid.NewString(),
		}), http.StatusCreated)
	}
	requireStatus(t, postJSON(t, srv.URL+"/user-roles", map[string]any{
		"user_id":    otherUser,
		"role_id":    roleA.ID,
		"company_id": uuid.NewString(),
	}), http.StatusCreated)

	var mine []userRoleFixture
	getJSON(t, srv.URL+"/user-roles?user_id="+user).decode(t, &mine)
	if len(mine) != 2 {
		t.Fatalf("expected 2 assignments for the user, got %d", len(mine))
	}
	for _, ur := range mine {
		if ur.UserID != user {
			t.Fatalf("penugasan milik user lain ikut terbawa: %+v", ur)
		}
	}
}

// Frontend mem-.map() hasil ini langsung; array kosong harus tetap `[]`,
// bukan `null`, supaya tidak perlu penjagaan tambahan di setiap pemanggil.
func TestListUserRolesReturnsEmptyArrayForUnknownUser(t *testing.T) {
	srv := newServer(t)

	resp := getJSON(t, srv.URL+"/user-roles?user_id="+uuid.NewString())
	requireStatus(t, resp, http.StatusOK)
	if got := string(resp.body); got != "[]\n" {
		t.Fatalf("expected an empty JSON array, got %q", got)
	}
}

func TestRevokeUserRoleRemovesTheAssignment(t *testing.T) {
	srv := newServer(t)
	role := mustCreateRole(t, srv)
	user := uuid.NewString()

	resp := postJSON(t, srv.URL+"/user-roles", map[string]any{
		"user_id":    user,
		"role_id":    role.ID,
		"company_id": uuid.NewString(),
	})
	requireStatus(t, resp, http.StatusCreated)
	var assigned userRoleFixture
	resp.decode(t, &assigned)

	requireStatus(t, deleteJSON(t, srv.URL+"/user-roles/"+assigned.ID), http.StatusOK)

	var remaining []userRoleFixture
	getJSON(t, srv.URL+"/user-roles?user_id="+user).decode(t, &remaining)
	if len(remaining) != 0 {
		t.Fatalf("expected no assignment left, got %d", len(remaining))
	}
}

func TestRevokeUserRoleUnknownIDReturns404(t *testing.T) {
	srv := newServer(t)

	requireStatus(t, deleteJSON(t, srv.URL+"/user-roles/"+uuid.NewString()), http.StatusNotFound)
}
