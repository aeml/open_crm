package emailsuppressions

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestUnsubscribeIsTenantScopedIdempotentAndPreservesStrongestReasonAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to email suppression postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_email_suppressions_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create email suppression schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := emailSuppressionsDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate email suppression schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated email suppression schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name, slug) VALUES('Suppression', $1) RETURNING id`, "suppression-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name, slug) VALUES('Foreign suppression', $1) RETURNING id`, "foreign-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}

	service := NewService(pool, "test-unsubscribe-signing-secret")
	email := "lead@example.test"
	token, err := service.UnsubscribeToken(organizationID, email)
	if err != nil {
		t.Fatalf("create unsubscribe token: %v", err)
	}
	if err := service.ValidateUnsubscribeToken(token); err != nil {
		t.Fatalf("validate unsubscribe token: %v", err)
	}
	assertSuppressionCount(t, ctx, pool, 0)

	bounce, err := service.Suppress(ctx, organizationID, email, "bounce", "dsn", 0)
	if err != nil || bounce.Reason != "bounce" {
		t.Fatalf("store bounce: suppression=%#v err=%v", bounce, err)
	}
	unsubscribed, err := service.UnsubscribeByToken(ctx, token)
	if err != nil || unsubscribed.OrganizationID != organizationID || unsubscribed.Reason != "unsubscribed" || unsubscribed.Source != "public_unsubscribe" {
		t.Fatalf("unsubscribe: suppression=%#v err=%v", unsubscribed, err)
	}
	replayed, err := service.UnsubscribeByToken(ctx, token)
	if err != nil || replayed.ID != unsubscribed.ID || !replayed.UpdatedAt.Equal(unsubscribed.UpdatedAt) {
		t.Fatalf("idempotent replay changed state: first=%#v replay=%#v err=%v", unsubscribed, replayed, err)
	}
	assertSuppressionCount(t, ctx, pool, 1)

	complaint, err := service.Suppress(ctx, organizationID, email, "complaint", "arf", 0)
	if err != nil || complaint.Reason != "complaint" || complaint.Source != "arf" {
		t.Fatalf("store complaint: suppression=%#v err=%v", complaint, err)
	}
	for _, weaker := range []struct {
		reason string
		source string
	}{
		{reason: "bounce", source: "later_dsn"},
		{reason: "unsubscribed", source: "public_unsubscribe"},
		{reason: "manual", source: "operator"},
	} {
		result, err := service.Suppress(ctx, organizationID, email, weaker.reason, weaker.source, 0)
		if err != nil || result.Reason != "complaint" || result.Source != "arf" || !result.UpdatedAt.Equal(complaint.UpdatedAt) {
			t.Fatalf("weaker %s changed complaint: suppression=%#v err=%v", weaker.reason, result, err)
		}
	}
	if result, err := service.UnsubscribeByToken(ctx, token); err != nil || result.Reason != "complaint" || result.Source != "arf" || !result.UpdatedAt.Equal(complaint.UpdatedAt) {
		t.Fatalf("public replay downgraded complaint: suppression=%#v err=%v", result, err)
	}

	foreignToken, err := service.UnsubscribeToken(foreignOrganizationID, email)
	if err != nil {
		t.Fatalf("create foreign unsubscribe token: %v", err)
	}
	foreign, err := service.UnsubscribeByToken(ctx, foreignToken)
	if err != nil || foreign.OrganizationID != foreignOrganizationID || foreign.Reason != "unsubscribed" {
		t.Fatalf("foreign unsubscribe: suppression=%#v err=%v", foreign, err)
	}
	assertSuppressionCount(t, ctx, pool, 2)
}

func assertSuppressionCount(t *testing.T, ctx context.Context, pool *moduledb.Pool, expected int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_suppressions`).Scan(&count); err != nil || count != expected {
		t.Fatalf("suppression count=%d want=%d err=%v", count, expected, err)
	}
}

func emailSuppressionsDatabaseURL(t *testing.T, databaseURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse email suppression database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
