package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
)

func authenticatedRecordEmailServer(dependencies Dependencies) http.Handler {
	dependencies.AuthService = &fakeAuthService{
		currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
			Membership:   moduleauth.Membership{Role: "owner"},
		},
	}
	return NewServer(config.Env{}, dependencies)
}

func TestSendCompanyEmailUsesSelectedLinkedContactAndRecordsCompany(t *testing.T) {
	companies := &fakeCompaniesService{
		getResult: modulecompanies.Detail{
			Summary: modulecompanies.Summary{ID: 5, Name: "Northstar Logistics", ClientType: "organization", Status: "prospect", Industry: "Logistics"},
			LinkedContacts: []modulecompanies.LinkedContact{
				{ID: 7, FirstName: "Morgan", LastName: "Lee", Email: "morgan@northstar.test", IsPrimary: true},
			},
		},
	}
	accounts := &fakeUserEmailService{configured: true}
	messages := &fakeEmailMessagesService{}
	server := authenticatedRecordEmailServer(Dependencies{CompaniesService: companies, UserEmailService: accounts, EmailMessagesService: messages})

	body := bytes.NewBufferString(`{"contactId":7,"subject":"Hello {{first_name}} from {{company_name}}","body":"Status: {{client_status}}"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/companies/5/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if accounts.sendTo != "morgan@northstar.test" {
		t.Fatalf("unexpected recipient: %q", accounts.sendTo)
	}
	if accounts.sendSubject != "Hello Morgan from Northstar Logistics" || accounts.sendBody != "Status: prospect" {
		t.Fatalf("merge fields were not rendered: subject=%q body=%q", accounts.sendSubject, accounts.sendBody)
	}
	if messages.lastRecord.EntityType != "company" || messages.lastRecord.EntityID != 5 || messages.lastRecord.Status != "sent" {
		t.Fatalf("email was not recorded against the company: %#v", messages.lastRecord)
	}
}

func TestSendDealEmailUsesPrimaryContactAndRecordsDeal(t *testing.T) {
	deals := &fakeDealsService{
		getResult: moduledeals.Detail{
			Summary: moduledeals.Summary{ID: 11, Name: "Northstar Expansion", StageName: "Proposal", Status: "open", CompanyName: "Northstar Logistics", PrimaryContactID: 7, PrimaryContactName: "Morgan Lee"},
		},
	}
	contacts := &fakeContactsService{
		getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 7, FirstName: "Morgan", LastName: "Lee", Email: "morgan@northstar.test"}},
	}
	accounts := &fakeUserEmailService{configured: true}
	messages := &fakeEmailMessagesService{}
	server := authenticatedRecordEmailServer(Dependencies{DealsService: deals, ContactsService: contacts, UserEmailService: accounts, EmailMessagesService: messages})

	body := bytes.NewBufferString(`{"contactId":7,"subject":"{{deal_name}} for {{first_name}}","body":"Stage: {{deal_stage}}"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/deals/11/email", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if contacts.lastDetailID != 7 || accounts.sendTo != "morgan@northstar.test" {
		t.Fatalf("deal email did not use the primary contact: contactID=%d recipient=%q", contacts.lastDetailID, accounts.sendTo)
	}
	if accounts.sendSubject != "Northstar Expansion for Morgan" || accounts.sendBody != "Stage: Proposal" {
		t.Fatalf("merge fields were not rendered: subject=%q body=%q", accounts.sendSubject, accounts.sendBody)
	}
	if messages.lastRecord.EntityType != "deal" || messages.lastRecord.EntityID != 11 || messages.lastRecord.Status != "sent" {
		t.Fatalf("email was not recorded against the deal: %#v", messages.lastRecord)
	}
}
