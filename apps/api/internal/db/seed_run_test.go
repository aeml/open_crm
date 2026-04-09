package db

import (
	"context"
	"testing"
)

type seedRecorder struct {
	organizationSeeded bool
	userEmails         []string
	stageNames         []string
}

func (r *seedRecorder) SeedOrganization() error {
	r.organizationSeeded = true
	return nil
}

func (r *seedRecorder) SeedUser(email string) error {
	r.userEmails = append(r.userEmails, email)
	return nil
}

func (r *seedRecorder) SeedStage(name string) error {
	r.stageNames = append(r.stageNames, name)
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

	if len(recorder.stageNames) != len(DefaultDealStages()) {
		t.Fatalf("expected %d default stages, got %d", len(DefaultDealStages()), len(recorder.stageNames))
	}
}
