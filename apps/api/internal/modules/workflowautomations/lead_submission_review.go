package workflowautomations

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const leadSubmissionSpamReason = "Submission was quarantined as spam before execution."

// LeadSubmissionReviewEffects summarizes workflow work changed by one review
// transition. Completed task effects are never rewritten; they remain visible
// history for the reviewer.
type LeadSubmissionReviewEffects struct {
	CancelledRuns int `json:"cancelledRuns"`
	RecoveredRuns int `json:"recoveredRuns"`
	CompletedRuns int `json:"completedRuns"`
}

// QuarantineLeadSubmissionRuns cancels queued follow-up work under the same
// transaction as the submission review. Locking runs before the contact keeps
// the order compatible with HandleLeadFollowUpJob.
func QuarantineLeadSubmissionRuns(ctx context.Context, tx pgx.Tx, organizationID, submissionID int64) (LeadSubmissionReviewEffects, error) {
	if tx == nil || organizationID <= 0 || submissionID <= 0 {
		return LeadSubmissionReviewEffects{}, ErrInvalidInput
	}
	effects := LeadSubmissionReviewEffects{}
	rows, err := tx.Query(ctx, `
		UPDATE workflow_automation_runs
		SET status='cancelled',
		    trigger_payload_json=jsonb_set(trigger_payload_json - 'createdTaskId','{terminalReason}',to_jsonb($3::text),TRUE),
		    condition_result=NULL,actions_completed=0,last_error=$3,
		    started_at=COALESCE(started_at,NOW()),completed_at=NOW(),updated_at=NOW()
		WHERE organization_id=$1
		  AND trigger_type='form_submitted' AND target_entity_type='lead_form'
		  AND trigger_payload_json->>'submissionId'=$2
		  AND status='queued'
		RETURNING id
	`, organizationID, strconv.FormatInt(submissionID, 10), leadSubmissionSpamReason)
	if err != nil {
		return LeadSubmissionReviewEffects{}, fmt.Errorf("cancel queued lead follow-up runs: %w", err)
	}
	runIDs := make([]int64, 0)
	for rows.Next() {
		var runID int64
		if err := rows.Scan(&runID); err != nil {
			rows.Close()
			return LeadSubmissionReviewEffects{}, fmt.Errorf("scan cancelled lead follow-up run: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return LeadSubmissionReviewEffects{}, fmt.Errorf("iterate cancelled lead follow-up runs: %w", err)
	}
	rows.Close()
	effects.CancelledRuns = len(runIDs)
	if len(runIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE workflow_automation_action_outcomes
			SET status='cancelled',completed_at=NOW(),last_error=$3,updated_at=NOW()
			WHERE organization_id=$1 AND run_id=ANY($2::bigint[])
			  AND status IN ('queued','running')
		`, organizationID, runIDs, leadSubmissionSpamReason); err != nil {
			return LeadSubmissionReviewEffects{}, fmt.Errorf("cancel queued lead follow-up action outcomes: %w", err)
		}
	}
	// Count after the queued-run update has waited on any worker-held run locks,
	// so a concurrent completion is represented in the review evidence.
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM workflow_automation_runs
		WHERE organization_id=$1
		  AND trigger_type='form_submitted' AND target_entity_type='lead_form'
		  AND trigger_payload_json->>'submissionId'=$2
		  AND status='succeeded' AND actions_completed > 0
	`, organizationID, strconv.FormatInt(submissionID, 10)).Scan(&effects.CompletedRuns); err != nil {
		return LeadSubmissionReviewEffects{}, fmt.Errorf("count completed lead follow-up runs: %w", err)
	}
	if len(runIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE background_jobs
			SET status='succeeded',
			    result_json=jsonb_build_object('status','cancelled','reason',$3::text),
			    completed_at=NOW(),last_error='',updated_at=NOW()
			WHERE organization_id=$1 AND job_type=$2
			  AND idempotency_key=ANY($4::text[])
			  AND status IN ('pending','retryable')
		`, organizationID, LeadFollowUpJobType, leadSubmissionSpamReason, leadFollowUpJobKeys(runIDs)); err != nil {
			return LeadSubmissionReviewEffects{}, fmt.Errorf("complete quarantined lead follow-up jobs: %w", err)
		}
	}
	return effects, nil
}

// RecoverLeadSubmissionRuns creates one successor for the latest spam-cancelled
// run per rule. The cancelled record stays immutable operational history and a
// recovery cannot duplicate an action that already created a task.
func RecoverLeadSubmissionRuns(ctx context.Context, tx pgx.Tx, organizationID, submissionID int64, reviewVersion int) (LeadSubmissionReviewEffects, error) {
	if tx == nil || organizationID <= 0 || submissionID <= 0 || reviewVersion <= 0 {
		return LeadSubmissionReviewEffects{}, ErrInvalidInput
	}
	effects := LeadSubmissionReviewEffects{}
	rows, err := tx.Query(ctx, `
		WITH latest AS (
		  SELECT DISTINCT ON (candidate.automation_id) candidate.id
		  FROM workflow_automation_runs candidate
		  WHERE candidate.organization_id=$1
		    AND candidate.trigger_type='form_submitted' AND candidate.target_entity_type='lead_form'
		    AND candidate.trigger_payload_json->>'submissionId'=$2
		    AND candidate.status='cancelled' AND candidate.last_error=$3
		    AND NOT EXISTS (
		      SELECT 1 FROM workflow_automation_runs completed
		      WHERE completed.organization_id=candidate.organization_id
		        AND completed.automation_id=candidate.automation_id
		        AND completed.trigger_payload_json->>'submissionId'=$2
		        AND completed.status='succeeded' AND completed.actions_completed > 0
		    )
		  ORDER BY candidate.automation_id,candidate.id DESC
		)
		SELECT run.id,run.automation_id,run.automation_name,run.target_entity_id,
		       run.trigger_payload_json,run.actions_total,run.scheduled_at
		FROM workflow_automation_runs run
		JOIN latest ON latest.id=run.id
		ORDER BY run.automation_id
		FOR UPDATE OF run
	`, organizationID, strconv.FormatInt(submissionID, 10), leadSubmissionSpamReason)
	if err != nil {
		return LeadSubmissionReviewEffects{}, fmt.Errorf("lock recoverable lead follow-up runs: %w", err)
	}
	type recoverableRun struct {
		id, automationID, targetEntityID int64
		name                             string
		payload                          []byte
		actionsTotal                     int
		scheduledAt                      time.Time
	}
	recoverable := make([]recoverableRun, 0)
	for rows.Next() {
		var run recoverableRun
		if err := rows.Scan(&run.id, &run.automationID, &run.name, &run.targetEntityID, &run.payload, &run.actionsTotal, &run.scheduledAt); err != nil {
			rows.Close()
			return LeadSubmissionReviewEffects{}, fmt.Errorf("scan recoverable lead follow-up run: %w", err)
		}
		recoverable = append(recoverable, run)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return LeadSubmissionReviewEffects{}, fmt.Errorf("iterate recoverable lead follow-up runs: %w", err)
	}
	rows.Close()

	for _, source := range recoverable {
		var payload leadFollowUpPayload
		if err := json.Unmarshal(source.payload, &payload); err != nil || payload.SubmissionID != submissionID {
			return LeadSubmissionReviewEffects{}, ErrInvalidLeadFollowUpJob
		}
		payload.CreatedTaskID = 0
		payload.TerminalReason = ""
		payload.RecoveryReviewVersion = reviewVersion
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return LeadSubmissionReviewEffects{}, fmt.Errorf("encode recovered lead follow-up snapshot: %w", err)
		}
		scheduledAt := source.scheduledAt
		if scheduledAt.Before(time.Now()) {
			scheduledAt = time.Now()
		}
		var runID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO workflow_automation_runs (
				organization_id,automation_id,automation_name,trigger_type,target_entity_type,
				target_entity_id,trigger_event_key,status,trigger_payload_json,actions_total,scheduled_at
			) VALUES ($1,$2,$3,'form_submitted','lead_form',$4,$5,'queued',$6::jsonb,$7,$8)
			ON CONFLICT (organization_id,automation_id,trigger_event_key) DO NOTHING
			RETURNING id
		`, organizationID, source.automationID, source.name, source.targetEntityID,
			leadFollowUpRecoveryEventKey(submissionID, reviewVersion), string(payloadJSON), source.actionsTotal, scheduledAt).Scan(&runID)
		if err == pgx.ErrNoRows {
			continue
		}
		if err != nil {
			return LeadSubmissionReviewEffects{}, fmt.Errorf("create recovered lead follow-up run: %w", err)
		}
		if err := recordTaskActionOutcome(ctx, tx, organizationID, runID, 1, payload.Definition.Action, "queued", 0, &scheduledAt, 0, nil, ""); err != nil {
			return LeadSubmissionReviewEffects{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO background_jobs (organization_id,job_type,idempotency_key,payload_json,max_attempts,run_at)
			VALUES ($1,$2,$3,jsonb_build_object('runId',($4::bigint)::text),$5,$6)
			ON CONFLICT (organization_id,job_type,idempotency_key) DO NOTHING
		`, organizationID, LeadFollowUpJobType, leadFollowUpJobKey(runID), runID, leadFollowUpMaxAttempts, scheduledAt); err != nil {
			return LeadSubmissionReviewEffects{}, fmt.Errorf("enqueue recovered lead follow-up run: %w", err)
		}
		effects.RecoveredRuns++
	}
	return effects, nil
}

func leadFollowUpJobKeys(runIDs []int64) []string {
	keys := make([]string, 0, len(runIDs))
	for _, runID := range runIDs {
		keys = append(keys, leadFollowUpJobKey(runID))
	}
	return keys
}

func leadFollowUpRecoveryEventKey(submissionID int64, reviewVersion int) string {
	return leadFollowUpEventKey(submissionID) + ":review-recovery:" + strconv.Itoa(reviewVersion)
}
