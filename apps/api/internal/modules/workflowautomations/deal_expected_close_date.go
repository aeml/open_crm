package workflowautomations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// DealSetExpectedCloseContract opts one reviewed update_field action into the
// bounded expected-close-date runtime. Legacy generic field mutations remain
// inert until an admin saves this exact contract through the authoring UI.
const DealSetExpectedCloseContract = "deal_set_expected_close_v1"

type dealExpectedCloseResult struct {
	Previous string
	Current  string
	Changed  bool
}

func executableDealExpectedCloseDate(config map[string]any, actions []Action) bool {
	contract, _ := stringConfig(config, "actionPlanContract")
	if contract != DealSetExpectedCloseContract || len(actions) != 1 {
		return false
	}
	action := actions[0]
	if action.Type != "update_field" || action.DelayMinutes != 0 || action.ScheduledAt != nil || len(action.Config) != 2 {
		return false
	}
	field, validField := stringConfig(action.Config, "field")
	_, validDays := exactBoundedNonNegativeInteger(action.Config["value"], 365)
	return validField && field == "expectedCloseDate" && validDays
}

func setDealExpectedCloseDateFromWorkflow(
	ctx context.Context,
	tx pgx.Tx,
	event DealTaskEvent,
	runID, automationID int64,
	action Action,
) (dealExpectedCloseResult, error) {
	daysFromEvent, valid := exactBoundedNonNegativeInteger(action.Config["value"], 365)
	if !valid {
		return dealExpectedCloseResult{}, ErrInvalidInput
	}

	var result dealExpectedCloseResult
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(TO_CHAR(expected_close_date,'YYYY-MM-DD'),''),
		       TO_CHAR((CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date + $3::int,'YYYY-MM-DD')
		FROM deals
		WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
		FOR UPDATE
	`, event.OrganizationID, event.DealID, daysFromEvent).Scan(&result.Previous, &result.Current); err != nil {
		if err == pgx.ErrNoRows {
			return dealExpectedCloseResult{}, ErrInvalidInput
		}
		return dealExpectedCloseResult{}, fmt.Errorf("lock deal for workflow expected close date: %w", err)
	}
	result.Changed = result.Previous != result.Current
	if result.Changed {
		command, err := tx.Exec(ctx, `
			UPDATE deals
			SET expected_close_date=$3::date,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
		`, event.OrganizationID, event.DealID, result.Current)
		if err != nil {
			return dealExpectedCloseResult{}, fmt.Errorf("set deal expected close date from workflow: %w", err)
		}
		if command.RowsAffected() != 1 {
			return dealExpectedCloseResult{}, ErrInvalidInput
		}
	}

	if err := recordExpectedCloseActionOutcome(ctx, tx, event.OrganizationID, runID, action, result); err != nil {
		return dealExpectedCloseResult{}, err
	}
	if result.Changed {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activities (
				organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json
			)
			VALUES ($1,'deal',$2,$3,'workflow.expected_close_date_set',
			        'Workflow set expected close date',
			        jsonb_build_object(
			          'automationId',$4::bigint,'runId',$5::bigint,'actionIndex',1,
			          'field','expectedCloseDate','previousValue',$6::text,
			          'currentValue',$7::text,'daysFromEvent',$8::int
			        ))
		`, event.OrganizationID, event.DealID, event.ActorUserID, automationID, runID,
			result.Previous, result.Current, daysFromEvent); err != nil {
			return dealExpectedCloseResult{}, fmt.Errorf("record workflow expected close date activity: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workflow_automation_runs
		SET status='succeeded',condition_result=TRUE,actions_completed=actions_total,
		    completed_at=NOW(),updated_at=NOW(),
		    trigger_payload_json=jsonb_set(
		      jsonb_set(
		        jsonb_set(trigger_payload_json,'{updatedField}',to_jsonb('expectedCloseDate'::text)),
		        '{updatedFieldValue}',to_jsonb($3::text)
		      ),
		      '{fieldValueChanged}',to_jsonb($4::boolean)
		    )
		WHERE organization_id=$1 AND id=$2
	`, event.OrganizationID, runID, result.Current, result.Changed); err != nil {
		return dealExpectedCloseResult{}, fmt.Errorf("complete expected close date automation run: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (
			organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json
		)
		VALUES ($1,$2,'workflow_automation.executed','workflow_automation',$3,
		        'Deal automation completed',
		        jsonb_build_object(
		          'dealId',$4::bigint,'event',$5::text,'field','expectedCloseDate',
		          'currentValue',$6::text,'fieldValueChanged',$7::boolean
		        ))
	`, event.OrganizationID, event.ActorUserID, automationID, event.DealID,
		event.EventType, result.Current, result.Changed); err != nil {
		return dealExpectedCloseResult{}, fmt.Errorf("audit expected close date automation run: %w", err)
	}
	return result, nil
}
