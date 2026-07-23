package marketingcampaigns_test

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
	modulemarketingcampaigns "github.com/aeml/open_crm/apps/api/internal/modules/marketingcampaigns"
	"github.com/jackc/pgx/v5"
)

func TestMarketingCampaignsAreBoundedAuthorizedAtomicAndTenantSafe(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to marketing campaign postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_marketing_campaigns_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create marketing campaign schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := marketingCampaignDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate marketing campaign schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated marketing campaign schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Marketing team',$1) RETURNING id`, "marketing-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create marketing organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign marketing team',$1) RETURNING id`, "foreign-marketing-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign marketing organization: %v", err)
	}

	users := map[string]int64{}
	for _, actor := range []string{"owner", "admin", "member", "viewer", "disabled", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'test-hash','Marketing',$2) RETURNING id
		`, actor+"-"+schema+"@example.test", actor).Scan(&userID); err != nil {
			t.Fatalf("create %s marketing user: %v", actor, err)
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
			t.Fatalf("create marketing membership: %v", err)
		}
	}

	for index, contact := range []struct {
		organizationID int64
		status         string
	}{
		{organizationID, "lead"},
		{organizationID, "lead"},
		{organizationID, "prospect"},
		{foreignOrganizationID, "lead"},
		{foreignOrganizationID, "lead"},
		{foreignOrganizationID, "lead"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO contacts (organization_id,first_name,last_name,email,status)
			VALUES ($1,$2,'Contact',$3,$4)
		`, contact.organizationID, fmt.Sprintf("Marketing %d", index+1), fmt.Sprintf("marketing-%d-%s@example.test", index+1, schema), contact.status); err != nil {
			t.Fatalf("create marketing contact: %v", err)
		}
	}

	var leadAudienceID, prospectAudienceID, inactiveAudienceID, foreignAudienceID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_audiences (organization_id,name,filters_json,is_active,created_by_user_id,updated_by_user_id)
		VALUES ($1,'Local leads','{"status":"lead"}'::jsonb,TRUE,$2,$2) RETURNING id
	`, organizationID, users["owner"]).Scan(&leadAudienceID); err != nil {
		t.Fatalf("create lead audience: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_audiences (organization_id,name,filters_json,is_active,created_by_user_id,updated_by_user_id)
		VALUES ($1,'Local prospects','{"status":"prospect"}'::jsonb,TRUE,$2,$2) RETURNING id
	`, organizationID, users["owner"]).Scan(&prospectAudienceID); err != nil {
		t.Fatalf("create prospect audience: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_audiences (organization_id,name,filters_json,is_active,created_by_user_id,updated_by_user_id)
		VALUES ($1,'Inactive leads','{"status":"lead"}'::jsonb,FALSE,$2,$2) RETURNING id
	`, organizationID, users["owner"]).Scan(&inactiveAudienceID); err != nil {
		t.Fatalf("create inactive lead audience: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_audiences (organization_id,name,filters_json,is_active,created_by_user_id,updated_by_user_id)
		VALUES ($1,'Foreign leads','{"status":"lead"}'::jsonb,TRUE,$2,$2) RETURNING id
	`, foreignOrganizationID, users["foreign"]).Scan(&foreignAudienceID); err != nil {
		t.Fatalf("create foreign lead audience: %v", err)
	}

	service := modulemarketingcampaigns.NewService(pool)
	blocker, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin marketing recipient-count blocker: %v", err)
	}
	if _, err := blocker.Exec(ctx, `LOCK TABLE contacts IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock contacts for marketing timeout: %v", err)
	}
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 150*time.Millisecond)
	_, createErr := service.Create(timeoutCtx, organizationID, users["owner"], validMarketingCampaignInput("Rolled back campaign", leadAudienceID))
	timeoutCancel()
	if !errors.Is(createErr, modulemarketingcampaigns.ErrQueryTimeout) {
		t.Fatalf("blocked marketing audience snapshot returned %v", createErr)
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatalf("release marketing recipient-count blocker: %v", err)
	}
	var rolledBackCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM marketing_email_campaigns WHERE organization_id=$1 AND name='Rolled back campaign'`, organizationID).Scan(&rolledBackCount); err != nil || rolledBackCount != 0 {
		t.Fatalf("timed-out marketing campaign left state: count=%d err=%v", rolledBackCount, err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO marketing_email_campaigns (organization_id,audience_id,name,subject,body,status,recipient_count,created_by_user_id,updated_by_user_id)
		SELECT $1,$2,'Campaign ' || lpad(series::text,3,'0'),'Subject','Body','draft',2,$3,$3
		FROM generate_series(1,99) AS series
	`, organizationID, leadAudienceID, users["owner"]); err != nil {
		t.Fatalf("seed marketing campaigns: %v", err)
	}
	var foreignCampaignID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO marketing_email_campaigns (organization_id,audience_id,name,subject,body,status,recipient_count,created_by_user_id,updated_by_user_id)
		VALUES ($1,$2,'Foreign campaign','Subject','Body','draft',3,$3,$3) RETURNING id
	`, foreignOrganizationID, foreignAudienceID, users["foreign"]).Scan(&foreignCampaignID); err != nil {
		t.Fatalf("seed foreign marketing campaign: %v", err)
	}

	campaigns, err := service.ListByOrganization(ctx, organizationID)
	if err != nil || len(campaigns) != 99 || campaigns[0].Analytics.RecipientCount != 2 {
		t.Fatalf("list tenant marketing campaigns: count=%d first=%+v err=%v", len(campaigns), firstMarketingCampaign(campaigns), err)
	}
	foreignCampaigns, err := service.ListByOrganization(ctx, foreignOrganizationID)
	if err != nil || len(foreignCampaigns) != 1 || foreignCampaigns[0].ID != foreignCampaignID || foreignCampaigns[0].Analytics.RecipientCount != 3 {
		t.Fatalf("marketing campaign list crossed tenants: campaigns=%+v err=%v", foreignCampaigns, err)
	}
	for actor, userID := range map[string]int64{
		"member": users["member"], "viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"],
	} {
		if _, err := service.Create(ctx, organizationID, userID, validMarketingCampaignInput("Forbidden "+actor, leadAudienceID)); !errors.Is(err, modulemarketingcampaigns.ErrForbidden) {
			t.Fatalf("%s actor created a marketing campaign: %v", actor, err)
		}
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validMarketingCampaignInput("Foreign audience campaign", foreignAudienceID)); !errors.Is(err, modulemarketingcampaigns.ErrInvalidAudience) {
		t.Fatalf("cross-tenant audience create returned %v", err)
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validMarketingCampaignInput("Inactive audience campaign", inactiveAudienceID)); !errors.Is(err, modulemarketingcampaigns.ErrInvalidAudience) {
		t.Fatalf("inactive audience create returned %v", err)
	}

	type createResult struct {
		campaign modulemarketingcampaigns.Campaign
		err      error
	}
	start := make(chan struct{})
	results := make(chan createResult, 2)
	for index, actorID := range []int64{users["owner"], users["admin"]} {
		go func(index int, actorID int64) {
			<-start
			campaign, err := service.Create(ctx, organizationID, actorID, validMarketingCampaignInput(fmt.Sprintf("Concurrent final campaign %d", index+1), leadAudienceID))
			results <- createResult{campaign: campaign, err: err}
		}(index, actorID)
	}
	close(start)
	var createdCampaign modulemarketingcampaigns.Campaign
	var succeeded, limited int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			succeeded++
			createdCampaign = result.campaign
		case errors.Is(result.err, modulemarketingcampaigns.ErrCampaignLimit):
			limited++
		default:
			t.Fatalf("unexpected concurrent marketing campaign create result: %v", result.err)
		}
	}
	if succeeded != 1 || limited != 1 || createdCampaign.ID <= 0 || createdCampaign.Analytics.RecipientCount != 2 {
		t.Fatalf("marketing campaign capacity was not serialized: succeeded=%d limited=%d campaign=%+v", succeeded, limited, createdCampaign)
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validMarketingCampaignInput("Campaign 101", leadAudienceID)); !errors.Is(err, modulemarketingcampaigns.ErrCampaignLimit) {
		t.Fatalf("marketing campaign capacity returned %v", err)
	}
	if _, err := service.Update(ctx, organizationID, createdCampaign.ID, users["member"], validMarketingCampaignInput(createdCampaign.Name, leadAudienceID)); !errors.Is(err, modulemarketingcampaigns.ErrForbidden) {
		t.Fatalf("ordinary member updated a marketing campaign: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, foreignCampaignID, users["owner"], validMarketingCampaignInput("Foreign campaign", leadAudienceID)); !errors.Is(err, modulemarketingcampaigns.ErrNotFound) {
		t.Fatalf("cross-tenant marketing campaign update returned %v", err)
	}
	if _, err := service.Update(ctx, organizationID, createdCampaign.ID, users["owner"], validMarketingCampaignInput(createdCampaign.Name, foreignAudienceID)); !errors.Is(err, modulemarketingcampaigns.ErrInvalidAudience) {
		t.Fatalf("cross-tenant marketing campaign audience update returned %v", err)
	}
	updatedInput := validMarketingCampaignInput(createdCampaign.Name, prospectAudienceID)
	updatedInput.Status = "paused"
	updated, err := service.Update(ctx, organizationID, createdCampaign.ID, users["admin"], updatedInput)
	if err != nil || updated.Analytics.RecipientCount != 1 || updated.AudienceID != prospectAudienceID || updated.Status != "paused" {
		t.Fatalf("update retained marketing campaign at capacity: campaign=%+v err=%v", updated, err)
	}
	campaigns, err = service.ListByOrganization(ctx, organizationID)
	if err != nil || len(campaigns) != modulemarketingcampaigns.MaxCampaignsPerOrganization {
		t.Fatalf("complete marketing campaign catalog at capacity: count=%d err=%v", len(campaigns), err)
	}
	expiredCtx, expiredCancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer expiredCancel()
	if _, err := service.ListByOrganization(expiredCtx, organizationID); !errors.Is(err, modulemarketingcampaigns.ErrQueryTimeout) {
		t.Fatalf("expired marketing campaign list returned %v", err)
	}

	var campaignCount, foreignCampaignCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM marketing_email_campaigns WHERE organization_id=$1`, organizationID).Scan(&campaignCount); err != nil {
		t.Fatalf("count retained marketing campaigns: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM marketing_email_campaigns WHERE organization_id=$1`, foreignOrganizationID).Scan(&foreignCampaignCount); err != nil {
		t.Fatalf("count foreign marketing campaigns: %v", err)
	}
	if campaignCount != modulemarketingcampaigns.MaxCampaignsPerOrganization || foreignCampaignCount != 1 {
		t.Fatalf("unexpected retained marketing campaign state: tenant=%d foreign=%d", campaignCount, foreignCampaignCount)
	}
}

func validMarketingCampaignInput(name string, audienceID int64) modulemarketingcampaigns.Input {
	return modulemarketingcampaigns.Input{
		Name: name, Description: "Bounded marketing campaign", AudienceID: audienceID,
		Subject: "Campaign subject", PreviewText: "Campaign preview", Body: "Campaign body", Status: "draft",
	}
}

func firstMarketingCampaign(campaigns []modulemarketingcampaigns.Campaign) modulemarketingcampaigns.Campaign {
	if len(campaigns) == 0 {
		return modulemarketingcampaigns.Campaign{}
	}
	return campaigns[0]
}

func marketingCampaignDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse marketing campaign database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
