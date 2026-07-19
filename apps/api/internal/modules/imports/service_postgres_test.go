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
	if batch.Status != "completed_with_errors" || batch.TotalRows != 3 || batch.SuccessRows != 1 || batch.ErrorRows != 2 {
		t.Fatalf("unexpected completed contacts batch: %#v", batch)
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
	if err != nil || companyBatch.Status != "completed" || companyBatch.SuccessRows != 1 {
		t.Fatalf("execute company import: batch=%#v err=%v", companyBatch, err)
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
