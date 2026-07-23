package workflowautomations

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
)

// DealAddToSequenceContract opts one reviewed action into the existing durable
// email-sequence runtime. Historical add_to_sequence actions stay inert until
// an admin saves this exact contract through the bounded authoring UI.
const DealAddToSequenceContract = "deal_add_to_sequence_v1"

type dealSequenceEnrollmentResult struct {
	SequenceID   int64
	EnrollmentID int64
	ContactID    int64
	Created      bool
}

func executableDealSequenceEnrollment(config map[string]any, actions []Action) bool {
	contract, _ := stringConfig(config, "actionPlanContract")
	if contract != DealAddToSequenceContract || len(actions) != 1 {
		return false
	}
	action := actions[0]
	if action.Type != "add_to_sequence" || action.DelayMinutes != 0 || action.ScheduledAt != nil || len(action.Config) != 1 {
		return false
	}
	_, valid := exactPositiveInteger(action.Config["sequenceId"])
	return valid
}

func enrollDealPrimaryContactInSequence(
	ctx context.Context,
	tx pgx.Tx,
	event DealTaskEvent,
	runID, automationID int64,
	action Action,
) (dealSequenceEnrollmentResult, error) {
	sequenceID, valid := exactPositiveInteger(action.Config["sequenceId"])
	if !valid {
		return dealSequenceEnrollmentResult{}, ErrInvalidInput
	}

	var contactID, ownerUserID int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(primary_contact_id,0),COALESCE(owner_user_id,0)
		FROM deals
		WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
		FOR SHARE
	`, event.OrganizationID, event.DealID).Scan(&contactID, &ownerUserID); err != nil {
		if err == pgx.ErrNoRows {
			return dealSequenceEnrollmentResult{}, ErrInvalidInput
		}
		return dealSequenceEnrollmentResult{}, fmt.Errorf("load deal sequence enrollment target: %w", err)
	}
	if contactID <= 0 || ownerUserID <= 0 {
		return dealSequenceEnrollmentResult{}, ErrInvalidInput
	}

	enrollment, err := moduleemailsequences.EnsureContactEnrollmentTx(ctx, tx, event.OrganizationID, moduleemailsequences.EnrollmentInput{
		SequenceID:       sequenceID,
		ContactID:        contactID,
		EnrolledByUserID: ownerUserID,
	})
	if err != nil {
		if errors.Is(err, moduleemailsequences.ErrApprovalRequired) ||
			errors.Is(err, moduleemailsequences.ErrContactEmailRequired) ||
			errors.Is(err, moduleemailsequences.ErrInvalidInput) ||
			errors.Is(err, moduleemailsequences.ErrNotFound) ||
			errors.Is(err, moduleemailsequences.ErrSenderUnavailable) {
			return dealSequenceEnrollmentResult{}, fmt.Errorf("%w: sequence, primary contact email, or active deal owner is unavailable", ErrActionBlocked)
		}
		return dealSequenceEnrollmentResult{}, fmt.Errorf("enroll deal primary contact from workflow: %w", err)
	}
	result := dealSequenceEnrollmentResult{
		SequenceID: sequenceID, EnrollmentID: enrollment.ID,
		ContactID: contactID, Created: enrollment.Created,
	}
	if err := recordSequenceEnrollmentActionOutcome(ctx, tx, event.OrganizationID, runID, action, result); err != nil {
		return dealSequenceEnrollmentResult{}, err
	}

	actionName := "workflow.sequence_enrollment_retained"
	summary := "Workflow retained existing sequence enrollment"
	if result.Created {
		actionName = "workflow.sequence_enrolled"
		summary = "Workflow enrolled deal contact in email sequence"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (
			organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json
		)
		VALUES ($1,'deal',$2,$3,$4,$5,
		        jsonb_build_object(
		          'automationId',$6::bigint,'runId',$7::bigint,'actionIndex',1,
		          'sequenceId',$8::bigint,'enrollmentId',$9::bigint,
		          'contactId',$10::bigint,'enrollmentCreated',$11::boolean
		        ))
	`, event.OrganizationID, event.DealID, event.ActorUserID, actionName, summary,
		automationID, runID, result.SequenceID, result.EnrollmentID, result.ContactID, result.Created); err != nil {
		return dealSequenceEnrollmentResult{}, fmt.Errorf("record workflow sequence enrollment activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workflow_automation_runs
		SET status='succeeded',condition_result=TRUE,actions_completed=actions_total,
		    completed_at=NOW(),updated_at=NOW(),
		    trigger_payload_json=jsonb_set(
		      jsonb_set(
		        jsonb_set(
		          jsonb_set(trigger_payload_json,'{sequenceId}',to_jsonb($3::bigint)),
		          '{sequenceEnrollmentId}',to_jsonb($4::bigint)
		        ),
		        '{sequenceContactId}',to_jsonb($5::bigint)
		      ),
		      '{sequenceEnrollmentCreated}',to_jsonb($6::boolean)
		    )
		WHERE organization_id=$1 AND id=$2
	`, event.OrganizationID, runID, result.SequenceID, result.EnrollmentID, result.ContactID, result.Created); err != nil {
		return dealSequenceEnrollmentResult{}, fmt.Errorf("complete deal sequence enrollment automation run: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (
			organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json
		)
		VALUES ($1,$2,'workflow_automation.executed','workflow_automation',$3,
		        'Deal automation completed',
		        jsonb_build_object(
		          'dealId',$4::bigint,'event',$5::text,'sequenceId',$6::bigint,
		          'enrollmentId',$7::bigint,'contactId',$8::bigint,
		          'enrollmentCreated',$9::boolean
		        ))
	`, event.OrganizationID, event.ActorUserID, automationID, event.DealID, event.EventType,
		result.SequenceID, result.EnrollmentID, result.ContactID, result.Created); err != nil {
		return dealSequenceEnrollmentResult{}, fmt.Errorf("audit deal sequence enrollment automation run: %w", err)
	}
	return result, nil
}
