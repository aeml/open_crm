package app

// Cross-org tenant isolation tests.
//
// Each test simulates the condition that arises when a user from Organization A
// (session org ID 42) attempts to modify or delete a resource that belongs to a
// different organization. In production the service layer enforces org-scoped
// SQL queries (WHERE org_id = $1 AND id = $2). When the resource does not
// belong to the requesting org, the service returns ErrNotFound. These tests
// verify that every handler surfaces that as HTTP 404 — never 200, never 500,
// and with no data leakage.
//
// Coverage by module:
//
//   contacts    — PATCH, DELETE (GET 404 already covered in contacts_test.go)
//   companies   — PATCH, DELETE (GET 404 already covered in companies_test.go)
//   deals       — PATCH, DELETE, PATCH /stage (GET 404 in deals_test.go)
//   tasks       — PATCH, DELETE (GET 404 already covered in tasks_test.go)
//   saved-views — PATCH, DELETE
//
// Modules with no individual resource paths (audit, dashboard, exports, imports,
// notes, orgprofile) have no cross-org resource-level path to test; tenant
// isolation there is enforced entirely by the session org ID passed to the
// service's list/summary methods, which is verified in their respective test files.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
)

// ── Contacts ─────────────────────────────────────────────────────────────────

func TestUpdateContactReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeContactsService{updateErr: modulecontacts.ErrNotFound}
	server := authenticatedContactsServer(service)

	body := bytes.NewBufferString(`{"firstName":"Ava","lastName":"Stone"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/contacts/99", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when updating contact from another org, got %d", recorder.Code)
	}
}

func TestArchiveContactReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeContactsService{archiveErr: modulecontacts.ErrNotFound}
	server := authenticatedContactsServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/contacts/99", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when archiving contact from another org, got %d", recorder.Code)
	}
}

// ── Companies ────────────────────────────────────────────────────────────────

func TestUpdateCompanyReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeCompaniesService{updateErr: modulecompanies.ErrNotFound}
	server := authenticatedCompaniesServer(service)

	body := bytes.NewBufferString(`{"name":"Ghost Co","clientType":"organization"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/companies/99", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when updating company from another org, got %d", recorder.Code)
	}
}

func TestArchiveCompanyReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeCompaniesService{archiveErr: modulecompanies.ErrNotFound}
	server := authenticatedCompaniesServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/companies/99", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when archiving company from another org, got %d", recorder.Code)
	}
}

// ── Deals ─────────────────────────────────────────────────────────────────────

func TestUpdateDealReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeDealsService{updateErr: moduledeals.ErrNotFound}
	server := authenticatedDealsServer(service)

	body := bytes.NewBufferString(`{"name":"Ghost Deal","status":"open"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/deals/99", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when updating deal from another org, got %d", recorder.Code)
	}
}

func TestArchiveDealReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeDealsService{archiveErr: moduledeals.ErrNotFound}
	server := authenticatedDealsServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/deals/99", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when archiving deal from another org, got %d", recorder.Code)
	}
}

func TestUpdateDealStageReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeDealsService{updateStageErr: moduledeals.ErrNotFound}
	server := authenticatedDealsServer(service)

	body := bytes.NewBufferString(`{"stageId":4}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/deals/99/stage", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when moving deal stage from another org, got %d", recorder.Code)
	}
}

// ── Tasks ─────────────────────────────────────────────────────────────────────

func TestUpdateTaskReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeTasksService{updateErr: moduletasks.ErrNotFound}
	server := authenticatedTasksServer(service)

	body := bytes.NewBufferString(`{"status":"completed","completedAt":"2026-04-10T14:15:00Z"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/tasks/99", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when updating task from another org, got %d", recorder.Code)
	}
}

func TestArchiveTaskReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeTasksService{archiveErr: moduletasks.ErrNotFound}
	server := authenticatedTasksServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/tasks/99", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when archiving task from another org, got %d", recorder.Code)
	}
}

// ── Saved Views ──────────────────────────────────────────────────────────────

func TestUpdateSavedViewReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeSavedViewsService{updateErr: modulesavedviews.ErrNotFound}
	server := authenticatedSavedViewsServer(service)

	body := bytes.NewBufferString(`{"entityType":"deals","name":"Ghost View","filters":{},"isDefault":false}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/saved-views/99", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when updating saved view from another org, got %d", recorder.Code)
	}
}

func TestDeleteSavedViewReturnsNotFoundForCrossOrgResource(t *testing.T) {
	service := &fakeSavedViewsService{deleteErr: modulesavedviews.ErrNotFound}
	server := authenticatedSavedViewsServer(service)

	request := httptest.NewRequest(http.MethodDelete, "/api/saved-views/99", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when deleting saved view from another org, got %d", recorder.Code)
	}
}
