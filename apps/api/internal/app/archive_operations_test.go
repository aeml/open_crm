package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	modulearchiveoperations "github.com/aeml/open_crm/apps/api/internal/modules/archiveoperations"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
)

type fakeArchiveOperationsService struct {
	records     []modulearchiveoperations.Record
	record      modulearchiveoperations.Record
	err         error
	lastOrgID   int64
	lastActorID int64
	lastEntity  string
	lastID      int64
	lastQuery   modulearchiveoperations.ListQuery
}

func (f *fakeArchiveOperationsService) List(_ context.Context, organizationID int64, query modulearchiveoperations.ListQuery) ([]modulearchiveoperations.Record, error) {
	f.lastOrgID, f.lastQuery = organizationID, query
	return f.records, f.err
}

func (f *fakeArchiveOperationsService) Restore(_ context.Context, organizationID, actorUserID int64, entityType string, entityID int64) (modulearchiveoperations.Record, error) {
	f.lastOrgID, f.lastActorID, f.lastEntity, f.lastID = organizationID, actorUserID, entityType, entityID
	return f.record, f.err
}

func authenticatedArchiveOperationsServer(role string, service archiveOperationsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User: moduleauth.User{ID: 7, Email: "member@example.test"}, Organization: moduleauth.Organization{ID: 42, Name: "Acme"}, Membership: moduleauth.Membership{Role: role},
		}},
		ArchiveOperationsService: service,
	})
}

func TestListArchivedRecordsAllowsMembersAndScopesFilters(t *testing.T) {
	service := &fakeArchiveOperationsService{records: []modulearchiveoperations.Record{{EntityType: "contact", EntityID: 8, Label: "Ava Stone", ArchivedAt: time.Now()}}}
	server := authenticatedArchiveOperationsServer("viewer", service)
	request := httptest.NewRequest(http.MethodGet, "/api/data-operations/archive?entityType=contact&q=Ava&limit=9", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastQuery.EntityType != "contact" || service.lastQuery.Search != "Ava" || service.lastQuery.Limit != 9 || !strings.Contains(recorder.Body.String(), "Ava Stone") {
		t.Fatalf("unexpected archive list: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestRestoreArchivedRecordRequiresWriterAndPassesTenantActor(t *testing.T) {
	service := &fakeArchiveOperationsService{record: modulearchiveoperations.Record{EntityType: "task", EntityID: 19, Label: "Follow up", ArchivedAt: time.Now()}}
	server := authenticatedArchiveOperationsServer("member", service)
	request := httptest.NewRequest(http.MethodPost, "/api/data-operations/archive/task/19/restore", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastActorID != 7 || service.lastEntity != "task" || service.lastID != 19 {
		t.Fatalf("unexpected archive restore: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}

	viewerService := &fakeArchiveOperationsService{}
	viewerServer := authenticatedArchiveOperationsServer("viewer", viewerService)
	viewerRequest := httptest.NewRequest(http.MethodPost, "/api/data-operations/archive/task/19/restore", nil)
	addSessionCookie(viewerRequest)
	viewerRecorder := httptest.NewRecorder()
	viewerServer.ServeHTTP(viewerRecorder, viewerRequest)
	if viewerRecorder.Code != http.StatusForbidden || viewerService.lastID != 0 {
		t.Fatalf("viewer should not restore: status=%d service=%#v", viewerRecorder.Code, viewerService)
	}
}

func TestArchiveOperationErrorsUseSafeStatuses(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{modulearchiveoperations.ErrInvalidInput, http.StatusBadRequest},
		{modulearchiveoperations.ErrNotFound, http.StatusNotFound},
		{fmtError(modulearchiveoperations.ErrConflict), http.StatusConflict},
		{modulearchiveoperations.ErrInactiveActor, http.StatusForbidden},
		{errors.New("database unavailable"), http.StatusInternalServerError},
	}
	for _, testCase := range tests {
		server := authenticatedArchiveOperationsServer("owner", &fakeArchiveOperationsService{err: testCase.err})
		request := httptest.NewRequest(http.MethodPost, "/api/data-operations/archive/contact/8/restore", nil)
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: got status %d want %d body=%s", testCase.err, recorder.Code, testCase.status, recorder.Body.String())
		}
	}
}

func fmtError(err error) error { return errors.Join(err, errors.New("linked record is archived")) }
