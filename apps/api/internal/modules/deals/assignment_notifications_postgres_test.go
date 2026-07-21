package deals_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
)

func TestDealAssignmentsAreTransactionalPreferenceAwareAndIdempotentAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to deal assignment postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_deal_assignment_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create deal assignment schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := winLossDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate deal assignment schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to deal assignment schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorID, assigneeID, otherAssigneeID, foreignAssigneeID, pipelineID, stageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Deal assignment',$1) RETURNING id`, "deal-assignment-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign deal assignment',$1) RETURNING id`, "foreign-deal-assignment-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	for _, user := range []struct {
		email     string
		firstName string
		id        *int64
	}{
		{"actor-" + schema + "@example.test", "Actor", &actorID},
		{"assignee-" + schema + "@example.test", "Assignee", &assigneeID},
		{"other-" + schema + "@example.test", "Other", &otherAssigneeID},
		{"foreign-" + schema + "@example.test", "Foreign", &foreignAssigneeID},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'test-hash',$2,'User') RETURNING id`, user.email, user.firstName).Scan(user.id); err != nil {
			t.Fatalf("create %s user: %v", user.firstName, err)
		}
		membershipOrganizationID := organizationID
		if user.firstName == "Foreign" {
			membershipOrganizationID = foreignOrganizationID
		}
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'member')`, membershipOrganizationID, *user.id); err != nil {
			t.Fatalf("create %s membership: %v", user.firstName, err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, organizationID, actorID).Scan(&pipelineID); err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position) VALUES ($1,$2,'Open',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create stage: %v", err)
	}

	service := moduledeals.NewService(pool)
	created, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Assigned deal", StageID: stageID, OwnerUserID: assigneeID})
	if err != nil {
		t.Fatalf("create assigned deal: %v", err)
	}
	dealID := created.Summary.ID
	assertDealAssignmentNotification(t, ctx, pool, organizationID, assigneeID, dealID, 1, "deal:"+fmt.Sprint(dealID)+":assigned:"+fmt.Sprint(assigneeID)+":v0")
	if _, err := service.Update(ctx, organizationID, dealID, actorID, moduledeals.UpdateInput{Name: "Cross-tenant owner must fail", OwnerUserID: foreignAssigneeID}); !errors.Is(err, moduledeals.ErrInvalidAssignee) {
		t.Fatalf("foreign deal assignee returned %v", err)
	}
	var foreignNotifications int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 OR user_id=$2`, foreignOrganizationID, foreignAssigneeID).Scan(&foreignNotifications); err != nil || foreignNotifications != 0 {
		t.Fatalf("foreign assignment emitted notifications: count=%d err=%v", foreignNotifications, err)
	}

	if _, err := service.Update(ctx, organizationID, dealID, actorID, moduledeals.UpdateInput{Name: "Assigned deal renamed", OwnerUserID: assigneeID}); err != nil {
		t.Fatalf("save unchanged owner: %v", err)
	}
	assertDealAssignmentNotification(t, ctx, pool, organizationID, assigneeID, dealID, 1, "deal:"+fmt.Sprint(dealID)+":assigned:"+fmt.Sprint(assigneeID)+":v0")

	if _, err := service.Update(ctx, organizationID, dealID, actorID, moduledeals.UpdateInput{Name: "Assigned deal renamed", OwnerUserID: otherAssigneeID}); err != nil {
		t.Fatalf("change deal owner: %v", err)
	}
	assertDealAssignmentNotification(t, ctx, pool, organizationID, otherAssigneeID, dealID, 1, "deal:"+fmt.Sprint(dealID)+":assigned:"+fmt.Sprint(otherAssigneeID)+":v1")

	if _, err := pool.Exec(ctx, `UPDATE users SET preferences=jsonb_build_object('notifyOnDealAssigned',FALSE) WHERE id=$1`, assigneeID); err != nil {
		t.Fatalf("disable deal assignment preference: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, dealID, actorID, moduledeals.UpdateInput{Name: "Assigned deal renamed", OwnerUserID: assigneeID}); err != nil {
		t.Fatalf("assign to opted-out owner: %v", err)
	}
	assertDealAssignmentNotification(t, ctx, pool, organizationID, assigneeID, dealID, 1, "deal:"+fmt.Sprint(dealID)+":assigned:"+fmt.Sprint(assigneeID)+":v0")

	if _, err := pool.Exec(ctx, `UPDATE users SET preferences=jsonb_build_object('notifyOnDealAssigned',TRUE) WHERE id=$1`, assigneeID); err != nil {
		t.Fatalf("enable deal assignment preference: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, dealID, actorID, moduledeals.UpdateInput{Name: "Assigned deal renamed", OwnerUserID: otherAssigneeID}); err != nil {
		t.Fatalf("assign deal away again: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, dealID, actorID, moduledeals.UpdateInput{Name: "Assigned deal renamed", OwnerUserID: assigneeID}); err != nil {
		t.Fatalf("assign deal back: %v", err)
	}
	assertDealAssignmentNotification(t, ctx, pool, organizationID, assigneeID, dealID, 2, "deal:"+fmt.Sprint(dealID)+":assigned:"+fmt.Sprint(assigneeID)+":v4")

	if _, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Self assigned", StageID: stageID, OwnerUserID: actorID}); err != nil {
		t.Fatalf("create self-assigned deal: %v", err)
	}
	var actorNotifications int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id=$1 AND user_id=$2 AND event_type='deal.assigned'`, organizationID, actorID).Scan(&actorNotifications); err != nil || actorNotifications != 0 {
		t.Fatalf("self assignment emitted notification: count=%d err=%v", actorNotifications, err)
	}

	if _, err := pool.Exec(ctx, `DROP TABLE notifications`); err != nil {
		t.Fatalf("remove notification sink for rollback proof: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, dealID, actorID, moduledeals.UpdateInput{Name: "Must roll back", OwnerUserID: otherAssigneeID}); err == nil {
		t.Fatal("expected deal update to fail with unavailable transactional notification sink")
	}
	var name string
	var ownerUserID int64
	var assignmentVersion int
	if err := pool.QueryRow(ctx, `SELECT name,owner_user_id,owner_assignment_version FROM deals WHERE organization_id=$1 AND id=$2`, organizationID, dealID).Scan(&name, &ownerUserID, &assignmentVersion); err != nil {
		t.Fatalf("load rolled-back deal: %v", err)
	}
	if name != "Assigned deal renamed" || ownerUserID != assigneeID || assignmentVersion != 4 {
		t.Fatalf("failed notification left deal mutation behind: name=%q owner=%d version=%d", name, ownerUserID, assignmentVersion)
	}
	if _, err := service.Create(ctx, organizationID, actorID, moduledeals.CreateInput{Name: "Must not exist", StageID: stageID, OwnerUserID: assigneeID}); err == nil {
		t.Fatal("expected deal create to fail with unavailable transactional notification sink")
	}
	var failedCreateCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM deals WHERE organization_id=$1 AND name='Must not exist'`, organizationID).Scan(&failedCreateCount); err != nil || failedCreateCount != 0 {
		t.Fatalf("failed notification left created deal behind: count=%d err=%v", failedCreateCount, err)
	}
}

func assertDealAssignmentNotification(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, userID, dealID int64, expectedCount int, expectedLatestKey string) {
	t.Helper()
	var count int
	var latestKey, eventType, entityType string
	var entityID int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*),COALESCE(MAX(idempotency_key) FILTER (WHERE id=(SELECT MAX(id) FROM notifications WHERE organization_id=$1 AND user_id=$2 AND event_type='deal.assigned')),''),
		       COALESCE(MAX(event_type),''),COALESCE(MAX(entity_type),''),COALESCE(MAX(entity_id),0)
		FROM notifications
		WHERE organization_id=$1 AND user_id=$2 AND event_type='deal.assigned'
	`, organizationID, userID).Scan(&count, &latestKey, &eventType, &entityType, &entityID); err != nil {
		t.Fatalf("load deal assignment notification: %v", err)
	}
	if count != expectedCount || latestKey != expectedLatestKey || eventType != "deal.assigned" || entityType != "deal" || entityID != dealID {
		t.Fatalf("unexpected deal assignment notifications: count=%d key=%q event=%q entity=%q/%d", count, latestKey, eventType, entityType, entityID)
	}
}
