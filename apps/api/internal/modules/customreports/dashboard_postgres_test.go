package customreports_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulecustomreports "github.com/aeml/open_crm/apps/api/internal/modules/customreports"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSharedReportDashboardIsRevisionedTenantSafeAndSnapshotBounded(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to shared dashboard postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_shared_report_dashboard_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create shared dashboard schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := customReportDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate shared dashboard schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to shared dashboard schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Dashboard tenant',$1) RETURNING id`, "dashboard-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create dashboard organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign dashboard',$1) RETURNING id`, "foreign-dashboard-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign dashboard organization: %v", err)
	}
	var ownerID, memberID, viewerID, disabledID, foreignOwnerID int64
	for _, user := range []struct {
		email string
		id    *int64
	}{
		{"dashboard-owner-" + schema + "@example.test", &ownerID},
		{"dashboard-member-" + schema + "@example.test", &memberID},
		{"dashboard-viewer-" + schema + "@example.test", &viewerID},
		{"dashboard-disabled-" + schema + "@example.test", &disabledID},
		{"foreign-dashboard-owner-" + schema + "@example.test", &foreignOwnerID},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Dashboard','User') RETURNING id`, user.email).Scan(user.id); err != nil {
			t.Fatalf("create dashboard user: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'owner','active'),($1,$3,'member','active'),($1,$4,'viewer','active'),
		       ($1,$5,'member','disabled'),($6,$7,'owner','active')
	`, organizationID, ownerID, memberID, viewerID, disabledID, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatalf("create dashboard memberships: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name,email,status,lead_source,lead_score,owner_user_id)
		SELECT $1,'Contact',value::text,'dashboard-' || value::text || '@example.test','lead',
		       'source-' || lpad(value::text,2,'0'),value,$2
		FROM generate_series(1,14) AS value
	`, organizationID, ownerID); err != nil {
		t.Fatalf("seed dashboard contacts: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name,email,status,lead_source,lead_score,owner_user_id)
		VALUES ($1,'Foreign','Marker','foreign-dashboard@example.test','customer','foreign-only',100,$2)
	`, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatalf("seed foreign dashboard contact: %v", err)
	}

	service := modulecustomreports.NewService(pool)
	barBySource := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Contacts by source", SourceType: "contacts", VisualizationType: "bar", VisualizationContract: "grouped_bar_v1",
		GroupBy: "leadSource", Aggregation: modulecustomreports.Aggregation{Function: "count"},
	})
	barByStatus := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Contacts by status", SourceType: "contacts", VisualizationType: "bar", VisualizationContract: "grouped_bar_v1",
		GroupBy: "status", Aggregation: modulecustomreports.Aggregation{Function: "count"},
	})
	tableReport := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Contact table", SourceType: "contacts", VisualizationType: "table", Columns: []string{"firstName"},
		Aggregation: modulecustomreports.Aggregation{Function: "none"},
	})
	inactive := false
	inactiveBar := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
		Name: "Inactive contact bars", SourceType: "contacts", VisualizationType: "bar", VisualizationContract: "grouped_bar_v1",
		GroupBy: "status", Aggregation: modulecustomreports.Aggregation{Function: "count"}, IsActive: &inactive,
	})
	foreignBar := createCustomReport(t, ctx, service, foreignOrganizationID, foreignOwnerID, modulecustomreports.Input{
		Name: "Foreign contact bars", SourceType: "contacts", VisualizationType: "bar", VisualizationContract: "grouped_bar_v1",
		GroupBy: "leadSource", Aggregation: modulecustomreports.Aggregation{Function: "count"},
	})

	empty, err := service.GetDashboard(ctx, organizationID)
	if err != nil || empty.ID != 0 || empty.Revision != 0 || len(empty.Widgets) != 0 {
		t.Fatalf("unexpected initial dashboard: dashboard=%#v err=%v", empty, err)
	}
	for _, test := range []struct {
		name    string
		actorID int64
		input   modulecustomreports.DashboardInput
		err     error
	}{
		{name: "viewer", actorID: viewerID, input: dashboardInput(0, barBySource.ID), err: modulecustomreports.ErrForbidden},
		{name: "disabled member", actorID: disabledID, input: dashboardInput(0, barBySource.ID), err: modulecustomreports.ErrForbidden},
		{name: "foreign actor", actorID: foreignOwnerID, input: dashboardInput(0, barBySource.ID), err: modulecustomreports.ErrForbidden},
		{name: "foreign definition", actorID: ownerID, input: dashboardInput(0, foreignBar.ID), err: modulecustomreports.ErrInvalidInput},
		{name: "table definition", actorID: ownerID, input: dashboardInput(0, tableReport.ID), err: modulecustomreports.ErrInvalidInput},
		{name: "inactive definition", actorID: ownerID, input: dashboardInput(0, inactiveBar.ID), err: modulecustomreports.ErrInvalidInput},
		{name: "duplicate definition", actorID: ownerID, input: modulecustomreports.DashboardInput{Widgets: []modulecustomreports.DashboardWidgetInput{{ReportDefinitionID: barBySource.ID}, {ReportDefinitionID: barBySource.ID}}}, err: modulecustomreports.ErrInvalidInput},
		{name: "unsupported width", actorID: ownerID, input: modulecustomreports.DashboardInput{Widgets: []modulecustomreports.DashboardWidgetInput{{ReportDefinitionID: barBySource.ID, Width: "third"}}}, err: modulecustomreports.ErrInvalidInput},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.UpdateDashboard(ctx, organizationID, test.actorID, test.input); !errors.Is(err, test.err) {
				t.Fatalf("dashboard update returned %v, want %v", err, test.err)
			}
		})
	}
	var dashboardCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM custom_report_dashboards WHERE organization_id=$1`, organizationID).Scan(&dashboardCount); err != nil || dashboardCount != 0 {
		t.Fatalf("rejected dashboard writes left state: count=%d err=%v", dashboardCount, err)
	}

	firstInput := modulecustomreports.DashboardInput{Revision: 0, Widgets: []modulecustomreports.DashboardWidgetInput{
		{ReportDefinitionID: barBySource.ID, Width: "half"},
		{ReportDefinitionID: barByStatus.ID, Width: "full"},
	}}
	first, err := service.UpdateDashboard(ctx, organizationID, memberID, firstInput)
	if err != nil || first.ID == 0 || first.Revision != 1 || len(first.Widgets) != 2 || first.Widgets[0].Position != 0 || first.Widgets[0].Width != "half" || first.Widgets[1].Position != 1 || first.Widgets[1].Width != "full" {
		t.Fatalf("unexpected saved dashboard: dashboard=%#v err=%v", first, err)
	}
	assertDashboardAuditCount(t, ctx, pool, organizationID, first.ID, 1, 1, 2)

	noOpInput := firstInput
	noOpInput.Revision = first.Revision
	unchanged, err := service.UpdateDashboard(ctx, organizationID, memberID, noOpInput)
	if err != nil || unchanged.Revision != 1 {
		t.Fatalf("idempotent dashboard save changed revision: dashboard=%#v err=%v", unchanged, err)
	}
	assertDashboardAuditCount(t, ctx, pool, organizationID, first.ID, 1, 1, 2)
	if _, err := service.UpdateDashboard(ctx, organizationID, ownerID, dashboardInput(0, barBySource.ID)); !errors.Is(err, modulecustomreports.ErrDashboardRevisionConflict) {
		t.Fatalf("stale dashboard revision returned %v", err)
	}

	execution, err := service.ExecuteDashboard(ctx, organizationID)
	if err != nil || execution.Revision != 1 || len(execution.Widgets) != 2 {
		t.Fatalf("execute shared dashboard: execution=%#v err=%v", execution, err)
	}
	if len(execution.Widgets[0].Result.Rows) != 12 || !execution.Widgets[0].Result.HasMore || execution.Widgets[0].Result.GeneratedAt != execution.GeneratedAt || execution.Widgets[1].Result.GeneratedAt != execution.GeneratedAt {
		t.Fatalf("dashboard snapshot limits/timestamp mismatch: %#v", execution)
	}
	if dashboardExecutionContains(execution, "foreign-only") {
		t.Fatal("foreign contact value leaked into local dashboard")
	}
	foreignEmpty, err := service.ExecuteDashboard(ctx, foreignOrganizationID)
	if err != nil || foreignEmpty.Revision != 0 || len(foreignEmpty.Widgets) != 0 {
		t.Fatalf("foreign workspace inherited local dashboard: execution=%#v err=%v", foreignEmpty, err)
	}
	foreignDashboard, err := service.UpdateDashboard(ctx, foreignOrganizationID, foreignOwnerID, dashboardInput(0, foreignBar.ID))
	if err != nil {
		t.Fatalf("configure independent foreign dashboard: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_report_dashboard_widgets (organization_id,dashboard_id,report_definition_id,position,width)
		VALUES ($1,$2,$3,1,'half')
	`, foreignOrganizationID, foreignDashboard.ID, barBySource.ID); err == nil {
		t.Fatal("dashboard widget accepted a cross-tenant report definition")
	}

	concurrentInputs := []modulecustomreports.DashboardInput{
		{Revision: 1, Widgets: []modulecustomreports.DashboardWidgetInput{{ReportDefinitionID: barBySource.ID, Width: "full"}}},
		{Revision: 1, Widgets: []modulecustomreports.DashboardWidgetInput{{ReportDefinitionID: barByStatus.ID, Width: "half"}}},
	}
	var wait sync.WaitGroup
	errorsOut := make(chan error, len(concurrentInputs))
	for _, input := range concurrentInputs {
		wait.Add(1)
		go func(input modulecustomreports.DashboardInput) {
			defer wait.Done()
			_, err := service.UpdateDashboard(ctx, organizationID, ownerID, input)
			errorsOut <- err
		}(input)
	}
	wait.Wait()
	close(errorsOut)
	var succeeded, conflicted int
	for err := range errorsOut {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, modulecustomreports.ErrDashboardRevisionConflict):
			conflicted++
		default:
			t.Fatalf("concurrent dashboard update returned %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent dashboard results: succeeded=%d conflicted=%d", succeeded, conflicted)
	}
	current, err := service.GetDashboard(ctx, organizationID)
	if err != nil || current.Revision != 2 || len(current.Widgets) != 1 {
		t.Fatalf("unexpected concurrent dashboard winner: dashboard=%#v err=%v", current, err)
	}
	assertDashboardAuditCount(t, ctx, pool, organizationID, current.ID, 2, 2, 1)

	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION reject_dashboard_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.event_type = 'report_dashboard.updated' THEN
				RAISE EXCEPTION 'forced dashboard audit failure';
			END IF;
			RETURN NEW;
		END
		$$;
		CREATE TRIGGER reject_dashboard_audit_trigger BEFORE INSERT ON audit_events
		FOR EACH ROW EXECUTE FUNCTION reject_dashboard_audit();
	`); err != nil {
		t.Fatalf("install dashboard audit failure trigger: %v", err)
	}
	if _, err := service.UpdateDashboard(ctx, organizationID, ownerID, modulecustomreports.DashboardInput{Revision: current.Revision, Widgets: []modulecustomreports.DashboardWidgetInput{}}); err == nil {
		t.Fatal("forced dashboard audit failure unexpectedly committed")
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_dashboard_audit_trigger ON audit_events; DROP FUNCTION reject_dashboard_audit()`); err != nil {
		t.Fatalf("remove dashboard audit failure trigger: %v", err)
	}
	afterFailure, err := service.GetDashboard(ctx, organizationID)
	if err != nil || afterFailure.Revision != current.Revision || len(afterFailure.Widgets) != len(current.Widgets) || afterFailure.Widgets[0].ReportDefinitionID != current.Widgets[0].ReportDefinitionID {
		t.Fatalf("audit failure left partial dashboard: dashboard=%#v err=%v", afterFailure, err)
	}

	staleDefinitionID := current.Widgets[0].ReportDefinitionID
	if _, err := pool.Exec(ctx, `UPDATE custom_report_definitions SET is_active=FALSE WHERE organization_id=$1 AND id=$2`, organizationID, staleDefinitionID); err != nil {
		t.Fatalf("deactivate configured dashboard report: %v", err)
	}
	if _, err := service.ExecuteDashboard(ctx, organizationID); !errors.Is(err, modulecustomreports.ErrInactive) {
		t.Fatalf("stale dashboard execution returned %v", err)
	}
	if _, err := service.UpdateDashboard(ctx, organizationID, ownerID, modulecustomreports.DashboardInput{
		Revision: current.Revision,
		Widgets:  []modulecustomreports.DashboardWidgetInput{{ReportDefinitionID: staleDefinitionID, Width: current.Widgets[0].Width}},
	}); !errors.Is(err, modulecustomreports.ErrInvalidInput) {
		t.Fatalf("unchanged inactive dashboard configuration returned %v", err)
	}
	cleared, err := service.UpdateDashboard(ctx, organizationID, ownerID, modulecustomreports.DashboardInput{Revision: current.Revision, Widgets: []modulecustomreports.DashboardWidgetInput{}})
	if err != nil || cleared.Revision != 3 || len(cleared.Widgets) != 0 {
		t.Fatalf("clear stale dashboard: dashboard=%#v err=%v", cleared, err)
	}

	activeReportID := barBySource.ID
	if activeReportID == staleDefinitionID {
		activeReportID = barByStatus.ID
	}
	reconfigured, err := service.UpdateDashboard(ctx, organizationID, ownerID, dashboardInput(cleared.Revision, activeReportID))
	if err != nil || reconfigured.Revision != 4 {
		t.Fatalf("reconfigure dashboard after stale recovery: dashboard=%#v err=%v", reconfigured, err)
	}
	lockTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin dashboard timeout blocker: %v", err)
	}
	if _, err := lockTx.Exec(ctx, `LOCK TABLE contacts IN ACCESS EXCLUSIVE MODE`); err != nil {
		_ = lockTx.Rollback(ctx)
		t.Fatalf("lock dashboard source table: %v", err)
	}
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 75*time.Millisecond)
	_, timeoutErr := service.ExecuteDashboard(timeoutCtx, organizationID)
	timeoutCancel()
	if !errors.Is(timeoutErr, modulecustomreports.ErrQueryTimeout) {
		_ = lockTx.Rollback(ctx)
		t.Fatalf("blocked dashboard execution returned %v", timeoutErr)
	}
	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release dashboard timeout blocker: %v", err)
	}
	if recovered, err := service.ExecuteDashboard(ctx, organizationID); err != nil || recovered.Revision != reconfigured.Revision || len(recovered.Widgets) != 1 {
		t.Fatalf("dashboard did not recover after timeout: execution=%#v err=%v", recovered, err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name,email,status,lead_source,lead_score,owner_user_id)
		SELECT $1,'Scale',value::text,'dashboard-scale-' || value::text || '@example.test','lead',
		       'source-' || lpad((value % 40)::text,2,'0'),value % 100,$2
		FROM generate_series(15,10000) AS value
	`, organizationID, ownerID); err != nil {
		t.Fatalf("seed dashboard performance contacts: %v", err)
	}
	performanceReports := make([]int64, 0, modulecustomreports.MaxDashboardWidgets)
	for index := 1; index <= modulecustomreports.MaxDashboardWidgets; index++ {
		report := createCustomReport(t, ctx, service, organizationID, ownerID, modulecustomreports.Input{
			Name: fmt.Sprintf("Dashboard performance bars %d", index), SourceType: "contacts", VisualizationType: "bar", VisualizationContract: "grouped_bar_v1",
			Filters: []modulecustomreports.Filter{{Field: "leadScore", Operator: "greaterThan", Value: fmt.Sprintf("%d", index-1)}},
			GroupBy: "leadSource", Aggregation: modulecustomreports.Aggregation{Function: "count"},
		})
		performanceReports = append(performanceReports, report.ID)
	}
	performanceInput := modulecustomreports.DashboardInput{Revision: reconfigured.Revision, Widgets: make([]modulecustomreports.DashboardWidgetInput, 0, len(performanceReports))}
	for index, reportID := range performanceReports {
		width := "half"
		if index == 0 {
			width = "full"
		}
		performanceInput.Widgets = append(performanceInput.Widgets, modulecustomreports.DashboardWidgetInput{ReportDefinitionID: reportID, Width: width})
	}
	performanceDashboard, err := service.UpdateDashboard(ctx, organizationID, ownerID, performanceInput)
	if err != nil {
		t.Fatalf("configure six-widget dashboard: %v", err)
	}
	started := time.Now()
	performanceExecution, err := service.ExecuteDashboard(ctx, organizationID)
	elapsed := time.Since(started)
	if err != nil || performanceExecution.Revision != performanceDashboard.Revision || len(performanceExecution.Widgets) != modulecustomreports.MaxDashboardWidgets {
		t.Fatalf("execute six-widget dashboard: execution=%#v err=%v", performanceExecution, err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("six-widget 10,000-contact dashboard exceeded two-second budget: %s", elapsed)
	}
	for _, widget := range performanceExecution.Widgets {
		if len(widget.Result.Rows) > 12 || !widget.Result.HasMore || widget.Result.GeneratedAt != performanceExecution.GeneratedAt {
			t.Fatalf("unbounded or inconsistent performance widget: %#v", widget)
		}
	}
}

func dashboardInput(revision, reportDefinitionID int64) modulecustomreports.DashboardInput {
	return modulecustomreports.DashboardInput{
		Revision: revision,
		Widgets:  []modulecustomreports.DashboardWidgetInput{{ReportDefinitionID: reportDefinitionID, Width: "half"}},
	}
}

func dashboardExecutionContains(execution modulecustomreports.DashboardExecution, expected string) bool {
	for _, widget := range execution.Widgets {
		for _, row := range widget.Result.Rows {
			for _, value := range row.Values {
				if value != nil && *value == expected {
					return true
				}
			}
		}
	}
	return false
}

func assertDashboardAuditCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, dashboardID int64, expectedCount int, expectedRevision int64, expectedWidgets int) {
	t.Helper()
	var count int
	var maxRevision int64
	var latestWidgets int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int,COALESCE(MAX((metadata_json->>'revision')::bigint),0),
		       COALESCE((ARRAY_AGG((metadata_json->>'widgetCount')::int ORDER BY id DESC))[1],0)
		FROM audit_events
		WHERE organization_id=$1 AND event_type='report_dashboard.updated' AND entity_id=$2
	`, organizationID, dashboardID).Scan(&count, &maxRevision, &latestWidgets); err != nil {
		t.Fatalf("read dashboard audit evidence: %v", err)
	}
	if count != expectedCount || maxRevision != expectedRevision || latestWidgets != expectedWidgets {
		t.Fatalf("dashboard audit mismatch: count=%d revision=%d widgets=%d", count, maxRevision, latestWidgets)
	}
}
