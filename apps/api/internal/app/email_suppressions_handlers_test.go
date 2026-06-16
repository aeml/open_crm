package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleemailsuppressions "github.com/aeml/open_crm/apps/api/internal/modules/emailsuppressions"
)

type fakeEmailSuppressionsService struct {
	suppressed        bool
	isErr             error
	token             string
	tokenErr          error
	unsubscribeResult moduleemailsuppressions.Suppression
	unsubscribeErr    error
	isCalled          bool
	tokenCalled       bool
	unsubscribeCalled bool
	lastOrgID         int64
	lastEmail         string
	lastToken         string
}

func (f *fakeEmailSuppressionsService) IsSuppressed(_ context.Context, organizationID int64, email string) (bool, error) {
	f.isCalled = true
	f.lastOrgID = organizationID
	f.lastEmail = email
	return f.suppressed, f.isErr
}

func (f *fakeEmailSuppressionsService) UnsubscribeToken(organizationID int64, email string) (string, error) {
	f.tokenCalled = true
	f.lastOrgID = organizationID
	f.lastEmail = email
	return f.token, f.tokenErr
}

func (f *fakeEmailSuppressionsService) UnsubscribeByToken(_ context.Context, token string) (moduleemailsuppressions.Suppression, error) {
	f.unsubscribeCalled = true
	f.lastToken = token
	return f.unsubscribeResult, f.unsubscribeErr
}

func TestEmailUnsubscribeByToken(t *testing.T) {
	suppressions := &fakeEmailSuppressionsService{
		unsubscribeResult: moduleemailsuppressions.Suppression{Email: "lead@example.test"},
	}
	server := NewServer(config.Env{}, Dependencies{EmailSuppressionsService: suppressions})

	request := httptest.NewRequest(http.MethodGet, "/api/email-unsubscribe/signed.token", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if !suppressions.unsubscribeCalled || suppressions.lastToken != "signed.token" {
		t.Fatalf("expected unsubscribe token to be consumed, got %#v", suppressions)
	}
	if !strings.Contains(recorder.Body.String(), "lead@example.test has been unsubscribed") {
		t.Fatalf("expected unsubscribe confirmation, got %q", recorder.Body.String())
	}
}

func TestEmailUnsubscribeRejectsInvalidToken(t *testing.T) {
	suppressions := &fakeEmailSuppressionsService{unsubscribeErr: moduleemailsuppressions.ErrInvalidToken}
	server := NewServer(config.Env{}, Dependencies{EmailSuppressionsService: suppressions})

	request := httptest.NewRequest(http.MethodGet, "/api/email-unsubscribe/bad", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}
