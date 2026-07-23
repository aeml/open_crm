package users_test

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
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

func TestInvitationLifecycleRotatesExpiresRevokesAndCompletesAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to invitation test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_invitations_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create invitation schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := invitationDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate invitation schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated invitation schema: %v", err)
	}
	defer pool.Close()

	passwordHash, err := moduleauth.SeedPasswordHash("Owner-Secure-Password-29!")
	if err != nil {
		t.Fatalf("hash invitation owner password: %v", err)
	}
	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Invitations', $1) RETURNING id`, "invitations-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create invitation organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Foreign invitations', $1) RETURNING id`, "foreign-invitations-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign invitation organization: %v", err)
	}
	ownerID := insertInvitationUser(t, ctx, pool, "owner-"+schema+"@example.test", passwordHash, "Primary", "Owner", true)
	foreignOwnerID := insertInvitationUser(t, ctx, pool, "foreign-owner-"+schema+"@example.test", passwordHash, "Foreign", "Owner", true)
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id, user_id, role)
		VALUES ($1, $2, 'owner'), ($3, $4, 'owner')
	`, organizationID, ownerID, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatalf("create invitation owner memberships: %v", err)
	}

	service := moduleusers.NewService(pool)
	inviteEmail := "invitee-" + schema + "@example.test"
	created, err := service.CreateForOrganization(ctx, organizationID, moduleusers.CreateUserInput{
		Email: inviteEmail, FirstName: "Jamie", LastName: "Pilot", Role: "member",
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if created.SetupToken == "" || created.DeliveryKey == "" || created.InvitationStatus != moduleusers.InvitationStatusPending || created.InvitationDeliveryStatus != "pending" || created.InvitationExpiresAt == nil {
		t.Fatalf("unexpected created invitation: %#v", created)
	}
	firstToken := created.SetupToken
	firstDeliveryKey := created.DeliveryKey
	var persistedHash, persistedDeliveryHash string
	if err := pool.QueryRow(ctx, `SELECT password_setup_token_hash, password_setup_delivery_key_hash FROM users WHERE id=$1`, created.ID).Scan(&persistedHash, &persistedDeliveryHash); err != nil || persistedHash == firstToken || persistedDeliveryHash == firstDeliveryKey {
		t.Fatalf("raw invitation credentials were persisted: token_hash=%q delivery_hash=%q err=%v", persistedHash, persistedDeliveryHash, err)
	}
	if _, err := service.RecordInvitationDelivery(ctx, foreignOrganizationID, created.ID, firstDeliveryKey, "sent", "foreign-message"); !errors.Is(err, moduleusers.ErrNotFound) {
		t.Fatalf("expected foreign delivery update denial, got %v", err)
	}
	if status, err := service.RecordInvitationDelivery(ctx, organizationID, created.ID, firstDeliveryKey, "sent", "postmark-invite-created"); err != nil || status != "sent" {
		t.Fatalf("record invitation delivery: status=%q err=%v", status, err)
	}

	listedPage, err := service.ListByOrganization(ctx, organizationID, moduleusers.ListQuery{})
	listed := listedPage.Users
	if err != nil || len(listed) != 2 || listed[1].InvitationStatus != moduleusers.InvitationStatusPending || listed[1].InvitationDeliveryStatus != "sent" || listed[1].InvitationExpiresAt == nil {
		t.Fatalf("unexpected listed pending invitation: users=%#v err=%v", listed, err)
	}
	if _, err := service.ResendInvitation(ctx, foreignOrganizationID, created.ID, foreignOwnerID); !errors.Is(err, moduleusers.ErrNotFound) {
		t.Fatalf("expected foreign resend denial, got %v", err)
	}
	if _, err := service.RevokeInvitation(ctx, foreignOrganizationID, created.ID, foreignOwnerID); !errors.Is(err, moduleusers.ErrNotFound) {
		t.Fatalf("expected foreign revoke denial, got %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE users SET password_setup_expires_at=NOW()-INTERVAL '1 minute' WHERE id=$1`, created.ID); err != nil {
		t.Fatalf("expire invitation: %v", err)
	}
	listedPage, err = service.ListByOrganization(ctx, organizationID, moduleusers.ListQuery{})
	listed = listedPage.Users
	if err != nil || listed[1].InvitationStatus != moduleusers.InvitationStatusExpired {
		t.Fatalf("expected explicit expired state, users=%#v err=%v", listed, err)
	}
	resent, err := service.ResendInvitation(ctx, organizationID, created.ID, ownerID)
	if err != nil {
		t.Fatalf("resend expired invitation: %v", err)
	}
	if resent.SetupToken == "" || resent.SetupToken == firstToken || resent.DeliveryKey == "" || resent.DeliveryKey == firstDeliveryKey || resent.InvitationStatus != moduleusers.InvitationStatusPending || resent.InvitationDeliveryStatus != "pending" {
		t.Fatalf("expected rotated pending invitation, got %#v", resent)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET password_setup_delivery_status='bounced' WHERE id=$1`, created.ID); err != nil {
		t.Fatalf("simulate feedback race: %v", err)
	}
	if status, err := service.RecordInvitationDelivery(ctx, organizationID, created.ID, resent.DeliveryKey, "sent", "postmark-late-send"); err != nil || status != "bounced" {
		t.Fatalf("feedback race was not preserved: status=%q err=%v", status, err)
	}
	if _, err := service.CompleteSetup(ctx, moduleusers.CompleteSetupInput{Token: firstToken, Password: "Invitee-Secure-Password-31!"}); !errors.Is(err, moduleusers.ErrInvalidSetupToken) {
		t.Fatalf("expected old setup token rejection, got %v", err)
	}

	revoked, err := service.RevokeInvitation(ctx, organizationID, created.ID, ownerID)
	if err != nil {
		t.Fatalf("revoke invitation: %v", err)
	}
	if !revoked.Changed || revoked.User.Status != moduleusers.MembershipStatusDisabled || revoked.User.InvitationStatus != moduleusers.InvitationStatusRevoked {
		t.Fatalf("unexpected revoked invitation: %#v", revoked)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET password_setup_token_hash='inconsistent-token' WHERE id=$1`, created.ID); err == nil {
		t.Fatal("expected the database to reject a token on a revoked invitation")
	}
	if _, err := service.CompleteSetup(ctx, moduleusers.CompleteSetupInput{Token: resent.SetupToken, Password: "Invitee-Secure-Password-31!"}); !errors.Is(err, moduleusers.ErrInvalidSetupToken) {
		t.Fatalf("expected revoked setup token rejection, got %v", err)
	}
	repeated, err := service.RevokeInvitation(ctx, organizationID, created.ID, ownerID)
	if err != nil || repeated.Changed {
		t.Fatalf("expected idempotent invitation revocation, result=%#v err=%v", repeated, err)
	}
	if _, err := service.ResendInvitation(ctx, organizationID, created.ID, ownerID); !errors.Is(err, moduleusers.ErrInvitationInactive) {
		t.Fatalf("expected disabled invitation resend rejection, got %v", err)
	}

	if _, err := service.SetStatus(ctx, organizationID, created.ID, ownerID, moduleusers.SetStatusInput{Status: moduleusers.MembershipStatusActive}); err != nil {
		t.Fatalf("reactivate revoked invitation: %v", err)
	}
	finalInvite, err := service.ResendInvitation(ctx, organizationID, created.ID, ownerID)
	if err != nil {
		t.Fatalf("resend reactivated invitation: %v", err)
	}
	newPassword := "Invitee-Secure-Password-31!"
	if _, err := service.CompleteSetup(ctx, moduleusers.CompleteSetupInput{Token: finalInvite.SetupToken, Password: newPassword}); err != nil {
		t.Fatalf("complete reissued invitation: %v", err)
	}
	if _, err := service.CompleteSetup(ctx, moduleusers.CompleteSetupInput{Token: finalInvite.SetupToken, Password: newPassword}); !errors.Is(err, moduleusers.ErrInvalidSetupToken) {
		t.Fatalf("expected one-time setup token rejection, got %v", err)
	}
	listedPage, err = service.ListByOrganization(ctx, organizationID, moduleusers.ListQuery{})
	listed = listedPage.Users
	if err != nil || listed[1].InvitationStatus != moduleusers.InvitationStatusAccepted || listed[1].SetupPending || listed[1].InvitationExpiresAt != nil {
		t.Fatalf("unexpected accepted invitation state: users=%#v err=%v", listed, err)
	}
	if _, err := moduleauth.NewService(pool).Login(ctx, inviteEmail, newPassword); err != nil {
		t.Fatalf("accepted invitee could not sign in: %v", err)
	}
	if _, err := service.RevokeInvitation(ctx, organizationID, created.ID, ownerID); !errors.Is(err, moduleusers.ErrInvitationNotPending) {
		t.Fatalf("expected accepted-user revoke rejection, got %v", err)
	}
	var completedDeliveryHash, completedProviderMessageID *string
	if err := pool.QueryRow(ctx, `SELECT password_setup_delivery_key_hash, password_setup_provider_message_id FROM users WHERE id=$1`, created.ID).Scan(&completedDeliveryHash, &completedProviderMessageID); err != nil || completedDeliveryHash != nil || completedProviderMessageID != nil {
		t.Fatalf("accepted invitation retained delivery correlation: hash=%v provider=%v err=%v", completedDeliveryHash, completedProviderMessageID, err)
	}

	var resendAudits, revokeAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND entity_id=$2 AND event_type='user.invitation_resent'`, organizationID, created.ID).Scan(&resendAudits); err != nil || resendAudits != 2 {
		t.Fatalf("expected two resend audit events, count=%d err=%v", resendAudits, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND entity_id=$2 AND event_type='user.invitation_revoked'`, organizationID, created.ID).Scan(&revokeAudits); err != nil || revokeAudits != 1 {
		t.Fatalf("expected one idempotent revoke audit event, count=%d err=%v", revokeAudits, err)
	}
}

func invitationDatabaseURL(t *testing.T, databaseURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse invitation database url: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func insertInvitationUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email, passwordHash, firstName, lastName string, verified bool) int64 {
	t.Helper()
	var userID int64
	var verifiedAt any
	if verified {
		verifiedAt = time.Now().UTC()
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, first_name, last_name, email_verified_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, email, passwordHash, firstName, lastName, verifiedAt).Scan(&userID); err != nil {
		t.Fatalf("insert invitation user: %v", err)
	}
	return userID
}
