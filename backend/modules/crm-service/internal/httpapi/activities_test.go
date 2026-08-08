package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreateActivity_ValidationErrors(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	l := mustSeedLead(t, srv, companyID)

	cases := map[string]map[string]any{
		"missing company_id":     {"reference_type": "LEAD", "reference_id": l.ID, "activity_type": "CALL", "subject": "Follow up"},
		"missing reference_id":   {"company_id": companyID, "reference_type": "LEAD", "activity_type": "CALL", "subject": "Follow up"},
		"missing subject":        {"company_id": companyID, "reference_type": "LEAD", "reference_id": l.ID, "activity_type": "CALL"},
		"invalid reference_type": {"company_id": companyID, "reference_type": "PLANET", "reference_id": l.ID, "activity_type": "CALL", "subject": "Follow up"},
		"invalid activity_type":  {"company_id": companyID, "reference_type": "LEAD", "reference_id": l.ID, "activity_type": "CARRIER_PIGEON", "subject": "Follow up"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/activities", payload)
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}
}

// TestCreateActivity_ReferenceNotFound covers all 4 polymorphic reference
// types -- each has its own lookup branch in referenceExists (see
// activities.go), so each must be exercised individually.
func TestCreateActivity_ReferenceNotFound(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)

	for _, refType := range []string{"LEAD", "ACCOUNT", "CONTACT", "OPPORTUNITY"} {
		t.Run(refType, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/activities", map[string]any{
				"company_id": companyID, "reference_type": refType, "reference_id": uuid.NewString(),
				"activity_type": "NOTE", "subject": "Test",
			})
			requireStatus(t, resp, http.StatusNotFound)
		})
	}
}

// TestCreateActivity_ReferenceFromOtherCompanyRejected confirms the
// reference lookup is scoped by company_id, not just id.
func TestCreateActivity_ReferenceFromOtherCompanyRejected(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)
	leadB := mustSeedLead(t, srv, companyB)

	resp := postJSON(t, srv.URL+"/activities", map[string]any{
		"company_id": companyA, "reference_type": "LEAD", "reference_id": leadB.ID,
		"activity_type": "NOTE", "subject": "Test",
	})
	requireStatus(t, resp, http.StatusNotFound)
}

func TestCreateActivity_Success(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	l := mustSeedLead(t, srv, companyID)

	resp := postJSON(t, srv.URL+"/activities", map[string]any{
		"company_id": companyID, "reference_type": "LEAD", "reference_id": l.ID,
		"activity_type": "CALL", "subject": "Follow up telepon", "description": "Diskusi kebutuhan awal",
	})
	requireStatus(t, resp, http.StatusCreated)

	var a struct {
		CompanyID     string `json:"company_id"`
		ReferenceType string `json:"reference_type"`
		ReferenceID   string `json:"reference_id"`
		ActivityType  string `json:"activity_type"`
		IsDone        bool   `json:"is_done"`
	}
	resp.decode(t, &a)
	if a.CompanyID != companyID {
		t.Errorf("company_id = %q, want %q", a.CompanyID, companyID)
	}
	if a.ReferenceType != "LEAD" || a.ReferenceID != l.ID {
		t.Errorf("reference = %s/%s, want LEAD/%s", a.ReferenceType, a.ReferenceID, l.ID)
	}
	if a.IsDone {
		t.Error("is_done = true, want default false")
	}
}

func TestUpdateActivity_NotFound(t *testing.T) {
	srv := newServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/activities/"+uuid.NewString(), map[string]any{
		"subject": "Updated",
	}, "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestUpdateActivity_MarkDone(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	l := mustSeedLead(t, srv, companyID)
	created := postJSON(t, srv.URL+"/activities", map[string]any{
		"company_id": companyID, "reference_type": "LEAD", "reference_id": l.ID,
		"activity_type": "TASK", "subject": "Kirim proposal",
	})
	requireStatus(t, created, http.StatusCreated)
	var a struct {
		ID string `json:"id"`
	}
	created.decode(t, &a)

	resp := doRequest(t, http.MethodPut, srv.URL+"/activities/"+a.ID, map[string]any{
		"subject": "Kirim proposal", "is_done": true,
	}, "")
	requireStatus(t, resp, http.StatusOK)
	var updated struct {
		IsDone bool `json:"is_done"`
	}
	resp.decode(t, &updated)
	if !updated.IsDone {
		t.Error("is_done = false after marking done, want true")
	}
}

func TestListActivities_ScopedByCompany(t *testing.T) {
	srv := newServer(t)
	companyA := newCompanyID(t)
	companyB := newCompanyID(t)
	leadA := mustSeedLead(t, srv, companyA)
	leadB := mustSeedLead(t, srv, companyB)

	requireStatus(t, postJSON(t, srv.URL+"/activities", map[string]any{
		"company_id": companyA, "reference_type": "LEAD", "reference_id": leadA.ID, "activity_type": "NOTE", "subject": "A",
	}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/activities", map[string]any{
		"company_id": companyB, "reference_type": "LEAD", "reference_id": leadB.ID, "activity_type": "NOTE", "subject": "B",
	}), http.StatusCreated)

	resp := getJSON(t, srv.URL+"/activities?company_id="+companyA)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		CompanyID string `json:"company_id"`
	}
	resp.decode(t, &list)
	if len(list) != 1 || list[0].CompanyID != companyA {
		t.Fatalf("expected exactly 1 activity scoped to companyA, got %+v", list)
	}
}

// TestListActivities_TimelineByReference confirms the reference_type +
// reference_id filter combination returns exactly the timeline for one
// entity, not the whole company's activity feed.
func TestListActivities_TimelineByReference(t *testing.T) {
	srv := newServer(t)
	companyID := newCompanyID(t)
	leadA := mustSeedLead(t, srv, companyID)
	leadB := mustSeedLead(t, srv, companyID)

	requireStatus(t, postJSON(t, srv.URL+"/activities", map[string]any{
		"company_id": companyID, "reference_type": "LEAD", "reference_id": leadA.ID, "activity_type": "NOTE", "subject": "A1",
	}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/activities", map[string]any{
		"company_id": companyID, "reference_type": "LEAD", "reference_id": leadA.ID, "activity_type": "CALL", "subject": "A2",
	}), http.StatusCreated)
	requireStatus(t, postJSON(t, srv.URL+"/activities", map[string]any{
		"company_id": companyID, "reference_type": "LEAD", "reference_id": leadB.ID, "activity_type": "NOTE", "subject": "B1",
	}), http.StatusCreated)

	resp := getJSON(t, srv.URL+"/activities?company_id="+companyID+"&reference_type=LEAD&reference_id="+leadA.ID)
	requireStatus(t, resp, http.StatusOK)
	var list []struct {
		ReferenceID string `json:"reference_id"`
	}
	resp.decode(t, &list)
	if len(list) != 2 {
		t.Fatalf("expected exactly 2 activities for leadA's timeline, got %d: %+v", len(list), list)
	}
	for _, a := range list {
		if a.ReferenceID != leadA.ID {
			t.Errorf("leadB activity leaked into leadA's timeline: %+v", list)
		}
	}
}

func TestListActivities_MissingCompanyID(t *testing.T) {
	srv := newServer(t)
	resp := getJSON(t, srv.URL+"/activities")
	requireStatus(t, resp, http.StatusBadRequest)
}
