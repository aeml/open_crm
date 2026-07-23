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
	formRevision  int
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
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
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

	form, organizationID, err := s.getActiveByPublicID(ctx, publicID)
	if err != nil {
		return SubmissionChallenge{}, err
	}
	if _, err := hydrateFormFields(ctx, s.pool, organizationID, form.Fields, true); err != nil {
		if errors.Is(err, ErrInvalidMapping) {
			return SubmissionChallenge{}, ErrFormUnavailable
		}
		return SubmissionChallenge{}, err
	}

	token, err := newSubmissionChallengeToken()
	if err != nil {
		return SubmissionChallenge{}, err
	}
	challenge := SubmissionChallenge{
		Token:        token,
		FormRevision: form.Revision,
		NotBefore:    now.Add(submissionChallengeMinAge),
		ExpiresAt:    now.Add(submissionChallengeTTL),
	}
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO lead_capture_submission_challenges (
			organization_id, form_id, token_digest, consent_text_snapshot, form_revision,
			issued_at, not_before, expires_at
		)
		SELECT organization_id, id, $2, consent_text, COALESCE(revision, 1), $3, $4, $5
		FROM lead_capture_forms
		WHERE public_id = $1 AND organization_id = $6 AND id = $7
		  AND is_active = TRUE AND COALESCE(revision, 1) = $8
		RETURNING consent_text_snapshot
	`, publicID, submissionChallengeDigest(token), now, challenge.NotBefore, challenge.ExpiresAt, organizationID, form.ID, form.Revision).Scan(&challenge.ConsentText); err != nil {
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
		SELECT consent_text_snapshot, COALESCE(form_revision, 1), request_digest, submission_id, not_before, expires_at, consumed_at
		FROM lead_capture_submission_challenges
		WHERE organization_id = $1 AND form_id = $2 AND token_digest = $3
		FOR UPDATE
	`, organizationID, formID, submissionChallengeDigest(token)).Scan(
		&challenge.consentText,
		&challenge.formRevision,
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
		SELECT consent_text_snapshot, COALESCE(form_revision, 1), request_digest, submission_id, not_before, expires_at, consumed_at
		FROM lead_capture_submission_challenges
		WHERE organization_id = $1 AND form_id = $2 AND token_digest = $3
	`, organizationID, formID, submissionChallengeDigest(token)).Scan(
		&challenge.consentText,
		&challenge.formRevision,
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

func (s *Service) replayedSubmission(ctx context.Context, queryer submissionQueryer, organizationID, formID int64, challenge lockedSubmissionChallenge, input SubmissionInput, successMessage string) (SubmissionResult, bool, error) {
	if challenge.consumedAt == nil {
		return SubmissionResult{}, false, nil
	}
	if challenge.requestDigest == nil || challenge.submissionID == nil {
		return SubmissionResult{}, false, ErrChallengeInvalid
	}
	var submission Submission
	var payloadJSON []byte
	var sourceURL, consentText string
	var attribution Attribution
	var formRevision int
	if err := queryer.QueryRow(ctx, `
		SELECT id, form_id, COALESCE(contact_id, 0), created_at,
		       payload_json, source_url, lead_source, utm_source, utm_medium,
		       utm_campaign, utm_term, utm_content, consent_text_snapshot,
		       COALESCE(form_revision, 1)
		FROM lead_capture_submissions
		WHERE organization_id = $1 AND form_id = $2 AND id = $3
		  AND COALESCE(form_revision, 1) = $4
	`, organizationID, formID, *challenge.submissionID, challenge.formRevision).Scan(
		&submission.ID,
		&submission.FormID,
		&submission.ContactID,
		&submission.CreatedAt,
		&payloadJSON,
		&sourceURL,
		&attribution.LeadSource,
		&attribution.UTMSource,
		&attribution.UTMMedium,
		&attribution.UTMCampaign,
		&attribution.UTMTerm,
		&attribution.UTMContent,
		&consentText,
		&formRevision,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SubmissionResult{}, false, ErrChallengeInvalid
		}
		return SubmissionResult{}, false, fmt.Errorf("load replayed lead submission: %w", err)
	}
	var payload map[string]string
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return SubmissionResult{}, false, fmt.Errorf("decode replayed lead submission payload: %w", err)
	}
	storedDigest, err := submissionRequestDigest(Form{ID: formID, Revision: formRevision}, payload, sourceURL, attribution, consentText)
	if err != nil {
		return SubmissionResult{}, false, err
	}
	if storedDigest != *challenge.requestDigest {
		// Challenges accepted before revision binding used the same canonical
		// request without formRevision. Honor those retained 24-hour replay
		// records while all newly consumed challenges use the revisioned digest.
		legacyDigest, err := legacySubmissionRequestDigest(formID, payload, sourceURL, attribution, consentText)
		if err != nil {
			return SubmissionResult{}, false, err
		}
		if legacyDigest != *challenge.requestDigest {
			return SubmissionResult{}, false, ErrChallengeInvalid
		}
	}
	normalizedValues, err := normalizeValues(input.Values)
	if err != nil {
		return SubmissionResult{}, false, ErrChallengeInvalid
	}
	incomingSourceURL := trimMax(input.SourceURL, 2048)
	// Use the retained lead-source value as the fallback so an exact retry stays
	// idempotent even if an administrator edits the form after acceptance.
	incomingAttribution := normalizeAttribution(Form{SourceLabel: attribution.LeadSource}, input, incomingSourceURL)
	if !sameSubmissionValues(payload, normalizedValues) || incomingSourceURL != sourceURL || incomingAttribution != attribution || consentText != challenge.consentText || formRevision != challenge.formRevision {
		return SubmissionResult{}, false, ErrChallengeInvalid
	}
	return SubmissionResult{Submission: submission, SuccessMessage: successMessage, Replayed: true}, true, nil
}

func sameSubmissionValues(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		candidate, ok := right[key]
		if !ok || candidate != value {
			return false
		}
	}
	return true
}

func submissionRequestDigest(form Form, payload map[string]string, sourceURL string, attribution Attribution, consentText string) (string, error) {
	canonical := struct {
		FormID       int64             `json:"formId"`
		FormRevision int               `json:"formRevision"`
		Values       map[string]string `json:"values"`
		SourceURL    string            `json:"sourceUrl"`
		Attribution  Attribution       `json:"attribution"`
		ConsentText  string            `json:"consentText"`
	}{
		FormID:       form.ID,
		FormRevision: form.Revision,
		Values:       payload,
		SourceURL:    sourceURL,
		Attribution:  attribution,
		ConsentText:  consentText,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode lead submission request digest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func legacySubmissionRequestDigest(formID int64, payload map[string]string, sourceURL string, attribution Attribution, consentText string) (string, error) {
	canonical := struct {
		FormID      int64             `json:"formId"`
		Values      map[string]string `json:"values"`
		SourceURL   string            `json:"sourceUrl"`
		Attribution Attribution       `json:"attribution"`
		ConsentText string            `json:"consentText"`
	}{
		FormID:      formID,
		Values:      payload,
		SourceURL:   sourceURL,
		Attribution: attribution,
		ConsentText: consentText,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode legacy lead submission request digest: %w", err)
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
