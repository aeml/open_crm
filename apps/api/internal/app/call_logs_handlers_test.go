package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecalllogs "github.com/aeml/open_crm/apps/api/internal/modules/calllogs"
)

type fakeCallLogsService struct {
	listResult        []modulecalllogs.Log
	listErr           error
	startResult       modulecalllogs.StartResult
	startErr          error
	completeResult    modulecalllogs.Log
	completeErr       error
	recordResult      modulecalllogs.Log
	recordErr         error
	lastOrgID         int64
	lastActorID       int64
	lastEntityType    string
	lastEntityID      int64
	lastLimit         int
	lastStartInput    modulecalllogs.StartInput
	lastCompleteID    int64
	lastCompleteInput modulecalllogs.CompleteInput
	lastRecordInput   modulecalllogs.RecordInput
}

func (f *fakeCallLogsService) ListByEntity(_ context.Context, organizationID int64, entityType string, entityID int64, limit int) ([]modulecalllogs.Log, error) {
	f.lastOrgID = organizationID
	f.lastEntityType = entityType
	f.lastEntityID = entityID
	f.lastLimit = limit
	return f.listResult, f.listErr
}

func (f *fakeCallLogsService) StartOutbound(_ context.Context, organizationID, actorUserID int64, input modulecalllogs.StartInput) (modulecalllogs.StartResult, error) {
	f.lastOrgID = organizationID
	f.lastActorID = actorUserID
	f.lastStartInput = input
	return f.startResult, f.startErr
}

func (f *fakeCallLogsService) Complete(_ context.Context, organizationID, actorUserID, callID int64, input modulecalllogs.CompleteInput) (modulecalllogs.Log, error) {
	f.lastOrgID = organizationID
	f.lastActorID = actorUserID
	f.lastCompleteID = callID
	f.lastCompleteInput = input
	return f.completeResult, f.completeErr
}

func (f *fakeCallLogsService) RecordManual(_ context.Context, organizationID, actorUserID int64, input modulecalllogs.RecordInput) (modulecalllogs.Log, error) {
	f.lastOrgID = organizationID
	f.lastActorID = actorUserID
	f.lastRecordInput = input
	return f.recordResult, f.recordErr
}

func callLogsServer(service *fakeCallLogsService, role string) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: role},
			},
		},
		CallLogsService: service,
	})
}

func TestListCallLogsAllowsMemberAndScopesEntity(t *testing.T) {
	service := &fakeCallLogsService{
		listResult: []modulecalllogs.Log{{ID: 3, EntityType: "contact", EntityID: 7, Direction: "outbound", PhoneNumber: "+15551234567", Status: "completed", Disposition: "Connected", CreatedByUserID: 1, StartedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}},
	}
	server := callLogsServer(service, "member")

	request := httptest.NewRequest(http.MethodGet, "/api/calls?entityType=contact&entityId=7&limit=25", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for member call logs, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastEntityType != "contact" || service.lastEntityID != 7 || service.lastLimit != 25 {
		t.Fatalf("unexpected call log scope: org=%d entity=%s/%d limit=%d", service.lastOrgID, service.lastEntityType, service.lastEntityID, service.lastLimit)
	}
	var response callLogsListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(response.Data.Calls) != 1 || response.Data.Calls[0].Disposition != "Connected" {
		t.Fatalf("unexpected call log payload: %#v", response.Data.Calls)
	}
}

func TestStartCallAllowsWriterAndReturnsDialURL(t *testing.T) {
	service := &fakeCallLogsService{
		startResult: modulecalllogs.StartResult{Call: modulecalllogs.Log{ID: 4, EntityType: "contact", EntityID: 8, Direction: "outbound", PhoneNumber: "+15550001111", Status: "initiated", CreatedByUserID: 1, StartedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}, DialURL: "tel:+15550001111"},
	}
	server := callLogsServer(service, "member")

	body := strings.NewReader(`{"entityType":"contact","entityId":8,"phoneNumber":"+15550001111"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/calls/start", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 for start call, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastActorID != 1 || service.lastStartInput.EntityType != "contact" || service.lastStartInput.EntityID != 8 || service.lastStartInput.PhoneNumber != "+15550001111" {
		t.Fatalf("unexpected start call input: org=%d actor=%d input=%#v", service.lastOrgID, service.lastActorID, service.lastStartInput)
	}
	var response callStartResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.DialURL != "tel:+15550001111" || response.Data.Call.Status != "initiated" {
		t.Fatalf("unexpected start call response: %#v", response.Data)
	}
}

func TestCompleteCallAllowsWriterAndCapturesDisposition(t *testing.T) {
	service := &fakeCallLogsService{
		completeResult: modulecalllogs.Log{ID: 4, EntityType: "contact", EntityID: 8, Direction: "outbound", PhoneNumber: "+15550001111", Status: "completed", Disposition: "Connected", Notes: "Asked for quote", CreatedByUserID: 1, StartedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	server := callLogsServer(service, "member")

	body := strings.NewReader(`{"status":"completed","disposition":"Connected","notes":"Asked for quote"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/calls/4/complete", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for complete call, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastActorID != 1 || service.lastCompleteID != 4 || service.lastCompleteInput.Disposition != "Connected" || service.lastCompleteInput.Notes != "Asked for quote" {
		t.Fatalf("unexpected complete call input: org=%d actor=%d id=%d input=%#v", service.lastOrgID, service.lastActorID, service.lastCompleteID, service.lastCompleteInput)
	}
}

func TestRecordCallAllowsWriterAndCapturesInboundCall(t *testing.T) {
	service := &fakeCallLogsService{
		recordResult: modulecalllogs.Log{ID: 5, EntityType: "contact", EntityID: 8, Direction: "inbound", PhoneNumber: "+15550001111", Status: "completed", Disposition: "Voicemail", Notes: "Asked for callback", CreatedByUserID: 1, StartedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	server := callLogsServer(service, "member")

	body := strings.NewReader(`{"entityType":"contact","entityId":8,"direction":"inbound","phoneNumber":"+15550001111","status":"completed","disposition":"Voicemail","notes":"Asked for callback"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/calls/log", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 for manual call log, got %d", recorder.Code)
	}
	if service.lastOrgID != 42 || service.lastActorID != 1 || service.lastRecordInput.Direction != "inbound" || service.lastRecordInput.Disposition != "Voicemail" || service.lastRecordInput.Notes != "Asked for callback" {
		t.Fatalf("unexpected manual call log input: org=%d actor=%d input=%#v", service.lastOrgID, service.lastActorID, service.lastRecordInput)
	}
	var response callLogResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response.Data.Call.Direction != "inbound" || response.Data.Call.Disposition != "Voicemail" {
		t.Fatalf("unexpected manual call response: %#v", response.Data.Call)
	}
}
