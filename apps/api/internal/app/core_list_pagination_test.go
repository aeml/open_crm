package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoreListsRejectUnsafePaginationBeforeCallingService(t *testing.T) {
	contactService := &fakeContactsService{}
	companyService := &fakeCompaniesService{}
	dealService := &fakeDealsService{}
	taskService := &fakeTasksService{}

	for _, testCase := range []struct {
		name      string
		target    string
		server    http.Handler
		lastOrgID func() int64
	}{
		{name: "contact oversized page size", target: "/api/contacts?pageSize=101", server: authenticatedContactsServer(contactService), lastOrgID: func() int64 { return contactService.lastListOrgID }},
		{name: "company malformed page", target: "/api/companies?page=not-a-number", server: authenticatedCompaniesServer(companyService), lastOrgID: func() int64 { return companyService.lastListOrgID }},
		{name: "deal excessive offset", target: "/api/deals?page=502&pageSize=100", server: authenticatedDealsServer(dealService), lastOrgID: func() int64 { return dealService.lastListOrgID }},
		{name: "task non-positive page", target: "/api/tasks?page=0", server: authenticatedTasksServer(taskService), lastOrgID: func() int64 { return taskService.lastListOrgID }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, testCase.target, nil)
			addSessionCookie(request)
			recorder := httptest.NewRecorder()

			testCase.server.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "50,000") {
				t.Fatalf("expected bounded pagination error, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if testCase.lastOrgID() != 0 {
				t.Fatalf("unsafe pagination reached service for organization %d", testCase.lastOrgID())
			}
		})
	}
}

func TestCoreListsAcceptMaximumDocumentedOffset(t *testing.T) {
	service := &fakeContactsService{}
	request := httptest.NewRequest(http.MethodGet, "/api/contacts?page=501&pageSize=100", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	authenticatedContactsServer(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected boundary page to be accepted, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if service.lastListQuery.Page != 501 || service.lastListQuery.PageSize != 100 {
		t.Fatalf("unexpected boundary query: %#v", service.lastListQuery)
	}
}
