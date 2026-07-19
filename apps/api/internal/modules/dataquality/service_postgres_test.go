package dataquality_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduledataquality "github.com/aeml/open_crm/apps/api/internal/modules/dataquality"
)

func TestDataQualityReportsAreExplainableBusinessAwareAndTenantSafeAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to data quality postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_data_quality_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create data quality schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := dataQualityDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate data quality schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to data quality schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, ownerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,business_type) VALUES ('Service team',$1,'services') RETURNING id`, "quality-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,business_type) VALUES ('Foreign',$1,'services') RETURNING id`, "foreign-quality-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Quality','Owner') RETURNING id`, "quality-"+schema+"@example.test").Scan(&ownerID); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,status) VALUES ($1,'No','Details','lead'),($2,'Foreign','Hidden','lead')`, organizationID, foreignOrganizationID); err != nil {
		t.Fatalf("create quality contacts: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status,owner_user_id,archived_at) VALUES ($1,'Archived','Issue','','lead',$2,NOW())`, organizationID, ownerID); err != nil {
		t.Fatalf("create archived quality contact: %v", err)
	}
	var companyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,client_type,owner_user_id) VALUES ($1,'Unlinked Service Client','customer','organization',$2) RETURNING id`, organizationID, ownerID).Scan(&companyID); err != nil {
		t.Fatalf("create quality company: %v", err)
	}
	var pipelineID, stageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, organizationID, ownerID).Scan(&pipelineID); err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position) VALUES ($1,$2,'Open',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create stage: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deals (organization_id,stage_id,name,status,updated_at) VALUES ($1,$2,'Stale incomplete deal','open',NOW()-INTERVAL '61 days')`, organizationID, stageID); err != nil {
		t.Fatalf("create quality deal: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,created_by_user_id) VALUES ($1,'company',$2,'Unscheduled follow-up','open',$3)`, organizationID, companyID, ownerID); err != nil {
		t.Fatalf("create quality task: %v", err)
	}

	summary, err := moduledataquality.NewService(pool).Summary(ctx, organizationID, moduledataquality.Query{StaleDays: 60})
	if err != nil {
		t.Fatalf("load data quality summary: %v", err)
	}
	if summary.BusinessType != "services" || summary.StaleDays != 60 || len(summary.Reports) != 6 {
		t.Fatalf("unexpected data quality summary: %#v", summary)
	}
	expected := map[string]int{"missing_owners": 3, "missing_contact_details": 1, "stale_deals": 1, "incomplete_deals": 1, "unscheduled_tasks": 1, "service_clients_without_people": 1}
	for key, count := range expected {
		report := qualityReport(t, summary, key)
		if report.Count != count || len(report.Records) == 0 || report.Description == "" || report.Records[0].Detail == "" {
			t.Fatalf("unexpected %s report: %#v", key, report)
		}
		for _, record := range report.Records {
			if record.Label == "Foreign Hidden" {
				t.Fatalf("foreign tenant leaked into %s", key)
			}
		}
	}
	for businessType, expectedKey := range map[string]string{
		"construction-services": "construction_clients_without_location",
		"product-sales":         "product_accounts_without_industry",
	} {
		if _, err := pool.Exec(ctx, `UPDATE organizations SET business_type=$1 WHERE id=$2`, businessType, organizationID); err != nil {
			t.Fatalf("set %s business profile: %v", businessType, err)
		}
		profileSummary, err := moduledataquality.NewService(pool).Summary(ctx, organizationID, moduledataquality.Query{StaleDays: 60})
		if err != nil {
			t.Fatalf("load %s data quality summary: %v", businessType, err)
		}
		profileReport := qualityReport(t, profileSummary, expectedKey)
		if profileSummary.BusinessType != businessType || profileReport.Count != 1 || len(profileReport.Records) != 1 {
			t.Fatalf("unexpected %s profile report: summary=%#v report=%#v", businessType, profileSummary, profileReport)
		}
	}
	if _, err := moduledataquality.NewService(pool).Summary(ctx, organizationID, moduledataquality.Query{StaleDays: 2}); err == nil {
		t.Fatal("expected invalid stale threshold rejection")
	}
}

func qualityReport(t *testing.T, summary moduledataquality.Summary, key string) moduledataquality.Report {
	t.Helper()
	for _, report := range summary.Reports {
		if report.Key == key {
			return report
		}
	}
	t.Fatalf("missing quality report %s", key)
	return moduledataquality.Report{}
}

func dataQualityDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse data quality URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
