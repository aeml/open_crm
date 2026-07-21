package leadforms

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	submissionChallengeMinAge    = 2 * time.Second
	submissionChallengeTTL       = 30 * time.Minute
	submissionChallengeRetention = 24 * time.Hour
	submissionChallengeTokenSize = 32
)

type lockedSubmissionChallenge struct {
	consentText   string
	requestDigest *string
	submissionID  *int64
	notBefore     time.Time
	expiresAt     time.Time
	consumedAt    *time.Time
}

type submissionQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) IssueSubmissionChallenge(ctx context.Context, publicID string) (SubmissionChallenge, error) {
	if s == nil || s.pool == nil {
		return SubmissionChallenge{}, fmt.Errorf("lead forms service not configured")
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return SubmissionChallenge{}, ErrNotFound
	}

	now := s.currentTime()
	if _, err := s.pool.Exec(ctx, `
		WITH expired AS (
			SELECT id
			FROM lead_capture_submission_challenges
			WHERE expires_at < $1
			ORDER BY expires_at, id
			LIMIT 100
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM lead_capture_submission_challenges c
		USING expired
		WHERE c.id = expired.id
	`, now.Add(-submissionChallengeRetention)); err != nil {
		return SubmissionChallenge{}, fmt.Errorf("clean expired lead submission challenges: %w", err)
	}

	token, err := newSubmissionChallengeToken()
	if err != nil {
		return SubmissionChallenge{}, err
	}
	challenge := SubmissionChallenge{
		Token:     token,
		NotBefore: now.Add(submissionChallengeMinAge),
		ExpiresAt: now.Add(submissionChallengeTTL),
	}
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO lead_capture_submission_challenges (
			organization_id, form_id, token_digest, consent_text_snapshot,
			issued_at, not_before, expires_at
		)
		SELECT organization_id, id, $2, consent_text, $3, $4, $5
		FROM lead_capture_forms
		WHERE public_id = $1 AND is_active = TRUE
		RETURNING consent_text_snapshot
	`, publicID, submissionChallengeDigest(token), now, challenge.NotBefore, challenge.ExpiresAt).Scan(&challenge.ConsentText); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SubmissionChallenge{}, ErrNotFound
		}
		return SubmissionChallenge{}, fmt.Errorf("issue lead submission challenge: %w", err)
	}
	return challenge, nil
}

func (s *Service) lockSubmissionChallenge(ctx context.Context, tx pgx.Tx, organizationID, formID int64, token string) (lockedSubmissionChallenge, error) {
	if !validSubmissionChallengeToken(token) {
		return lockedSubmissionChallenge{}, ErrChallengeInvalid
	}
	var challenge lockedSubmissionChallenge
	if err := tx.QueryRow(ctx, `
		SELECT consent_text_snapshot, request_digest, submission_id, not_before, expires_at, consumed_at
		FROM lead_capture_submission_challenges
		WHERE organization_id = $1 AND form_id = $2 AND token_digest = $3
		FOR UPDATE
	`, organizationID, formID, submissionChallengeDigest(token)).Scan(
		&challenge.consentText,
		&challenge.requestDigest,
		&challenge.submissionID,
		&challenge.notBefore,
		&challenge.expiresAt,
		&challenge.consumedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return lockedSubmissionChallenge{}, ErrChallengeInvalid
		}
		return lockedSubmissionChallenge{}, fmt.Errorf("lock lead submission challenge: %w", err)
	}
	return challenge, nil
}

func (s *Service) loadSubmissionChallenge(ctx context.Context, organizationID, formID int64, token string) (lockedSubmissionChallenge, error) {
	if !validSubmissionChallengeToken(token) {
		return lockedSubmissionChallenge{}, ErrChallengeInvalid
	}
	var challenge lockedSubmissionChallenge
	if err := s.pool.QueryRow(ctx, `
		SELECT consent_text_snapshot, request_digest, submission_id, not_before, expires_at, consumed_at
		FROM lead_capture_submission_challenges
		WHERE organization_id = $1 AND form_id = $2 AND token_digest = $3
	`, organizationID, formID, submissionChallengeDigest(token)).Scan(
		&challenge.consentText,
		&challenge.requestDigest,
		&challenge.submissionID,
		&challenge.notBefore,
		&challenge.expiresAt,
		&challenge.consumedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return lockedSubmissionChallenge{}, ErrChallengeInvalid
		}
		return lockedSubmissionChallenge{}, fmt.Errorf("load lead submission challenge: %w", err)
	}
	return challenge, nil
}

func (s *Service) replayedSubmission(ctx context.Context, queryer submissionQueryer, organizationID, formID int64, challenge lockedSubmissionChallenge, requestDigest, successMessage string) (SubmissionResult, bool, error) {
	if challenge.consumedAt == nil {
		return SubmissionResult{}, false, nil
	}
	if challenge.requestDigest == nil || challenge.submissionID == nil || *challenge.requestDigest != requestDigest {
		return SubmissionResult{}, false, ErrChallengeInvalid
	}
	var submission Submission
	if err := queryer.QueryRow(ctx, `
		SELECT id, form_id, COALESCE(contact_id, 0), created_at
		FROM lead_capture_submissions
		WHERE organization_id = $1 AND form_id = $2 AND id = $3
	`, organizationID, formID, *challenge.submissionID).Scan(
		&submission.ID,
		&submission.FormID,
		&submission.ContactID,
		&submission.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SubmissionResult{}, false, ErrChallengeInvalid
		}
		return SubmissionResult{}, false, fmt.Errorf("load replayed lead submission: %w", err)
	}
	return SubmissionResult{Submission: submission, SuccessMessage: successMessage, Replayed: true}, true, nil
}

func submissionRequestDigest(form Form, payload map[string]string, sourceURL string, attribution Attribution, consentText string) (string, error) {
	canonical := struct {
		FormID      int64             `json:"formId"`
		Values      map[string]string `json:"values"`
		SourceURL   string            `json:"sourceUrl"`
		Attribution Attribution       `json:"attribution"`
		ConsentText string            `json:"consentText"`
	}{
		FormID:      form.ID,
		Values:      payload,
		SourceURL:   sourceURL,
		Attribution: attribution,
		ConsentText: consentText,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode lead submission request digest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func newSubmissionChallengeToken() (string, error) {
	raw := make([]byte, submissionChallengeTokenSize)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate lead submission challenge: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func validSubmissionChallengeToken(token string) bool {
	token = strings.TrimSpace(token)
	if len(token) != submissionChallengeTokenSize*2 {
		return false
	}
	decoded, err := hex.DecodeString(token)
	return err == nil && len(decoded) == submissionChallengeTokenSize
}

func submissionChallengeDigest(token string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(digest[:])
}

func (s *Service) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}
