package archiveoperations_test

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
	modulearchiveoperations "github.com/aeml/open_crm/apps/api/internal/modules/archiveoperations"
)

func TestArchiveRecoveryIsTenantSafeDependencyAwareAndAuditedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to archive recovery postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_archive_recovery_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create archive recovery schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := archiveDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate archive recovery schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated archive recovery schema: %v", err)
	}
	defer pool.Close()

	organizationID := archiveInsertOrganization(t, ctx, pool, "Archive recovery", "archive-"+schema)
	foreignOrganizationID := archiveInsertOrganization(t, ctx, pool, "Foreign archive", "foreign-archive-"+schema)
	ownerID := archiveInsertUser(t, ctx, pool, "owner-"+schema+"@example.test", "Archive", "Owner")
	disabledID := archiveInsertUser(t, ctx, pool, "disabled-"+schema+"@example.test", "Disabled", "Member")
	foreignOwnerID := archiveInsertUser(t, ctx, pool, "foreign-"+schema+"@example.test", "Foreign", "Owner")
	for _, membership := range []struct {
		organizationID, userID int64
		status                 string
	}{
		{organizationID, ownerID, "active"}, {organizationID, disabledID, "disabled"}, {foreignOrganizationID, foreignOwnerID, "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner',$3)`, membership.organizationID, membership.userID, membership.status); err != nil {
			t.Fatalf("create archive membership: %v", err)
		}
	}
	contactID := archiveInsertContact(t, ctx, pool, organizationID, ownerID, "Ava", "Stone", true)
	companyID := archiveInsertCompany(t, ctx, pool, organizationID, ownerID, "Atlas Services", true)
	dealID := archiveInsertDeal(t, ctx, pool, organizationID, ownerID, companyID, contactID, true)
	taskID := archiveInsertTask(t, ctx, pool, organizationID, ownerID, "contact", contactID, true)
	foreignContactID := archiveInsertContact(t, ctx, pool, foreignOrganizationID, foreignOwnerID, "Foreign", "Record", true)

	service := modulearchiveoperations.NewService(pool)
	records, err := service.List(ctx, organizationID, modulearchiveoperations.ListQuery{Search: "ava", Limit: 50})
	if err != nil || len(records) != 1 || records[0].EntityID != contactID || records[0].EntityType != "contact" || records[0].OwnerName != "Archive Owner" {
		t.Fatalf("unexpected archived record search: records=%#v err=%v", records, err)
	}
	allRecords, err := service.List(ctx, organizationID, modulearchiveoperations.ListQuery{Limit: 50})
	if err != nil || len(allRecords) != 4 {
		t.Fatalf("expected four tenant archived records: records=%#v err=%v", allRecords, err)
	}
	if findArchivedRecord(t, allRecords, "deal").RestoreBlockedReason == "" || findArchivedRecord(t, allRecords, "task").RestoreBlockedReason == "" {
		t.Fatalf("expected dependency blocks in archive list: records=%#v", allRecords)
	}
	if _, err := service.Restore(ctx, organizationID, ownerID, "contact", foreignContactID); !errors.Is(err, modulearchiveoperations.ErrNotFound) {
		t.Fatalf("expected cross-tenant restore to look missing, got %v", err)
	}
	if _, err := service.Restore(ctx, organizationID, disabledID, "contact", contactID); !errors.Is(err, modulearchiveoperations.ErrInactiveActor) {
		t.Fatalf("expected disabled actor rejection, got %v", err)
	}
	if _, err := service.Restore(ctx, organizationID, ownerID, "task", taskID); !errors.Is(err, modulearchiveoperations.ErrConflict) || !strings.Contains(err.Error(), "linked contact") {
		t.Fatalf("expected task dependency conflict, got %v", err)
	}
	if _, err := service.Restore(ctx, organizationID, ownerID, "deal", dealID); !errors.Is(err, modulearchiveoperations.ErrConflict) || !strings.Contains(err.Error(), "linked company") {
		t.Fatalf("expected deal dependency conflict, got %v", err)
	}

	restoredContact, err := service.Restore(ctx, organizationID, ownerID, "contact", contactID)
	if err != nil || restoredContact.Label != "Ava Stone" {
		t.Fatalf("restore contact: record=%#v err=%v", restoredContact, err)
	}
	if _, err := service.Restore(ctx, organizationID, ownerID, "task", taskID); err != nil {
		t.Fatalf("restore task after contact: %v", err)
	}
	if _, err := service.Restore(ctx, organizationID, ownerID, "company", companyID); err != nil {
		t.Fatalf("restore company: %v", err)
	}
	if _, err := service.Restore(ctx, organizationID, ownerID, "deal", dealID); err != nil {
		t.Fatalf("restore deal after dependencies: %v", err)
	}
	var restoredCount, activityCount, auditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM (SELECT archived_at FROM contacts WHERE organization_id=$1 UNION ALL SELECT archived_at FROM companies WHERE organization_id=$1 UNION ALL SELECT archived_at FROM deals WHERE organization_id=$1 UNION ALL SELECT archived_at FROM tasks WHERE organization_id=$1) records WHERE archived_at IS NULL`, organizationID).Scan(&restoredCount); err != nil || restoredCount != 4 {
		t.Fatalf("expected all records restored: count=%d err=%v", restoredCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND action LIKE '%.restored'`, organizationID).Scan(&activityCount); err != nil || activityCount != 4 {
		t.Fatalf("expected restore activities: count=%d err=%v", activityCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='record.restored'`, organizationID).Scan(&auditCount); err != nil || auditCount != 4 {
		t.Fatalf("expected restore audit events: count=%d err=%v", auditCount, err)
	}

	mergedSourceID := archiveInsertContact(t, ctx, pool, organizationID, ownerID, "Merged", "Source", true)
	targetID := archiveInsertContact(t, ctx, pool, organizationID, ownerID, "Merged", "Target", false)
	if _, err := pool.Exec(ctx, `INSERT INTO duplicate_merge_operations (organization_id,created_by_user_id,entity_type,source_entity_id,target_entity_id,idempotency_key,request_sha256,source_updated_at,target_updated_at,target_applied_updated_at) VALUES ($1,$2,'contact',$3,$4,'archive-merge-history-001',$5,NOW(),NOW(),NOW())`, organizationID, ownerID, mergedSourceID, targetID, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("record duplicate merge history: %v", err)
	}
	mergedRecords, err := service.List(ctx, organizationID, modulearchiveoperations.ListQuery{EntityType: "contact", Search: "Merged Source"})
	if err != nil || len(mergedRecords) != 1 || mergedRecords[0].RestoreBlockedReason == "" {
		t.Fatalf("expected visible blocked merge source: records=%#v err=%v", mergedRecords, err)
	}
	if _, err := service.Restore(ctx, organizationID, ownerID, "contact", mergedSourceID); !errors.Is(err, modulearchiveoperations.ErrConflict) || !strings.Contains(err.Error(), "duplicate merge") {
		t.Fatalf("expected permanent merge-source conflict, got %v", err)
	}

	spamContactID := archiveInsertContact(t, ctx, pool, organizationID, ownerID, "Spam", "Candidate", true)
	var formID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms (organization_id,public_id,name,slug,title,fields_json,success_message,consent_text)
		VALUES ($1,$2,'Spam review','spam-review','Spam review','[]'::jsonb,'Thanks','I agree') RETURNING id
	`, organizationID, "lf-spam-"+schema).Scan(&formID); err != nil {
		t.Fatalf("create spam review form: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_capture_submissions (
		  organization_id,form_id,contact_id,payload_json,consent_text_snapshot,consented_at,
		  review_status,review_version,reviewed_at,reviewed_by_user_id,quarantined_contact_at
		)
		SELECT $1,$2,$3,'{}'::jsonb,'I agree',NOW(),'spam',1,NOW(),$4,archived_at
		FROM contacts WHERE organization_id=$1 AND id=$3
	`, organizationID, formID, spamContactID, ownerID); err != nil {
		t.Fatalf("create spam quarantine evidence: %v", err)
	}
	spamRecords, err := service.List(ctx, organizationID, modulearchiveoperations.ListQuery{EntityType: "contact", Search: "Spam Candidate"})
	if err != nil || len(spamRecords) != 1 || !strings.Contains(spamRecords[0].RestoreBlockedReason, "Lead Forms") {
		t.Fatalf("expected visible spam recovery boundary: records=%#v err=%v", spamRecords, err)
	}
	if _, err := service.Restore(ctx, organizationID, ownerID, "contact", spamContactID); !errors.Is(err, modulearchiveoperations.ErrConflict) || !strings.Contains(err.Error(), "Lead Forms") {
		t.Fatalf("generic archive restore bypassed spam quarantine: %v", err)
	}
}

func archiveInsertOrganization(t *testing.T, ctx context.Context, pool *moduledb.Pool, name, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ($1,$2) RETURNING id`, name, slug).Scan(&id); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	return id
}

func archiveInsertUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email, firstName, lastName string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'test-hash',$2,$3) RETURNING id`, email, firstName, lastName).Scan(&id); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func archiveInsertContact(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID int64, firstName, lastName string, archived bool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,status,owner_user_id,archived_at) VALUES ($1,$2,$3,'lead',$4,CASE WHEN $5 THEN NOW() ELSE NULL END) RETURNING id`, organizationID, firstName, lastName, ownerID, archived).Scan(&id); err != nil {
		t.Fatalf("create contact: %v", err)
	}
	return id
}

func archiveInsertCompany(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID int64, name string, archived bool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id,archived_at) VALUES ($1,$2,'prospect',$3,CASE WHEN $4 THEN NOW() ELSE NULL END) RETURNING id`, organizationID, name, ownerID, archived).Scan(&id); err != nil {
		t.Fatalf("create company: %v", err)
	}
	return id
}

func archiveInsertDeal(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID, companyID, contactID int64, archived bool) int64 {
	t.Helper()
	var pipelineID, stageID, dealID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, organizationID, ownerID).Scan(&pipelineID); err != nil {
		t.Fatalf("create deal pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position) VALUES ($1,$2,'Qualified',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create deal stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deals (organization_id,company_id,primary_contact_id,stage_id,name,status,owner_user_id,archived_at) VALUES ($1,$2,$3,$4,'Pilot deal','open',$5,CASE WHEN $6 THEN NOW() ELSE NULL END) RETURNING id`, organizationID, companyID, contactID, stageID, ownerID, archived).Scan(&dealID); err != nil {
		t.Fatalf("create deal: %v", err)
	}
	return dealID
}

func archiveInsertTask(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID int64, entityType string, entityID int64, archived bool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,assigned_to_user_id,created_by_user_id,archived_at) VALUES ($1,$2,$3,'Follow up','open',$4,$4,CASE WHEN $5 THEN NOW() ELSE NULL END) RETURNING id`, organizationID, entityType, entityID, ownerID, archived).Scan(&id); err != nil {
		t.Fatalf("create task: %v", err)
	}
	return id
}

func archiveDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse archive database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func findArchivedRecord(t *testing.T, records []modulearchiveoperations.Record, entityType string) modulearchiveoperations.Record {
	t.Helper()
	for _, record := range records {
		if record.EntityType == entityType {
			return record
		}
	}
	t.Fatalf("missing archived %s record", entityType)
	return modulearchiveoperations.Record{}
}
