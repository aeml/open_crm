package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// handoffWonDeal promotes the existing record selected on a won deal into the
// existing client model. An explicit company wins over the primary contact so
// an organization sale does not also create a duplicate individual client.
func handoffWonDeal(ctx context.Context, tx pgx.Tx, organizationID, dealID, actorUserID int64) error {
	var dealName string
	var companyID, contactID int64
	if err := tx.QueryRow(ctx, `
		SELECT name,COALESCE(company_id,0),COALESCE(primary_contact_id,0)
		FROM deals
		WHERE organization_id=$1 AND id=$2 AND status='won' AND archived_at IS NULL
	`, organizationID, dealID).Scan(&dealName, &companyID, &contactID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("load won deal handoff target: %w", err)
	}

	entityType := ""
	entityID := int64(0)
	if companyID > 0 {
		result, err := tx.Exec(ctx, `
			UPDATE companies
			SET status='customer',updated_at=NOW()
			WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
			  AND COALESCE(status,'')<>'customer'
		`, organizationID, companyID)
		if err != nil {
			return fmt.Errorf("promote won-deal company to customer: %w", err)
		}
		if result.RowsAffected() > 0 {
			entityType, entityID = "company", companyID
		}
	} else if contactID > 0 {
		result, err := tx.Exec(ctx, `
			UPDATE contacts
			SET status='customer',is_client=TRUE,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
			  AND (COALESCE(status,'')<>'customer' OR is_client=FALSE)
		`, organizationID, contactID)
		if err != nil {
			return fmt.Errorf("promote won-deal contact to client: %w", err)
		}
		if result.RowsAffected() > 0 {
			entityType, entityID = "contact", contactID
		}
	}
	if entityID == 0 {
		return nil
	}

	summary := fmt.Sprintf("Customer handoff from won deal: %s", dealName)
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary,metadata_json)
		VALUES ($1,$2,$3,$4,'client.handoff',$5,jsonb_build_object('dealId',$6::bigint,'dealName',$7::text))
	`, organizationID, entityType, entityID, actorUserID, summary, dealID, dealName); err != nil {
		return fmt.Errorf("record won-deal handoff activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'client.handoff',$3,$4,$5,jsonb_build_object('dealId',$6::bigint,'dealName',$7::text))
	`, organizationID, actorUserID, entityType, entityID, summary, dealID, dealName); err != nil {
		return fmt.Errorf("audit won-deal handoff: %w", err)
	}
	return nil
}
