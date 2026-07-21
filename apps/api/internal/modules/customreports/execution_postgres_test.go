package customreports_test

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
	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
)

func TestSavedTableReportsExecuteTenantSafeTypedQueriesAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to custom report postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_custom_report_execution_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create custom report schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := customReportDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate custom report schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to custom report schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, ownerID, foreignOwnerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Report tenant',$1) RETURNING id`, "reports-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create report organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign reports',$1) RETURNING id`, "foreign-reports-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign report organization: %v", err)
	}
	for _, user := range []struct {
		email string
		id    *int64
	}{
		{"report-owner-" + schema + "@example.test", &ownerID},
		{"foreign-report-owner-" + schema + "@example.test", &foreignOwnerID},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Report','Owner') RETURNING id`, user.email).Scan(user.id); err != nil {
			t.Fatalf("create report user: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner','active'),($3,$4,'owner','active')`, organizationID, ownerID, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatalf("create report memberships: %v", err)
	}

	var pipelineID, stageID, foreignPipelineID, foreignStageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, organizationID, ownerID).Scan(&pipelineID); err != nil {
		t.Fatalf("create report pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position) VALUES ($1,$2,'Discovery',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create report stage: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Foreign sales',1,TRUE,$2) RETURNING id`, foreignOrganizationID, foreignOwnerID).Scan(&foreignPipelineID); err != nil {
		t.Fatalf("create foreign report pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position) VALUES ($1,$2,'Foreign stage',1) RETURNING id`, foreignOrganizationID, foreignPipelineID).Scan(&foreignStageID); err != nil {
		t.Fatalf("create foreign report stage: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name,email,status,owner_user_id,lead_source,lead_score,created_at,updated_at)
		VALUES ($1,'Alpha','Qualified','alpha@example.test','lead',$2,'referral',85,'2026-01-01T10:00:00Z','2026-01-01T10:00:00Z'),
		       ($1,'Beta','Nurture','beta@example.test','lead',$2,'organic',30,'2026-01-02T10:00:00Z','2026-01-02T10:00:00Z'),
		       ($1,'Archived','Hidden','archived@example.test','lead',$2,'referral',99,NOW(),NOW()),
		       ($3,'Foreign','Marker','foreign@example.test','lead',$4,'referral',100,NOW(),NOW())
	`, organizationID, ownerID, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatalf("seed report contacts: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE contacts SET archived_at=NOW() WHERE organization_id=$1 AND first_name='Archived'`, organizationID); err != nil {
		t.Fatalf("archive report contact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO companies (organization_id,name,client_type,industry,status,city,country,owner_user_id,updated_at)
		VALUES ($1,'Acme Services','organization','Consulting','customer','Austin','US',$2,'2026-01-02T10:00:00Z'),
		       ($1,'Beta Studio','organization','Design','prospect','Boston','US',$2,'2026-01-01T10:00:00Z'),
		       ($1,'Archived Company','organization','Hidden','customer','Denver','US',$2,NOW()),
		       ($3,'Foreign Company','organization','Hidden','customer','Paris','FR',$4,NOW())
	`, organizationID, ownerID, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatalf("seed report companies: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE companies SET archived_at=NOW() WHERE organization_id=$1 AND name='Archived Company'`, organizationID); err != nil {
		t.Fatalf("archive report company: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,status,value_amount,value_currency,owner_user_id,expected_close_date,updated_at)
		VALUES ($1,$2,'Alpha deal','open',1250.50,'USD',$3,'2026-08-01','2026-01-02T10:00:00Z'),
		       ($1,$2,'Beta deal','open',500.00,'USD',$3,'2026-09-01','2026-01-01T10:00:00Z'),
		       ($1,$2,'Archived jackpot','open',888888.00,'USD',$3,'2026-08-01',NOW()),
		       ($4,$5,'Foreign jackpot','open',999999.00,'USD',$6,'2026-08-01',NOW())
	`, organizationID, stageID, ownerID, foreignOrganizationID, foreignStageID, foreignOwnerID); err != nil {
		t.Fatalf("seed report deals: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deals SET archived_at=NOW() WHERE organization_id=$1 AND name='Archived jackpot'`, organizationID); err != nil {
		t.Fatalf("archive report deal: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,due_at,assigned_to_user_id,created_by_user_id,updated_at)
		VALUES ($1,'contact',1,'Call Alpha','open','2026-08-01T12:00:00Z',$2,$2,'2026-01-02T10:00:00Z'),
		       ($1,'contact',2,'Call Beta','open','2027-08-01T12:00:00Z',$2,$2,'2026-01-01T10:00:00Z'),
		       ($1,'contact',1,'Archived task','open','2026-07-01T12:00:00Z',$2,$2,NOW()),
		       ($3,'contact',1,'Foreign task','open','2026-07-01T12:00:00Z',$4,$4,NOW())
	`, organizationID, ownerID, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatalf("seed report tasks: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tasks SET archived_at=NOW() WHERE organization_id=$1 AND title='Archived task'`, organizationID); err != nil {
		t.Fatalf("archive report task: %v", err)
	}

	service := modulecustomreports.NewService(pool)
	contactReport := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Qualified contacts", SourceType: "contacts", VisualizationType: "table",
		Columns:     []string{"firstName", "leadScore", "createdAt"},
		Filters:     []modulecustomreports.Filter{{Field: "firstName", Operator: "contains", Value: "ph"}, {Field: "leadScore", Operator: "greaterThan", Value: "50"}},
		Aggregation: modulecustomreports.Aggregation{Function: "none"},
	})
	contacts, err := service.Execute(ctx, organizationID, contactReport.ID, modulecustomreports.ExecuteQuery{})
	if err != nil || len(contacts.Rows) != 1 || reportValue(contacts, 0, "firstName") != "Alpha" || reportValue(contacts, 0, "leadScore") != "85" || reportValue(contacts, 0, "createdAt") != "2026-01-01T10:00:00Z" {
		t.Fatalf("unexpected contact report: result=%#v err=%v", contacts, err)
	}

	pageReport := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Paged contacts", SourceType: "contacts", VisualizationType: "table", Columns: []string{"id", "firstName"}, Aggregation: modulecustomreports.Aggregation{Function: "none"},
	})
	firstPage, err := service.Execute(ctx, organizationID, pageReport.ID, modulecustomreports.ExecuteQuery{Page: 1, PageSize: 1})
	if err != nil || len(firstPage.Rows) != 1 || !firstPage.HasMore || reportValue(firstPage, 0, "firstName") != "Beta" {
		t.Fatalf("unexpected first report page: result=%#v err=%v", firstPage, err)
	}
	secondPage, err := service.Execute(ctx, organizationID, pageReport.ID, modulecustomreports.ExecuteQuery{Page: 2, PageSize: 1})
	if err != nil || len(secondPage.Rows) != 1 || secondPage.HasMore || reportValue(secondPage, 0, "firstName") != "Alpha" {
		t.Fatalf("unexpected second report page: result=%#v err=%v", secondPage, err)
	}

	companyReport := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Company cities", SourceType: "companies", VisualizationType: "table", Columns: []string{"name", "city"},
		Filters: []modulecustomreports.Filter{{Field: "status", Operator: "equals", Value: "customer"}}, Aggregation: modulecustomreports.Aggregation{Function: "none"},
	})
	companies, err := service.Execute(ctx, organizationID, companyReport.ID, modulecustomreports.ExecuteQuery{})
	if err != nil || len(companies.Rows) != 1 || reportValue(companies, 0, "name") != "Acme Services" {
		t.Fatalf("unexpected company report: result=%#v err=%v", companies, err)
	}

	dealReport := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Pipeline by stage", SourceType: "deals", VisualizationType: "table", Columns: []string{"name", "valueAmount"},
		Filters: []modulecustomreports.Filter{{Field: "status", Operator: "equals", Value: "open"}, {Field: "expectedCloseDate", Operator: "before", Value: "2027-01-01"}},
		GroupBy: "stageName", Aggregation: modulecustomreports.Aggregation{Function: "sum", Field: "valueAmount"},
	})
	deals, err := service.Execute(ctx, organizationID, dealReport.ID, modulecustomreports.ExecuteQuery{})
	if err != nil || len(deals.Rows) != 1 || reportValue(deals, 0, "stageName") != "Discovery" || reportValue(deals, 0, "sumValueAmount") != "1750.50" {
		t.Fatalf("unexpected grouped deal report: result=%#v err=%v", deals, err)
	}

	taskReport := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Tasks due this year", SourceType: "tasks", VisualizationType: "table", Columns: []string{"title", "dueAt"},
		Filters: []modulecustomreports.Filter{{Field: "dueAt", Operator: "before", Value: "2027-01-01T00:00:00Z"}}, Aggregation: modulecustomreports.Aggregation{Function: "none"},
	})
	tasks, err := service.Execute(ctx, organizationID, taskReport.ID, modulecustomreports.ExecuteQuery{})
	if err != nil || len(tasks.Rows) != 1 || reportValue(tasks, 0, "title") != "Call Alpha" || reportValue(tasks, 0, "dueAt") != "2026-08-01T12:00:00Z" {
		t.Fatalf("unexpected task report: result=%#v err=%v", tasks, err)
	}

	foreignReport := createCustomReport(t, ctx, service, foreignOrganizationID, foreignOwnerID, modulecustomreports.Input{
		Name: "Foreign contacts", SourceType: "contacts", VisualizationType: "table", Columns: []string{"firstName"}, Aggregation: modulecustomreports.Aggregation{Function: "none"},
	})
	if _, err := service.Execute(ctx, foreignOrganizationID, contactReport.ID, modulecustomreports.ExecuteQuery{}); !errors.Is(err, modulecustomreports.ErrNotFound) {
		t.Fatalf("foreign tenant executed local report: %v", err)
	}
	if _, err := service.Execute(ctx, organizationID, foreignReport.ID, modulecustomreports.ExecuteQuery{}); !errors.Is(err, modulecustomreports.ErrNotFound) {
		t.Fatalf("local tenant executed foreign report: %v", err)
	}

	inactive := false
	inactiveReport := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Inactive report", SourceType: "contacts", VisualizationType: "table", Columns: []string{"firstName"}, Aggregation: modulecustomreports.Aggregation{Function: "none"}, IsActive: &inactive,
	})
	if _, err := service.Execute(ctx, organizationID, inactiveReport.ID, modulecustomreports.ExecuteQuery{}); !errors.Is(err, modulecustomreports.ErrInactive) {
		t.Fatalf("inactive report execution returned %v", err)
	}
	chartReport := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Future chart", SourceType: "deals", VisualizationType: "bar", Columns: []string{"name"}, GroupBy: "stageName", Aggregation: modulecustomreports.Aggregation{Function: "sum", Field: "valueAmount"},
	})
	if _, err := service.Execute(ctx, organizationID, chartReport.ID, modulecustomreports.ExecuteQuery{}); !errors.Is(err, modulecustomreports.ErrUnsupportedVisualization) {
		t.Fatalf("unfinished chart execution returned %v", err)
	}
	if _, err := service.Execute(ctx, organizationID, pageReport.ID, modulecustomreports.ExecuteQuery{Page: 101, PageSize: 50}); !errors.Is(err, modulecustomreports.ErrInvalidQuery) {
		t.Fatalf("unbounded report page returned %v", err)
	}
	if _, err := service.Create(ctx, organizationID, ownerID, modulecustomreports.Input{
		Name: "Invalid typed filter", SourceType: "contacts", VisualizationType: "table", Columns: []string{"firstName"},
		Filters: []modulecustomreports.Filter{{Field: "leadScore", Operator: "contains", Value: "8"}}, Aggregation: modulecustomreports.Aggregation{Function: "none"},
	}); !errors.Is(err, modulecustomreports.ErrInvalidInput) {
		t.Fatalf("invalid typed report filter returned %v", err)
	}
}

func createCustomReport(t *testing.T, ctx context.Context, service *modulecustomreports.Service, organizationID, ownerID int64, input modulecustomreports.Input) modulecustomreports.Definition {
	t.Helper()
	definition, err := service.Create(ctx, organizationID, ownerID, input)
	if err != nil {
		t.Fatalf("create custom report %q: %v", input.Name, err)
	}
	return definition
}

func reportValue(result modulecustomreports.Execution, row int, key string) string {
	if row >= len(result.Rows) || result.Rows[row].Values[key] == nil {
		return ""
	}
	return *result.Rows[row].Values[key]
}

func customReportDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse custom report URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
