package audit

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestAuditRetentionExportAndTenantBoundaryAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_audit_retention_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create audit retention schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := auditSearchPathURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate audit retention schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to audit retention schema: %v", err)
	}
	defer pool.Close()

	organizationID := insertAuditOrganization(t, ctx, pool, "Audit Pilot", "audit-pilot-"+schema)
	foreignOrganizationID := insertAuditOrganization(t, ctx, pool, "Foreign Audit", "foreign-audit-"+schema)
	actorUserID := insertAuditUser(t, ctx, pool, "audit-owner-"+schema+"@example.test")
	service := NewService(pool)
	if err := service.Record(ctx, organizationID, RecordInput{
		ActorUserID: actorUserID,
		EventType:   "user.invited",
		EntityType:  "user",
		EntityID:    actorUserID,
		Summary:     "Invited pilot owner",
		Metadata: map[string]string{
			"email":         "pilot@example.test",
			"accessToken":   "must-not-persist",
			"sessionCookie": "must-not-persist",
		},
	}); err != nil {
		t.Fatalf("record sanitized audit event: %v", err)
	}

	var mixedEventID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO audit_events(organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json,created_at)
		VALUES($1,$2,'billing.subscription_updated','organization',$1,'=HYPERLINK("https://invalid.test")',
		       '{"attempt":2,"approved":true,"nested":{"reason":"provider"}}'::jsonb,
		       '2026-07-21T12:00:00Z')
		RETURNING id
	`, organizationID, actorUserID).Scan(&mixedEventID); err != nil {
		t.Fatalf("insert mixed audit metadata: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events(organization_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES($1,'foreign.event','organization',$1,'Foreign tenant event','{}'::jsonb)
	`, foreignOrganizationID); err != nil {
		t.Fatalf("insert foreign audit event: %v", err)
	}

	events, err := service.ListByOrganization(ctx, organizationID, ListQuery{Limit: 100})
	if err != nil || len(events) != 2 {
		t.Fatalf("unexpected tenant audit list: events=%#v err=%v", events, err)
	}
	var mixedEvent Event
	for _, event := range events {
		if event.ID == mixedEventID {
			mixedEvent = event
			break
		}
	}
	if mixedEvent.ID == 0 || mixedEvent.Metadata["approved"] != true || mixedEvent.Metadata["attempt"] != float64(2) {
		t.Fatalf("mixed JSON metadata did not decode: %#v", mixedEvent)
	}
	if _, ok := mixedEvent.Metadata["nested"].(map[string]any); !ok {
		t.Fatalf("nested audit metadata did not decode: %#v", mixedEvent.Metadata)
	}
	for _, event := range events {
		if _, exists := event.Metadata["accessToken"]; exists {
			t.Fatalf("token metadata survived sanitization: %#v", event.Metadata)
		}
	}

	file, err := service.ExportCSV(ctx, organizationID, ListQuery{EventType: "billing.subscription_updated"})
	if err != nil || !strings.HasPrefix(file.Filename, "audit-events-") || !strings.HasSuffix(file.Filename, ".csv") {
		t.Fatalf("unexpected audit CSV: file=%#v err=%v", file, err)
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(file.Content), "\ufeff"))).ReadAll()
	if err != nil || len(records) != 2 {
		t.Fatalf("read audit CSV: rows=%#v err=%v", records, err)
	}
	if records[1][2] != "billing.subscription_updated" || records[1][8] != "'=HYPERLINK(\"https://invalid.test\")" || !strings.Contains(records[1][9], `"approved": true`) {
		t.Fatalf("audit CSV content or spreadsheet protection mismatch: %#v", records[1])
	}
	if strings.Contains(string(file.Content), "foreign.event") {
		t.Fatalf("audit CSV leaked foreign tenant data: %s", file.Content)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events(organization_id,event_type,entity_type,summary,metadata_json)
		VALUES($1,'unsafe.event','organization','Unsafe metadata','{"providerCredential":"no"}'::jsonb)
	`, organizationID); err == nil || !strings.Contains(err.Error(), "audit_events_metadata_keys_safe") {
		t.Fatalf("unsafe top-level metadata key was not rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_events SET summary='rewritten' WHERE id=$1`, mixedEventID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("audit update was not rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM audit_events WHERE id=$1`, mixedEventID); err == nil || !strings.Contains(err.Error(), "workspace lifetime") {
		t.Fatalf("audit delete was not rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE audit_events`); err == nil || !strings.Contains(err.Error(), "workspace lifetime") {
		t.Fatalf("audit truncate was not rejected: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, foreignOrganizationID); err != nil {
		t.Fatalf("workspace deletion could not cascade its audit history: %v", err)
	}
	var foreignAuditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1`, foreignOrganizationID).Scan(&foreignAuditCount); err != nil || foreignAuditCount != 0 {
		t.Fatalf("deleted workspace retained audit rows: count=%d err=%v", foreignAuditCount, err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events(organization_id,event_type,entity_type,summary,metadata_json)
		SELECT $1,'volume.event','organization','Volume event ' || value,'{}'::jsonb
		FROM generate_series(1,10001) AS value
	`, organizationID); err != nil {
		t.Fatalf("seed audit export ceiling: %v", err)
	}
	if _, err := service.ExportCSV(ctx, organizationID, ListQuery{}); !errors.Is(err, ErrTooManyRows) {
		t.Fatalf("unfiltered audit export error=%v, want explicit ceiling", err)
	}
}

func auditSearchPathURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse audit test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func insertAuditOrganization(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES($1,$2) RETURNING id`, name, slug).Scan(&id); err != nil {
		t.Fatalf("insert audit organization: %v", err)
	}
	return id
}

func insertAuditUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES($1,'hash','Audit','Owner') RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert audit user: %v", err)
	}
	return id
}
