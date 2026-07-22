package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modulequotetemplates "github.com/aeml/open_crm/apps/api/internal/modules/quotetemplates"
)

type fakeQuoteTemplatesService struct {
	listResult      modulequotetemplates.ListPage
	listErr         error
	policyResult    modulequotetemplates.Policy
	policyErr       error
	templateResult  modulequotetemplates.Template
	saveErr         error
	lastOperation   string
	lastOrgID       int64
	lastTemplateID  int64
	lastActorID     int64
	lastRevision    int
	lastInput       modulequotetemplates.Input
	lastPolicyValue bool
	lastListQuery   modulequotetemplates.ListQuery
}

func (f *fakeQuoteTemplatesService) ListByOrganization(_ context.Context, organizationID int64, query modulequotetemplates.ListQuery) (modulequotetemplates.ListPage, error) {
	f.lastOperation, f.lastOrgID, f.lastListQuery = "list", organizationID, query
	return f.listResult, f.listErr
}

func (f *fakeQuoteTemplatesService) GetPolicy(_ context.Context, organizationID int64) (modulequotetemplates.Policy, error) {
	f.lastOperation, f.lastOrgID = "get_policy", organizationID
	return f.policyResult, f.policyErr
}

func (f *fakeQuoteTemplatesService) Create(_ context.Context, organizationID, actorUserID int64, input modulequotetemplates.Input) (modulequotetemplates.Template, error) {
	f.lastOperation, f.lastOrgID, f.lastActorID, f.lastInput = "create", organizationID, actorUserID, input
	return f.templateResult, f.saveErr
}

func (f *fakeQuoteTemplatesService) Update(_ context.Context, organizationID, templateID, actorUserID int64, input modulequotetemplates.Input) (modulequotetemplates.Template, error) {
	f.lastOperation, f.lastOrgID, f.lastTemplateID, f.lastActorID, f.lastInput = "update", organizationID, templateID, actorUserID, input
	return f.templateResult, f.saveErr
}

func (f *fakeQuoteTemplatesService) Archive(_ context.Context, organizationID, templateID, actorUserID int64, revision int) (modulequotetemplates.Template, error) {
	f.lastOperation, f.lastOrgID, f.lastTemplateID, f.lastActorID, f.lastRevision = "archive", organizationID, templateID, actorUserID, revision
	return f.templateResult, f.saveErr
}

func (f *fakeQuoteTemplatesService) UpdatePolicy(_ context.Context, organizationID, actorUserID int64, approvalRequired bool) (modulequotetemplates.Policy, error) {
	f.lastOperation, f.lastOrgID, f.lastActorID, f.lastPolicyValue = "update_policy", organizationID, actorUserID, approvalRequired
	return f.policyResult, f.policyErr
}

func TestQuoteTemplateReadEndpointsUseSessionTenant(t *testing.T) {
	service := &fakeQuoteTemplatesService{
		listResult:   modulequotetemplates.ListPage{Templates: []modulequotetemplates.Template{{ID: 9, Name: "Standard", Revision: 2}}, Page: 2, PageSize: 25, Total: 27},
		policyResult: modulequotetemplates.Policy{ApprovalRequired: true, ActiveApprovers: 2},
	}
	server := serverWithRole("viewer", Dependencies{QuoteTemplatesService: service})

	for _, path := range []string{"/api/quote-templates?q=standard&status=active&page=2&pageSize=25", "/api/quote-templates/policy", "/api/quote-templates/merge-tokens"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d: %s", path, recorder.Code, recorder.Body.String())
		}
	}
	body := serviceBody(server, "/api/quote-templates?q=standard&status=active&page=2&pageSize=25")
	if service.lastOrgID != 42 || service.lastListQuery.Search != "standard" || service.lastListQuery.Status != "active" ||
		service.lastListQuery.Page != 2 || service.lastListQuery.PageSize != 25 || !strings.Contains(body, `"name":"Standard"`) ||
		!strings.Contains(body, `"meta":{"page":2,"pageSize":25,"total":27}`) {
		t.Fatalf("quote template read did not retain the session tenant: %#v", service)
	}
}

func TestQuoteTemplateListRejectsInvalidBoundsBeforeService(t *testing.T) {
	for _, path := range []string{
		"/api/quote-templates?pageSize=101",
		"/api/quote-templates?page=502&pageSize=100",
		"/api/quote-templates?status=unknown",
		"/api/quote-templates?q=" + strings.Repeat("x", modulequotetemplates.MaxListSearchLength+1),
	} {
		service := &fakeQuoteTemplatesService{}
		server := serverWithRole("viewer", Dependencies{QuoteTemplatesService: service})
		request := httptest.NewRequest(http.MethodGet, path, nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || service.lastOperation != "" {
			t.Fatalf("invalid quote template query reached service: path=%s status=%d service=%#v body=%s", path, recorder.Code, service, recorder.Body.String())
		}
	}
}

func TestQuoteTemplateCreateRequiresAdminAndForwardsRevisionedDefinition(t *testing.T) {
	body := `{"name":"Standard","terms":"Net 30","defaultValidityDays":30,"deliverySubjectTemplate":"Quote {{quote_number}}","deliveryMessageTemplate":"Hi {{recipient_name}}","requestSignature":true,"requiresApproval":true}`
	memberServer := serverWithRole("member", Dependencies{QuoteTemplatesService: &fakeQuoteTemplatesService{}})
	memberRequest := httptest.NewRequest(http.MethodPost, "/api/quote-templates", bytes.NewBufferString(body))
	memberRequest.Header.Set("Content-Type", "application/json")
	addSessionCookie(memberRequest)
	memberRecorder := httptest.NewRecorder()
	memberServer.ServeHTTP(memberRecorder, memberRequest)
	if memberRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected member quote template create to be forbidden, got %d", memberRecorder.Code)
	}

	service := &fakeQuoteTemplatesService{templateResult: modulequotetemplates.Template{ID: 11, Name: "Standard", Revision: 1}}
	adminServer := serverWithRole("admin", Dependencies{QuoteTemplatesService: service})
	request := httptest.NewRequest(http.MethodPost, "/api/quote-templates", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	adminServer.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || service.lastOperation != "create" || service.lastOrgID != 42 || service.lastActorID != 5 ||
		service.lastInput.Name != "Standard" || !service.lastInput.RequestSignature || !service.lastInput.RequiresApproval {
		t.Fatalf("unexpected quote template create: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestQuoteTemplateUpdateArchiveAndPolicyExposeStableConflicts(t *testing.T) {
	service := &fakeQuoteTemplatesService{templateResult: modulequotetemplates.Template{ID: 7, Revision: 4}}
	server := serverWithRole("owner", Dependencies{QuoteTemplatesService: service})

	request := httptest.NewRequest(http.MethodPatch, "/api/quote-templates/7", bytes.NewBufferString(`{"name":"Revised","terms":"Due","defaultValidityDays":14,"deliverySubjectTemplate":"{{quote_number}}","deliveryMessageTemplate":"{{deal_name}}","expectedRevision":3}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOperation != "update" || service.lastTemplateID != 7 || service.lastInput.ExpectedRevision != 3 {
		t.Fatalf("unexpected template update: status=%d service=%#v", recorder.Code, service)
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/quote-templates/7?revision=4", nil)
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOperation != "archive" || service.lastRevision != 4 {
		t.Fatalf("unexpected template archive: status=%d service=%#v", recorder.Code, service)
	}

	service.policyErr = modulequotetemplates.ErrInsufficientApprovers
	request = httptest.NewRequest(http.MethodPut, "/api/quote-templates/policy", bytes.NewBufferString(`{"approvalRequired":true}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), `"code":"QUOTE_APPROVER_REQUIRED"`) {
		t.Fatalf("expected independent-approver error, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestQuoteTemplatePolicyRequiresExplicitBoolean(t *testing.T) {
	service := &fakeQuoteTemplatesService{policyResult: modulequotetemplates.Policy{ApprovalRequired: false, ActiveApprovers: 2}}
	server := serverWithRole("owner", Dependencies{QuoteTemplatesService: service})

	request := httptest.NewRequest(http.MethodPut, "/api/quote-templates/policy", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || service.lastOperation != "" {
		t.Fatalf("empty policy body changed policy: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/quote-templates/policy", bytes.NewBufferString(`{"approvalRequired":false}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOperation != "update_policy" || service.lastPolicyValue {
		t.Fatalf("explicit false policy was not retained: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestQuoteTemplateErrorsRemainActionable(t *testing.T) {
	tests := []struct {
		err  error
		code int
		body string
	}{
		{modulequotetemplates.ErrInvalidInput, http.StatusBadRequest, `"code":"BAD_REQUEST"`},
		{modulequotetemplates.ErrDuplicateName, http.StatusConflict, `"code":"QUOTE_TEMPLATE_NAME_CONFLICT"`},
		{modulequotetemplates.ErrConflict, http.StatusConflict, `"code":"QUOTE_TEMPLATE_CHANGED"`},
		{modulequotetemplates.ErrActiveLimit, http.StatusUnprocessableEntity, `"code":"QUOTE_TEMPLATE_ACTIVE_LIMIT"`},
		{modulequotetemplates.ErrNotFound, http.StatusNotFound, `"code":"NOT_FOUND"`},
		{errors.New("database unavailable"), http.StatusInternalServerError, `"code":"INTERNAL_SERVER_ERROR"`},
	}
	for _, test := range tests {
		service := &fakeQuoteTemplatesService{saveErr: test.err}
		server := serverWithRole("admin", Dependencies{QuoteTemplatesService: service})
		request := httptest.NewRequest(http.MethodPost, "/api/quote-templates", bytes.NewBufferString(`{"name":"A"}`))
		request.Header.Set("Content-Type", "application/json")
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != test.code || !strings.Contains(recorder.Body.String(), test.body) {
			t.Fatalf("error %v mapped to %d %s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}

func serviceBody(server http.Handler, path string) string {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder.Body.String()
}
