package auth

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
)

func TestSessionManagementIsPrivateGlobalAndAuditedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to session postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_sessions_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create session schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := sessionDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate session schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated session schema: %v", err)
	}
	defer pool.Close()

	primaryOrgID := sessionInsertOrganization(t, ctx, pool, "Primary workspace", "primary-"+schema)
	historicalOrgID := sessionInsertOrganization(t, ctx, pool, "Historical workspace", "historical-"+schema)
	foreignOrgID := sessionInsertOrganization(t, ctx, pool, "Foreign workspace", "foreign-"+schema)
	userID := sessionInsertUser(t, ctx, pool, "owner-"+schema+"@example.test")
	foreignUserID := sessionInsertUser(t, ctx, pool, "foreign-"+schema+"@example.test")
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships(organization_id,user_id,role,membership_status)
		VALUES($1,$3,'owner','active'),($2,$3,'member','disabled'),($4,$5,'owner','active')
	`, primaryOrgID, historicalOrgID, userID, foreignOrgID, foreignUserID); err != nil {
		t.Fatalf("insert session memberships: %v", err)
	}

	currentToken := "current-session-token"
	otherToken := "other-session-token"
	foreignToken := "foreign-session-token"
	currentID := sessionInsertSession(t, ctx, pool, userID, primaryOrgID, currentToken, "2 hours", "3 hours")
	otherID := sessionInsertSession(t, ctx, pool, userID, primaryOrgID, otherToken, "1 hour", "3 hours")
	foreignID := sessionInsertSession(t, ctx, pool, foreignUserID, foreignOrgID, foreignToken, "1 hour", "30 minutes")
	sessionInsertSession(t, ctx, pool, userID, primaryOrgID, "expired-session-token", "-2 hours", "-1 hour")

	service := NewService(pool)
	sessions, err := service.ListSessions(ctx, userID, currentToken)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("list active sessions: sessions=%#v err=%v", sessions, err)
	}
	if sessions[0].ID != otherID || sessions[0].Current || sessions[0].Organization.Name != "Primary workspace" {
		t.Fatalf("unexpected newest session: %#v", sessions[0])
	}
	if sessions[1].ID != currentID || !sessions[1].Current {
		t.Fatalf("current session was not identified: %#v", sessions[1])
	}
	var expiredCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id=$1 AND expires_at<=NOW()`, userID).Scan(&expiredCount); err != nil || expiredCount != 0 {
		t.Fatalf("expired sessions were not pruned: count=%d err=%v", expiredCount, err)
	}
	if _, err := service.ListSessions(ctx, userID, "wrong-token"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong current token returned %v", err)
	}
	if err := service.RevokeSession(ctx, userID, currentID, currentToken); !errors.Is(err, ErrCurrentSession) {
		t.Fatalf("current session revocation returned %v", err)
	}
	if err := service.RevokeSession(ctx, userID, foreignID, currentToken); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("foreign session revocation returned %v", err)
	}
	if err := service.RevokeSession(ctx, userID, otherID, currentToken); err != nil {
		t.Fatalf("revoke another session: %v", err)
	}
	var otherCount, singleAuditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE id=$1`, otherID).Scan(&otherCount); err != nil || otherCount != 0 {
		t.Fatalf("revoked session remains: count=%d err=%v", otherCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE actor_user_id=$1 AND event_type='user.session_revoked' AND entity_id=$2`, userID, otherID).Scan(&singleAuditCount); err != nil || singleAuditCount != 2 {
		t.Fatalf("single revocation was not audited in every membership: count=%d err=%v", singleAuditCount, err)
	}

	sessionInsertSession(t, ctx, pool, userID, primaryOrgID, "other-session-two", "1 hour", "4 hours")
	sessionInsertSession(t, ctx, pool, userID, primaryOrgID, "other-session-three", "30 minutes", "5 hours")
	revoked, err := service.RevokeOtherSessions(ctx, userID, currentToken)
	if err != nil || revoked != 2 {
		t.Fatalf("revoke other sessions: revoked=%d err=%v", revoked, err)
	}
	var remaining, bulkAuditCount, recordedRevoked int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id=$1`, userID).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("expected only current session to remain: count=%d err=%v", remaining, err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*),COALESCE(MAX((metadata_json->>'revokedCount')::int),0)
		FROM audit_events WHERE actor_user_id=$1 AND event_type='user.other_sessions_revoked'
	`, userID).Scan(&bulkAuditCount, &recordedRevoked); err != nil || bulkAuditCount != 2 || recordedRevoked != 2 {
		t.Fatalf("bulk revocation audit mismatch: count=%d revoked=%d err=%v", bulkAuditCount, recordedRevoked, err)
	}
	revoked, err = service.RevokeOtherSessions(ctx, userID, currentToken)
	if err != nil || revoked != 0 {
		t.Fatalf("idempotent empty revocation: revoked=%d err=%v", revoked, err)
	}
	if _, err := service.CurrentSession(ctx, currentToken); err != nil {
		t.Fatalf("bulk revocation ended current session: %v", err)
	}
	canceledCtx, cancelCurrentSession := context.WithCancel(ctx)
	cancelCurrentSession()
	if _, err := service.CurrentSession(canceledCtx, currentToken); !errors.Is(err, context.Canceled) || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("canceled lookup must remain an infrastructure error, got %v", err)
	}
}

func sessionInsertOrganization(t *testing.T, ctx context.Context, pool *moduledb.Pool, name, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES($1,$2) RETURNING id`, name, slug).Scan(&id); err != nil {
		t.Fatalf("insert session organization: %v", err)
	}
	return id
}

func sessionInsertUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email string) int64 {
	t.Helper()
	passwordHash, err := SeedPasswordHash("Session-Test-Password-27!")
	if err != nil {
		t.Fatalf("hash session password: %v", err)
	}
	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(email,password_hash,first_name,last_name,email_verified_at)
		VALUES($1,$2,'Session','Owner',NOW()) RETURNING id
	`, email, passwordHash).Scan(&id); err != nil {
		t.Fatalf("insert session user: %v", err)
	}
	return id
}

func sessionInsertSession(t *testing.T, ctx context.Context, pool *moduledb.Pool, userID, organizationID int64, token, lastSeenAgo, expiresIn string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions(user_id,organization_id,token_hash,created_at,last_seen_at,expires_at)
		VALUES($1,$2,$3,NOW()-INTERVAL '6 hours',NOW()-$4::interval,NOW()+$5::interval)
		RETURNING id
	`, userID, organizationID, HashSessionToken(token), lastSeenAgo, expiresIn).Scan(&id); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	return id
}

func sessionDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse session database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
