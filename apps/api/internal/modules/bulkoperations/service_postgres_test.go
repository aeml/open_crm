package bulkoperations_test

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
	modulebulkoperations "github.com/aeml/open_crm/apps/api/internal/modules/bulkoperations"
)

func TestBulkOperationsAreIdempotentTenantSafeAndChangeAwareAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to bulk operations test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_bulk_operations_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create bulk operations schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := bulkOperationsDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate bulk operations schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated bulk operations schema: %v", err)
	}
	defer pool.Close()

	organizationID := insertOrganization(t, ctx, pool, "Bulk operations", "bulk-"+schema)
	foreignOrganizationID := insertOrganization(t, ctx, pool, "Foreign bulk operations", "foreign-bulk-"+schema)
	ownerID := insertUser(t, ctx, pool, "owner-"+schema+"@example.test", "Bulk", "Owner")
	memberID := insertUser(t, ctx, pool, "member-"+schema+"@example.test", "Bulk", "Member")
	disabledID := insertUser(t, ctx, pool, "disabled-"+schema+"@example.test", "Disabled", "Member")
	foreignID := insertUser(t, ctx, pool, "foreign-"+schema+"@example.test", "Foreign", "Owner")
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		role           string
		status         string
	}{
		{organizationID, ownerID, "owner", "active"},
		{organizationID, memberID, "member", "active"},
		{organizationID, disabledID, "member", "disabled"},
		{foreignOrganizationID, foreignID, "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role, membership_status) VALUES ($1, $2, $3, $4)`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("create bulk operations membership: %v", err)
		}
	}

	contactOne := insertContact(t, ctx, pool, organizationID, ownerID, "Ava", "Stone")
	contactTwo := insertContact(t, ctx, pool, organizationID, ownerID, "Mina", "Park")
	foreignContact := insertContact(t, ctx, pool, foreignOrganizationID, foreignID, "Foreign", "Contact")
	companyID := insertCompany(t, ctx, pool, organizationID, ownerID)
	dealID := insertDeal(t, ctx, pool, organizationID, ownerID)
	taskID := insertTask(t, ctx, pool, organizationID, ownerID, contactOne)

	service := modulebulkoperations.NewService(pool)
	reassign, err := service.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contact", Action: "reassign",
		TargetUserID: &memberID, EntityIDs: []int64{contactTwo, contactOne, contactTwo}, IdempotencyKey: "bulk-contact-reassign-001",
	})
	if err != nil || reassign.TargetCount != 2 || reassign.ChangedCount != 2 || reassign.Status != "completed" {
		t.Fatalf("execute contact reassignment: operation=%#v err=%v", reassign, err)
	}
	assertOwner(t, ctx, pool, "contacts", organizationID, contactOne, memberID)
	assertOwner(t, ctx, pool, "contacts", organizationID, contactTwo, memberID)
	replayed, err := service.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contact", Action: "reassign",
		TargetUserID: &memberID, EntityIDs: []int64{contactOne, contactTwo}, IdempotencyKey: "bulk-contact-reassign-001",
	})
	if err != nil || !replayed.Replayed || replayed.ID != reassign.ID {
		t.Fatalf("expected idempotent bulk replay: operation=%#v err=%v", replayed, err)
	}
	if _, err := service.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contact", Action: "archive",
		EntityIDs: []int64{contactOne}, IdempotencyKey: "bulk-contact-reassign-001",
	}); !errors.Is(err, modulebulkoperations.ErrIdempotencyConflict) {
		t.Fatalf("expected bulk idempotency conflict, got %v", err)
	}

	if _, err := service.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contact", Action: "archive",
		EntityIDs: []int64{contactOne, foreignContact}, IdempotencyKey: "bulk-tenant-mix-001",
	}); !errors.Is(err, modulebulkoperations.ErrNotFound) {
		t.Fatalf("expected strict mixed-tenant rejection, got %v", err)
	}
	var contactOneArchived bool
	if err := pool.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM contacts WHERE organization_id = $1 AND id = $2`, organizationID, contactOne).Scan(&contactOneArchived); err != nil || contactOneArchived {
		t.Fatalf("mixed-tenant rejection changed own record: archived=%v err=%v", contactOneArchived, err)
	}
	if _, err := service.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contact", Action: "reassign",
		TargetUserID: &foreignID, EntityIDs: []int64{contactOne}, IdempotencyKey: "bulk-foreign-assignee-001",
	}); !errors.Is(err, modulebulkoperations.ErrInvalidAssignee) {
		t.Fatalf("expected foreign assignee rejection, got %v", err)
	}
	if _, err := service.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: disabledID, EntityType: "contact", Action: "archive",
		EntityIDs: []int64{contactOne}, IdempotencyKey: "bulk-disabled-actor-001",
	}); !errors.Is(err, modulebulkoperations.ErrInactiveActor) {
		t.Fatalf("expected disabled actor rejection, got %v", err)
	}
	foreignHistory, err := service.List(ctx, foreignOrganizationID, "", 20)
	if err != nil || len(foreignHistory) != 0 {
		t.Fatalf("foreign tenant saw bulk history: history=%#v err=%v", foreignHistory, err)
	}
	if _, err := service.Rollback(ctx, foreignOrganizationID, foreignID, reassign.ID); !errors.Is(err, modulebulkoperations.ErrNotFound) {
		t.Fatalf("expected cross-tenant rollback miss, got %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE contacts SET status = 'prospect', updated_at = NOW() + INTERVAL '1 second' WHERE organization_id = $1 AND id = $2`, organizationID, contactTwo); err != nil {
		t.Fatalf("modify bulk target before rollback: %v", err)
	}
	partial, err := service.Rollback(ctx, organizationID, ownerID, reassign.ID)
	if err != nil || partial.Status != "partially_rolled_back" || partial.RolledBackCount != 1 || partial.RollbackSkippedCount != 1 {
		t.Fatalf("expected change-aware partial rollback: operation=%#v err=%v", partial, err)
	}
	assertOwner(t, ctx, pool, "contacts", organizationID, contactOne, ownerID)
	assertOwner(t, ctx, pool, "contacts", organizationID, contactTwo, memberID)
	rollbackReplay, err := service.Rollback(ctx, organizationID, ownerID, reassign.ID)
	if err != nil || !rollbackReplay.Replayed {
		t.Fatalf("expected idempotent rollback replay: operation=%#v err=%v", rollbackReplay, err)
	}

	archiveCompany, err := service.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: memberID, EntityType: "company", Action: "archive",
		EntityIDs: []int64{companyID}, IdempotencyKey: "bulk-company-archive-001",
	})
	if err != nil || archiveCompany.ChangedCount != 1 {
		t.Fatalf("archive company: operation=%#v err=%v", archiveCompany, err)
	}
	if _, err := service.Rollback(ctx, organizationID, memberID, archiveCompany.ID); err != nil {
		t.Fatalf("restore archived company through rollback: %v", err)
	}
	var companyActive bool
	if err := pool.QueryRow(ctx, `SELECT archived_at IS NULL FROM companies WHERE organization_id = $1 AND id = $2`, organizationID, companyID).Scan(&companyActive); err != nil || !companyActive {
		t.Fatalf("company rollback did not restore active record: active=%v err=%v", companyActive, err)
	}

	_, err = service.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "deal", Action: "set_status", ActionValue: "won",
		EntityIDs: []int64{dealID}, IdempotencyKey: "bulk-deal-status-001",
	})
	if !errors.Is(err, modulebulkoperations.ErrInvalidInput) {
		t.Fatalf("deal outcome must require a stage transition with close context, got %v", err)
	}
	var restoredDealStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM deals WHERE organization_id = $1 AND id = $2`, organizationID, dealID).Scan(&restoredDealStatus); err != nil || restoredDealStatus != "open" {
		t.Fatalf("invalid bulk outcome changed deal status, status=%q err=%v", restoredDealStatus, err)
	}
	var legacyDealStatusOperationID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO bulk_operations (
			organization_id,created_by_user_id,entity_type,action,action_value,
			idempotency_key,request_sha256,target_count,changed_count
		) VALUES ($1,$2,'deal','set_status','won','legacy-deal-status',repeat('a',64),1,1)
		RETURNING id
	`, organizationID, ownerID).Scan(&legacyDealStatusOperationID); err != nil {
		t.Fatalf("record legacy deal-status operation: %v", err)
	}
	if _, err := service.Rollback(ctx, organizationID, ownerID, legacyDealStatusOperationID); !errors.Is(err, modulebulkoperations.ErrConflict) {
		t.Fatalf("legacy deal-status rollback bypassed stage outcome controls: %v", err)
	}

	taskStatus, err := service.Execute(ctx, modulebulkoperations.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "task", Action: "set_status", ActionValue: "completed",
		EntityIDs: []int64{taskID}, IdempotencyKey: "bulk-task-status-001",
	})
	if err != nil || taskStatus.ChangedCount != 1 {
		t.Fatalf("complete task in bulk: operation=%#v err=%v", taskStatus, err)
	}
	var completed bool
	if err := pool.QueryRow(ctx, `SELECT status = 'completed' AND completed_at IS NOT NULL FROM tasks WHERE organization_id = $1 AND id = $2`, organizationID, taskID).Scan(&completed); err != nil || !completed {
		t.Fatalf("bulk task completion incomplete: completed=%v err=%v", completed, err)
	}
	if _, err := service.Rollback(ctx, organizationID, ownerID, taskStatus.ID); err != nil {
		t.Fatalf("rollback task completion: %v", err)
	}
	var taskRestored bool
	if err := pool.QueryRow(ctx, `SELECT status = 'open' AND completed_at IS NULL FROM tasks WHERE organization_id = $1 AND id = $2`, organizationID, taskID).Scan(&taskRestored); err != nil || !taskRestored {
		t.Fatalf("bulk task rollback incomplete: restored=%v err=%v", taskRestored, err)
	}

	contactHistory, err := service.List(ctx, organizationID, "contact", 20)
	if err != nil || len(contactHistory) != 1 || contactHistory[0].ID != reassign.ID {
		t.Fatalf("unexpected entity-filtered bulk history: history=%#v err=%v", contactHistory, err)
	}
	var activityCount, auditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activities WHERE organization_id = $1 AND action LIKE '%.bulk_%'`, organizationID).Scan(&activityCount); err != nil || activityCount != 7 {
		t.Fatalf("expected per-record apply/rollback activity, count=%d err=%v", activityCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id = $1 AND event_type IN ('bulk_operation.completed', 'bulk_operation.rolled_back')`, organizationID).Scan(&auditCount); err != nil || auditCount != 6 {
		t.Fatalf("expected aggregate bulk audit events, count=%d err=%v", auditCount, err)
	}
}

func insertOrganization(t *testing.T, ctx context.Context, pool *moduledb.Pool, name, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`, name, slug).Scan(&id); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	return id
}

func insertUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email, firstName, lastName string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'test-hash', $2, $3) RETURNING id`, email, firstName, lastName).Scan(&id); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func insertContact(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID int64, firstName, lastName string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name, status, owner_user_id) VALUES ($1, $2, $3, 'lead', $4) RETURNING id`, organizationID, firstName, lastName, ownerID).Scan(&id); err != nil {
		t.Fatalf("create contact: %v", err)
	}
	return id
}

func insertCompany(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id, name, status, owner_user_id) VALUES ($1, 'Atlas Services', 'prospect', $2) RETURNING id`, organizationID, ownerID).Scan(&id); err != nil {
		t.Fatalf("create company: %v", err)
	}
	return id
}

func insertDeal(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID int64) int64 {
	t.Helper()
	var pipelineID, stageID, dealID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id, name, position, is_default, created_by_user_id) VALUES ($1, 'Sales', 1, TRUE, $2) RETURNING id`, organizationID, ownerID).Scan(&pipelineID); err != nil {
		t.Fatalf("create deal pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id, pipeline_id, name, position) VALUES ($1, $2, 'Qualified', 1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create deal stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deals (organization_id, stage_id, name, status, owner_user_id) VALUES ($1, $2, 'Pilot deal', 'open', $3) RETURNING id`, organizationID, stageID, ownerID).Scan(&dealID); err != nil {
		t.Fatalf("create deal: %v", err)
	}
	return dealID
}

func insertTask(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID, contactID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO tasks (organization_id, entity_type, entity_id, title, status, assigned_to_user_id, created_by_user_id) VALUES ($1, 'contact', $2, 'Follow up', 'open', $3, $3) RETURNING id`, organizationID, contactID, ownerID).Scan(&id); err != nil {
		t.Fatalf("create task: %v", err)
	}
	return id
}

func assertOwner(t *testing.T, ctx context.Context, pool *moduledb.Pool, table string, organizationID, entityID, expectedOwnerID int64) {
	t.Helper()
	ownerColumn := "owner_user_id"
	if table == "tasks" {
		ownerColumn = "assigned_to_user_id"
	}
	var ownerID int64
	if err := pool.QueryRow(ctx, `SELECT `+ownerColumn+` FROM `+table+` WHERE organization_id = $1 AND id = $2`, organizationID, entityID).Scan(&ownerID); err != nil || ownerID != expectedOwnerID {
		t.Fatalf("unexpected %s owner: got=%d want=%d err=%v", table, ownerID, expectedOwnerID, err)
	}
}

func bulkOperationsDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse bulk operations database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
