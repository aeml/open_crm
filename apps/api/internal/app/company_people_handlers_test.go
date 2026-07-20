package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
)

func companyPeopleServer(role string, contacts *fakeContactsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 7, Email: "owner@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		ContactsService: contacts,
	})
}

func TestCreateLinkedCompanyPersonIsTenantScopedAndReturnsCreatedLink(t *testing.T) {
	service := &fakeContactsService{linkedResult: modulecontacts.LinkedCompanyPersonResult{
		Contact:  modulecontacts.Summary{ID: 9, FirstName: "Riley", LastName: "Chen", Email: "riley@atlas.test", JobTitle: "Procurement Lead"},
		Link:     modulecontacts.CompanyLink{RelationshipTitle: "Procurement Lead", IsPrimary: false},
		Activity: modulecontacts.CompanyActivity{ID: 33, Action: "company.contact_linked", Summary: "Contact linked: Riley Chen"},
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/companies/6/linked-contacts", bytes.NewBufferString(`{
		"firstName":" Riley ","lastName":" Chen ","email":"riley@atlas.test",
		"jobTitle":"Procurement Lead","status":"prospect","isClient":true,
		"customFields":{"region":"West"}
	}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	companyPeopleServer("owner", service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, recorder.Code, recorder.Body.String())
	}
	if service.lastLinkedOrgID != 42 || service.lastLinkedCompanyID != 6 || service.lastLinkedActorID != 7 {
		t.Fatalf("unexpected service scope: org=%d company=%d actor=%d", service.lastLinkedOrgID, service.lastLinkedCompanyID, service.lastLinkedActorID)
	}
	if service.lastLinkedInput.FirstName != "Riley" || service.lastLinkedInput.LastName != "Chen" || service.lastLinkedInput.IsClient {
		t.Fatalf("unexpected linked-person input: %#v", service.lastLinkedInput)
	}
	var response linkedCompanyPersonResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.Contact.ID != 9 || response.Data.Link.RelationshipTitle != "Procurement Lead" || response.Data.Activity.ID != 33 {
		t.Fatalf("unexpected response: %#v", response.Data)
	}
}

func TestCreateLinkedCompanyPersonRejectsViewerBeforeService(t *testing.T) {
	service := &fakeContactsService{}
	request := httptest.NewRequest(http.MethodPost, "/api/companies/6/linked-contacts", bytes.NewBufferString(`{"firstName":"Riley","lastName":"Chen"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	companyPeopleServer("viewer", service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden || service.lastLinkedOrgID != 0 {
		t.Fatalf("expected viewer rejection before service, got status=%d org=%d", recorder.Code, service.lastLinkedOrgID)
	}
}

func TestCreateLinkedCompanyPersonMapsSafeDomainErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantText   string
	}{
		{name: "cross tenant company", err: modulecontacts.ErrLinkedCompanyNotFound, wantStatus: http.StatusNotFound, wantText: "NOT_FOUND"},
		{name: "individual company", err: modulecontacts.ErrIndividualCompany, wantStatus: http.StatusConflict, wantText: "additional linked people"},
		{name: "duplicate contact", err: &modulecontacts.DuplicateError{ID: 11, Label: "Riley Chen", Reason: "email"}, wantStatus: http.StatusConflict, wantText: `"entityType":"contact"`},
		{name: "capacity limit", err: modulebilling.ErrLimitReached, wantStatus: http.StatusPaymentRequired, wantText: "PLAN_LIMIT_REACHED"},
		{name: "internal", err: errors.New("database password"), wantStatus: http.StatusInternalServerError, wantText: "Unable to add linked person"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeContactsService{linkedErr: test.err}
			request := httptest.NewRequest(http.MethodPost, "/api/companies/6/linked-contacts", bytes.NewBufferString(`{"firstName":"Riley","lastName":"Chen"}`))
			request.Header.Set("Content-Type", "application/json")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			companyPeopleServer("member", service).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus || !bytes.Contains(recorder.Body.Bytes(), []byte(test.wantText)) {
				t.Fatalf("expected status=%d body containing %q, got status=%d body=%s", test.wantStatus, test.wantText, recorder.Code, recorder.Body.String())
			}
		})
	}
}
