package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
)

type fakeNotesService struct {
	listResult         []modulenotes.Entry
	listErr            error
	createResult       modulenotes.CreateResult
	createErr          error
	lastListOrgID      int64
	lastListEntityType string
	lastListEntityID   int64
	lastCreateOrgID    int64
	lastCreateActorID  int64
	lastCreateInput    modulenotes.CreateInput
}

func (f *fakeNotesService) ListByEntity(_ context.Context, organizationID int64, entityType string, entityID int64) ([]modulenotes.Entry, error) {
	f.lastListOrgID = organizationID
	f.lastListEntityType = entityType
	f.lastListEntityID = entityID
	return f.listResult, f.listErr
}

func (f *fakeNotesService) Create(_ context.Context, organizationID, actorUserID int64, input modulenotes.CreateInput) (modulenotes.CreateResult, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateActorID = actorUserID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func authenticatedNotesServer(service *fakeNotesService) http.Handler {
	return NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		NotesService: service,
	})
}

func TestListNotesUsesCurrentOrganizationAndEntity(t *testing.T) {
	service := &fakeNotesService{
		listResult: []modulenotes.Entry{{
			ID:                17,
			EntityType:        "contact",
			EntityID:          7,
			Body:              "Initial discovery call logged.",
			CreatedByUserID:   1,
			CreatedByUserName: "Demo Owner",
			CreatedAt:         time.Date(2026, 4, 10, 9, 30, 0, 0, time.UTC),
			UpdatedAt:         time.Date(2026, 4, 10, 9, 30, 0, 0, time.UTC),
		}},
	}
	server := authenticatedNotesServer(service)

	request := httptest.NewRequest(http.MethodGet, "/api/notes?entityType=contact&entityId=7", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if service.lastListOrgID != 42 || service.lastListEntityType != "contact" || service.lastListEntityID != 7 {
		t.Fatalf("unexpected note list routing: org=%d type=%s id=%d", service.lastListOrgID, service.lastListEntityType, service.lastListEntityID)
	}

	var response struct {
		Data struct {
			Notes []modulenotes.Entry `json:"notes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if len(response.Data.Notes) != 1 || response.Data.Notes[0].Body != "Initial discovery call logged." {
		t.Fatalf("unexpected notes payload: %#v", response.Data.Notes)
	}
}

func TestCreateNoteUsesCurrentOrganizationAndActor(t *testing.T) {
	service := &fakeNotesService{
		createResult: modulenotes.CreateResult{
			Note: modulenotes.Entry{
				ID:                18,
				EntityType:        "deal",
				EntityID:          12,
				Body:              "Sent pricing recap and legal packet.",
				CreatedByUserID:   1,
				CreatedByUserName: "Demo Owner",
				CreatedAt:         time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC),
				UpdatedAt:         time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC),
			},
			Activity: modulenotes.ActivityEntry{
				ID:        201,
				Action:    "note.created",
				Summary:   "Note added",
				CreatedAt: time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	server := authenticatedNotesServer(service)

	body := bytes.NewBufferString(`{"entityType":"deal","entityId":12,"body":"Sent pricing recap and legal packet."}`)
	request := httptest.NewRequest(http.MethodPost, "/api/notes", body)
	request.Header.Set("Content-Type", "application/json")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if service.lastCreateOrgID != 42 || service.lastCreateActorID != 1 {
		t.Fatalf("unexpected note create routing: org=%d actor=%d", service.lastCreateOrgID, service.lastCreateActorID)
	}
	if service.lastCreateInput.EntityType != "deal" || service.lastCreateInput.EntityID != 12 || service.lastCreateInput.Body != "Sent pricing recap and legal packet." {
		t.Fatalf("unexpected note create input: %#v", service.lastCreateInput)
	}
}
