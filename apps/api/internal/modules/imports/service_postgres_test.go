package imports_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleimports "github.com/aeml/open_crm/apps/api/internal/modules/imports"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

func TestTrackedImportIdempotencyErrorsIsolationAndRollbackAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to imports test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_imports_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create imports schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := importsDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate imports schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated imports schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Imports', $1) RETURNING id`, "imports-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create imports organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Foreign imports', $1) RETURNING id`, "foreign-imports-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign imports organization: %v", err)
	}
	ownerID := insertImportUser(t, ctx, pool, "owner-"+schema+"@example.test", "Import", "Owner")
	disabledID := insertImportUser(t, ctx, pool, "disabled-"+schema+"@example.test", "Disabled", "Admin")
	foreignID := insertImportUser(t, ctx, pool, "foreign-"+schema+"@example.test", "Foreign", "Owner")
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		role           string
		status         string
	}{
		{organizationID, ownerID, "owner", "active"},
		{organizationID, disabledID, "admin", "disabled"},
		{foreignOrganizationID, foreignID, "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role, membership_status) VALUES ($1, $2, $3, $4)`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("create import membership: %v", err)
		}
	}

	service := moduleimports.NewService(pool)
	queue := modulejobs.NewService(pool)
	worker := modulejobs.NewWorker(queue, map[string]modulejobs.Handler{
		moduleimports.JobType: func(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
			result, handleErr := service.HandleJob(ctx, job)
			if moduleimports.IsPermanentFailure(handleErr) {
				return nil, modulejobs.Permanent(handleErr)
			}
			return result, handleErr
		},
	}, "imports-postgres-test", nil)
	contactCSV := "First Name,Last Name,Email Address,Status,Client\nAva,Stone,ava@example.test,lead,true\nMissing,,missing@example.test,lead,false\nAva,Stone,ava@example.test,lead,true\n"
	preview, err := service.Preview(ctx, moduleimports.PreviewInput{EntityType: "contacts", Reader: strings.NewReader(contactCSV)})
	if err != nil {
		t.Fatalf("preview mapped contact csv: %v", err)
	}
	if preview.Mapping["first_name"] != "First Name" || preview.Mapping["email"] != "Email Address" || preview.Summary.ErrorRows != 1 {
		t.Fatalf("unexpected mapping preview: %#v", preview)
	}
	batch, err := service.Execute(ctx, moduleimports.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contacts",
		OriginalName: "real-contacts.csv", IdempotencyKey: "contacts-import-001",
		Reader: strings.NewReader(contactCSV), Mapping: preview.Mapping,
	})
	if err != nil {
		t.Fatalf("execute contacts import: %v", err)
	}
	if batch.Status != "processing" || batch.TotalRows != 3 || batch.ProcessedRows != 0 || batch.JobStatus != "pending" {
		t.Fatalf("unexpected queued contacts batch: %#v", batch)
	}
	var retainedSourceBytes, importJobCount int
	if err := pool.QueryRow(ctx, `SELECT octet_length(source_csv) FROM import_batches WHERE organization_id=$1 AND id=$2`, organizationID, batch.ID).Scan(&retainedSourceBytes); err != nil || retainedSourceBytes != len(contactCSV) {
		t.Fatalf("queued import source bytes=%d want=%d err=%v", retainedSourceBytes, len(contactCSV), err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id=$1 AND job_type=$2`, organizationID, moduleimports.JobType).Scan(&importJobCount); err != nil || importJobCount != 1 {
		t.Fatalf("queued import jobs=%d want=1 err=%v", importJobCount, err)
	}
	if summary, err := worker.RunOnce(ctx); err != nil || summary.Succeeded != 1 {
		t.Fatalf("run contacts import worker: summary=%#v err=%v", summary, err)
	}
	batches, err := service.List(ctx, organizationID, 50)
	if err != nil || len(batches) != 1 {
		t.Fatalf("load completed contacts import: batches=%#v err=%v", batches, err)
	}
	batch = batches[0]
	if batch.Status != "completed_with_errors" || batch.TotalRows != 3 || batch.SuccessRows != 1 || batch.ErrorRows != 2 || batch.JobStatus != "succeeded" || batch.JobAttempts != 1 {
		t.Fatalf("unexpected completed contacts batch: %#v", batch)
	}
	if err := pool.QueryRow(ctx, `SELECT COALESCE(octet_length(source_csv),0) FROM import_batches WHERE organization_id=$1 AND id=$2`, organizationID, batch.ID).Scan(&retainedSourceBytes); err != nil || retainedSourceBytes != 0 {
		t.Fatalf("completed import retained source bytes=%d err=%v", retainedSourceBytes, err)
	}
	var importedContactID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM contacts WHERE organization_id = $1 AND email = 'ava@example.test' AND owner_user_id = $2`, organizationID, ownerID).Scan(&importedContactID); err != nil {
		t.Fatalf("find imported tenant contact: %v", err)
	}
	var foreignContacts int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE organization_id = $1`, foreignOrganizationID).Scan(&foreignContacts); err != nil || foreignContacts != 0 {
		t.Fatalf("import leaked into foreign tenant: count=%d err=%v", foreignContacts, err)
	}
	errorFile, err := service.ErrorCSV(ctx, organizationID, batch.ID)
	if err != nil {
		t.Fatalf("download import errors: %v", err)
	}
	errorText := string(errorFile.Content)
	if !strings.Contains(errorText, "3,last_name,Last name is required") || !strings.Contains(errorText, "4,,Possible duplicate contact") || strings.Contains(errorText, "missing@example.test") {
		t.Fatalf("unexpected privacy-safe error csv: %q", errorText)
	}

	replayed, err := service.Execute(ctx, moduleimports.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contacts",
		OriginalName: "real-contacts.csv", IdempotencyKey: "contacts-import-001",
		Reader: strings.NewReader(contactCSV), Mapping: preview.Mapping,
	})
	if err != nil || !replayed.Replayed || replayed.ID != batch.ID {
		t.Fatalf("expected idempotent import replay, batch=%#v err=%v", replayed, err)
	}
	if _, err := service.Execute(ctx, moduleimports.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contacts",
		OriginalName: "different.csv", IdempotencyKey: "contacts-import-001",
		Reader: strings.NewReader(strings.Replace(contactCSV, "Ava", "Ada", 1)), Mapping: preview.Mapping,
	}); !errors.Is(err, moduleimports.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency payload conflict, got %v", err)
	}
	foreignBatches, err := service.List(ctx, foreignOrganizationID, 50)
	if err != nil || len(foreignBatches) != 0 {
		t.Fatalf("foreign tenant saw import history: batches=%#v err=%v", foreignBatches, err)
	}
	if _, err := service.Rollback(ctx, foreignOrganizationID, foreignID, batch.ID); !errors.Is(err, moduleimports.ErrNotFound) {
		t.Fatalf("expected cross-tenant rollback miss, got %v", err)
	}
	if _, err := service.Execute(ctx, moduleimports.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: disabledID, EntityType: "contacts",
		OriginalName: "disabled.csv", IdempotencyKey: "disabled-import-001",
		Reader: strings.NewReader(contactCSV), Mapping: preview.Mapping,
	}); !errors.Is(err, moduleimports.ErrInactiveActor) {
		t.Fatalf("expected disabled actor denial, got %v", err)
	}
	if result, err := service.HandleJob(ctx, modulejobs.Job{OrganizationID: organizationID, Payload: map[string]any{"batchId": fmt.Sprint(batch.ID)}}); err != nil || result["status"] != "completed_with_errors" {
		t.Fatalf("completed import handler replay was not idempotent: result=%#v err=%v", result, err)
	}
	if _, err := service.HandleJob(ctx, modulejobs.Job{OrganizationID: foreignOrganizationID, Payload: map[string]any{"batchId": fmt.Sprint(batch.ID)}}); !errors.Is(err, moduleimports.ErrNotFound) {
		t.Fatalf("foreign import job should not resolve local batch, got %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='active' WHERE organization_id=$1 AND user_id=$2`, organizationID, disabledID); err != nil {
		t.Fatalf("reactivate import admin for queued-deactivation test: %v", err)
	}
	interruptedCSV := "First Name,Last Name,Email Address\nInterrupted,Worker,interrupted@example.test\n"
	interruptedPreview, err := service.Preview(ctx, moduleimports.PreviewInput{EntityType: "contacts", Reader: strings.NewReader(interruptedCSV)})
	if err != nil {
		t.Fatalf("preview interrupted import: %v", err)
	}
	interrupted, err := service.Execute(ctx, moduleimports.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: disabledID, EntityType: "contacts",
		OriginalName: "interrupted.csv", IdempotencyKey: "interrupted-import-001",
		Reader: strings.NewReader(interruptedCSV), Mapping: interruptedPreview.Mapping,
	})
	if err != nil {
		t.Fatalf("queue interrupted import: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, disabledID); err != nil {
		t.Fatalf("disable queued import actor: %v", err)
	}
	if summary, runErr := worker.RunOnce(ctx); runErr != nil || summary.Dead != 1 {
		t.Fatalf("inactive import actor should dead-letter safely: summary=%#v err=%v", summary, runErr)
	}
	interruptedBatches, err := service.List(ctx, organizationID, 50)
	if err != nil {
		t.Fatalf("list interrupted import: %v", err)
	}
	for _, current := range interruptedBatches {
		if current.ID == interrupted.ID {
			interrupted = current
		}
	}
	if interrupted.Status != "failed" || interrupted.JobStatus != "dead" || interrupted.ProcessedRows != 0 || !strings.Contains(interrupted.FailureMessage, "no longer active") {
		t.Fatalf("unexpected inactive-actor import outcome: %#v", interrupted)
	}
	if _, err := pool.Exec(ctx, `UPDATE import_batches SET source_expires_at=NOW()-INTERVAL '1 second' WHERE organization_id=$1 AND id=$2`, organizationID, interrupted.ID); err != nil {
		t.Fatalf("age failed import source: %v", err)
	}
	if expired, expireErr := service.ExpireSources(ctx); expireErr != nil || expired != 1 {
		t.Fatalf("expire failed import source: count=%d err=%v", expired, expireErr)
	}
	var expiredSourceBytes int
	var expiredMessage string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(octet_length(source_csv),0),failure_message FROM import_batches WHERE organization_id=$1 AND id=$2`, organizationID, interrupted.ID).Scan(&expiredSourceBytes, &expiredMessage); err != nil || expiredSourceBytes != 0 || !strings.Contains(expiredMessage, "expired") {
		t.Fatalf("failed import source cleanup bytes=%d message=%q err=%v", expiredSourceBytes, expiredMessage, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE contacts SET status = 'prospect', updated_at = NOW() + INTERVAL '1 second' WHERE organization_id = $1 AND id = $2`, organizationID, importedContactID); err != nil {
		t.Fatalf("modify imported contact before rollback: %v", err)
	}
	partial, err := service.Rollback(ctx, organizationID, ownerID, batch.ID)
	if err != nil {
		t.Fatalf("rollback changed contacts import: %v", err)
	}
	if partial.Status != "partially_rolled_back" || partial.RollbackSkippedRows != 1 || partial.RolledBackRows != 0 {
		t.Fatalf("expected changed contact rollback skip, got %#v", partial)
	}
	var contactArchived bool
	if err := pool.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM contacts WHERE organization_id = $1 AND id = $2`, organizationID, importedContactID).Scan(&contactArchived); err != nil || contactArchived {
		t.Fatalf("changed imported contact should remain active, archived=%v err=%v", contactArchived, err)
	}

	companyCSV := "Account Name,Website,Status\nAtlas Manufacturing,atlas.example,customer\n"
	companyPreview, err := service.Preview(ctx, moduleimports.PreviewInput{EntityType: "companies", Reader: strings.NewReader(companyCSV)})
	if err != nil || len(companyPreview.MappingErrors) != 0 {
		t.Fatalf("preview company import: preview=%#v err=%v", companyPreview, err)
	}
	companyBatch, err := service.Execute(ctx, moduleimports.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "companies",
		OriginalName: "accounts.csv", IdempotencyKey: "companies-import-001",
		Reader: strings.NewReader(companyCSV), Mapping: companyPreview.Mapping,
	})
	if err != nil || companyBatch.Status != "processing" || companyBatch.SuccessRows != 0 {
		t.Fatalf("queue company import: batch=%#v err=%v", companyBatch, err)
	}
	if summary, runErr := worker.RunOnce(ctx); runErr != nil || summary.Succeeded != 1 {
		t.Fatalf("run company import worker: summary=%#v err=%v", summary, runErr)
	}
	allBatches, err := service.List(ctx, organizationID, 50)
	if err != nil {
		t.Fatalf("load completed company import: %v", err)
	}
	for _, current := range allBatches {
		if current.ID == companyBatch.ID {
			companyBatch = current
		}
	}
	if companyBatch.Status != "completed" || companyBatch.SuccessRows != 1 {
		t.Fatalf("complete company import: batch=%#v", companyBatch)
	}
	if _, err := pool.Exec(ctx, `UPDATE import_batches SET status='failed',completed_at=NULL,failure_message='simulated post-checkpoint interruption' WHERE organization_id=$1 AND id=$2`, organizationID, companyBatch.ID); err != nil {
		t.Fatalf("simulate failed import after committed checkpoint: %v", err)
	}
	rolledBack, err := service.Rollback(ctx, organizationID, ownerID, companyBatch.ID)
	if err != nil || rolledBack.Status != "rolled_back" || rolledBack.RolledBackRows != 1 {
		t.Fatalf("rollback unchanged company import: batch=%#v err=%v", rolledBack, err)
	}
	rollbackReplay, err := service.Rollback(ctx, organizationID, ownerID, companyBatch.ID)
	if err != nil || !rollbackReplay.Replayed {
		t.Fatalf("expected idempotent rollback replay, batch=%#v err=%v", rollbackReplay, err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id = $1 AND event_type IN ('import.completed', 'import.rolled_back')`, organizationID).Scan(&auditCount); err != nil || auditCount != 4 {
		t.Fatalf("expected import completion and rollback audit events, count=%d err=%v", auditCount, err)
	}
}

func insertImportUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email, firstName, lastName string) int64 {
	t.Helper()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'test-hash', $2, $3) RETURNING id`, email, firstName, lastName).Scan(&userID); err != nil {
		t.Fatalf("create import user: %v", err)
	}
	return userID
}

func importsDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse imports database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
