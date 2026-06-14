package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
)

type fakeEmailSequencesService struct {
	listResult       []moduleemailsequences.Sequence
	createResult     moduleemailsequences.Sequence
	createErr        error
	updateResult     moduleemailsequences.Sequence
	updateErr        error
	deleteErr        error
	lastListOrgID    int64
	lastCreateOrgID  int64
	lastCreateUserID int64
	lastCreateInput  moduleemailsequences.Input
	lastUpdateOrgID  int64
	lastUpdateID     int64
	lastDeleteOrgID  int64
	lastDeleteID     int64
}

func (f *fakeEmailSequencesService) ListByOrganization(_ context.Context, organizationID int64) ([]moduleemailsequences.Sequence, error) {
	f.lastListOrgID = organizationID
	return f.listResult, nil
}

func (f *fakeEmailSequencesService) Create(_ context.Context, organizationID, userID int64, input moduleemailsequences.Input) (moduleemailsequences.Sequence, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateUserID = userID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeEmailSequencesService) Update(_ context.Context, organizationID, sequenceID int64, input moduleemailsequences.Input) (moduleemailsequences.Sequence, error) {
	f.lastUpdateOrgID = organizationID
	f.lastUpdateID = sequenceID
	return f.updateResult, f.updateErr
}

func (f *fakeEmailSequencesService) Delete(_ context.Context, organizationID, sequenceID int64) error {
	f.lastDeleteOrgID = organizationID
	f.lastDeleteID = sequenceID
	return f.deleteErr
}

func authenticatedEmailSequencesServer(service *fakeEmailSequencesService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		EmailSequencesService: service,
	})
}

func TestListEmailSequencesScopesToOrganization(t *testing.T) {
	service := &fakeEmailSequencesService{
		listResult: []moduleemailsequences.Sequence{{ID: 3, Name: "Outbound trial", Status: "draft", Steps: []moduleemailsequences.Step{{ID: 9, StepOrder: 1, Subject: "Intro", Body: "Hello"}}}},
	}
	server := authenticatedEmailSequencesServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-sequences", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 {
		t.Fatalf("expected list scoped to org 42, got %d", service.lastListOrgID)
	}

	var response struct {
		Data struct {
			Sequences []moduleemailsequences.Sequence `json:"sequences"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Sequences) != 1 || response.Data.Sequences[0].Name != "Outbound trial" {
		t.Fatalf("unexpected sequences payload: %#v", response.Data.Sequences)
	}
}

func TestCreateEmailSequenceUsesCurrentOrganizationAndUser(t *testing.T) {
	service := &fakeEmailSequencesService{
		createResult: moduleemailsequences.Sequence{ID: 7, Name: "Trial follow-up", Status: "draft"},
	}
	server := authenticatedEmailSequencesServer(service, "owner")

	body := bytes.NewBufferString(`{"name":"Trial follow-up","description":"New trial leads","status":"draft","steps":[{"delayDays":0,"subject":"Welcome","body":"Hi {{first_name}}"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/email-sequences", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateUserID != 1 || service.lastCreateInput.Name != "Trial follow-up" || len(service.lastCreateInput.Steps) != 1 {
		t.Fatalf("unexpected create routing/input: org=%d user=%d input=%#v", service.lastCreateOrgID, service.lastCreateUserID, service.lastCreateInput)
	}
}

func TestCreateEmailSequenceRejectsViewer(t *testing.T) {
	service := &fakeEmailSequencesService{}
	server := authenticatedEmailSequencesServer(service, "viewer")

	body := bytes.NewBufferString(`{"name":"Trial","status":"draft","steps":[{"delayDays":0,"subject":"Hi","body":"Body"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/email-sequences", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastCreateOrgID != 0 {
		t.Fatalf("viewer should not reach the service")
	}
}

func TestDeleteEmailSequenceScopesToOrganization(t *testing.T) {
	service := &fakeEmailSequencesService{}
	server := authenticatedEmailSequencesServer(service, "admin")

	request := httptest.NewRequest(http.MethodDelete, "/api/email-sequences/9", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastDeleteOrgID != 42 || service.lastDeleteID != 9 {
		t.Fatalf("unexpected delete routing: org=%d id=%d", service.lastDeleteOrgID, service.lastDeleteID)
	}
}
