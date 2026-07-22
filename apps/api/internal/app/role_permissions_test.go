package app

// Role permissions tests.
//
// Verifies the three-tier role gate applied to every CRM endpoint:
//
//	requireOrgMember  — viewer, member, admin, owner (read endpoints)
//	requireOrgWriter  — member, admin, owner         (CRM mutation endpoints)
//	requireOrgAdmin   — admin, owner                 (admin-only endpoints)
//
// Each test creates a minimal server with a specific role and fires one
// request. No service logic runs for the gate-rejected cases.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
)

// serverWithRole returns a server that authenticates every request as the
// given role and wires up the supplied service dependencies.
func serverWithRole(role string, deps Dependencies) http.Handler {
	deps.AuthService = &fakeAuthService{
		currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 5, Email: "user@acme.test", FirstName: "Test", LastName: "User"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
			Membership:   moduleauth.Membership{Role: role},
		},
	}
	return NewServer(config.Env{}, deps)
}

// ── Viewer: read access allowed ───────────────────────────────────────────────

func TestViewerCanReadContacts(t *testing.T) {
	server := serverWithRole("viewer", Dependencies{
		ContactsService: &fakeContactsService{},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/contacts", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for viewer reading contacts, got %d", recorder.Code)
	}
}

func TestViewerCanReadDeals(t *testing.T) {
	server := serverWithRole("viewer", Dependencies{
		DealsService: &fakeDealsService{},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/deals", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for viewer reading deals, got %d", recorder.Code)
	}
}

func TestViewerCanReadCompanyLinkedContacts(t *testing.T) {
	service := &fakeCompaniesService{}
	server := serverWithRole("viewer", Dependencies{CompaniesService: service})
	request := httptest.NewRequest(http.MethodGet, "/api/companies/6/linked-contacts", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || service.lastLinkedListOrgID != 42 {
		t.Fatalf("expected viewer-scoped linked-contact read, got status=%d org=%d", recorder.Code, service.lastLinkedListOrgID)
	}
}

// ── Viewer: write access blocked ─────────────────────────────────────────────

func TestViewerCannotCreateContact(t *testing.T) {
	server := serverWithRole("viewer", Dependencies{
		ContactsService: &fakeContactsService{},
	})

	body := bytes.NewBufferString(`{"firstName":"Test","lastName":"Viewer"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer creating contact, got %d", recorder.Code)
	}
}

func TestViewerCannotCreateDeal(t *testing.T) {
	server := serverWithRole("viewer", Dependencies{
		DealsService: &fakeDealsService{},
	})

	body := bytes.NewBufferString(`{"name":"Ghost Deal","stageId":1,"status":"open"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/deals", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer creating deal, got %d", recorder.Code)
	}
}

func TestViewerCannotMutateCompanyLinkedContacts(t *testing.T) {
	for _, test := range []struct {
		method string
		body   string
	}{
		{method: http.MethodPut, body: `{}`},
		{method: http.MethodDelete},
	} {
		service := &fakeCompaniesService{}
		server := serverWithRole("viewer", Dependencies{CompaniesService: service})
		request := httptest.NewRequest(test.method, "/api/companies/6/linked-contacts/9", bytes.NewBufferString(test.body))
		request.Header.Set("Content-Type", "application/json")
		addSessionCookie(request)
		recorder := httptest.NewRecorder()

		server.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusForbidden || service.lastLinkOrgID != 0 || service.lastUnlinkOrgID != 0 {
			t.Fatalf("%s: expected viewer rejection before service, got status=%d link_org=%d unlink_org=%d", test.method, recorder.Code, service.lastLinkOrgID, service.lastUnlinkOrgID)
		}
	}
}

func TestViewerCannotFinalizeDealQuote(t *testing.T) {
	server := serverWithRole("viewer", Dependencies{DealsService: &fakeDealsService{}})
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes", bytes.NewBufferString(`{"recipientName":"Ava","recipientEmail":"ava@example.test","validUntil":"2026-08-20","terms":"Net 30"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "viewer-quote-key-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer finalizing quote, got %d", recorder.Code)
	}
}

func TestViewerCannotDeliverDealQuote(t *testing.T) {
	server := serverWithRole("viewer", Dependencies{DealsService: &fakeDealsService{}, UserEmailService: &fakeUserEmailService{}})
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/deliveries", bytes.NewBufferString(`{"subject":"Quote","messageBody":"Review"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "viewer-quote-delivery-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer delivering quote, got %d", recorder.Code)
	}
}

func TestViewerCannotReissueExpiredDealQuote(t *testing.T) {
	server := serverWithRole("viewer", Dependencies{DealsService: &fakeDealsService{}})
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/reissue", bytes.NewBufferString(`{"validUntil":"2026-09-20"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "viewer-quote-reissue-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer reissuing quote, got %d", recorder.Code)
	}
}

func TestViewerCannotCreateTask(t *testing.T) {
	server := serverWithRole("viewer", Dependencies{
		TasksService: &fakeTasksService{},
	})

	body := bytes.NewBufferString(`{"title":"Ghost Task","entityType":"deal","entityId":1}`)
	request := httptest.NewRequest(http.MethodPost, "/api/tasks", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer creating task, got %d", recorder.Code)
	}
}

func TestViewerCannotCreateNote(t *testing.T) {
	server := serverWithRole("viewer", Dependencies{
		NotesService: &fakeNotesService{},
	})

	body := bytes.NewBufferString(`{"entityType":"deal","entityId":1,"body":"Ghost note"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/notes", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer creating note, got %d", recorder.Code)
	}
}

func TestViewerCannotCreateSavedView(t *testing.T) {
	server := serverWithRole("viewer", Dependencies{
		SavedViewsService: &fakeSavedViewsService{},
	})

	body := bytes.NewBufferString(`{"entityType":"contacts","name":"My view","filters":{},"isDefault":false}`)
	request := httptest.NewRequest(http.MethodPost, "/api/saved-views", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer creating saved view, got %d", recorder.Code)
	}
}

// ── Member: write access allowed ─────────────────────────────────────────────

func TestMemberCanCreateContact(t *testing.T) {
	service := &fakeContactsService{
		createResult: modulecontacts.Detail{
			Summary: modulecontacts.Summary{ID: 99, FirstName: "Test", LastName: "Member"},
		},
	}
	server := serverWithRole("member", Dependencies{
		ContactsService: service,
	})

	body := bytes.NewBufferString(`{"firstName":"Test","lastName":"Member"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/contacts", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 for member creating contact, got %d", recorder.Code)
	}
}

// ── Member: admin endpoints blocked ──────────────────────────────────────────

func TestMemberCannotListAuditEvents(t *testing.T) {
	server := serverWithRole("member", Dependencies{
		AuditService: &fakeAuditService{},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/audit-events", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member accessing audit events, got %d", recorder.Code)
	}
}

func TestMemberCannotUpdateOrganizationProfile(t *testing.T) {
	server := serverWithRole("member", Dependencies{
		OrgProfileService: &fakeOrgProfileService{},
	})

	body := bytes.NewBufferString(`{"businessType":"services"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/organization/profile", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member updating organization profile, got %d", recorder.Code)
	}
}
