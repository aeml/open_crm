package db

import (
	"context"
	"testing"
)

type seededUser struct {
	email        string
	passwordHash string
}

type seedRecorder struct {
	organizationSeeded bool
	pipelineSeeded     bool
	userEmails         []string
	seededUsers        []seededUser
	stageNames         []string
}

func (r *seedRecorder) SeedOrganization() error {
	r.organizationSeeded = true
	return nil
}

func (r *seedRecorder) SeedUser(email, passwordHash string) error {
	r.userEmails = append(r.userEmails, email)
	r.seededUsers = append(r.seededUsers, seededUser{email: email, passwordHash: passwordHash})
	return nil
}

func (r *seedRecorder) SeedPipeline() error {
	r.pipelineSeeded = true
	return nil
}

func (r *seedRecorder) SeedStage(stage DealStageSeed) error {
	r.stageNames = append(r.stageNames, stage.Name)
	return nil
}

func TestSeedDatabaseRejectsMissingDatabaseURL(t *testing.T) {
	err := SeedDatabase(context.Background(), Config{})
	if err == nil {
		t.Fatal("expected seed without database url to fail")
	}
}

func TestSeedDatabaseSeedsDefaultWorkspaceShape(t *testing.T) {
	recorder := &seedRecorder{}
	if err := seedDatabase(context.Background(), recorder); err != nil {
		t.Fatalf("expected seed routine to succeed, got error: %v", err)
	}

	if !recorder.organizationSeeded {
		t.Fatal("expected organization seed to run")
	}

	if len(recorder.userEmails) != 4 {
		t.Fatalf("expected 4 seeded users, got %d", len(recorder.userEmails))
	}

	if !recorder.pipelineSeeded {
		t.Fatal("expected pipeline seed to run")
	}

	if len(recorder.stageNames) != len(DefaultDealStages()) {
		t.Fatalf("expected %d default stages, got %d", len(DefaultDealStages()), len(recorder.stageNames))
	}
}

func TestSeedDatabaseHashesSeededPasswords(t *testing.T) {
	recorder := &seedRecorder{}
	if err := seedDatabase(context.Background(), recorder); err != nil {
		t.Fatalf("expected seed routine to succeed, got error: %v", err)
	}

	if len(recorder.seededUsers) != 4 {
		t.Fatalf("expected 4 seeded users, got %d", len(recorder.seededUsers))
	}

	for _, user := range recorder.seededUsers {
		if user.passwordHash == "" {
			t.Fatalf("expected password hash for %s to be set", user.email)
		}
		if user.passwordHash == defaultSeedPassword {
			t.Fatalf("expected password for %s to be hashed, got plaintext", user.email)
		}
	}
}
