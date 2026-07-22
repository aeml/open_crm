package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
)

func TestRecordEmailPreviewRendersCurrentCustomFieldsAndReportsUnknownTokens(t *testing.T) {
	contacts := &fakeContactsService{getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{
		ID: 8, FirstName: "Ada", Email: "ada@acme.test",
		CustomFields: modulecustomfields.Values{"region": json.RawMessage(`"West"`)},
	}}}
	server := authenticatedRecordEmailServer(Dependencies{
		ContactsService: contacts, CompaniesService: &fakeCompaniesService{}, DealsService: &fakeDealsService{},
		CustomFieldsService: &fakeCustomFieldsService{definitions: []modulecustomfields.Definition{{EntityType: "contact", FieldKey: "region", Label: "Region"}}},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email-preview", bytes.NewBufferString(`{
		"subject":"Hello {{first_name}} in {{contact.custom.region}}",
		"body":"Known {{contact.custom.region}}; unknown {{not_real}}"
	}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response recordEmailPreviewResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if response.Data.To != "ada@acme.test" || response.Data.Subject != "Hello Ada in West" || !strings.Contains(response.Data.Body, "Known West") {
		t.Fatalf("unexpected preview %#v", response.Data)
	}
	if len(response.Data.UnresolvedMergeFields) != 1 || response.Data.UnresolvedMergeFields[0] != "{{not_real}}" {
		t.Fatalf("unexpected unresolved fields %#v", response.Data.UnresolvedMergeFields)
	}
}

func TestRecordEmailTemplateTestUsesOwnMailboxAndNeverCustomerTracking(t *testing.T) {
	contacts := &fakeContactsService{getResult: modulecontacts.Detail{Summary: modulecontacts.Summary{ID: 8, FirstName: "Ada", Email: "ada@acme.test"}}}
	accounts := &fakeUserEmailService{configured: true}
	messages := &fakeEmailMessagesService{}
	suppressions := &fakeEmailSuppressionsService{suppressed: true, token: "must-not-be-used"}
	server := authenticatedRecordEmailServer(Dependencies{
		ContactsService: contacts, UserEmailService: accounts, EmailMessagesService: messages,
		EmailSuppressionsService: suppressions, CustomFieldsService: &fakeCustomFieldsService{},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email-test", bytes.NewBufferString(`{
		"subject":"Hello {{first_name}}","body":"Hi {{first_name}}","trackEngagement":true
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "template-test-handler-key-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("test delivery status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if accounts.sendTo != "owner@acme.test" || accounts.sendSubject != "[TEST] Hello Ada" {
		t.Fatalf("template test recipient/subject = %q / %q", accounts.sendTo, accounts.sendSubject)
	}
	if !strings.Contains(accounts.sendBody, "CRM recipient did not receive it") || !strings.HasSuffix(accounts.sendBody, "Hi Ada") {
		t.Fatalf("template test safety notice/body missing: %q", accounts.sendBody)
	}
	if messages.lastPrepareDelivery.Request.Purpose != "test" || messages.lastPrepareDelivery.ResolvedRecipientUserID != 1 || messages.lastPrepareDelivery.Request.TrackEngagement {
		t.Fatalf("unsafe template test intent %#v", messages.lastPrepareDelivery)
	}
	if suppressions.isCalled || suppressions.tokenCalled || accounts.listUnsubscribeURL != "" || accounts.sendHTMLBody != "" {
		t.Fatalf("template test used customer compliance/tracking effects: suppression=%#v html=%q unsubscribe=%q", suppressions, accounts.sendHTMLBody, accounts.listUnsubscribeURL)
	}
	if messages.lastRecord.EntityType != "" || messages.lastRecord.EntityID != 0 {
		t.Fatalf("template test was linked to the customer record: %#v", messages.lastRecord)
	}
}

func TestRecordEmailPreviewRequiresWriter(t *testing.T) {
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 2, Email: "viewer@acme.test"}, Organization: moduleauth.Organization{ID: 42}, Membership: moduleauth.Membership{Role: "viewer"},
		}},
		ContactsService: &fakeContactsService{}, CompaniesService: &fakeCompaniesService{}, DealsService: &fakeDealsService{}, CustomFieldsService: &fakeCustomFieldsService{},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/contacts/8/email-preview", bytes.NewBufferString(`{"subject":"Hi","body":"Body"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer preview status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
