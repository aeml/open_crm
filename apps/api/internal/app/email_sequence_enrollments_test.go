package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

type fakeEmailSequenceEnrollmentsService struct {
	listResult             []moduleemailsequences.Enrollment
	sequencePageResult     moduleemailsequences.EnrollmentPage
	sequencePageErr        error
	enrollResult           moduleemailsequences.Enrollment
	enrollErr              error
	cancelErr              error
	lastListOrgID          int64
	lastListContactID      int64
	lastListSequenceID     int64
	lastSequenceQuery      platformtimeline.Query
	lastEnrollOrgID        int64
	lastEnrollInput        moduleemailsequences.EnrollmentInput
	lastCancelOrgID        int64
	lastCancelEnrollmentID int64
}

func (f *fakeEmailSequenceEnrollmentsService) ListEnrollmentsBySequence(_ context.Context, organizationID, sequenceID int64, query platformtimeline.Query) (moduleemailsequences.EnrollmentPage, error) {
	f.lastListOrgID = organizationID
	f.lastListSequenceID = sequenceID
	f.lastSequenceQuery = query
	return f.sequencePageResult, f.sequencePageErr
}

func (f *fakeEmailSequenceEnrollmentsService) ListEnrollmentsByContact(_ context.Context, organizationID, contactID int64) ([]moduleemailsequences.Enrollment, error) {
	f.lastListOrgID = organizationID
	f.lastListContactID = contactID
	return f.listResult, nil
}

func (f *fakeEmailSequenceEnrollmentsService) EnrollContact(_ context.Context, organizationID int64, input moduleemailsequences.EnrollmentInput) (moduleemailsequences.Enrollment, error) {
	f.lastEnrollOrgID = organizationID
	f.lastEnrollInput = input
	return f.enrollResult, f.enrollErr
}

func (f *fakeEmailSequenceEnrollmentsService) CancelEnrollment(_ context.Context, organizationID, enrollmentID int64) error {
	f.lastCancelOrgID = organizationID
	f.lastCancelEnrollmentID = enrollmentID
	return f.cancelErr
}

func authenticatedEmailSequenceEnrollmentsServer(service *fakeEmailSequenceEnrollmentsService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		EmailSequenceEnrollmentsService: service,
	})
}

func TestListEmailSequenceEnrollmentsScopesToContact(t *testing.T) {
	nextSendAt := time.Now().UTC()
	service := &fakeEmailSequenceEnrollmentsService{
		listResult: []moduleemailsequences.Enrollment{{ID: 3, SequenceID: 4, SequenceName: "Trial nurture", ContactID: 7, Status: "active", CurrentStepOrder: 1, NextSendAt: &nextSendAt}},
	}
	server := authenticatedEmailSequenceEnrollmentsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-sequence-enrollments?contactId=7", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 || service.lastListContactID != 7 {
		t.Fatalf("unexpected list routing: org=%d contact=%d", service.lastListOrgID, service.lastListContactID)
	}
}

func TestListEmailSequenceEnrollmentHistoryUsesBoundedSequenceCursor(t *testing.T) {
	service := &fakeEmailSequenceEnrollmentsService{
		sequencePageResult: moduleemailsequences.EnrollmentPage{
			Enrollments: []moduleemailsequences.Enrollment{{ID: 9, SequenceID: 4, ContactID: 7, ContactName: "Pilot Buyer", Status: "completed"}},
			Meta:        platformtimeline.Meta{Limit: 25, HasMore: true, NextCursor: "next-page"},
		},
	}
	server := authenticatedEmailSequenceEnrollmentsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/email-sequence-enrollments?sequenceId=4&limit=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.lastListOrgID != 42 || service.lastListSequenceID != 4 || service.lastSequenceQuery.Limit != 25 || service.lastSequenceQuery.Cursor != nil {
		t.Fatalf("unexpected sequence history routing: org=%d sequence=%d query=%+v", service.lastListOrgID, service.lastListSequenceID, service.lastSequenceQuery)
	}
	var response struct {
		Data struct {
			Enrollments []moduleemailsequences.Enrollment `json:"enrollments"`
			Meta        platformtimeline.Meta             `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode sequence enrollment history response: %v", err)
	}
	if len(response.Data.Enrollments) != 1 || response.Data.Meta.Limit != 25 || !response.Data.Meta.HasMore || response.Data.Meta.NextCursor != "next-page" {
		t.Fatalf("unexpected sequence enrollment history response: %+v", response.Data)
	}
}

func TestListEmailSequenceEnrollmentsRejectsAmbiguousAndInvalidSelectors(t *testing.T) {
	tests := []string{
		"/api/email-sequence-enrollments",
		"/api/email-sequence-enrollments?contactId=7&sequenceId=4",
		"/api/email-sequence-enrollments?contactId=7&limit=25",
		"/api/email-sequence-enrollments?contactId=invalid",
		"/api/email-sequence-enrollments?sequenceId=invalid",
		"/api/email-sequence-enrollments?sequenceId=4&limit=101",
		"/api/email-sequence-enrollments?sequenceId=4&cursor=not-a-cursor",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			service := &fakeEmailSequenceEnrollmentsService{}
			server := authenticatedEmailSequenceEnrollmentsServer(service, "member")
			request := httptest.NewRequest(http.MethodGet, target, nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
			}
			if service.lastListContactID != 0 || service.lastListSequenceID != 0 {
				t.Fatalf("invalid selector reached service: %#v", service)
			}
		})
	}
}

func TestCreateEmailSequenceEnrollmentUsesCurrentOrganizationAndUser(t *testing.T) {
	service := &fakeEmailSequenceEnrollmentsService{
		enrollResult: moduleemailsequences.Enrollment{ID: 5, SequenceID: 4, SequenceName: "Trial nurture", ContactID: 7, Status: "active", CurrentStepOrder: 1},
	}
	server := authenticatedEmailSequenceEnrollmentsServer(service, "owner")

	body := bytes.NewBufferString(`{"sequenceId":4,"contactId":7}`)
	request := httptest.NewRequest(http.MethodPost, "/api/email-sequence-enrollments", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastEnrollOrgID != 42 || service.lastEnrollInput.SequenceID != 4 || service.lastEnrollInput.ContactID != 7 || service.lastEnrollInput.EnrolledByUserID != 1 {
		t.Fatalf("unexpected enroll routing/input: org=%d input=%#v", service.lastEnrollOrgID, service.lastEnrollInput)
	}
}

func TestCreateEmailSequenceEnrollmentRejectsViewer(t *testing.T) {
	service := &fakeEmailSequenceEnrollmentsService{}
	server := authenticatedEmailSequenceEnrollmentsServer(service, "viewer")

	body := bytes.NewBufferString(`{"sequenceId":4,"contactId":7}`)
	request := httptest.NewRequest(http.MethodPost, "/api/email-sequence-enrollments", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if service.lastEnrollOrgID != 0 {
		t.Fatalf("viewer should not reach enrollment service")
	}
}

func TestCreateEmailSequenceEnrollmentMapsDuplicateConflict(t *testing.T) {
	service := &fakeEmailSequenceEnrollmentsService{enrollErr: moduleemailsequences.ErrAlreadyEnrolled}
	server := authenticatedEmailSequenceEnrollmentsServer(service, "admin")

	body := bytes.NewBufferString(`{"sequenceId":4,"contactId":7}`)
	request := httptest.NewRequest(http.MethodPost, "/api/email-sequence-enrollments", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, recorder.Code)
	}
}

func TestCancelEmailSequenceEnrollmentScopesToOrganization(t *testing.T) {
	service := &fakeEmailSequenceEnrollmentsService{}
	server := authenticatedEmailSequenceEnrollmentsServer(service, "admin")

	request := httptest.NewRequest(http.MethodDelete, "/api/email-sequence-enrollments/9", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if service.lastCancelOrgID != 42 || service.lastCancelEnrollmentID != 9 {
		t.Fatalf("unexpected cancel routing: org=%d id=%d", service.lastCancelOrgID, service.lastCancelEnrollmentID)
	}
}
