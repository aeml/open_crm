package leadforms

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func requireLeadFormAdmin(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	var role string
	if err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2 AND membership_status='active'
		FOR SHARE
	`, organizationID, actorUserID).Scan(&role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("validate lead form administrator: %w", err)
	}
	if role != "owner" && role != "admin" {
		return ErrNotFound
	}
	return nil
}

func auditLeadFormDefinition(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, form Form, eventType, summary string, previousRevision int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_events (
			organization_id, actor_user_id, event_type, entity_type,
			entity_id, summary, metadata_json
		)
		VALUES (
			$1, $2, $3, 'lead_capture_form', $4, $5,
			jsonb_build_object(
				'revision', $6::int,
				'previousRevision', $7::int,
				'isActive', $8::boolean,
				'fieldCount', $9::int,
				'customFieldMappingCount', $10::int
			)
		)
	`, organizationID, actorUserID, eventType, form.ID, summary, form.Revision, previousRevision, form.IsActive, len(form.Fields), countCustomFieldMappings(form.Fields))
	if err != nil {
		return fmt.Errorf("audit lead capture form definition: %w", err)
	}
	return nil
}

func countCustomFieldMappings(fields []Field) int {
	count := 0
	for _, field := range fields {
		if _, ok := customFieldKey(field.MapTo); ok {
			count++
		}
	}
	return count
}
