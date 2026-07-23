package leadaudiences_test

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
	moduleleadaudiences "github.com/aeml/open_crm/apps/api/internal/modules/leadaudiences"
	"github.com/jackc/pgx/v5"
)

func TestLeadAudiencesAreBoundedAuthorizedAtomicAndTenantSafe(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead audience postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_audiences_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead audience schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := leadAudienceDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead audience schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated lead audience schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Audience team',$1) RETURNING id`, "audience-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create audience organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign audience team',$1) RETURNING id`, "foreign-audience-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign audience organization: %v", err)
	}

	users := map[string]int64{}
	for _, actor := range []string{"owner", "admin", "member", "viewer", "disabled", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'test-hash','Audience',$2) RETURNING id
		`, actor+"-"+schema+"@example.test", actor).Scan(&userID); err != nil {
			t.Fatalf("create %s audience user: %v", actor, err)
		}
		users[actor] = userID
	}
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		role           string
		status         string
	}{
		{organizationID, users["owner"], "owner", "active"},
		{organizationID, users["admin"], "admin", "active"},
		{organizationID, users["member"], "member", "active"},
		{organizationID, users["viewer"], "viewer", "active"},
		{organizationID, users["disabled"], "admin", "disabled"},
		{foreignOrganizationID, users["foreign"], "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,$3,$4)`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("create audience membership: %v", err)
		}
	}

	for index, contact := range []struct {
		organizationID int64
		status         string
		archived       bool
	}{
		{organizationID, "lead", false},
		{organizationID, "lead", false},
		{organizationID, "prospect", false},
		{organizationID, "lead", true},
		{foreignOrganizationID, "lead", false},
		{foreignOrganizationID, "lead", false},
		{foreignOrganizationID, "lead", false},
	} {
		var archivedAt any
		if contact.archived {
			archivedAt = time.Now().UTC()
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO contacts (organization_id,first_name,last_name,email,status,archived_at)
			VALUES ($1,$2,'Contact',$3,$4,$5)
		`, contact.organizationID, fmt.Sprintf("Audience %d", index+1), fmt.Sprintf("audience-%d-%s@example.test", index+1, schema), contact.status, archivedAt); err != nil {
			t.Fatalf("create audience contact: %v", err)
		}
	}

	service := moduleleadaudiences.NewService(pool)
	blocker, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin contact-count blocker: %v", err)
	}
	if _, err := blocker.Exec(ctx, `LOCK TABLE contacts IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock contacts for timeout: %v", err)
	}
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 150*time.Millisecond)
	_, createErr := service.Create(timeoutCtx, organizationID, users["owner"], validAudienceInput("Rolled back audience", "lead"))
	timeoutCancel()
	if !errors.Is(createErr, moduleleadaudiences.ErrQueryTimeout) {
		t.Fatalf("blocked post-insert count returned %v", createErr)
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatalf("release contact-count blocker: %v", err)
	}
	var rolledBackCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_audiences WHERE organization_id=$1 AND name='Rolled back audience'`, organizationID).Scan(&rolledBackCount); err != nil || rolledBackCount != 0 {
		t.Fatalf("timed-out audience create left state: count=%d err=%v", rolledBackCount, err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_audiences (organization_id,name,filters_json,is_active,created_by_user_id,updated_by_user_id)
		SELECT $1,'Audience ' || lpad(series::text,3,'0'),'{"status":"lead"}'::jsonb,series=1,$2,$2
		FROM generate_series(1,99) AS series
	`, organizationID, users["owner"]); err != nil {
		t.Fatalf("seed lead audiences: %v", err)
	}
	var foreignAudienceID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_audiences (organization_id,name,filters_json,is_active,created_by_user_id,updated_by_user_id)
		VALUES ($1,'Foreign audience','{"status":"lead"}'::jsonb,TRUE,$2,$2) RETURNING id
	`, foreignOrganizationID, users["foreign"]).Scan(&foreignAudienceID); err != nil {
		t.Fatalf("seed foreign audience: %v", err)
	}

	audiences, err := service.ListByOrganization(ctx, organizationID)
	if err != nil || len(audiences) != 99 || audiences[0].MemberCount != 2 {
		t.Fatalf("list tenant lead audiences: count=%d first=%+v err=%v", len(audiences), firstAudience(audiences), err)
	}
	foreignAudiences, err := service.ListByOrganization(ctx, foreignOrganizationID)
	if err != nil || len(foreignAudiences) != 1 || foreignAudiences[0].ID != foreignAudienceID || foreignAudiences[0].MemberCount != 3 {
		t.Fatalf("lead audience list crossed tenants: audiences=%+v err=%v", foreignAudiences, err)
	}
	for actor, userID := range map[string]int64{
		"member": users["member"], "viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"],
	} {
		if _, err := service.Create(ctx, organizationID, userID, validAudienceInput("Forbidden "+actor, "lead")); !errors.Is(err, moduleleadaudiences.ErrForbidden) {
			t.Fatalf("%s actor created a lead audience: %v", actor, err)
		}
	}

	type createResult struct {
		audience moduleleadaudiences.Audience
		err      error
	}
	start := make(chan struct{})
	results := make(chan createResult, 2)
	for index, actorID := range []int64{users["owner"], users["admin"]} {
		go func(index int, actorID int64) {
			<-start
			audience, err := service.Create(ctx, organizationID, actorID, validAudienceInput(fmt.Sprintf("Concurrent final audience %d", index+1), "lead"))
			results <- createResult{audience: audience, err: err}
		}(index, actorID)
	}
	close(start)
	var createdAudience moduleleadaudiences.Audience
	var succeeded, limited int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			succeeded++
			createdAudience = result.audience
		case errors.Is(result.err, moduleleadaudiences.ErrAudienceLimit):
			limited++
		default:
			t.Fatalf("unexpected concurrent lead audience create result: %v", result.err)
		}
	}
	if succeeded != 1 || limited != 1 || createdAudience.ID <= 0 || createdAudience.MemberCount != 2 {
		t.Fatalf("lead audience capacity was not serialized: succeeded=%d limited=%d audience=%+v", succeeded, limited, createdAudience)
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validAudienceInput("Audience 101", "lead")); !errors.Is(err, moduleleadaudiences.ErrAudienceLimit) {
		t.Fatalf("lead audience capacity returned %v", err)
	}
	if _, err := service.Update(ctx, organizationID, createdAudience.ID, users["member"], validAudienceInput(createdAudience.Name, "prospect")); !errors.Is(err, moduleleadaudiences.ErrForbidden) {
		t.Fatalf("ordinary member updated a lead audience: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, foreignAudienceID, users["owner"], validAudienceInput("Foreign audience", "lead")); !errors.Is(err, moduleleadaudiences.ErrNotFound) {
		t.Fatalf("cross-tenant audience update returned %v", err)
	}
	updated, err := service.Update(ctx, organizationID, createdAudience.ID, users["admin"], validAudienceInput(createdAudience.Name, "prospect"))
	if err != nil || updated.MemberCount != 1 || updated.Filters["status"] != "prospect" {
		t.Fatalf("update retained audience at capacity: audience=%+v err=%v", updated, err)
	}
	audiences, err = service.ListByOrganization(ctx, organizationID)
	if err != nil || len(audiences) != moduleleadaudiences.MaxAudiencesPerOrganization {
		t.Fatalf("complete audience catalog at capacity: count=%d err=%v", len(audiences), err)
	}

	preview, err := service.Preview(ctx, organizationID, map[string]string{"status": "lead"})
	if err != nil || preview.MemberCount != 2 {
		t.Fatalf("preview tenant audience: preview=%+v err=%v", preview, err)
	}
	foreignPreview, err := service.Preview(ctx, foreignOrganizationID, map[string]string{"status": "lead"})
	if err != nil || foreignPreview.MemberCount != 3 {
		t.Fatalf("preview foreign tenant audience: preview=%+v err=%v", foreignPreview, err)
	}
	expiredCtx, expiredCancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer expiredCancel()
	if _, err := service.Preview(expiredCtx, organizationID, map[string]string{"status": "lead"}); !errors.Is(err, moduleleadaudiences.ErrQueryTimeout) {
		t.Fatalf("expired lead audience preview returned %v", err)
	}

	var audienceCount, foreignAudienceCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_audiences WHERE organization_id=$1`, organizationID).Scan(&audienceCount); err != nil {
		t.Fatalf("count retained lead audiences: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_audiences WHERE organization_id=$1`, foreignOrganizationID).Scan(&foreignAudienceCount); err != nil {
		t.Fatalf("count foreign lead audiences: %v", err)
	}
	if audienceCount != moduleleadaudiences.MaxAudiencesPerOrganization || foreignAudienceCount != 1 {
		t.Fatalf("unexpected retained audience state: tenant=%d foreign=%d", audienceCount, foreignAudienceCount)
	}
}

func validAudienceInput(name, status string) moduleleadaudiences.Input {
	active := false
	return moduleleadaudiences.Input{
		Name: name, Description: "Bounded lead audience", Filters: map[string]string{"status": status}, IsActive: &active,
	}
}

func firstAudience(audiences []moduleleadaudiences.Audience) moduleleadaudiences.Audience {
	if len(audiences) == 0 {
		return moduleleadaudiences.Audience{}
	}
	return audiences[0]
}

func leadAudienceDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse lead audience database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
