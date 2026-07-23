package leadforms

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
	"github.com/jackc/pgx/v5"
)

const (
	ReviewStatusUnreviewed = "unreviewed"
	ReviewStatusLegitimate = "legitimate"
	ReviewStatusSpam       = "spam"
	maxReviewNoteLength    = 500
)

var (
	ErrInvalidReview             = errors.New("invalid lead submission review")
	ErrReviewConflict            = errors.New("lead submission review conflict")
	ErrReviewIdempotencyConflict = errors.New("lead submission review idempotency conflict")
)

type ReviewedSubmission struct {
	ID                    int64                                                 `json:"id"`
	FormID                int64                                                 `json:"formId"`
	FormName              string                                                `json:"formName"`
	ContactID             int64                                                 `json:"contactId"`
	ContactName           string                                                `json:"contactName"`
	ContactEmail          string                                                `json:"contactEmail"`
	ContactActive         bool                                                  `json:"contactActive"`
	ContactQuarantined    bool                                                  `json:"contactQuarantined"`
	Values                map[string]string                                     `json:"values"`
	SourceURL             string                                                `json:"sourceUrl"`
	LeadSource            string                                                `json:"leadSource"`
	UTMSource             string                                                `json:"utmSource"`
	UTMMedium             string                                                `json:"utmMedium"`
	UTMCampaign           string                                                `json:"utmCampaign"`
	ConsentText           string                                                `json:"consentText"`
	ConsentedAt           *time.Time                                            `json:"consentedAt,omitempty"`
	ReviewStatus          string                                                `json:"reviewStatus"`
	ReviewVersion         int                                                   `json:"reviewVersion"`
	ReviewNote            string                                                `json:"reviewNote"`
	ReviewedAt            *time.Time                                            `json:"reviewedAt,omitempty"`
	ReviewedByName        string                                                `json:"reviewedByName"`
	QueuedFollowUpRuns    int                                                   `json:"queuedFollowUpRuns"`
	CancelledFollowUpRuns int                                                   `json:"cancelledFollowUpRuns"`
	CompletedFollowUpRuns int                                                   `json:"completedFollowUpRuns"`
	CreatedAt             time.Time                                             `json:"createdAt"`
	Replayed              bool                                                  `json:"-"`
	Effects               moduleworkflowautomations.LeadSubmissionReviewEffects `json:"effects,omitempty"`
}

type SubmissionReviewInput struct {
	Status         string
	Note           string
	IdempotencyKey string
}

type SubmissionReviewOperationalStats struct {
	Unreviewed          int64
	Legitimate          int64
	Spam                int64
	OldestUnreviewedAge int64
}

func (s *Service) SubmissionReviewOperationalStats(ctx context.Context) (SubmissionReviewOperationalStats, error) {
	if s == nil || s.pool == nil {
		return SubmissionReviewOperationalStats{}, fmt.Errorf("lead forms service not configured")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	var result SubmissionReviewOperationalStats
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE review_status='unreviewed')::bigint,
		       COUNT(*) FILTER (WHERE review_status='legitimate')::bigint,
		       COUNT(*) FILTER (WHERE review_status='spam')::bigint,
		       COALESCE(EXTRACT(EPOCH FROM (NOW()-MIN(created_at) FILTER (WHERE review_status='unreviewed')))::bigint,0)
		FROM lead_capture_submissions
	`).Scan(&result.Unreviewed, &result.Legitimate, &result.Spam, &result.OldestUnreviewedAge); err != nil {
		return SubmissionReviewOperationalStats{}, fmt.Errorf("collect lead submission review health: %w", err)
	}
	if result.OldestUnreviewedAge < 0 {
		result.OldestUnreviewedAge = 0
	}
	return result, nil
}

func (s *Service) ReviewSubmission(ctx context.Context, organizationID, submissionID, actorUserID int64, input SubmissionReviewInput) (ReviewedSubmission, error) {
	if s == nil || s.pool == nil {
		return ReviewedSubmission{}, fmt.Errorf("lead forms service not configured")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.Note = strings.TrimSpace(input.Note)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if organizationID <= 0 || submissionID <= 0 || actorUserID <= 0 ||
		(input.Status != ReviewStatusLegitimate && input.Status != ReviewStatusSpam) ||
		len(input.Note) > maxReviewNoteLength || len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 200 {
		return ReviewedSubmission{}, ErrInvalidReview
	}
	if err := moduleusers.RequireActiveMember(ctx, s.pool, organizationID, actorUserID); err != nil {
		return ReviewedSubmission{}, ErrInvalidReview
	}

	preflightRestore := false
	if input.Status == ReviewStatusLegitimate {
		if err := s.pool.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM lead_capture_submissions submission
			  JOIN contacts contact ON contact.organization_id=submission.organization_id AND contact.id=submission.contact_id
			  WHERE submission.organization_id=$1 AND submission.id=$2
			    AND submission.review_status='spam' AND submission.quarantined_contact_at IS NOT NULL
			    AND contact.archived_at=submission.quarantined_contact_at
			)
		`, organizationID, submissionID).Scan(&preflightRestore); err != nil {
			return ReviewedSubmission{}, fmt.Errorf("inspect lead submission recovery capacity: %w", err)
		}
	}
	var reservation modulebilling.CapacityReservation
	var err error
	if preflightRestore {
		reservation, err = modulebilling.ReserveCapacity(ctx, s.capacity, organizationID, modulebilling.ResourceContacts, 1)
		if err != nil {
			return ReviewedSubmission{}, err
		}
		defer modulebilling.CancelReservation(s.capacity, reservation)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReviewedSubmission{}, fmt.Errorf("begin lead submission review: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := modulebilling.LockCapacityEffect(ctx, tx, reservation); err != nil {
		return ReviewedSubmission{}, err
	}
	if err := moduleusers.RequireActiveMember(ctx, tx, organizationID, actorUserID); err != nil {
		return ReviewedSubmission{}, ErrInvalidReview
	}

	var contactID int64
	var currentStatus string
	var currentVersion int
	var quarantinedAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(contact_id,0),review_status,review_version,quarantined_contact_at
		FROM lead_capture_submissions
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, submissionID).Scan(&contactID, &currentStatus, &currentVersion, &quarantinedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReviewedSubmission{}, ErrNotFound
		}
		return ReviewedSubmission{}, fmt.Errorf("lock lead submission review: %w", err)
	}
	keyDigest := reviewDigest(input.IdempotencyKey)
	requestDigest := reviewDigest(input.Status + "\x00" + input.Note)
	replay, found, err := loadReviewReplay(ctx, tx, organizationID, submissionID, keyDigest)
	if err != nil {
		return ReviewedSubmission{}, err
	}
	if found {
		if replay.RequestDigest != requestDigest {
			return ReviewedSubmission{}, ErrReviewIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return ReviewedSubmission{}, fmt.Errorf("commit replayed lead submission review: %w", err)
		}
		result, err := s.getReviewedSubmission(ctx, organizationID, submissionID)
		result.Replayed = true
		if result.ReviewVersion == replay.ResultReviewVersion {
			result.Effects = replay.Effects
		}
		return result, err
	}
	if currentStatus == input.Status {
		if err := recordReviewRequest(ctx, tx, organizationID, submissionID, keyDigest, requestDigest, currentVersion, moduleworkflowautomations.LeadSubmissionReviewEffects{}); err != nil {
			return ReviewedSubmission{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return ReviewedSubmission{}, fmt.Errorf("commit unchanged lead submission review: %w", err)
		}
		return s.getReviewedSubmission(ctx, organizationID, submissionID)
	}

	nextVersion := currentVersion + 1
	now := s.currentTime().UTC()
	effects := moduleworkflowautomations.LeadSubmissionReviewEffects{}
	var archivedByReview *time.Time
	if input.Status == ReviewStatusSpam {
		effects, err = moduleworkflowautomations.QuarantineLeadSubmissionRuns(ctx, tx, organizationID, submissionID)
		if err != nil {
			return ReviewedSubmission{}, err
		}
		if contactID > 0 {
			var archivedAt time.Time
			err = tx.QueryRow(ctx, `
				UPDATE contacts SET archived_at=$3,updated_at=$3
				WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
				RETURNING archived_at
			`, organizationID, contactID, now).Scan(&archivedAt)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return ReviewedSubmission{}, fmt.Errorf("quarantine spam lead contact: %w", err)
			}
			if err == nil {
				archivedByReview = &archivedAt
			}
		}
	} else {
		if quarantinedAt != nil && !preflightRestore {
			return ReviewedSubmission{}, ErrReviewConflict
		}
		effects, err = moduleworkflowautomations.RecoverLeadSubmissionRuns(ctx, tx, organizationID, submissionID, nextVersion)
		if err != nil {
			return ReviewedSubmission{}, err
		}
		if quarantinedAt != nil && contactID > 0 {
			command, err := tx.Exec(ctx, `
				UPDATE contacts SET archived_at=NULL,updated_at=$4
				WHERE organization_id=$1 AND id=$2 AND archived_at=$3
			`, organizationID, contactID, *quarantinedAt, now)
			if err != nil {
				return ReviewedSubmission{}, fmt.Errorf("restore quarantined lead contact: %w", err)
			}
			if command.RowsAffected() != 1 {
				return ReviewedSubmission{}, ErrReviewConflict
			}
			if err := modulebilling.ConsumeCapacity(ctx, s.capacity, tx, reservation); err != nil {
				return ReviewedSubmission{}, err
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE lead_capture_submissions
		SET review_status=$3,review_version=$4,review_note=NULLIF($5,''),
		    reviewed_at=$6,reviewed_by_user_id=$7,quarantined_contact_at=$8
		WHERE organization_id=$1 AND id=$2
	`, organizationID, submissionID, input.Status, nextVersion, input.Note, now, actorUserID, archivedByReview); err != nil {
		return ReviewedSubmission{}, fmt.Errorf("update lead submission review: %w", err)
	}
	if contactID > 0 {
		action := "lead_form.reviewed_legitimate"
		summary := "Lead submission marked legitimate"
		if input.Status == ReviewStatusSpam {
			action = "lead_form.quarantined_spam"
			summary = "Lead submission quarantined as spam"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json)
			VALUES ($1,'contact',$2,$3,$4,$5,jsonb_build_object('submissionId',$6::bigint,'reviewVersion',$7::int))
		`, organizationID, contactID, actorUserID, action, summary, submissionID, nextVersion); err != nil {
			return ReviewedSubmission{}, fmt.Errorf("record lead submission review activity: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'lead_submission.reviewed','lead_capture_submission',$3,$4,
		        jsonb_build_object('from',$5::text,'to',$6::text,'reviewVersion',$7::int,
		                           'contactId',$8::bigint,'contactQuarantined',$9::boolean,
		                           'cancelledRuns',$10::int,'recoveredRuns',$11::int,'completedRuns',$12::int))
	`, organizationID, actorUserID, submissionID, "Lead submission marked "+input.Status,
		currentStatus, input.Status, nextVersion, contactID, archivedByReview != nil,
		effects.CancelledRuns, effects.RecoveredRuns, effects.CompletedRuns); err != nil {
		return ReviewedSubmission{}, fmt.Errorf("audit lead submission review: %w", err)
	}
	if err := recordReviewRequest(ctx, tx, organizationID, submissionID, keyDigest, requestDigest, nextVersion, effects); err != nil {
		return ReviewedSubmission{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewedSubmission{}, fmt.Errorf("commit lead submission review: %w", err)
	}
	result, err := s.getReviewedSubmission(ctx, organizationID, submissionID)
	result.Effects = effects
	return result, err
}

type reviewReplay struct {
	RequestDigest       string
	ResultReviewVersion int
	Effects             moduleworkflowautomations.LeadSubmissionReviewEffects
}

func loadReviewReplay(ctx context.Context, tx pgx.Tx, organizationID, submissionID int64, keyDigest string) (reviewReplay, bool, error) {
	result := reviewReplay{}
	err := tx.QueryRow(ctx, `
		SELECT request_sha256,result_review_version,cancelled_runs,recovered_runs,completed_runs
		FROM lead_capture_submission_review_requests
		WHERE organization_id=$1 AND submission_id=$2 AND key_digest=$3
	`, organizationID, submissionID, keyDigest).Scan(
		&result.RequestDigest, &result.ResultReviewVersion,
		&result.Effects.CancelledRuns, &result.Effects.RecoveredRuns, &result.Effects.CompletedRuns,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return reviewReplay{}, false, nil
	}
	if err != nil {
		return reviewReplay{}, false, fmt.Errorf("load lead submission review replay: %w", err)
	}
	return result, true, nil
}

func recordReviewRequest(ctx context.Context, tx pgx.Tx, organizationID, submissionID int64, keyDigest, requestDigest string, reviewVersion int, effects moduleworkflowautomations.LeadSubmissionReviewEffects) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO lead_capture_submission_review_requests (
			organization_id,submission_id,key_digest,request_sha256,result_review_version,
			cancelled_runs,recovered_runs,completed_runs
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, organizationID, submissionID, keyDigest, requestDigest, reviewVersion,
		effects.CancelledRuns, effects.RecoveredRuns, effects.CompletedRuns); err != nil {
		return fmt.Errorf("record lead submission review request: %w", err)
	}
	return nil
}

func (s *Service) getReviewedSubmission(ctx context.Context, organizationID, submissionID int64) (ReviewedSubmission, error) {
	row := s.pool.QueryRow(ctx, submissionReviewListSQL, organizationID, "", int64(0), 1, submissionID)
	result, err := scanReviewedSubmission(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewedSubmission{}, ErrNotFound
	}
	return result, err
}

const submissionReviewListSQL = `
	WITH run_stats AS (
	  SELECT (trigger_payload_json->>'submissionId')::bigint AS submission_id,
	         COUNT(*) FILTER (WHERE status='queued')::int AS queued_runs,
	         COUNT(*) FILTER (WHERE status='cancelled')::int AS cancelled_runs,
	         COUNT(*) FILTER (WHERE status='succeeded' AND actions_completed > 0)::int AS completed_runs
	  FROM workflow_automation_runs
	  WHERE organization_id=$1 AND trigger_type='form_submitted' AND target_entity_type='lead_form'
	    AND (trigger_payload_json->>'submissionId') ~ '^[0-9]+$'
	  GROUP BY (trigger_payload_json->>'submissionId')::bigint
	)
	SELECT submission.id,submission.form_id,form.name,COALESCE(submission.contact_id,0),
	       trim(COALESCE(contact.first_name,'') || ' ' || COALESCE(contact.last_name,'')),
	       COALESCE(contact.email,''),(contact.id IS NOT NULL AND contact.archived_at IS NULL),
	       (submission.quarantined_contact_at IS NOT NULL AND contact.archived_at=submission.quarantined_contact_at),
	       submission.payload_json,COALESCE(submission.source_url,''),COALESCE(submission.lead_source,''),
	       COALESCE(submission.utm_source,''),COALESCE(submission.utm_medium,''),COALESCE(submission.utm_campaign,''),
	       COALESCE(submission.consent_text_snapshot,''),submission.consented_at,
	       submission.review_status,submission.review_version,COALESCE(submission.review_note,''),submission.reviewed_at,
	       trim(COALESCE(reviewer.first_name,'') || ' ' || COALESCE(reviewer.last_name,'')),
	       COALESCE(run_stats.queued_runs,0),COALESCE(run_stats.cancelled_runs,0),COALESCE(run_stats.completed_runs,0),
	       submission.created_at
	FROM lead_capture_submissions submission
	JOIN lead_capture_forms form ON form.organization_id=submission.organization_id AND form.id=submission.form_id
	LEFT JOIN contacts contact ON contact.organization_id=submission.organization_id AND contact.id=submission.contact_id
	LEFT JOIN users reviewer ON reviewer.id=submission.reviewed_by_user_id
	LEFT JOIN run_stats ON run_stats.submission_id=submission.id
	WHERE submission.organization_id=$1
	  AND ($2::text='' OR submission.review_status=$2)
	  AND ($3::bigint=0 OR submission.form_id=$3)
	  AND (COALESCE($5::bigint,0)=0 OR submission.id=$5)
	ORDER BY submission.created_at DESC,submission.id DESC
	LIMIT $4`

type reviewScanner interface{ Scan(...any) error }

func scanReviewedSubmission(scanner reviewScanner) (ReviewedSubmission, error) {
	var result ReviewedSubmission
	var valuesJSON []byte
	if err := scanner.Scan(
		&result.ID, &result.FormID, &result.FormName, &result.ContactID, &result.ContactName,
		&result.ContactEmail, &result.ContactActive, &result.ContactQuarantined, &valuesJSON,
		&result.SourceURL, &result.LeadSource, &result.UTMSource, &result.UTMMedium, &result.UTMCampaign,
		&result.ConsentText, &result.ConsentedAt, &result.ReviewStatus, &result.ReviewVersion,
		&result.ReviewNote, &result.ReviewedAt, &result.ReviewedByName,
		&result.QueuedFollowUpRuns, &result.CancelledFollowUpRuns, &result.CompletedFollowUpRuns,
		&result.CreatedAt,
	); err != nil {
		return ReviewedSubmission{}, err
	}
	if err := json.Unmarshal(valuesJSON, &result.Values); err != nil {
		return ReviewedSubmission{}, fmt.Errorf("decode lead submission review values: %w", err)
	}
	if result.Values == nil {
		result.Values = map[string]string{}
	}
	return result, nil
}

func validReviewStatus(value string) bool {
	return value == ReviewStatusUnreviewed || value == ReviewStatusLegitimate || value == ReviewStatusSpam
}

func reviewDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
