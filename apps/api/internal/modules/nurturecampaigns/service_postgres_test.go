package nurturecampaigns_test

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
	modulenurturecampaigns "github.com/aeml/open_crm/apps/api/internal/modules/nurturecampaigns"
	"github.com/jackc/pgx/v5"
)

func TestNurtureCampaignsAreBoundedAuthorizedAtomicAndTenantSafe(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to nurture campaign postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_nurture_campaigns_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create nurture campaign schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := nurtureCampaignDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate nurture campaign schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated nurture campaign schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Nurture team',$1) RETURNING id`, "nurture-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create nurture organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign nurture team',$1) RETURNING id`, "foreign-nurture-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign nurture organization: %v", err)
	}

	users := map[string]int64{}
	for _, actor := range []string{"owner", "admin", "member", "viewer", "disabled", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'test-hash','Nurture',$2) RETURNING id
		`, actor+"-"+schema+"@example.test", actor).Scan(&userID); err != nil {
			t.Fatalf("create %s nurture user: %v", actor, err)
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
			t.Fatalf("create nurture membership: %v", err)
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
		`, contact.organizationID, fmt.Sprintf("Nurture %d", index+1), fmt.Sprintf("nurture-%d-%s@example.test", index+1, schema), contact.status); err != nil {
			t.Fatalf("create nurture contact: %v", err)
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

	var activeSequenceID, draftSequenceID, staleSequenceID, foreignSequenceID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_sequences (organization_id,name,status,created_by_user_id,revision,approved_revision,approved_by_user_id,approved_at)
		VALUES ($1,'Approved sequence','active',$2,1,1,$2,NOW()) RETURNING id
	`, organizationID, users["owner"]).Scan(&activeSequenceID); err != nil {
		t.Fatalf("create active sequence: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_sequences (organization_id,name,status,created_by_user_id)
		VALUES ($1,'Draft sequence','draft',$2) RETURNING id
	`, organizationID, users["owner"]).Scan(&draftSequenceID); err != nil {
		t.Fatalf("create draft sequence: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_sequences (organization_id,name,status,created_by_user_id,revision,approved_revision,approved_by_user_id,approved_at)
		VALUES ($1,'Stale approval','active',$2,2,1,$2,NOW()) RETURNING id
	`, organizationID, users["owner"]).Scan(&staleSequenceID); err != nil {
		t.Fatalf("create stale-approved sequence: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_sequences (organization_id,name,status,created_by_user_id,revision,approved_revision,approved_by_user_id,approved_at)
		VALUES ($1,'Foreign sequence','active',$2,1,1,$2,NOW()) RETURNING id
	`, foreignOrganizationID, users["foreign"]).Scan(&foreignSequenceID); err != nil {
		t.Fatalf("create foreign sequence: %v", err)
	}

	service := modulenurturecampaigns.NewService(pool)
	blocker, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin nurture eligible-count blocker: %v", err)
	}
	if _, err := blocker.Exec(ctx, `LOCK TABLE contacts IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock contacts for nurture timeout: %v", err)
	}
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 150*time.Millisecond)
	_, createErr := service.Create(timeoutCtx, organizationID, users["owner"], validNurtureCampaignInput("Rolled back nurture", leadAudienceID, activeSequenceID))
	timeoutCancel()
	if !errors.Is(createErr, modulenurturecampaigns.ErrQueryTimeout) {
		t.Fatalf("blocked nurture audience snapshot returned %v", createErr)
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatalf("release nurture eligible-count blocker: %v", err)
	}
	var rolledBackCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_nurture_campaigns WHERE organization_id=$1 AND name='Rolled back nurture'`, organizationID).Scan(&rolledBackCount); err != nil || rolledBackCount != 0 {
		t.Fatalf("timed-out nurture campaign left state: count=%d err=%v", rolledBackCount, err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_nurture_campaigns (organization_id,audience_id,sequence_id,name,status,eligible_count,created_by_user_id,updated_by_user_id)
		SELECT $1,$2,$3,'Nurture ' || lpad(series::text,3,'0'),'draft',2,$4,$4
		FROM generate_series(1,99) AS series
	`, organizationID, leadAudienceID, activeSequenceID, users["owner"]); err != nil {
		t.Fatalf("seed nurture campaigns: %v", err)
	}
	var foreignCampaignID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_nurture_campaigns (organization_id,audience_id,sequence_id,name,status,eligible_count,created_by_user_id,updated_by_user_id)
		VALUES ($1,$2,$3,'Foreign nurture','draft',3,$4,$4) RETURNING id
	`, foreignOrganizationID, foreignAudienceID, foreignSequenceID, users["foreign"]).Scan(&foreignCampaignID); err != nil {
		t.Fatalf("seed foreign nurture campaign: %v", err)
	}

	campaigns, err := service.ListByOrganization(ctx, organizationID)
	if err != nil || len(campaigns) != 99 || campaigns[0].EligibleCount != 2 {
		t.Fatalf("list tenant nurture campaigns: count=%d first=%+v err=%v", len(campaigns), firstNurtureCampaign(campaigns), err)
	}
	foreignCampaigns, err := service.ListByOrganization(ctx, foreignOrganizationID)
	if err != nil || len(foreignCampaigns) != 1 || foreignCampaigns[0].ID != foreignCampaignID || foreignCampaigns[0].EligibleCount != 3 {
		t.Fatalf("nurture campaign list crossed tenants: campaigns=%+v err=%v", foreignCampaigns, err)
	}
	for actor, userID := range map[string]int64{
		"member": users["member"], "viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"],
	} {
		if _, err := service.Create(ctx, organizationID, userID, validNurtureCampaignInput("Forbidden "+actor, leadAudienceID, activeSequenceID)); !errors.Is(err, modulenurturecampaigns.ErrForbidden) {
			t.Fatalf("%s actor created a nurture campaign: %v", actor, err)
		}
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validNurtureCampaignInput("Foreign audience nurture", foreignAudienceID, activeSequenceID)); !errors.Is(err, modulenurturecampaigns.ErrInvalidAudience) {
		t.Fatalf("cross-tenant audience create returned %v", err)
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validNurtureCampaignInput("Inactive audience nurture", inactiveAudienceID, activeSequenceID)); !errors.Is(err, modulenurturecampaigns.ErrInvalidAudience) {
		t.Fatalf("inactive audience create returned %v", err)
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validNurtureCampaignInput("Foreign sequence nurture", leadAudienceID, foreignSequenceID)); !errors.Is(err, modulenurturecampaigns.ErrInvalidSequence) {
		t.Fatalf("cross-tenant sequence create returned %v", err)
	}
	for name, sequenceID := range map[string]int64{"draft": draftSequenceID, "stale": staleSequenceID} {
		input := validNurtureCampaignInput("Invalid active "+name, leadAudienceID, sequenceID)
		input.Status = "active"
		if _, err := service.Create(ctx, organizationID, users["owner"], input); !errors.Is(err, modulenurturecampaigns.ErrInvalidSequence) {
			t.Fatalf("%s sequence activated a nurture campaign: %v", name, err)
		}
	}

	type createResult struct {
		campaign modulenurturecampaigns.Campaign
		err      error
	}
	start := make(chan struct{})
	results := make(chan createResult, 2)
	for index, actorID := range []int64{users["owner"], users["admin"]} {
		go func(index int, actorID int64) {
			<-start
			campaign, err := service.Create(ctx, organizationID, actorID, validNurtureCampaignInput(fmt.Sprintf("Concurrent final nurture %d", index+1), leadAudienceID, activeSequenceID))
			results <- createResult{campaign: campaign, err: err}
		}(index, actorID)
	}
	close(start)
	var createdCampaign modulenurturecampaigns.Campaign
	var succeeded, limited int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			succeeded++
			createdCampaign = result.campaign
		case errors.Is(result.err, modulenurturecampaigns.ErrCampaignLimit):
			limited++
		default:
			t.Fatalf("unexpected concurrent nurture campaign create result: %v", result.err)
		}
	}
	if succeeded != 1 || limited != 1 || createdCampaign.ID <= 0 || createdCampaign.EligibleCount != 2 {
		t.Fatalf("nurture campaign capacity was not serialized: succeeded=%d limited=%d campaign=%+v", succeeded, limited, createdCampaign)
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validNurtureCampaignInput("Nurture 101", leadAudienceID, activeSequenceID)); !errors.Is(err, modulenurturecampaigns.ErrCampaignLimit) {
		t.Fatalf("nurture campaign capacity returned %v", err)
	}
	if _, err := service.Update(ctx, organizationID, createdCampaign.ID, users["member"], validNurtureCampaignInput(createdCampaign.Name, leadAudienceID, activeSequenceID)); !errors.Is(err, modulenurturecampaigns.ErrForbidden) {
		t.Fatalf("ordinary member updated a nurture campaign: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, foreignCampaignID, users["owner"], validNurtureCampaignInput("Foreign nurture", leadAudienceID, activeSequenceID)); !errors.Is(err, modulenurturecampaigns.ErrNotFound) {
		t.Fatalf("cross-tenant nurture campaign update returned %v", err)
	}
	if _, err := service.Update(ctx, organizationID, createdCampaign.ID, users["owner"], validNurtureCampaignInput(createdCampaign.Name, foreignAudienceID, activeSequenceID)); !errors.Is(err, modulenurturecampaigns.ErrInvalidAudience) {
		t.Fatalf("cross-tenant nurture audience update returned %v", err)
	}
	if _, err := service.Update(ctx, organizationID, createdCampaign.ID, users["owner"], validNurtureCampaignInput(createdCampaign.Name, leadAudienceID, foreignSequenceID)); !errors.Is(err, modulenurturecampaigns.ErrInvalidSequence) {
		t.Fatalf("cross-tenant nurture sequence update returned %v", err)
	}
	updatedInput := validNurtureCampaignInput(createdCampaign.Name, prospectAudienceID, activeSequenceID)
	updatedInput.Status = "active"
	updated, err := service.Update(ctx, organizationID, createdCampaign.ID, users["admin"], updatedInput)
	if err != nil || updated.EligibleCount != 1 || updated.AudienceID != prospectAudienceID || updated.Status != "active" {
		t.Fatalf("update retained nurture campaign at capacity: campaign=%+v err=%v", updated, err)
	}
	campaigns, err = service.ListByOrganization(ctx, organizationID)
	if err != nil || len(campaigns) != modulenurturecampaigns.MaxCampaignsPerOrganization {
		t.Fatalf("complete nurture campaign catalog at capacity: count=%d err=%v", len(campaigns), err)
	}
	expiredCtx, expiredCancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer expiredCancel()
	if _, err := service.ListByOrganization(expiredCtx, organizationID); !errors.Is(err, modulenurturecampaigns.ErrQueryTimeout) {
		t.Fatalf("expired nurture campaign list returned %v", err)
	}

	var campaignCount, foreignCampaignCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_nurture_campaigns WHERE organization_id=$1`, organizationID).Scan(&campaignCount); err != nil {
		t.Fatalf("count retained nurture campaigns: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_nurture_campaigns WHERE organization_id=$1`, foreignOrganizationID).Scan(&foreignCampaignCount); err != nil {
		t.Fatalf("count foreign nurture campaigns: %v", err)
	}
	if campaignCount != modulenurturecampaigns.MaxCampaignsPerOrganization || foreignCampaignCount != 1 {
		t.Fatalf("unexpected retained nurture campaign state: tenant=%d foreign=%d", campaignCount, foreignCampaignCount)
	}
}

func validNurtureCampaignInput(name string, audienceID, sequenceID int64) modulenurturecampaigns.Input {
	return modulenurturecampaigns.Input{
		Name: name, Description: "Bounded nurture campaign", AudienceID: audienceID, SequenceID: sequenceID, Status: "draft",
	}
}

func firstNurtureCampaign(campaigns []modulenurturecampaigns.Campaign) modulenurturecampaigns.Campaign {
	if len(campaigns) == 0 {
		return modulenurturecampaigns.Campaign{}
	}
	return campaigns[0]
}

func nurtureCampaignDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse nurture campaign database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
