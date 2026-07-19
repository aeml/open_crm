package touchpoints_test

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
	moduletouchpoints "github.com/aeml/open_crm/apps/api/internal/modules/touchpoints"
)

func TestTouchpointsAreTraceableTenantSafeAndViewerAwareAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to touchpoint postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_touchpoints_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create touchpoint schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := touchpointDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate touchpoint schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to touchpoint schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Touchpoint team',$1) RETURNING id`, "touchpoints-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign touchpoint team',$1) RETURNING id`, "foreign-touchpoints-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	var ownerID, viewerID, foreignUserID int64
	for _, user := range []struct {
		first, last, email string
		id                 *int64
	}{
		{"Olivia", "Owner", "owner-" + schema + "@example.test", &ownerID},
		{"Victor", "Viewer", "viewer-" + schema + "@example.test", &viewerID},
		{"Farah", "Foreign", "foreign-" + schema + "@example.test", &foreignUserID},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash',$2,$3) RETURNING id`, user.email, user.first, user.last).Scan(user.id); err != nil {
			t.Fatalf("create user %s: %v", user.email, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'member','disabled'),($1,$3,'viewer','active'),($4,$5,'owner','active')
	`, organizationID, ownerID, viewerID, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create memberships: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-90 * 24 * time.Hour)
	var staleContactID, freshContactID, touchedContactID, privateContactID, foreignContactID int64
	for _, contact := range []struct {
		organizationID int64
		first          string
		createdAt      time.Time
		ownerID        int64
		id             *int64
	}{
		{organizationID, "Stale", old, ownerID, &staleContactID},
		{organizationID, "Fresh", now.Add(-2 * 24 * time.Hour), ownerID, &freshContactID},
		{organizationID, "Touched", old, ownerID, &touchedContactID},
		{organizationID, "Private", old, ownerID, &privateContactID},
		{foreignOrganizationID, "Foreign", old, foreignUserID, &foreignContactID},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,status,owner_user_id,created_at) VALUES ($1,$2,'Contact','lead',$3,$4) RETURNING id`, contact.organizationID, contact.first, contact.ownerID, contact.createdAt).Scan(contact.id); err != nil {
			t.Fatalf("create %s contact: %v", contact.first, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE contacts SET status='customer',is_client=TRUE WHERE id IN ($1,$2,$3,$4)`, staleContactID, freshContactID, privateContactID, foreignContactID); err != nil {
		t.Fatalf("mark individual clients: %v", err)
	}
	var linkedCompanyID, staleCompanyID, healthyCompanyID, foreignCompanyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id,created_at) VALUES ($1,'Linked client','customer',$2,$3) RETURNING id`, organizationID, ownerID, old).Scan(&linkedCompanyID); err != nil {
		t.Fatalf("create linked company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id,created_at) VALUES ($1,'Stale client','customer',$2,$3) RETURNING id`, organizationID, ownerID, old).Scan(&staleCompanyID); err != nil {
		t.Fatalf("create stale company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id,created_at) VALUES ($1,'Healthy client','customer',$2,$3) RETURNING id`, organizationID, ownerID, now.Add(-2*24*time.Hour)).Scan(&healthyCompanyID); err != nil {
		t.Fatalf("create healthy company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,status,owner_user_id,created_at) VALUES ($1,'Foreign health client','customer',$2,$3) RETURNING id`, foreignOrganizationID, foreignUserID, old).Scan(&foreignCompanyID); err != nil {
		t.Fatalf("create foreign health company: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contact_company_links (organization_id,contact_id,company_id,is_primary) VALUES ($1,$2,$3,TRUE)`, organizationID, touchedContactID, linkedCompanyID); err != nil {
		t.Fatalf("link contact to company: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,due_at,assigned_to_user_id,created_by_user_id)
		VALUES ($1,'company',$2,'Recover account','open',$3,$4,$4),
		       ($1,'contact',$5,'Prepare account review','open',$6,$4,$4),
		       ($1,'company',$7,'Unscheduled account work','open',NULL,$4,$4),
		       ($8,'company',$9,'Foreign overdue work','open',$3,$10,$10)
	`, organizationID, staleCompanyID, now.Add(-2*24*time.Hour), ownerID, touchedContactID, now.Add(2*24*time.Hour), healthyCompanyID, foreignOrganizationID, foreignCompanyID, foreignUserID); err != nil {
		t.Fatalf("create client-health task signals: %v", err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO notes (organization_id,entity_type,entity_id,body,created_by_user_id,created_at) VALUES ($1,'contact',$2,'Customer context',$3,$4)`, organizationID, touchedContactID, ownerID, now.Add(-10*24*time.Hour)); err != nil {
		t.Fatalf("create note touch: %v", err)
	}
	var taskID int64
	if err := pool.QueryRow(ctx, `INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,assigned_to_user_id,created_by_user_id,created_at,completed_at) VALUES ($1,'contact',$2,'Confirm requirements','completed',$3,$3,$4,$5) RETURNING id`, organizationID, touchedContactID, ownerID, now.Add(-12*24*time.Hour), now.Add(-9*24*time.Hour)).Scan(&taskID); err != nil {
		t.Fatalf("create completed task: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,created_at) VALUES ($1,'task',$2,$3,'task.completed','Task completed',$4)`, organizationID, taskID, ownerID, now.Add(-9*24*time.Hour)); err != nil {
		t.Fatalf("create task completion event: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO call_logs (organization_id,entity_type,entity_id,direction,phone_number,status,disposition,created_by_user_id,created_at,completed_at)
		VALUES ($1,'contact',$2,'outbound','+15555550100','completed','connected',$3,$4,$4),
		       ($1,'contact',$2,'outbound','+15555550100','failed','no-answer',$3,$5,NULL)
	`, organizationID, touchedContactID, ownerID, now.Add(-8*24*time.Hour), now.Add(-1*time.Hour)); err != nil {
		t.Fatalf("create call records: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sms_messages (organization_id,entity_type,entity_id,direction,phone_number,phone_key,body,status,created_by_user_id,sent_at,created_at)
		VALUES ($1,'contact',$2,'outbound','+15555550100','15555550100','Checking in','sent',$3,$4,$4),
		       ($1,'contact',$2,'outbound','+15555550100','15555550100','Failed text','failed',$3,NULL,$5)
	`, organizationID, touchedContactID, ownerID, now.Add(-7*24*time.Hour), now.Add(-30*time.Minute)); err != nil {
		t.Fatalf("create SMS records: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO calendar_events (organization_id,entity_type,entity_id,title,start_at,end_at,status,visibility,calendar_user_id,created_by_user_id,created_at)
		VALUES ($1,'contact',$2,'Discovery',NOW()+INTERVAL '1 day',NOW()+INTERVAL '1 day 1 hour','scheduled','shared',$3,$3,$4),
		       ($1,'contact',$2,'Private review',NOW()+INTERVAL '2 days',NOW()+INTERVAL '2 days 1 hour','scheduled','private',$3,$3,$5),
		       ($1,'contact',$2,'Cancelled',NOW()+INTERVAL '3 days',NOW()+INTERVAL '3 days 1 hour','cancelled','shared',$3,$3,$6)
	`, organizationID, touchedContactID, ownerID, now.Add(-6*24*time.Hour), now.Add(-1*24*time.Hour), now.Add(-10*time.Minute)); err != nil {
		t.Fatalf("create meeting records: %v", err)
	}

	outboundAt := now.Add(-5 * 24 * time.Hour)
	var outboundEmailID, privateEmailID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_messages (organization_id,to_email,subject,body,status,entity_type,entity_id,sent_by_user_id,direction,from_email,mailbox_user_id,visibility,created_at)
		VALUES ($1,'customer@example.test','Proposal','Body','sent','contact',$2,$3,'outbound','owner@example.test',$3,'shared',$4) RETURNING id
	`, organizationID, touchedContactID, ownerID, outboundAt).Scan(&outboundEmailID); err != nil {
		t.Fatalf("create outbound email: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_message_entity_links (organization_id,email_message_id,entity_type,entity_id) VALUES ($1,$2,'contact',$3)`, organizationID, outboundEmailID, touchedContactID); err != nil {
		t.Fatalf("link outbound email: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notes (organization_id,entity_type,entity_id,body,created_by_user_id,created_at) VALUES ($1,'contact',$2,'Sent email: Proposal',$3,$4)`, organizationID, touchedContactID, ownerID, outboundAt.Add(2*time.Second)); err != nil {
		t.Fatalf("create fallback email note: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_messages (organization_id,to_email,subject,body,status,entity_type,entity_id,direction,from_email,mailbox_user_id,provider_message_id,visibility,received_at,created_at)
		VALUES ($1,'owner@example.test','Private reply','Body','received','contact',$2,'inbound','customer@example.test',$3,$4,'private',$5,$5) RETURNING id
	`, organizationID, privateContactID, ownerID, "private-"+schema, now.Add(-2*24*time.Hour)).Scan(&privateEmailID); err != nil {
		t.Fatalf("create private inbound email: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_message_entity_links (organization_id,email_message_id,entity_type,entity_id) VALUES ($1,$2,'contact',$3)`, organizationID, privateEmailID, privateContactID); err != nil {
		t.Fatalf("link private inbound email: %v", err)
	}

	service := moduletouchpoints.NewService(pool)
	viewerSummary, err := service.Summary(ctx, organizationID, viewerID, "contact", touchedContactID, 30)
	if err != nil {
		t.Fatalf("load viewer touchpoint summary: %v", err)
	}
	if viewerSummary.IsStale || viewerSummary.LastTouch == nil || viewerSummary.LastTouch.SourceType != "email" || len(viewerSummary.Recent) != 6 {
		t.Fatalf("unexpected viewer summary or email-note deduplication: %#v", viewerSummary)
	}
	wantSources := map[string]bool{"note": false, "task": false, "call": false, "sms": false, "meeting": false, "email": false}
	for _, touch := range viewerSummary.Recent {
		wantSources[touch.SourceType] = true
		if strings.Contains(strings.ToLower(touch.Summary), "failed") || strings.Contains(strings.ToLower(touch.Summary), "cancelled") || strings.Contains(strings.ToLower(touch.Summary), "private") {
			t.Fatalf("failed, cancelled, or private work became a viewer touch: %#v", touch)
		}
	}
	for source, found := range wantSources {
		if !found {
			t.Fatalf("missing %s touch from %#v", source, viewerSummary.Recent)
		}
	}
	ownerSummary, err := service.Summary(ctx, organizationID, ownerID, "contact", touchedContactID, 30)
	if err != nil || ownerSummary.LastTouch == nil || ownerSummary.LastTouch.SourceType != "meeting" || ownerSummary.LastTouch.Summary != "Meeting scheduled: Private review" {
		t.Fatalf("record owner could not see private meeting: summary=%#v err=%v", ownerSummary, err)
	}

	viewerPrivateSummary, err := service.Summary(ctx, organizationID, viewerID, "contact", privateContactID, 30)
	if err != nil || !viewerPrivateSummary.IsStale || viewerPrivateSummary.HealthStatus != "needs_attention" || len(viewerPrivateSummary.Recent) != 0 || viewerPrivateSummary.LastTouch != nil {
		t.Fatalf("private email leaked to viewer or changed staleness: summary=%#v err=%v", viewerPrivateSummary, err)
	}
	ownerPrivateSummary, err := service.Summary(ctx, organizationID, ownerID, "contact", privateContactID, 30)
	if err != nil || ownerPrivateSummary.IsStale || ownerPrivateSummary.HealthStatus != "healthy" || ownerPrivateSummary.LastTouch == nil || ownerPrivateSummary.LastTouch.Action != "email.received" {
		t.Fatalf("mailbox owner could not see private email: summary=%#v err=%v", ownerPrivateSummary, err)
	}

	contactReport, err := service.Stale(ctx, organizationID, viewerID, moduletouchpoints.Query{EntityType: "contact", StaleDays: 30})
	if err != nil || contactReport.Count != 2 || len(contactReport.Records) != 2 || !hasTouchpointRecord(contactReport.Records, staleContactID) || !hasTouchpointRecord(contactReport.Records, privateContactID) || hasTouchpointRecord(contactReport.Records, freshContactID) || hasTouchpointRecord(contactReport.Records, touchedContactID) {
		t.Fatalf("unexpected viewer stale contacts: report=%#v err=%v", contactReport, err)
	}
	filtered, err := service.Stale(ctx, organizationID, viewerID, moduletouchpoints.Query{EntityType: "contact", StaleDays: 30, OwnerUserID: ownerID, Limit: 1})
	if err != nil || filtered.Count != 2 || len(filtered.Records) != 1 || filtered.OwnerUserID != ownerID {
		t.Fatalf("disabled retained owner filter or pagination count failed: report=%#v err=%v", filtered, err)
	}
	if _, err := service.Stale(ctx, organizationID, viewerID, moduletouchpoints.Query{EntityType: "contact", OwnerUserID: foreignUserID}); !errors.Is(err, moduletouchpoints.ErrInvalidInput) {
		t.Fatalf("foreign owner filter returned %v", err)
	}

	companySummary, err := service.Summary(ctx, organizationID, viewerID, "company", linkedCompanyID, 30)
	if err != nil || companySummary.IsStale || companySummary.HealthStatus != "watch" || companySummary.DueSoonTaskCount != 1 || companySummary.OpenTaskCount != 1 || companySummary.LastTouch == nil || companySummary.LastTouch.RecordEntityType != "contact" || companySummary.LastTouch.RecordEntityID != touchedContactID || companySummary.LastTouch.RecordLabel != "Touched Contact" {
		t.Fatalf("linked contact work did not roll up traceably: summary=%#v err=%v", companySummary, err)
	}
	companyReport, err := service.Stale(ctx, organizationID, viewerID, moduletouchpoints.Query{EntityType: "company", StaleDays: 30})
	if err != nil || companyReport.Count != 1 || len(companyReport.Records) != 1 || companyReport.Records[0].EntityID != staleCompanyID || companyReport.Records[0].LastTouch != nil {
		t.Fatalf("unexpected stale companies: report=%#v err=%v", companyReport, err)
	}

	companyHealth, err := service.Health(ctx, organizationID, viewerID, moduletouchpoints.HealthQuery{EntityType: "company", StaleDays: 30})
	if err != nil || companyHealth.Count != 3 || companyHealth.Totals != (moduletouchpoints.HealthTotals{Total: 3, Healthy: 1, Watch: 1, NeedsAttention: 1}) || len(companyHealth.Records) != 3 {
		t.Fatalf("unexpected company health totals: report=%#v err=%v", companyHealth, err)
	}
	staleHealth := healthRecordByID(t, companyHealth.Records, staleCompanyID)
	if staleHealth.HealthStatus != "needs_attention" || staleHealth.OverdueTaskCount != 1 || !staleHealth.IsStale || len(staleHealth.HealthReasons) != 2 {
		t.Fatalf("stale overdue company health is not explainable: %#v", staleHealth)
	}
	linkedHealth := healthRecordByID(t, companyHealth.Records, linkedCompanyID)
	if linkedHealth.HealthStatus != "watch" || linkedHealth.DueSoonTaskCount != 1 || linkedHealth.LastTouch == nil || linkedHealth.LastTouch.RecordEntityID != touchedContactID {
		t.Fatalf("linked-contact health signals did not roll up: %#v", linkedHealth)
	}
	healthyHealth := healthRecordByID(t, companyHealth.Records, healthyCompanyID)
	if healthyHealth.HealthStatus != "healthy" || healthyHealth.OpenTaskCount != 1 || healthyHealth.DueSoonTaskCount != 0 || len(healthyHealth.HealthReasons) != 1 {
		t.Fatalf("unscheduled work incorrectly changed health: %#v", healthyHealth)
	}
	watchHealth, err := service.Health(ctx, organizationID, viewerID, moduletouchpoints.HealthQuery{EntityType: "company", Status: "watch", OwnerUserID: ownerID, StaleDays: 30, Limit: 1})
	if err != nil || watchHealth.Count != 1 || len(watchHealth.Records) != 1 || watchHealth.Records[0].EntityID != linkedCompanyID || watchHealth.Totals != companyHealth.Totals {
		t.Fatalf("health status/retained-owner filter lost totals: report=%#v err=%v", watchHealth, err)
	}
	viewerContactHealth, err := service.Health(ctx, organizationID, viewerID, moduletouchpoints.HealthQuery{EntityType: "contact", StaleDays: 30})
	if err != nil || viewerContactHealth.Totals != (moduletouchpoints.HealthTotals{Total: 3, Healthy: 1, NeedsAttention: 2}) || healthRecordByID(t, viewerContactHealth.Records, privateContactID).HealthStatus != "needs_attention" {
		t.Fatalf("viewer-private client health leaked: report=%#v err=%v", viewerContactHealth, err)
	}
	ownerContactHealth, err := service.Health(ctx, organizationID, ownerID, moduletouchpoints.HealthQuery{EntityType: "contact", StaleDays: 30})
	if err != nil || ownerContactHealth.Totals != (moduletouchpoints.HealthTotals{Total: 3, Healthy: 2, NeedsAttention: 1}) || healthRecordByID(t, ownerContactHealth.Records, privateContactID).HealthStatus != "healthy" {
		t.Fatalf("mailbox-owner client health lost private context: report=%#v err=%v", ownerContactHealth, err)
	}
	if hasHealthRecord(companyHealth.Records, foreignCompanyID) || len(companyHealth.Semantics) == 0 {
		t.Fatalf("foreign client leaked or health rules were omitted: %#v", companyHealth)
	}

	if _, err := service.Summary(ctx, organizationID, viewerID, "contact", foreignContactID, 30); !errors.Is(err, moduletouchpoints.ErrNotFound) {
		t.Fatalf("cross-tenant record lookup returned %v", err)
	}
	for _, query := range []moduletouchpoints.Query{{EntityType: "deal"}, {EntityType: "contact", StaleDays: 6}, {EntityType: "contact", StaleDays: 366}, {EntityType: "contact", Limit: 101}} {
		if _, err := service.Stale(ctx, organizationID, viewerID, query); !errors.Is(err, moduletouchpoints.ErrInvalidInput) {
			t.Fatalf("invalid query %#v returned %v", query, err)
		}
	}
	for _, query := range []moduletouchpoints.HealthQuery{{EntityType: "deal"}, {EntityType: "company", Status: "critical"}, {EntityType: "company", StaleDays: 6}, {EntityType: "company", Limit: 101}, {EntityType: "company", OwnerUserID: foreignUserID}} {
		if _, err := service.Health(ctx, organizationID, viewerID, query); !errors.Is(err, moduletouchpoints.ErrInvalidInput) {
			t.Fatalf("invalid health query %#v returned %v", query, err)
		}
	}
	if len(contactReport.Semantics) == 0 || len(companySummary.Semantics) == 0 {
		t.Fatalf("touchpoint inference was not explained: report=%#v summary=%#v", contactReport, companySummary)
	}
}

func healthRecordByID(t *testing.T, records []moduletouchpoints.HealthRecord, entityID int64) moduletouchpoints.HealthRecord {
	t.Helper()
	for _, record := range records {
		if record.EntityID == entityID {
			return record
		}
	}
	t.Fatalf("health record %d missing from %#v", entityID, records)
	return moduletouchpoints.HealthRecord{}
}

func hasHealthRecord(records []moduletouchpoints.HealthRecord, entityID int64) bool {
	for _, record := range records {
		if record.EntityID == entityID {
			return true
		}
	}
	return false
}

func hasTouchpointRecord(records []moduletouchpoints.Record, entityID int64) bool {
	for _, record := range records {
		if record.EntityID == entityID {
			return true
		}
	}
	return false
}

func touchpointDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse touchpoint URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
