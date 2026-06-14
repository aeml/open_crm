package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
)

type fakeEmailSequenceEnrollmentsService struct {
	listResult             []moduleemailsequences.Enrollment
	enrollResult           moduleemailsequences.Enrollment
	enrollErr              error
	cancelErr              error
	lastListOrgID          int64
	lastListContactID      int64
	lastEnrollOrgID        int64
	lastEnrollInput        moduleemailsequences.EnrollmentInput
	lastCancelOrgID        int64
	lastCancelEnrollmentID int64
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
