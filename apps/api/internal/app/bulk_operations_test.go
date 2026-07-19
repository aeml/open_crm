package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulebulkoperations "github.com/aeml/open_crm/apps/api/internal/modules/bulkoperations"
)

type fakeBulkOperationsService struct {
	operation     modulebulkoperations.Operation
	operations    []modulebulkoperations.Operation
	err           error
	lastExecute   modulebulkoperations.ExecuteInput
	lastOrgID     int64
	lastActorID   int64
	lastEntity    string
	lastLimit     int
	lastOperation int64
}

func (f *fakeBulkOperationsService) Execute(_ context.Context, input modulebulkoperations.ExecuteInput) (modulebulkoperations.Operation, error) {
	f.lastExecute = input
	return f.operation, f.err
}

func (f *fakeBulkOperationsService) List(_ context.Context, organizationID int64, entityType string, limit int) ([]modulebulkoperations.Operation, error) {
	f.lastOrgID = organizationID
	f.lastEntity = entityType
	f.lastLimit = limit
	return f.operations, f.err
}

func (f *fakeBulkOperationsService) Rollback(_ context.Context, organizationID, actorUserID, operationID int64) (modulebulkoperations.Operation, error) {
	f.lastOrgID = organizationID
	f.lastActorID = actorUserID
	f.lastOperation = operationID
	return f.operation, f.err
}

func authenticatedBulkOperationsServer(role string, service bulkOperationsService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{currentSessionResult: moduleauth.SessionState{
			User:         moduleauth.User{ID: 7, Email: "member@acme.test"},
			Organization: moduleauth.Organization{ID: 42, Name: "Acme"},
			Membership:   moduleauth.Membership{Role: role},
		}},
		BulkOperationsService: service,
	})
}

func TestExecuteBulkOperationPassesTenantActorAndRecoveryInput(t *testing.T) {
	service := &fakeBulkOperationsService{operation: modulebulkoperations.Operation{ID: 9, EntityType: "contact", Action: "reassign", ChangedCount: 2, Status: "completed"}}
	server := authenticatedBulkOperationsServer("member", service)
	request := httptest.NewRequest(http.MethodPost, "/api/data-operations/bulk", strings.NewReader(`{"entityType":"contact","action":"reassign","targetUserId":11,"entityIds":[3,4],"idempotencyKey":"bulk-browser-001"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected created, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if service.lastExecute.OrganizationID != 42 || service.lastExecute.ActorUserID != 7 || service.lastExecute.TargetUserID == nil || *service.lastExecute.TargetUserID != 11 || service.lastExecute.IdempotencyKey != "bulk-browser-001" {
		t.Fatalf("unexpected bulk execute scope: %#v", service.lastExecute)
	}
	var response bulkOperationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Data.Operation.ID != 9 {
		t.Fatalf("unexpected bulk operation response: response=%#v err=%v", response, err)
	}
}

func TestBulkOperationHistoryAndRollbackStayTenantScoped(t *testing.T) {
	service := &fakeBulkOperationsService{
		operation:  modulebulkoperations.Operation{ID: 12, Status: "rolled_back", RolledBackCount: 2},
		operations: []modulebulkoperations.Operation{{ID: 12, EntityType: "task", Status: "completed"}},
	}
	server := authenticatedBulkOperationsServer("admin", service)
	listRequest := httptest.NewRequest(http.MethodGet, "/api/data-operations/bulk?entityType=task&limit=5", nil)
	addSessionCookie(listRequest)
	listRecorder := httptest.NewRecorder()
	server.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastEntity != "task" || service.lastLimit != 5 || !strings.Contains(listRecorder.Body.String(), `"id":12`) {
		t.Fatalf("unexpected bulk history response: status=%d service=%#v body=%s", listRecorder.Code, service, listRecorder.Body.String())
	}

	rollbackRequest := httptest.NewRequest(http.MethodPost, "/api/data-operations/bulk/12/rollback", nil)
	addSessionCookie(rollbackRequest)
	rollbackRecorder := httptest.NewRecorder()
	server.ServeHTTP(rollbackRecorder, rollbackRequest)
	if rollbackRecorder.Code != http.StatusOK || service.lastOrgID != 42 || service.lastActorID != 7 || service.lastOperation != 12 {
		t.Fatalf("unexpected rollback routing: status=%d service=%#v body=%s", rollbackRecorder.Code, service, rollbackRecorder.Body.String())
	}
}

func TestBulkOperationRejectsViewerAndMapsConflicts(t *testing.T) {
	viewerService := &fakeBulkOperationsService{}
	viewerServer := authenticatedBulkOperationsServer("viewer", viewerService)
	request := httptest.NewRequest(http.MethodPost, "/api/data-operations/bulk", strings.NewReader(`{"entityType":"contact","action":"archive","entityIds":[3],"idempotencyKey":"bulk-viewer-001"}`))
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	viewerServer.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || len(viewerService.lastExecute.EntityIDs) != 0 {
		t.Fatalf("viewer should be denied before service call: status=%d input=%#v", recorder.Code, viewerService.lastExecute)
	}

	conflictServer := authenticatedBulkOperationsServer("member", &fakeBulkOperationsService{err: modulebulkoperations.ErrIdempotencyConflict})
	conflictRequest := httptest.NewRequest(http.MethodPost, "/api/data-operations/bulk", strings.NewReader(`{"entityType":"contact","action":"archive","entityIds":[3],"idempotencyKey":"bulk-conflict-001"}`))
	conflictRequest.Header.Set("Content-Type", "application/json")
	addSessionCookie(conflictRequest)
	conflictRecorder := httptest.NewRecorder()
	conflictServer.ServeHTTP(conflictRecorder, conflictRequest)
	if conflictRecorder.Code != http.StatusConflict {
		t.Fatalf("expected idempotency conflict, got %d: %s", conflictRecorder.Code, conflictRecorder.Body.String())
	}
}
