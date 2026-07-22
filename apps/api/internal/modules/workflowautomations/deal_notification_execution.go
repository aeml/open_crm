package workflowautomations

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const maxWorkflowNotificationRecipients = 50

func createDealAutomationNotification(
	ctx context.Context,
	tx pgx.Tx,
	event DealTaskEvent,
	runID, automationID int64,
	action Action,
	actionPosition, actionCount int,
) (int, error) {
	role, _ := stringConfig(action.Config, "recipientRole")
	message, _ := stringConfig(action.Config, "message")
	key := fmt.Sprintf("workflow-run:%d:action:%d", runID, actionPosition)
	var eligibleCount, retainedCount int
	if err := tx.QueryRow(ctx, `
		WITH eligible AS MATERIALIZED (
			SELECT membership.user_id
			FROM organization_memberships membership
			WHERE membership.organization_id=$1
			  AND COALESCE(membership.membership_status,'active')='active'
			  AND CASE $6::text
			    WHEN 'owner' THEN membership.role='owner'
			    WHEN 'admin' THEN membership.role IN ('owner','admin')
			    WHEN 'record_owner' THEN membership.user_id=COALESCE(
			      (
			        SELECT owner.user_id
			        FROM organization_memberships owner
			        WHERE owner.organization_id=$1 AND owner.user_id=$7
			          AND COALESCE(owner.membership_status,'active')='active'
			      ),
			      $8
			    )
			    ELSE FALSE
			  END
			ORDER BY membership.user_id
			LIMIT 51
		), bounded AS (
			SELECT user_id,COUNT(*) OVER() AS recipient_count
			FROM eligible
		), inserted AS (
			INSERT INTO notifications (
				organization_id,user_id,event_type,entity_type,entity_id,summary,idempotency_key
			)
			SELECT $1,user_id,'workflow.custom_notification','deal',$2,$3,$4
			FROM bounded
			WHERE recipient_count BETWEEN 1 AND $5
			ON CONFLICT (organization_id,user_id,idempotency_key)
			  WHERE idempotency_key IS NOT NULL
			DO UPDATE SET summary=notifications.summary
			RETURNING id
		)
		SELECT COALESCE((SELECT MAX(recipient_count) FROM bounded),0)::int,
		       (SELECT COUNT(*) FROM inserted)::int
	`, event.OrganizationID, event.DealID, message, key, maxWorkflowNotificationRecipients, role, event.OwnerUserID, event.ActorUserID).Scan(&eligibleCount, &retainedCount); err != nil {
		return 0, fmt.Errorf("create deal workflow notification: %w", err)
	}
	if eligibleCount <= 0 {
		return 0, fmt.Errorf("create deal workflow notification: no eligible recipients")
	}
	if eligibleCount > maxWorkflowNotificationRecipients {
		return 0, fmt.Errorf("create deal workflow notification: recipient limit exceeded")
	}
	if retainedCount != eligibleCount {
		return 0, fmt.Errorf("create deal workflow notification: retained %d of %d recipients", retainedCount, eligibleCount)
	}
	if err := recordNotificationActionOutcome(ctx, tx, event.OrganizationID, runID, actionPosition, action, retainedCount); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (
			organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json
		)
		VALUES ($1,'deal',$2,$3,'workflow.notification','Workflow notified teammates',
		        jsonb_build_object(
		          'automationId',$4::bigint,'runId',$5::bigint,
		          'actionIndex',$6::int,'actionCount',$7::int,
		          'recipientRole',$8::text,'recipientCount',$9::int
		        ))
	`, event.OrganizationID, event.DealID, event.ActorUserID, automationID, runID, actionPosition, actionCount, strings.TrimSpace(role), retainedCount); err != nil {
		return 0, fmt.Errorf("record deal workflow notification activity: %w", err)
	}
	return retainedCount, nil
}
