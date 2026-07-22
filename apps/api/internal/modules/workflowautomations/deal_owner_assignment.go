package workflowautomations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
)

func executableDealOwnerAssignment(config map[string]any, actions []Action) bool {
	contract, _ := stringConfig(config, "actionPlanContract")
	if contract != DealAssignOwnerContract || len(actions) != 1 {
		return false
	}
	action := actions[0]
	if action.Type != "assign_owner" || action.DelayMinutes != 0 || action.ScheduledAt != nil || len(action.Config) != 1 {
		return false
	}
	_, valid := exactPositiveInteger(action.Config["userId"])
	return valid
}

func assignDealOwnerFromWorkflow(
	ctx context.Context,
	tx pgx.Tx,
	event DealTaskEvent,
	runID, automationID int64,
	action Action,
) (int64, bool, error) {
	targetUserID, valid := exactPositiveInteger(action.Config["userId"])
	if !valid {
		return 0, false, ErrInvalidInput
	}
	if err := requireActiveAssignmentTarget(ctx, tx, event.OrganizationID, targetUserID); err != nil {
		return 0, false, err
	}

	var dealName, stageName string
	var stageID, previousOwnerUserID int64
	var assignmentVersion int
	if err := tx.QueryRow(ctx, `
		SELECT deal.name,deal.stage_id,stage.name,COALESCE(deal.owner_user_id,0),
		       COALESCE(deal.owner_assignment_version,0)
		FROM deals deal
		JOIN deal_stages stage
		  ON stage.organization_id=deal.organization_id AND stage.id=deal.stage_id
		WHERE deal.organization_id=$1 AND deal.id=$2 AND deal.archived_at IS NULL
		FOR UPDATE OF deal
	`, event.OrganizationID, event.DealID).Scan(&dealName, &stageID, &stageName, &previousOwnerUserID, &assignmentVersion); err != nil {
		if err == pgx.ErrNoRows {
			return 0, false, ErrInvalidInput
		}
		return 0, false, fmt.Errorf("lock deal for workflow owner assignment: %w", err)
	}

	changed := previousOwnerUserID != targetUserID
	if changed {
		if err := tx.QueryRow(ctx, `
			UPDATE deals
			SET owner_user_id=$3,
			    owner_assignment_version=COALESCE(owner_assignment_version,0)+1,
			    updated_at=NOW()
			WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
			RETURNING COALESCE(owner_assignment_version,0)
		`, event.OrganizationID, event.DealID, targetUserID).Scan(&assignmentVersion); err != nil {
			return 0, false, fmt.Errorf("assign deal owner from workflow: %w", err)
		}
		if err := modulenotifications.RecordDealAssignment(ctx, tx, modulenotifications.DealAssignment{
			OrganizationID: event.OrganizationID,
			DealID:         event.DealID,
			DealName:       dealName,
			UserID:         targetUserID,
			Version:        assignmentVersion,
		}, event.ActorUserID); err != nil {
			return 0, false, err
		}
	}

	if err := recordAssignmentActionOutcome(ctx, tx, event.OrganizationID, runID, action, targetUserID, changed); err != nil {
		return 0, false, err
	}
	if !changed {
		return targetUserID, false, nil
	}

	var activityID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO activities (
			organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json
		)
		VALUES ($1,'deal',$2,$3,'workflow.owner_assigned','Workflow assigned deal owner',
		        jsonb_build_object(
		          'automationId',$4::bigint,'runId',$5::bigint,'actionIndex',1,
		          'previousOwnerUserId',$6::bigint,'assignedOwnerUserId',$7::bigint,
		          'assignmentVersion',$8::int
		        ))
		RETURNING id
	`, event.OrganizationID, event.DealID, event.ActorUserID, automationID, runID, previousOwnerUserID, targetUserID, assignmentVersion).Scan(&activityID); err != nil {
		return 0, false, fmt.Errorf("record workflow owner assignment activity: %w", err)
	}

	cause := &WorkflowCausation{RunID: runID, ActionPosition: 1}
	if err := ExecuteDealTaskRules(ctx, tx, DealTaskEvent{
		OrganizationID: event.OrganizationID,
		ActorUserID:    event.ActorUserID,
		DealID:         event.DealID,
		DealName:       dealName,
		StageID:        stageID,
		StageName:      stageName,
		OwnerUserID:    targetUserID,
		EventType:      DealEventOwnerChanged,
		EventKey:       fmt.Sprintf("deal:%d:activity:%d", event.DealID, activityID),
		Cause:          cause,
	}); err != nil {
		return 0, false, fmt.Errorf("execute workflow-caused deal owner rules: %w", err)
	}
	return targetUserID, true, nil
}

func requireActiveAssignmentTarget(ctx context.Context, tx pgx.Tx, organizationID, userID int64) error {
	var retainedUserID int64
	if err := tx.QueryRow(ctx, `
		SELECT user_id
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2
		  AND COALESCE(membership_status,'active')='active'
		FOR SHARE
	`, organizationID, userID).Scan(&retainedUserID); err != nil {
		if err == pgx.ErrNoRows {
			return ErrInvalidInput
		}
		return fmt.Errorf("validate workflow owner assignment target: %w", err)
	}
	return nil
}
