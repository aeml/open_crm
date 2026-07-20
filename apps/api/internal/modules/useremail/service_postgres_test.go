package useremail

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	platformsecrets "github.com/aeml/open_crm/apps/api/internal/platform/secrets"
)

type postgresOAuthRefresher struct {
	mu     sync.Mutex
	calls  int
	tokens OAuthTokenSet
}

func (f *postgresOAuthRefresher) RefreshOAuthToken(_ context.Context, _ SyncCredentials) (OAuthTokenSet, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	time.Sleep(25 * time.Millisecond)
	return f.tokens, nil
}

func (f *postgresOAuthRefresher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type postgresOAuthSender struct {
	mu      sync.Mutex
	access  []string
	message []moduleemail.Message
}

func (s *postgresOAuthSender) Send(_ context.Context, creds SyncCredentials, message moduleemail.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.access = append(s.access, creds.OAuthAccess)
	s.message = append(s.message, message)
	return nil
}

func TestOAuthMailDeliveryIsEncryptedSerializedAndTenantBoundAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to OAuth delivery test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_oauth_delivery_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create OAuth delivery schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := oauthDeliveryDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate OAuth delivery schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated OAuth delivery schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('OAuth Delivery', $1) RETURNING id`, "oauth-delivery-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create OAuth delivery organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Foreign OAuth Delivery', $1) RETURNING id`, "foreign-oauth-delivery-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign OAuth delivery organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'hash', 'Revenue', 'Rep') RETURNING id`, "oauth-rep-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("create OAuth delivery user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, 'member'), ($3, $2, 'member')`, organizationID, userID, foreignOrganizationID); err != nil {
		t.Fatalf("create OAuth delivery memberships: %v", err)
	}

	cipher, err := platformsecrets.NewCipher([]byte("12345678901234567890123456789012"))
	if err != nil {
		t.Fatalf("create OAuth delivery cipher: %v", err)
	}
	setup := NewService(pool, cipher)
	if _, err := setup.Upsert(ctx, organizationID, userID, UpsertInput{
		FromEmail: "rep@example.test", FromName: "Revenue Rep", Provider: "imap", AuthMethod: "password", SyncEnabled: true,
		SMTPHost: "smtp.example.test", SMTPPort: 587, SMTPUsername: "rep@example.test", SMTPPassword: "smtp-secret", SMTPUseTLS: true,
		IMAPHost: "imap.example.test", IMAPPort: 993, IMAPUsername: "rep@example.test", IMAPPassword: "imap-secret", IMAPUseTLS: true,
	}); err != nil {
		t.Fatalf("save password-backed account before OAuth transition: %v", err)
	}
	account, err := setup.Upsert(ctx, organizationID, userID, UpsertInput{
		FromEmail: "rep@example.test", FromName: "Revenue Rep", Provider: "google", AuthMethod: "oauth", SyncEnabled: true,
	})
	if err != nil {
		t.Fatalf("save OAuth-only account: %v", err)
	}
	if account.HasPassword || account.HasIMAPPassword || account.SMTPHost != "" || account.SMTPUsername != "" || account.IMAPHost != "" || account.IMAPUsername != "" {
		t.Fatalf("OAuth-only account retained SMTP credentials: %#v", account)
	}
	past := time.Now().UTC().Add(-time.Hour)
	account, err = setup.SaveOAuthConnection(ctx, organizationID, userID, OAuthConnectionInput{
		Provider: "google", Subject: "google-subject", AccessToken: "old-access-token", RefreshToken: "old-refresh-token", ExpiresAt: &past,
		Scopes: []string{GoogleReadScope, GoogleSendScope},
	})
	if err != nil {
		t.Fatalf("save OAuth connection: %v", err)
	}
	if !account.OAuthConnected || !account.OAuthSendReady {
		t.Fatalf("expected send-ready OAuth account, got %#v", account)
	}

	var smtpPassword, imapPassword, accessToken, refreshToken, scopes string
	if err := pool.QueryRow(ctx, `
		SELECT smtp_password_enc, imap_password_enc, oauth_access_token_enc, oauth_refresh_token_enc, COALESCE(oauth_scopes, '')
		FROM user_email_accounts WHERE organization_id=$1 AND user_id=$2
	`, organizationID, userID).Scan(&smtpPassword, &imapPassword, &accessToken, &refreshToken, &scopes); err != nil {
		t.Fatalf("load persisted OAuth connection: %v", err)
	}
	if smtpPassword != "" || imapPassword != "" || accessToken == "old-access-token" || refreshToken == "old-refresh-token" || !strings.Contains(scopes, GoogleSendScope) {
		t.Fatalf("OAuth connection persistence was unsafe: smtp=%q imap=%q access=%q refresh=%q scopes=%q", smtpPassword, imapPassword, accessToken, refreshToken, scopes)
	}

	future := time.Now().UTC().Add(time.Hour)
	refresher := &postgresOAuthRefresher{tokens: OAuthTokenSet{AccessToken: "fresh-access-token", RefreshToken: "rotated-refresh-token", ExpiresAt: &future}}
	sender := &postgresOAuthSender{}
	service := NewServiceWithProviders(pool, cipher, nil, refresher, sender)
	start := make(chan struct{})
	errorsBySend := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			errorsBySend <- service.SendAs(context.Background(), organizationID, userID, "lead@buyer.test", "Follow up", "Plain body", "<p>HTML body</p>")
		}()
	}
	close(start)
	for range 2 {
		if err := <-errorsBySend; err != nil {
			t.Fatalf("send through OAuth provider: %v", err)
		}
	}
	if refresher.callCount() != 1 {
		t.Fatalf("expected one serialized token refresh, got %d", refresher.callCount())
	}
	sender.mu.Lock()
	if len(sender.access) != 2 || sender.access[0] != "fresh-access-token" || sender.access[1] != "fresh-access-token" || sender.message[0].To != "lead@buyer.test" {
		t.Fatalf("unexpected provider sends: access=%#v messages=%#v", sender.access, sender.message)
	}
	sender.mu.Unlock()

	creds, err := service.SyncCredentials(ctx, organizationID, userID)
	if err != nil || creds.OAuthAccess != "fresh-access-token" || creds.OAuthRefresh != "rotated-refresh-token" {
		t.Fatalf("refreshed tokens were not persisted: creds=%#v err=%v", creds, err)
	}
	if err := service.SendAs(ctx, foreignOrganizationID, userID, "lead@buyer.test", "Foreign", "Body", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected foreign tenant mailbox selection denial, got %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE user_email_accounts SET oauth_refresh_token_enc='' WHERE organization_id=$1 AND user_id=$2`, organizationID, userID); err != nil {
		t.Fatalf("simulate revoked OAuth connection: %v", err)
	}
	if err := service.SendAs(ctx, organizationID, userID, "lead@buyer.test", "Revoked", "Body", ""); !errors.Is(err, ErrOAuthReconnectRequired) {
		t.Fatalf("expected missing refresh-token reconnect requirement, got %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE user_email_accounts SET oauth_scopes=NULL WHERE organization_id=$1 AND user_id=$2`, organizationID, userID); err != nil {
		t.Fatalf("simulate legacy OAuth connection: %v", err)
	}
	if err := service.SendAs(ctx, organizationID, userID, "lead@buyer.test", "Legacy", "Body", ""); !errors.Is(err, ErrOAuthReconnectRequired) {
		t.Fatalf("expected legacy scope reconnect requirement, got %v", err)
	}
}

func oauthDeliveryDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse OAuth delivery test URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
