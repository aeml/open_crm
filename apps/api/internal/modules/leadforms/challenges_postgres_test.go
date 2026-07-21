package leadforms

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
)

func TestPublicLeadChallengeConsentReplayAndTenantIsolationAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_challenge_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithLeadFormSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead challenge schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated lead challenge schema: %v", err)
	}
	defer pool.Close()

	var firstOrgID, secondOrgID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Lead Challenge One', $1) RETURNING id`, "lead-challenge-one-"+schema).Scan(&firstOrgID); err != nil {
		t.Fatalf("create first organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Lead Challenge Two', $1) RETURNING id`, "lead-challenge-two-"+schema).Scan(&secondOrgID); err != nil {
		t.Fatalf("create second organization: %v", err)
	}
	formFields := `[{"key":"first","label":"First name","fieldType":"text","required":true,"mapTo":"firstName"},{"key":"last","label":"Last name","fieldType":"text","required":true,"mapTo":"lastName"},{"key":"email","label":"Email","fieldType":"email","required":true,"mapTo":"email"}]`
	for _, seed := range []struct {
		orgID    int64
		publicID string
		slug     string
		consent  string
	}{{firstOrgID, "lf_challenge_one", "challenge-one", "I agree to receive a reply about this request."}, {secondOrgID, "lf_challenge_two", "challenge-two", "I agree to receive a reply from the second team."}} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO lead_capture_forms (
				organization_id, public_id, name, slug, title, fields_json,
				success_message, source_label, consent_text, is_active
			) VALUES ($1, $2, 'Public request', $3, 'Contact us', $4::jsonb, 'Thanks', 'Website', $5, TRUE)
		`, seed.orgID, seed.publicID, seed.slug, formFields, seed.consent); err != nil {
			t.Fatalf("seed form %s: %v", seed.publicID, err)
		}
	}

	service := NewService(pool)
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	challenge, err := service.IssueSubmissionChallenge(ctx, "lf_challenge_one")
	if err != nil {
		t.Fatalf("issue challenge: %v", err)
	}
	if challenge.Token == "" || challenge.ConsentText != "I agree to receive a reply about this request." || !challenge.NotBefore.Equal(now.Add(submissionChallengeMinAge)) {
		t.Fatalf("unexpected challenge: %#v", challenge)
	}
	input := SubmissionInput{
		Values:         map[string]string{"first": "Ada", "last": "Lovelace", "email": "ada@example.com"},
		SourceURL:      "https://example.test/contact?utm_source=search",
		Attribution:    Attribution{UTMSource: "search"},
		ChallengeToken: challenge.Token,
		ConsentGranted: true,
	}
	if _, err := service.SubmitByPublicID(ctx, "lf_challenge_one", input); !errors.Is(err, ErrChallengeNotReady) {
		t.Fatalf("too-early challenge error = %v", err)
	}
	now = challenge.NotBefore
	first, err := service.SubmitByPublicID(ctx, "lf_challenge_one", input)
	if err != nil || first.Replayed || first.Submission.ID <= 0 {
		t.Fatalf("first accepted submission = %#v err=%v", first, err)
	}
	replay, err := service.SubmitByPublicID(ctx, "lf_challenge_one", input)
	if err != nil || !replay.Replayed || replay.Submission.ID != first.Submission.ID || replay.Submission.ContactID != first.Submission.ContactID {
		t.Fatalf("exact replay = %#v err=%v", replay, err)
	}
	mutated := input
	mutated.Values = map[string]string{"first": "Ada", "last": "Byron", "email": "ada@example.com"}
	if _, err := service.SubmitByPublicID(ctx, "lf_challenge_one", mutated); !errors.Is(err, ErrChallengeInvalid) {
		t.Fatalf("mutated replay error = %v", err)
	}
	if _, err := service.SubmitByPublicID(ctx, "lf_challenge_two", input); !errors.Is(err, ErrChallengeInvalid) {
		t.Fatalf("foreign-form challenge error = %v", err)
	}

	var consentText, remoteAddr, userAgent, storedTokenDigest string
	var consentedAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT s.consent_text_snapshot, s.consented_at, s.remote_addr, s.user_agent, c.token_digest
		FROM lead_capture_submissions s
		JOIN lead_capture_submission_challenges c
		  ON c.organization_id=s.organization_id AND c.form_id=s.form_id AND c.submission_id=s.id
		WHERE s.id=$1
	`, first.Submission.ID).Scan(&consentText, &consentedAt, &remoteAddr, &userAgent, &storedTokenDigest); err != nil {
		t.Fatalf("load consent and challenge evidence: %v", err)
	}
	if consentText != challenge.ConsentText || !consentedAt.Equal(now) || remoteAddr != "" || userAgent != "" || storedTokenDigest == challenge.Token || len(storedTokenDigest) != 64 {
		t.Fatalf("unsafe or incomplete lead evidence: consent=%q at=%s remote=%q agent=%q token=%q", consentText, consentedAt, remoteAddr, userAgent, storedTokenDigest)
	}

	now = now.Add(time.Hour)
	expired, err := service.IssueSubmissionChallenge(ctx, "lf_challenge_one")
	if err != nil {
		t.Fatalf("issue expiring challenge: %v", err)
	}
	now = expired.ExpiresAt
	expiredInput := input
	expiredInput.ChallengeToken = expired.Token
	if _, err := service.SubmitByPublicID(ctx, "lf_challenge_one", expiredInput); !errors.Is(err, ErrChallengeInvalid) {
		t.Fatalf("expired challenge error = %v", err)
	}

	now = now.Add(time.Hour)
	concurrentChallenge, err := service.IssueSubmissionChallenge(ctx, "lf_challenge_one")
	if err != nil {
		t.Fatalf("issue concurrent challenge: %v", err)
	}
	now = concurrentChallenge.NotBefore
	concurrentInput := input
	concurrentInput.ChallengeToken = concurrentChallenge.Token
	results := make([]SubmissionResult, 2)
	errorsFound := make([]error, 2)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], errorsFound[index] = service.SubmitByPublicID(ctx, "lf_challenge_one", concurrentInput)
		}(index)
	}
	wait.Wait()
	if errorsFound[0] != nil || errorsFound[1] != nil || results[0].Submission.ID != results[1].Submission.ID || results[0].Replayed == results[1].Replayed {
		t.Fatalf("concurrent exact replay results=%#v errors=%#v", results, errorsFound)
	}
	var firstOrgContacts, secondOrgContacts int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1`, firstOrgID).Scan(&firstOrgContacts); err != nil {
		t.Fatalf("count first-tenant contacts: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1`, secondOrgID).Scan(&secondOrgContacts); err != nil {
		t.Fatalf("count second-tenant contacts: %v", err)
	}
	if firstOrgContacts != 2 || secondOrgContacts != 0 {
		t.Fatalf("unexpected tenant contact counts: first=%d second=%d", firstOrgContacts, secondOrgContacts)
	}
}
