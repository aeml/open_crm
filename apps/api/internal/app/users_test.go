package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

type fakeUsersService struct {
	listResult      []moduleusers.UserSummary
	listErr         error
	createResult    moduleusers.UserSummary
	createErr       error
	setupErr        error
	lastListOrgID   int64
	lastCreateOrgID int64
	lastRoleOrgID   int64
	lastRoleUserID  int64
	lastRoleActorID int64
	lastRole        string
	lastCreateInput moduleusers.CreateUserInput
	lastSetupInput  moduleusers.CompleteSetupInput
}

func (f *fakeUsersService) ListByOrganization(_ context.Context, organizationID int64) ([]moduleusers.UserSummary, error) {
	f.lastListOrgID = organizationID
	return f.listResult, f.listErr
}

func (f *fakeUsersService) CreateForOrganization(_ context.Context, organizationID int64, input moduleusers.CreateUserInput) (moduleusers.UserSummary, error) {
	f.lastCreateOrgID = organizationID
	f.lastCreateInput = input
	return f.createResult, f.createErr
}

func (f *fakeUsersService) UpdateRole(_ context.Context, organizationID, userID, actorUserID int64, role string) (moduleusers.UserSummary, error) {
	f.lastRoleOrgID = organizationID
	f.lastRoleUserID = userID
	f.lastRoleActorID = actorUserID
	f.lastRole = role
	return moduleusers.UserSummary{ID: userID, Email: "admin@acme.test", FirstName: "Demo", LastName: "Admin", Role: role}, nil
}

func (f *fakeUsersService) CompleteSetup(_ context.Context, input moduleusers.CompleteSetupInput) (moduleusers.SetupCompletion, error) {
	f.lastSetupInput = input
	return moduleusers.SetupCompletion{UserID: 9, OrganizationID: 42, Email: "new.admin@acme.test"}, f.setupErr
}

func (f *fakeUsersService) UpdateProfile(_ context.Context, _ int64, _ moduleusers.UpdateProfileInput) (moduleusers.UserProfile, error) {
	return moduleusers.UserProfile{}, nil
}

func (f *fakeUsersService) GetPreferences(_ context.Context, _ int64) (moduleusers.UserPreferences, error) {
	return moduleusers.UserPreferences{}, nil
}

func (f *fakeUsersService) UpdatePreferences(_ context.Context, _ int64, _ moduleusers.UserPreferences) (moduleusers.UserPreferences, error) {
	return moduleusers.UserPreferences{}, nil
}

func TestListUsersUsesCurrentSessionOrganization(t *testing.T) {
	usersService := &fakeUsersService{
		listResult: []moduleusers.UserSummary{{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner", Role: "owner"}},
	}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "admin"},
			},
		},
		UsersService: usersService,
	})

	request := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if usersService.lastListOrgID != 42 {
		t.Fatalf("expected users list to use org id 42, got %d", usersService.lastListOrgID)
	}

	var response struct {
		Data struct {
			Users []moduleusers.UserSummary `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON response, got error: %v", err)
	}

	if len(response.Data.Users) != 1 || response.Data.Users[0].Email != "owner@acme.test" {
		t.Fatalf("expected returned user list, got %#v", response.Data.Users)
	}
}

func TestListUsersAllowsAllAuthenticatedRoles(t *testing.T) {
	// Any org member (including viewer) may list users so that they can see
	// who to assign tasks, notes, etc. to. Only admin/owner may create or
	// modify user accounts.
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 3, Email: "member@acme.test", FirstName: "Demo", LastName: "Member"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "member"},
			},
		},
		UsersService: &fakeUsersService{},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
}

func TestCreateUserCreatesUserInCurrentOrganization(t *testing.T) {
	usersService := &fakeUsersService{
		createResult: moduleusers.UserSummary{ID: 9, Email: "new.admin@acme.test", FirstName: "New", LastName: "Admin", Role: "admin", SetupLink: "/setup-password?token=setup-token-123"},
	}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		UsersService: usersService,
	})

	body := bytes.NewBufferString(`{"email":"new.admin@acme.test","firstName":"New","lastName":"Admin","role":"admin"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/users", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}

	if usersService.lastCreateOrgID != 42 {
		t.Fatalf("expected create user to use org id 42, got %d", usersService.lastCreateOrgID)
	}

	if usersService.lastCreateInput.Email != "new.admin@acme.test" || usersService.lastCreateInput.Role != "admin" {
		t.Fatalf("unexpected create user input: %#v", usersService.lastCreateInput)
	}

	var response struct {
		Data struct {
			User moduleusers.UserSummary `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON response, got error: %v", err)
	}

	if response.Data.User.Email != "new.admin@acme.test" || response.Data.User.SetupLink == "" {
		t.Fatalf("expected created user in response, got %#v", response.Data.User)
	}
}

func TestUpdateUserRoleRecordsCurrentOrganizationAndActor(t *testing.T) {
	usersService := &fakeUsersService{}
	server := NewServer(config.Env{}, Dependencies{
		AuthService: &fakeAuthService{
			currentSessionResult: moduleauth.SessionState{
				User:         moduleauth.User{ID: 1, Email: "owner@acme.test", FirstName: "Demo", LastName: "Owner"},
				Organization: moduleauth.Organization{ID: 42, Name: "Acme, Inc.", Slug: "acme-inc"},
				Membership:   moduleauth.Membership{Role: "owner"},
			},
		},
		UsersService: usersService,
	})

	body := bytes.NewBufferString(`{"role":"admin"}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/users/9/role", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token-123"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if usersService.lastRoleOrgID != 42 || usersService.lastRoleUserID != 9 || usersService.lastRoleActorID != 1 || usersService.lastRole != "admin" {
		t.Fatalf("unexpected role update routing: org=%d user=%d actor=%d role=%q", usersService.lastRoleOrgID, usersService.lastRoleUserID, usersService.lastRoleActorID, usersService.lastRole)
	}
}

func TestCompleteUserSetupConsumesSetupToken(t *testing.T) {
	usersService := &fakeUsersService{}
	server := NewServer(config.Env{}, Dependencies{UsersService: usersService})

	body := bytes.NewBufferString(`{"token":"setup-token-123","password":"new-password"}`)
	request := httptest.NewRequest(http.MethodPost, "/auth/setup-password", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if usersService.lastSetupInput.Token != "setup-token-123" || usersService.lastSetupInput.Password != "new-password" {
		t.Fatalf("unexpected setup input: %#v", usersService.lastSetupInput)
	}
}

func TestCompleteUserSetupRejectsInvalidToken(t *testing.T) {
	usersService := &fakeUsersService{setupErr: moduleusers.ErrInvalidSetupToken}
	server := NewServer(config.Env{}, Dependencies{UsersService: usersService})

	body := bytes.NewBufferString(`{"token":"bad-token","password":"new-password"}`)
	request := httptest.NewRequest(http.MethodPost, "/auth/setup-password", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestCompleteUserSetupRateLimitsRepeatedAttempts(t *testing.T) {
	usersService := &fakeUsersService{setupErr: moduleusers.ErrInvalidSetupToken}
	server := NewServer(config.Env{}, Dependencies{UsersService: usersService})

	for i := 0; i < authRateLimit; i++ {
		request := httptest.NewRequest(http.MethodPost, "/auth/setup-password", bytes.NewBufferString(`{"token":"bad-token","password":"new-password"}`))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "198.51.100.30:12345"
		recorder := httptest.NewRecorder()

		server.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("attempt %d: expected status %d, got %d", i+1, http.StatusBadRequest, recorder.Code)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/auth/setup-password", bytes.NewBufferString(`{"token":"bad-token","password":"new-password"}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "198.51.100.30:12345"
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, recorder.Code)
	}
}
