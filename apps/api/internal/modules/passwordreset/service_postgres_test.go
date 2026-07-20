package passwordreset

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
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
)

type resetDelivery struct {
	email     string
	firstName string
	token     string
}

type fakeResetMailer struct {
	deliveries []resetDelivery
	fail       error
}

func (m *fakeResetMailer) ProviderName() string { return "fake" }
func (m *fakeResetMailer) PasswordResetLink(token string) string {
	return "/reset-password?token=" + url.QueryEscape(token)
}
func (m *fakeResetMailer) SendPasswordReset(_ context.Context, email, firstName, token string) error {
	if m.fail != nil {
		return m.fail
	}
	m.deliveries = append(m.deliveries, resetDelivery{email: email, firstName: firstName, token: token})
	return nil
}

func TestPasswordResetLifecycleIsPrivateRecoverableAndGlobalAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to password reset postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_password_reset_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create password reset schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := passwordResetDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate password reset schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to password reset schema: %v", err)
	}
	defer pool.Close()

	oldPassword := "Existing-Secure-Password-27!"
	oldHash, err := moduleauth.SeedPasswordHash(oldPassword)
	if err != nil {
		t.Fatalf("hash seed password: %v", err)
	}
	var primaryOrgID, historicalOrgID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Primary','reset-primary') RETURNING id`).Scan(&primaryOrgID); err != nil {
		t.Fatalf("insert primary organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Historical','reset-historical') RETURNING id`).Scan(&historicalOrgID); err != nil {
		t.Fatalf("insert historical organization: %v", err)
	}
	var userID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(email,password_hash,first_name,last_name,email_verified_at)
		VALUES('owner@example.test',$1,'Morgan','Lee',NOW()) RETURNING id
	`, oldHash).Scan(&userID); err != nil {
		t.Fatalf("insert reset user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships(organization_id,user_id,role,membership_status)
		VALUES($1,$3,'owner','active'),($2,$3,'member','disabled')
	`, primaryOrgID, historicalOrgID, userID); err != nil {
		t.Fatalf("insert reset memberships: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions(user_id,organization_id,token_hash,expires_at)
		VALUES($1,$2,'session-one',NOW()+INTERVAL '1 day'),($1,$2,'session-two',NOW()+INTERVAL '1 day')
	`, userID, primaryOrgID); err != nil {
		t.Fatalf("insert reset sessions: %v", err)
	}

	for _, account := range []struct {
		email    string
		verified bool
		status   string
	}{
		{"unverified@example.test", false, "active"},
		{"disabled@example.test", true, "disabled"},
	} {
		var otherID int64
		verified := any(nil)
		if account.verified {
			verified = time.Now()
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO users(email,password_hash,first_name,last_name,email_verified_at)
			VALUES($1,$2,'Other','User',$3) RETURNING id
		`, account.email, oldHash, verified).Scan(&otherID); err != nil {
			t.Fatalf("insert %s: %v", account.email, err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO organization_memberships(organization_id,user_id,role,membership_status)
			VALUES($1,$2,'member',$3)
		`, primaryOrgID, otherID, account.status); err != nil {
			t.Fatalf("insert %s membership: %v", account.email, err)
		}
	}

	now := time.Date(2026, time.July, 20, 15, 0, 0, 0, time.UTC)
	mailer := &fakeResetMailer{}
	service := NewService(pool, mailer, WithLocalResetLinks(true))
	service.now = func() time.Time { return now }
	for _, email := range []string{"missing@example.test", "unverified@example.test", "disabled@example.test"} {
		result, err := service.Request(ctx, email)
		if err != nil || result.ResetLink != "" || len(mailer.deliveries) != 0 {
			t.Fatalf("generic ineligible request leaked for %s: result=%#v deliveries=%d err=%v", email, result, len(mailer.deliveries), err)
		}
	}

	requested, err := service.Request(ctx, " OWNER@EXAMPLE.TEST ")
	if err != nil || !strings.HasPrefix(requested.ResetLink, "/reset-password?token=") || len(mailer.deliveries) != 1 {
		t.Fatalf("eligible request did not deliver: result=%#v deliveries=%#v err=%v", requested, mailer.deliveries, err)
	}
	firstToken := mailer.deliveries[0].token
	var storedHash, deliveryStatus string
	var expiresAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT password_reset_token_hash,password_reset_expires_at,password_reset_delivery_status
		FROM users WHERE id=$1
	`, userID).Scan(&storedHash, &expiresAt, &deliveryStatus); err != nil {
		t.Fatalf("load persisted reset state: %v", err)
	}
	if storedHash == firstToken || storedHash != hashToken(firstToken) || len(storedHash) != 64 || deliveryStatus != "sent" || !expiresAt.Equal(now.Add(resetTokenTTL)) {
		t.Fatalf("unsafe or wrong reset state: hash=%q status=%q expires=%v", storedHash, deliveryStatus, expiresAt)
	}
	if replayedRequest, err := service.Request(ctx, "owner@example.test"); err != nil || replayedRequest.ResetLink != "" || len(mailer.deliveries) != 1 {
		t.Fatalf("recipient cooldown resent or exposed a link: result=%#v deliveries=%d err=%v", replayedRequest, len(mailer.deliveries), err)
	}
	stats, err := service.OperationalStats(ctx)
	if err != nil || stats.Outstanding != 1 || stats.StalePending != 0 || stats.FailedLast24h != 0 {
		t.Fatalf("unexpected active reset stats: stats=%#v err=%v", stats, err)
	}
	if err := service.Complete(ctx, CompleteInput{Token: "wrong", Password: "Replacement-Password-28!"}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong token returned %v", err)
	}

	newPassword := "Replacement-Password-28!"
	if err := service.Complete(ctx, CompleteInput{Token: firstToken, Password: newPassword}); err != nil {
		t.Fatalf("complete password reset: %v", err)
	}
	var sessions, audits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id=$1`, userID).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("reset sessions remain: count=%d err=%v", sessions, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE actor_user_id=$1 AND event_type='user.password_reset'`, userID).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("reset was not audited in every membership: count=%d err=%v", audits, err)
	}
	var tokenHash *string
	if err := pool.QueryRow(ctx, `SELECT password_reset_token_hash,password_reset_delivery_status FROM users WHERE id=$1`, userID).Scan(&tokenHash, &deliveryStatus); err != nil || tokenHash != nil || deliveryStatus != "consumed" {
		t.Fatalf("reset token was not consumed: hash=%v status=%q err=%v", tokenHash, deliveryStatus, err)
	}
	if err := service.Complete(ctx, CompleteInput{Token: firstToken, Password: "Another-Password-29!"}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("consumed reset token returned %v", err)
	}
	auth := moduleauth.NewService(pool)
	if _, err := auth.Login(ctx, "owner@example.test", oldPassword); !errors.Is(err, moduleauth.ErrUnauthorized) {
		t.Fatalf("old password remained usable: %v", err)
	}
	if login, err := auth.Login(ctx, "owner@example.test", newPassword); err != nil || login.State.Organization.ID != primaryOrgID {
		t.Fatalf("new password login failed: login=%#v err=%v", login, err)
	}

	now = now.Add(resetCooldown + time.Second)
	mailer.fail = errors.New("provider unavailable")
	failed, err := service.Request(ctx, "owner@example.test")
	if err != nil || failed.ResetLink != "" || len(mailer.deliveries) != 1 {
		t.Fatalf("provider failure was not generic: result=%#v deliveries=%d err=%v", failed, len(mailer.deliveries), err)
	}
	var failedTokenHash string
	if err := pool.QueryRow(ctx, `SELECT password_reset_token_hash,password_reset_delivery_status FROM users WHERE id=$1`, userID).Scan(&failedTokenHash, &deliveryStatus); err != nil || deliveryStatus != "failed" {
		t.Fatalf("failed delivery was not retained: status=%q err=%v", deliveryStatus, err)
	}
	stats, err = service.OperationalStats(ctx)
	if err != nil || stats.FailedLast24h != 1 {
		t.Fatalf("failed delivery not observable: stats=%#v err=%v", stats, err)
	}

	mailer.fail = nil
	recovered, err := service.Request(ctx, "owner@example.test")
	if err != nil || recovered.ResetLink == "" || len(mailer.deliveries) != 2 {
		t.Fatalf("failed delivery was not immediately retryable: result=%#v deliveries=%d err=%v", recovered, len(mailer.deliveries), err)
	}
	recoveryToken := mailer.deliveries[1].token
	if hashToken(recoveryToken) == failedTokenHash {
		t.Fatal("delivery retry did not rotate the reset token")
	}
	if err := service.Complete(ctx, CompleteInput{Token: recoveryToken, Password: "Recovered-Password-30!"}); err != nil {
		t.Fatalf("recovered reset completion failed: %v", err)
	}

	now = now.Add(resetCooldown + time.Second)
	expiring, err := service.Request(ctx, "owner@example.test")
	if err != nil || expiring.ResetLink == "" || len(mailer.deliveries) != 3 {
		t.Fatalf("expiring reset request failed: result=%#v err=%v", expiring, err)
	}
	expiringToken := mailer.deliveries[2].token
	now = now.Add(resetTokenTTL + time.Second)
	if err := service.Complete(ctx, CompleteInput{Token: expiringToken, Password: "Expired-Password-31!"}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token returned %v", err)
	}
	stats, err = service.OperationalStats(ctx)
	if err != nil || stats.Outstanding != 0 {
		t.Fatalf("expired token counted outstanding: stats=%#v err=%v", stats, err)
	}

	productionMailer := &fakeResetMailer{}
	productionService := NewService(pool, productionMailer)
	productionService.now = func() time.Time { return now }
	productionResult, err := productionService.Request(ctx, "owner@example.test")
	if err != nil || productionResult.ResetLink != "" || len(productionMailer.deliveries) != 1 {
		t.Fatalf("default runtime exposed or failed fake delivery: result=%#v deliveries=%d err=%v", productionResult, len(productionMailer.deliveries), err)
	}
}

func TestConcurrentOldPasswordLoginCannotSurvivePasswordReset(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to login/reset race postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_login_reset_race_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create login/reset race schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := passwordResetDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate login/reset race schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to login/reset race schema: %v", err)
	}
	defer pool.Close()

	oldPassword := "Concurrent-Old-Password-27!"
	passwordHash, err := moduleauth.SeedPasswordHash(oldPassword)
	if err != nil {
		t.Fatalf("hash race password: %v", err)
	}
	var organizationID, userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Race workspace',$1) RETURNING id`, "race-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("insert race organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(email,password_hash,first_name,last_name,email_verified_at)
		VALUES($1,$2,'Race','Owner',NOW()) RETURNING id
	`, "race-"+schema+"@example.test", passwordHash).Scan(&userID); err != nil {
		t.Fatalf("insert race user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,membership_status) VALUES($1,$2,'owner','active')`, organizationID, userID); err != nil {
		t.Fatalf("insert race membership: %v", err)
	}

	mailer := &fakeResetMailer{}
	resetService := NewService(pool, mailer, WithLocalResetLinks(true))
	if _, err := resetService.Request(ctx, "race-"+schema+"@example.test"); err != nil || len(mailer.deliveries) != 1 {
		t.Fatalf("request race reset: deliveries=%d err=%v", len(mailer.deliveries), err)
	}

	organizationBlocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin organization blocker: %v", err)
	}
	defer func() { _ = organizationBlocker.Rollback(context.Background()) }()
	if err := organizationBlocker.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, organizationID).Scan(&organizationID); err != nil {
		t.Fatalf("lock organization row: %v", err)
	}

	loginService := moduleauth.NewService(pool)
	loginResult := make(chan error, 1)
	go func() {
		_, loginErr := loginService.Login(ctx, "race-"+schema+"@example.test", oldPassword)
		loginResult <- loginErr
	}()
	waitForPostgresLock(t, ctx, pool, "%INSERT INTO sessions (user_id, organization_id, token_hash%")

	resetResult := make(chan error, 1)
	go func() {
		resetResult <- resetService.Complete(ctx, CompleteInput{
			Token:    mailer.deliveries[0].token,
			Password: "Concurrent-New-Password-28!",
		})
	}()
	waitForPostgresLock(t, ctx, pool, "%password_reset_token_hash%FOR UPDATE%")
	if err := organizationBlocker.Commit(ctx); err != nil {
		t.Fatalf("release organization row: %v", err)
	}
	if err := <-loginResult; err != nil {
		t.Fatalf("serialized old-password login should finish before reset: %v", err)
	}
	if err := <-resetResult; err != nil {
		t.Fatalf("complete concurrent reset: %v", err)
	}

	var sessions int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id=$1`, userID).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("old-password session survived reset: count=%d err=%v", sessions, err)
	}
	if _, err := loginService.Login(ctx, "race-"+schema+"@example.test", oldPassword); !errors.Is(err, moduleauth.ErrUnauthorized) {
		t.Fatalf("old password remained valid after concurrent reset: %v", err)
	}
}

func waitForPostgresLock(t *testing.T, ctx context.Context, pool *moduledb.Pool, queryPattern string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM pg_stat_activity
			WHERE pid<>pg_backend_pid() AND datname=current_database()
			  AND wait_event_type='Lock' AND query LIKE $1
		`, queryPattern).Scan(&waiting); err != nil {
			t.Fatalf("inspect PostgreSQL lock wait: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for PostgreSQL lock matching %q", queryPattern)
}

func passwordResetDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
