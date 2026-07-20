package onboarding

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

type recordedVerification struct {
	email       string
	firstName   string
	token       string
	deliveryKey string
}

type fakeVerificationMailer struct {
	messages []recordedVerification
	fail     error
}

func (m *fakeVerificationMailer) ProviderName() string { return "fake" }
func (m *fakeVerificationMailer) VerificationLink(token string) string {
	return "/verify-email?token=" + token
}
func (m *fakeVerificationMailer) SendEmailVerification(_ context.Context, email, firstName, token string, _, _ int64, deliveryKey string) (string, error) {
	if m.fail != nil {
		return "", m.fail
	}
	m.messages = append(m.messages, recordedVerification{email: email, firstName: firstName, token: token, deliveryKey: deliveryKey})
	return "postmark-verification-test", nil
}

func TestVerifiedWorkspaceSignupIsIdempotentAndStartsTrialAfterVerificationAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to onboarding postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_onboarding_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create onboarding schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := onboardingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate onboarding schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to onboarding schema: %v", err)
	}
	defer pool.Close()

	mailer := &fakeVerificationMailer{}
	service := NewService(pool, mailer)
	auth := moduleauth.NewService(pool)
	input := BootstrapInput{
		OrganizationName: "Northstar Logistics", BusinessType: "product-sales",
		FirstName: "Morgan", LastName: "Lee", Email: "Owner@Northstar.Test",
		Password: "Correct-Horse-Battery-27!", IdempotencyKey: "workspace-northstar-request-001",
	}
	created, err := service.BootstrapOrganization(ctx, input)
	if err != nil || !created.Created || !created.VerificationRequired || created.Email != "owner@northstar.test" || !strings.HasPrefix(created.VerificationLink, "/verify-email?token=") {
		t.Fatalf("unexpected verified-signup result: result=%#v err=%v", created, err)
	}
	if len(mailer.messages) != 1 || mailer.messages[0].email != "owner@northstar.test" || mailer.messages[0].firstName != "Morgan" || mailer.messages[0].token == "" || mailer.messages[0].deliveryKey == "" {
		t.Fatalf("unexpected verification delivery: %#v", mailer.messages)
	}
	verificationToken := mailer.messages[0].token
	verificationDeliveryKey := mailer.messages[0].deliveryKey

	var organizationID, userID int64
	var slug, status string
	var trialStartedAt, trialEndsAt, verifiedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT organization.id, users.id, organization.slug, organization.subscription_status,
		       organization.trial_started_at, organization.trial_ends_at, users.email_verified_at
		FROM workspace_bootstrap_requests request
		JOIN organizations organization ON organization.id=request.organization_id
		JOIN users ON users.id=request.user_id
	`).Scan(&organizationID, &userID, &slug, &status, &trialStartedAt, &trialEndsAt, &verifiedAt); err != nil {
		t.Fatalf("load pending workspace: %v", err)
	}
	if slug != "northstar-logistics" || status != "trialing" || trialStartedAt != nil || trialEndsAt != nil || verifiedAt != nil {
		t.Fatalf("unverified workspace started early: slug=%q status=%q start=%v end=%v verified=%v", slug, status, trialStartedAt, trialEndsAt, verifiedAt)
	}
	var deliveryStatus, deliveryKeyHash, providerMessageID string
	if err := pool.QueryRow(ctx, `
		SELECT email_verification_delivery_status, email_verification_delivery_key_hash, email_verification_provider_message_id
		FROM users WHERE id=$1
	`, userID).Scan(&deliveryStatus, &deliveryKeyHash, &providerMessageID); err != nil || deliveryStatus != "sent" || deliveryKeyHash != hashValue(verificationDeliveryKey) || deliveryKeyHash == verificationDeliveryKey || providerMessageID != "postmark-verification-test" {
		t.Fatalf("unsafe or wrong verification delivery state: status=%q key=%q provider=%q err=%v", deliveryStatus, deliveryKeyHash, providerMessageID, err)
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id=$1`, userID).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("unverified workspace created session: count=%d err=%v", sessions, err)
	}
	if _, err := auth.Login(ctx, input.Email, input.Password); !errors.Is(err, moduleauth.ErrEmailUnverified) {
		t.Fatalf("unverified password login returned %v", err)
	}

	replayed, err := service.BootstrapOrganization(ctx, input)
	if err != nil || replayed.Created || !replayed.VerificationRequired || len(mailer.messages) != 1 {
		t.Fatalf("idempotent replay changed workspace or resent inside cooldown: result=%#v deliveries=%d err=%v", replayed, len(mailer.messages), err)
	}
	conflicting := input
	conflicting.OrganizationName = "Changed name"
	if _, err := service.BootstrapOrganization(ctx, conflicting); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting idempotency replay returned %v", err)
	}
	conflicting = input
	conflicting.Password = "Different-Password-For-Retry!"
	if _, err := service.BootstrapOrganization(ctx, conflicting); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("secret-changing idempotency replay returned %v", err)
	}
	var workspaces, owners, requests, provisionAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&workspaces); err != nil {
		t.Fatalf("count workspaces: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&owners); err != nil {
		t.Fatalf("count owners: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM workspace_bootstrap_requests`).Scan(&requests); err != nil {
		t.Fatalf("count bootstrap requests: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE event_type='workspace.provisioned'`).Scan(&provisionAudits); err != nil {
		t.Fatalf("count provisioning audits: %v", err)
	}
	if workspaces != 1 || owners != 1 || requests != 1 || provisionAudits != 1 {
		t.Fatalf("replay duplicated state: workspaces=%d owners=%d requests=%d audits=%d", workspaces, owners, requests, provisionAudits)
	}

	if _, err := service.VerifyEmail(ctx, "wrong-token"); !errors.Is(err, ErrInvalidVerificationToken) {
		t.Fatalf("wrong verification token returned %v", err)
	}
	verified, err := service.VerifyEmail(ctx, verificationToken)
	if err != nil || verified.SessionToken == "" || verified.State.User.ID != userID || verified.State.Organization.ID != organizationID || verified.State.Membership.Role != "owner" {
		t.Fatalf("unexpected verification result: result=%#v err=%v", verified, err)
	}
	if err := pool.QueryRow(ctx, `SELECT trial_started_at,trial_ends_at FROM organizations WHERE id=$1`, organizationID).Scan(&trialStartedAt, &trialEndsAt); err != nil {
		t.Fatalf("load started trial: %v", err)
	}
	if trialStartedAt == nil || trialEndsAt == nil || trialEndsAt.Sub(*trialStartedAt) < trialLength-time.Minute || trialEndsAt.Sub(*trialStartedAt) > trialLength+time.Minute {
		t.Fatalf("verified trial window is not 14 days: start=%v end=%v", trialStartedAt, trialEndsAt)
	}
	if _, err := service.VerifyEmail(ctx, verificationToken); !errors.Is(err, ErrInvalidVerificationToken) {
		t.Fatalf("consumed verification token returned %v", err)
	}
	var completedDeliveryKeyHash, completedProviderMessageID *string
	if err := pool.QueryRow(ctx, `SELECT email_verification_delivery_key_hash,email_verification_provider_message_id FROM users WHERE id=$1`, userID).Scan(&completedDeliveryKeyHash, &completedProviderMessageID); err != nil || completedDeliveryKeyHash != nil || completedProviderMessageID != nil {
		t.Fatalf("verified user retained delivery correlation: key=%v provider=%v err=%v", completedDeliveryKeyHash, completedProviderMessageID, err)
	}
	if _, err := auth.CurrentSession(ctx, verified.SessionToken); err != nil {
		t.Fatalf("verified session was not usable: %v", err)
	}
	if login, err := auth.Login(ctx, input.Email, input.Password); err != nil || login.State.Organization.ID != organizationID {
		t.Fatalf("verified password login failed: result=%#v err=%v", login, err)
	}
	if _, err := service.BootstrapOrganization(ctx, input); !errors.Is(err, ErrAlreadyVerified) {
		t.Fatalf("verified bootstrap replay returned %v", err)
	}
	var verificationAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND actor_user_id=$2 AND event_type='user.email_verified'`, organizationID, userID).Scan(&verificationAudits); err != nil || verificationAudits != 1 {
		t.Fatalf("missing verification audit: count=%d err=%v", verificationAudits, err)
	}

	second := BootstrapInput{
		OrganizationName: "Northstar Logistics", BusinessType: "services", FirstName: "Taylor", LastName: "Owner",
		Email: "taylor@northstar.test", Password: "Second-Correct-Password-28!", IdempotencyKey: "workspace-northstar-request-002",
	}
	secondResult, err := service.BootstrapOrganization(ctx, second)
	if err != nil || secondResult.Created != true {
		t.Fatalf("create same-name workspace: result=%#v err=%v", secondResult, err)
	}
	var secondSlug string
	if err := pool.QueryRow(ctx, `SELECT organization.slug FROM organizations organization JOIN users ON users.email=$1 JOIN organization_memberships membership ON membership.user_id=users.id AND membership.organization_id=organization.id`, second.Email).Scan(&secondSlug); err != nil || secondSlug == slug || !strings.HasPrefix(secondSlug, slug+"-") {
		t.Fatalf("same-name workspace slug was not made unique: slug=%q err=%v", secondSlug, err)
	}
	beforeResend := len(mailer.messages)
	if _, err := service.ResendVerification(ctx, "missing@example.test"); err != nil || len(mailer.messages) != beforeResend {
		t.Fatalf("generic missing-account resend leaked or delivered: deliveries=%d err=%v", len(mailer.messages), err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET email_verification_sent_at=NOW()-INTERVAL '2 minutes' WHERE email=$1`, second.Email); err != nil {
		t.Fatalf("age resend cooldown: %v", err)
	}
	resent, err := service.ResendVerification(ctx, second.Email)
	if err != nil || resent.VerificationLink == "" || len(mailer.messages) != beforeResend+1 || mailer.messages[len(mailer.messages)-1].token == verificationToken || mailer.messages[len(mailer.messages)-1].deliveryKey == "" || mailer.messages[len(mailer.messages)-1].deliveryKey == verificationDeliveryKey {
		t.Fatalf("verification resend did not rotate and deliver: result=%#v deliveries=%d err=%v", resent, len(mailer.messages), err)
	}
}

func TestWorkspaceSignupDeliveryFailureIsRecoverableWithSameIdempotencyKeyAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to onboarding recovery postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_onboarding_recovery_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create onboarding recovery schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := onboardingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate onboarding recovery schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to onboarding recovery schema: %v", err)
	}
	defer pool.Close()

	mailer := &fakeVerificationMailer{fail: errors.New("provider unavailable")}
	service := NewService(pool, mailer)
	input := BootstrapInput{OrganizationName: "Recoverable", BusinessType: "general", FirstName: "Riley", LastName: "Owner", Email: "riley@recover.test", Password: "Recovery-Password-123!", IdempotencyKey: "workspace-recovery-request-001"}
	if _, err := service.BootstrapOrganization(ctx, input); !errors.Is(err, ErrVerificationDelivery) {
		t.Fatalf("provider failure returned %v", err)
	}
	mailer.fail = nil
	recovered, err := service.BootstrapOrganization(ctx, input)
	if err != nil || recovered.Created || recovered.VerificationLink == "" || len(mailer.messages) != 1 {
		t.Fatalf("same-key recovery failed: result=%#v deliveries=%d err=%v", recovered, len(mailer.messages), err)
	}
	var workspaces, users int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&workspaces); err != nil {
		t.Fatalf("count recovered workspaces: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		t.Fatalf("count recovered users: %v", err)
	}
	if workspaces != 1 || users != 1 {
		t.Fatalf("delivery recovery duplicated provisioning: workspaces=%d users=%d", workspaces, users)
	}
}

func onboardingDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse onboarding database url: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
