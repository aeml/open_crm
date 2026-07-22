package leadforms

import (
	"context"
	"fmt"
	"strings"

	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

type SubmissionReviewQuery struct {
	Status string
	FormID int64
	Cursor *platformtimeline.Cursor
	Limit  int
}

type SubmissionReviewCounts struct {
	Unreviewed int `json:"unreviewed"`
	Legitimate int `json:"legitimate"`
	Spam       int `json:"spam"`
}

type SubmissionReviewPage struct {
	Submissions []ReviewedSubmission   `json:"submissions"`
	Counts      SubmissionReviewCounts `json:"counts"`
	Limit       int                    `json:"limit"`
	Meta        platformtimeline.Meta  `json:"meta"`
}

// ListSubmissionReviews returns a bounded newest-first review page. Submission
// creation keys never change, so an opaque keyset cursor excludes later
// arrivals from older pages while review mutations remain visible on refresh.
func (s *Service) ListSubmissionReviews(ctx context.Context, organizationID int64, query SubmissionReviewQuery) (SubmissionReviewPage, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return SubmissionReviewPage{}, fmt.Errorf("lead forms service not configured")
	}
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))
	if query.Status != "" && !validReviewStatus(query.Status) {
		return SubmissionReviewPage{}, ErrInvalidReview
	}
	if query.FormID < 0 {
		return SubmissionReviewPage{}, ErrInvalidReview
	}
	pagination, err := platformtimeline.Normalize(platformtimeline.Query{Cursor: query.Cursor, Limit: query.Limit})
	if err != nil {
		return SubmissionReviewPage{}, ErrInvalidReview
	}
	query.Cursor, query.Limit = pagination.Cursor, pagination.Limit

	page := SubmissionReviewPage{
		Submissions: []ReviewedSubmission{},
		Limit:       query.Limit,
		Meta:        platformtimeline.Meta{Limit: query.Limit},
	}
	if err := s.countSubmissionReviews(ctx, organizationID, query.FormID, &page.Counts); err != nil {
		return SubmissionReviewPage{}, err
	}

	args := []any{organizationID}
	filters := ""
	if query.Status != "" {
		args = append(args, query.Status)
		filters += fmt.Sprintf(" AND review_status=$%d", len(args))
	}
	if query.FormID > 0 {
		args = append(args, query.FormID)
		filters += fmt.Sprintf(" AND form_id=$%d", len(args))
	}
	if query.Cursor != nil {
		args = append(args, query.Cursor.CreatedAt, query.Cursor.ID)
		filters += fmt.Sprintf(" AND (created_at,id)<($%d,$%d)", len(args)-1, len(args))
	}
	args = append(args, query.Limit+1)

	rows, err := s.pool.Query(ctx, submissionReviewPageSQL(filters, len(args)), args...)
	if err != nil {
		return SubmissionReviewPage{}, fmt.Errorf("list lead submission reviews: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		submission, err := scanReviewedSubmission(rows)
		if err != nil {
			return SubmissionReviewPage{}, err
		}
		page.Submissions = append(page.Submissions, submission)
	}
	if err := rows.Err(); err != nil {
		return SubmissionReviewPage{}, fmt.Errorf("iterate lead submission reviews: %w", err)
	}

	hasMore := len(page.Submissions) > query.Limit
	if hasMore {
		page.Submissions = page.Submissions[:query.Limit]
	}
	if len(page.Submissions) > 0 {
		last := page.Submissions[len(page.Submissions)-1]
		page.Meta, err = platformtimeline.MetaForPage(query.Limit, hasMore, last.CreatedAt, last.ID)
		if err != nil {
			return SubmissionReviewPage{}, err
		}
	}
	return page, nil
}

func (s *Service) countSubmissionReviews(ctx context.Context, organizationID, formID int64, counts *SubmissionReviewCounts) error {
	args := []any{organizationID}
	filter := ""
	if formID > 0 {
		args = append(args, formID)
		filter = " AND form_id=$2"
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE review_status='unreviewed')::int,
		       COUNT(*) FILTER (WHERE review_status='legitimate')::int,
		       COUNT(*) FILTER (WHERE review_status='spam')::int
		FROM lead_capture_submissions
		WHERE organization_id=$1`+filter, args...).Scan(&counts.Unreviewed, &counts.Legitimate, &counts.Spam); err != nil {
		return fmt.Errorf("count lead submission reviews: %w", err)
	}
	return nil
}

func submissionReviewPageSQL(filters string, limitArgument int) string {
	return `
	WITH selected_submissions AS MATERIALIZED (
	  SELECT id,created_at
	  FROM lead_capture_submissions
	  WHERE organization_id=$1` + filters + `
	  ORDER BY created_at DESC,id DESC
	  LIMIT $` + fmt.Sprint(limitArgument) + `
	), run_stats AS (
	  SELECT selected.id AS submission_id,
	         COUNT(*) FILTER (WHERE run.status='queued')::int AS queued_runs,
	         COUNT(*) FILTER (WHERE run.status='cancelled')::int AS cancelled_runs,
	         COUNT(*) FILTER (WHERE run.status='succeeded' AND run.actions_completed > 0)::int AS completed_runs
	  FROM selected_submissions selected
	  JOIN workflow_automation_runs run
	    ON CASE WHEN (run.trigger_payload_json->>'submissionId') ~ '^[0-9]+$'
	            THEN (run.trigger_payload_json->>'submissionId')::bigint END=selected.id
	   AND run.organization_id=$1 AND run.trigger_type='form_submitted' AND run.target_entity_type='lead_form'
	  GROUP BY selected.id
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
	FROM selected_submissions selected
	JOIN lead_capture_submissions submission ON submission.organization_id=$1 AND submission.id=selected.id
	JOIN lead_capture_forms form ON form.organization_id=submission.organization_id AND form.id=submission.form_id
	LEFT JOIN contacts contact ON contact.organization_id=submission.organization_id AND contact.id=submission.contact_id
	LEFT JOIN users reviewer ON reviewer.id=submission.reviewed_by_user_id
	LEFT JOIN run_stats ON run_stats.submission_id=submission.id
	ORDER BY submission.created_at DESC,submission.id DESC`
}
