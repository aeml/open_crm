package workflowautomations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
)

const (
	LeadFollowUpJobType       = "workflow.lead_follow_up"
	leadFollowUpMaxAttempts   = 5
	leadFollowUpMaxConditions = 1
)

var ErrInvalidLeadFollowUpJob = errors.New("invalid lead follow-up workflow job")

// LeadFormSubmittedEvent is the immutable identity of one accepted public lead
// submission. CaptureLeadFormSubmitted must run in the submission transaction
// so the source record, workflow run, and durable job commit together.
type LeadFormSubmittedEvent struct {
	OrganizationID int64
	FormID         int64
	FormPublicID   string
	SubmissionID   int64
	ContactID      int64
}

type leadFollowUpSnapshot struct {
	AuthorizedByUserID int64       `json:"authorizedByUserId"`
	ConditionLogic     string      `json:"conditionLogic"`
	Conditions         []Condition `json:"conditions"`
	Action             Action      `json:"action"`
}

type leadFollowUpPayload struct {
	Event                 string               `json:"event"`
	FormID                int64                `json:"formId"`
	FormPublicID          string               `json:"formPublicId"`
	SubmissionID          int64                `json:"submissionId"`
	ContactID             int64                `json:"contactId"`
	Definition            leadFollowUpSnapshot `json:"definition"`
	RecoveryReviewVersion int                  `json:"recoveryReviewVersion,omitempty"`
	CreatedTaskID         int64                `json:"createdTaskId,omitempty"`
	TerminalReason        string               `json:"terminalReason,omitempty"`
}

func validateLeadFollowUpReferences(ctx context.Context, tx pgx.Tx, organizationID int64, input Input) error {
	if input.TriggerType != "form_submitted" || input.TargetEntityType != "lead_form" || len(input.Actions) != 1 || input.Actions[0].Type != "create_task" {
		return nil
	}
	if rawFormID, configured := input.TriggerConfig["formId"]; configured {
		formID, valid := exactPositiveInteger(rawFormID)
		if !valid {
			return ErrInvalidInput
		}
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM lead_capture_forms
				WHERE organization_id=$1 AND id=$2 AND is_active=TRUE
			)
		`, organizationID, formID).Scan(&exists); err != nil {
			return fmt.Errorf("validate lead follow-up form: %w", err)
		}
		if !exists {
			return ErrInvalidInput
		}
	}
	rawAssigneeID, configured := input.Actions[0].Config["assignedToUserId"]
	if !configured {
		return nil
	}
	assigneeID, valid := exactPositiveInteger(rawAssigneeID)
	if !valid {
		return ErrInvalidInput
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organization_memberships
			WHERE organization_id=$1 AND user_id=$2
			  AND COALESCE(membership_status,'active')='active'
		)
	`, organizationID, assigneeID).Scan(&exists); err != nil {
		return fmt.Errorf("validate lead follow-up assignee: %w", err)
	}
	if !exists {
		return ErrInvalidInput
	}
	return nil
}

// CaptureLeadFormSubmitted snapshots every matching executable rule and
// transactionally enqueues one run per rule. Unsupported legacy definitions
// remain inert and hidden rather than weakening the public lead path.
func CaptureLeadFormSubmitted(ctx context.Context, tx pgx.Tx, event LeadFormSubmittedEvent) error {
	if tx == nil || event.OrganizationID <= 0 || event.FormID <= 0 || event.SubmissionID <= 0 || event.ContactID <= 0 || strings.TrimSpace(event.FormPublicID) == "" {
		return ErrInvalidInput
	}

	rows, err := tx.Query(ctx, `
		SELECT id,name,trigger_config_json,condition_logic,conditions_json,actions_json,
		       COALESCE(updated_by_user_id,created_by_user_id,0)
		FROM workflow_automations
		WHERE organization_id=$1 AND is_active=TRUE
		  AND trigger_type='form_submitted' AND target_entity_type='lead_form'
		ORDER BY position,id
	`, event.OrganizationID)
	if err != nil {
		return fmt.Errorf("list lead follow-up workflows: %w", err)
	}
	type definitionRow struct {
		id           int64
		name         string
		trigger      []byte
		logic        string
		conditions   []byte
		actions      []byte
		authorizedBy int64
	}
	definitions := make([]definitionRow, 0)
	for rows.Next() {
		var definition definitionRow
		if err := rows.Scan(&definition.id, &definition.name, &definition.trigger, &definition.logic, &definition.conditions, &definition.actions, &definition.authorizedBy); err != nil {
			rows.Close()
			return fmt.Errorf("scan lead follow-up workflow: %w", err)
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate lead follow-up workflows: %w", err)
	}
	rows.Close()

	for _, definition := range definitions {
		var trigger map[string]any
		var conditions []Condition
		var actions []Action
		if json.Unmarshal(definition.trigger, &trigger) != nil || json.Unmarshal(definition.conditions, &conditions) != nil || json.Unmarshal(definition.actions, &actions) != nil {
			continue
		}
		if !validLeadFollowUpTrigger(trigger, event.FormID) {
			continue
		}
		snapshot := leadFollowUpSnapshot{
			AuthorizedByUserID: definition.authorizedBy,
			ConditionLogic:     definition.logic,
			Conditions:         conditions,
		}
		if len(actions) == 1 {
			snapshot.Action = actions[0]
		}
		if !validLeadFollowUpSnapshot(snapshot) {
			continue
		}

		payload := leadFollowUpPayload{
			Event:        "form_submitted",
			FormID:       event.FormID,
			FormPublicID: strings.TrimSpace(event.FormPublicID),
			SubmissionID: event.SubmissionID,
			ContactID:    event.ContactID,
			Definition:   snapshot,
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode lead follow-up workflow snapshot: %w", err)
		}

		var runID int64
		var scheduledAt time.Time
		executionDelayMinutes := leadFollowUpExecutionMinutes(snapshot.Action)
		err = tx.QueryRow(ctx, `
			INSERT INTO workflow_automation_runs (
				organization_id,automation_id,automation_name,trigger_type,target_entity_type,
				target_entity_id,trigger_event_key,status,trigger_payload_json,actions_total,scheduled_at
			) VALUES ($1,$2,$3,'form_submitted','lead_form',$4,$5,'queued',$6::jsonb,1,
			          NOW()+($7 * INTERVAL '1 minute'))
			ON CONFLICT (organization_id,automation_id,trigger_event_key) DO NOTHING
			RETURNING id,scheduled_at
		`, event.OrganizationID, definition.id, definition.name, event.FormID, leadFollowUpEventKey(event.SubmissionID), string(payloadJSON), executionDelayMinutes).Scan(&runID, &scheduledAt)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("reserve lead follow-up workflow run: %w", err)
		}
		if err := recordTaskActionOutcome(ctx, tx, event.OrganizationID, runID, 1, snapshot.Action, "queued", 0, &scheduledAt, 0, nil, ""); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO background_jobs (organization_id,job_type,idempotency_key,payload_json,max_attempts,run_at)
			VALUES ($1,$2,$3,jsonb_build_object('runId',($4::bigint)::text),$5,$6)
			ON CONFLICT (organization_id,job_type,idempotency_key) DO NOTHING
		`, event.OrganizationID, LeadFollowUpJobType, leadFollowUpJobKey(runID), runID, leadFollowUpMaxAttempts, scheduledAt); err != nil {
			return fmt.Errorf("enqueue lead follow-up workflow: %w", err)
		}
	}
	return nil
}

// HandleLeadFollowUpJob rehydrates retained form/contact evidence and creates
// the task plus reminders and run/audit evidence in one transaction. A replay
// after the worker lost its final queue acknowledgement returns the same task.
func (s *Service) HandleLeadFollowUpJob(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
	if s == nil || s.pool == nil || job.OrganizationID <= 0 || job.Type != LeadFollowUpJobType {
		return nil, ErrInvalidLeadFollowUpJob
	}
	runID, ok := leadFollowUpRunID(job.Payload)
	if !ok || job.IdempotencyKey != leadFollowUpJobKey(runID) {
		return nil, ErrInvalidLeadFollowUpJob
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin lead follow-up workflow: %w", err)
	}
	defer tx.Rollback(ctx)

	var automationID int64
	var status string
	var rawPayload []byte
	if err := tx.QueryRow(ctx, `
		SELECT automation_id,status,trigger_payload_json
		FROM workflow_automation_runs
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, job.OrganizationID, runID).Scan(&automationID, &status, &rawPayload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidLeadFollowUpJob
		}
		return nil, fmt.Errorf("lock lead follow-up workflow run: %w", err)
	}
	var payload leadFollowUpPayload
	if json.Unmarshal(rawPayload, &payload) != nil || payload.Event != "form_submitted" || payload.FormID <= 0 || payload.SubmissionID <= 0 || payload.ContactID <= 0 || !validLeadFollowUpSnapshot(payload.Definition) {
		return nil, ErrInvalidLeadFollowUpJob
	}
	if isTerminalRunStatus(status) {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit lead follow-up replay: %w", err)
		}
		return leadFollowUpResult(runID, payload.CreatedTaskID, status, payload.TerminalReason), nil
	}

	var active bool
	if err := tx.QueryRow(ctx, `
		SELECT is_active FROM workflow_automations
		WHERE organization_id=$1 AND id=$2
	`, job.OrganizationID, automationID).Scan(&active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidLeadFollowUpJob
		}
		return nil, fmt.Errorf("load lead follow-up workflow state: %w", err)
	}
	if !active {
		return s.finishLeadFollowUp(ctx, tx, job.OrganizationID, runID, payload, "cancelled", nil, 0, nil, "Rule was inactive before execution.", job.Attempts)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workflow_automation_runs
		SET status='running',started_at=COALESCE(started_at,NOW()),retry_count=$3,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2
	`, job.OrganizationID, runID, maxInt(job.Attempts-1, 0)); err != nil {
		return nil, fmt.Errorf("start lead follow-up workflow run: %w", err)
	}
	if err := recordTaskActionOutcome(ctx, tx, job.OrganizationID, runID, 1, payload.Definition.Action, "running", job.Attempts, nil, 0, nil, ""); err != nil {
		return nil, err
	}

	fields, archived, err := hydrateLeadSubmission(ctx, tx, job.OrganizationID, payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.finishLeadFollowUp(ctx, tx, job.OrganizationID, runID, payload, "skipped", nil, 0, nil, "Submission or contact no longer exists.", job.Attempts)
		}
		return nil, err
	}
	if archived {
		return s.finishLeadFollowUp(ctx, tx, job.OrganizationID, runID, payload, "skipped", nil, 0, nil, "Contact was archived before execution.", job.Attempts)
	}
	matched := EvaluateConditions(payload.Definition.ConditionLogic, payload.Definition.Conditions, fields)
	if !matched {
		return s.finishLeadFollowUp(ctx, tx, job.OrganizationID, runID, payload, "skipped", &matched, 0, nil, "Conditions did not match.", job.Attempts)
	}

	assigneeID := integerConfig(payload.Definition.Action.Config["assignedToUserId"])
	var assigneeActive bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organization_memberships
			WHERE organization_id=$1 AND user_id=$2
			  AND COALESCE(membership_status,'active')='active'
		)
	`, job.OrganizationID, assigneeID).Scan(&assigneeActive); err != nil {
		return nil, fmt.Errorf("validate lead follow-up assignee: %w", err)
	}
	if !assigneeActive {
		return s.finishLeadFollowUp(ctx, tx, job.OrganizationID, runID, payload, "failed", &matched, 0, nil, "Assigned teammate is no longer active.", job.Attempts)
	}

	title, _ := stringConfig(payload.Definition.Action.Config, "title")
	description, _ := stringConfig(payload.Definition.Action.Config, "description")
	dueMinutes, _ := leadFollowUpDueMinutes(payload.Definition.Action)
	dueAt := time.Now().UTC().Add(time.Duration(dueMinutes) * time.Minute)
	var taskID int64
	var reminderVersion int
	if err := tx.QueryRow(ctx, `
		INSERT INTO tasks (
			organization_id,entity_type,entity_id,title,description,status,due_at,
			assigned_to_user_id,created_by_user_id
		) VALUES ($1,'contact',$2,$3,NULLIF($4,''),'open',$5,$6,$7)
		RETURNING id,COALESCE(reminder_version,0)
	`, job.OrganizationID, payload.ContactID, title, description, dueAt, assigneeID, payload.Definition.AuthorizedByUserID).Scan(&taskID, &reminderVersion); err != nil {
		return nil, fmt.Errorf("create lead follow-up task: %w", err)
	}
	reminder := moduletaskreminders.State{
		OrganizationID: job.OrganizationID,
		TaskID:         taskID,
		Title:          title,
		UserID:         assigneeID,
		Status:         "open",
		DueAt:          dueAt,
		Version:        reminderVersion,
	}
	if err := moduletaskreminders.Sync(ctx, tx, reminder); err != nil {
		return nil, fmt.Errorf("schedule lead follow-up reminders: %w", err)
	}
	if err := moduletaskreminders.RecordAssignment(ctx, tx, reminder, payload.Definition.AuthorizedByUserID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json)
		VALUES ($1,'task',$2,$3,'task.automated','Task created by lead follow-up automation',
		        jsonb_build_object('automationId',$4::bigint,'runId',$5::bigint,'submissionId',$6::bigint,'contactId',$7::bigint))
	`, job.OrganizationID, taskID, payload.Definition.AuthorizedByUserID, automationID, runID, payload.SubmissionID, payload.ContactID); err != nil {
		return nil, fmt.Errorf("record lead follow-up task activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'workflow_automation.executed','workflow_automation',$3,'Lead follow-up automation completed',
		        jsonb_build_object('runId',$4::bigint,'submissionId',$5::bigint,'contactId',$6::bigint,'taskId',$7::bigint))
	`, job.OrganizationID, payload.Definition.AuthorizedByUserID, automationID, runID, payload.SubmissionID, payload.ContactID, taskID); err != nil {
		return nil, fmt.Errorf("audit lead follow-up workflow: %w", err)
	}

	return s.finishLeadFollowUp(ctx, tx, job.OrganizationID, runID, payload, "succeeded", &matched, taskID, &dueAt, "", job.Attempts)
}

func (s *Service) finishLeadFollowUp(ctx context.Context, tx pgx.Tx, organizationID, runID int64, payload leadFollowUpPayload, status string, matched *bool, taskID int64, taskDueAt *time.Time, reason string, attempts int) (map[string]any, error) {
	payload.CreatedTaskID = taskID
	payload.TerminalReason = reason
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode completed lead follow-up workflow: %w", err)
	}
	actionsCompleted := 0
	if taskID > 0 {
		actionsCompleted = 1
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workflow_automation_runs
		SET status=$3,trigger_payload_json=$4::jsonb,condition_result=$5,
		    actions_completed=$6,retry_count=$7,last_error=$8,
		    started_at=COALESCE(started_at,NOW()),completed_at=NOW(),updated_at=NOW()
		WHERE organization_id=$1 AND id=$2
	`, organizationID, runID, status, string(payloadJSON), matched, actionsCompleted, maxInt(attempts-1, 0), reason); err != nil {
		return nil, fmt.Errorf("complete lead follow-up workflow run: %w", err)
	}
	if err := recordTaskActionOutcome(ctx, tx, organizationID, runID, 1, payload.Definition.Action, status, attempts, nil, taskID, taskDueAt, reason); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit lead follow-up workflow: %w", err)
	}
	return leadFollowUpResult(runID, taskID, status, reason), nil
}

func hydrateLeadSubmission(ctx context.Context, tx pgx.Tx, organizationID int64, payload leadFollowUpPayload) (map[string]any, bool, error) {
	var formID, contactID int64
	var formPublicID, sourceURL, leadSource, utmSource, utmMedium, utmCampaign string
	var archived bool
	err := tx.QueryRow(ctx, `
		SELECT submission.form_id,submission.contact_id,form.public_id,
		       COALESCE(submission.source_url,''),COALESCE(submission.lead_source,''),
		       COALESCE(submission.utm_source,''),COALESCE(submission.utm_medium,''),
		       COALESCE(submission.utm_campaign,''),(contact.archived_at IS NOT NULL)
		FROM lead_capture_submissions submission
		JOIN lead_capture_forms form
		  ON form.organization_id=submission.organization_id AND form.id=submission.form_id
		JOIN contacts contact
		  ON contact.organization_id=submission.organization_id AND contact.id=submission.contact_id
		WHERE submission.organization_id=$1 AND submission.id=$2
		FOR UPDATE OF contact
	`, organizationID, payload.SubmissionID).Scan(&formID, &contactID, &formPublicID, &sourceURL, &leadSource, &utmSource, &utmMedium, &utmCampaign, &archived)
	if err != nil {
		return nil, false, fmt.Errorf("hydrate lead follow-up submission: %w", err)
	}
	if formID != payload.FormID || contactID != payload.ContactID || formPublicID != payload.FormPublicID {
		return nil, false, ErrInvalidLeadFollowUpJob
	}
	return map[string]any{
		"formId":       formID,
		"formPublicId": formPublicID,
		"sourceUrl":    sourceURL,
		"leadSource":   leadSource,
		"utmSource":    utmSource,
		"utmMedium":    utmMedium,
		"utmCampaign":  utmCampaign,
	}, archived, nil
}

func leadFollowUpEventKey(submissionID int64) string {
	return "lead-form-submission:" + strconv.FormatInt(submissionID, 10)
}

func leadFollowUpJobKey(runID int64) string {
	return "workflow-run:" + strconv.FormatInt(runID, 10)
}

func leadFollowUpRunID(payload map[string]any) (int64, bool) {
	value, ok := payload["runId"]
	if !ok {
		return 0, false
	}
	runID := integerConfig(value)
	return runID, runID > 0
}

func leadFollowUpResult(runID, taskID int64, status, reason string) map[string]any {
	result := map[string]any{"runId": strconv.FormatInt(runID, 10), "status": status}
	if taskID > 0 {
		result["taskId"] = strconv.FormatInt(taskID, 10)
	}
	if reason != "" {
		result["reason"] = reason
	}
	return result
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
