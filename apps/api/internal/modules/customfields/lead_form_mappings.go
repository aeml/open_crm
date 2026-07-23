package customfields

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func ensureRequiredLeadFormCoverage(ctx context.Context, tx pgx.Tx, organizationID int64, fieldKey string) error {
	var missing bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM lead_capture_forms form
			WHERE form.organization_id=$1 AND form.is_active=TRUE
			  AND NOT EXISTS (
				SELECT 1
				FROM jsonb_array_elements(form.fields_json) field
				WHERE field->>'mapTo'=$2
			  )
		)
	`, organizationID, "custom:"+fieldKey).Scan(&missing); err != nil {
		return fmt.Errorf("validate required lead form coverage: %w", err)
	}
	if missing {
		return fmt.Errorf("%w: map this field on every active lead form before making it required", ErrConflict)
	}
	return nil
}

func advanceMappedLeadForms(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, fieldKey, summary string) error {
	_, err := tx.Exec(ctx, `
		WITH updated AS (
			UPDATE lead_capture_forms form
			SET revision=COALESCE(form.revision, 1)+1,
			    updated_by_user_id=$2,
			    updated_at=NOW()
			WHERE form.organization_id=$1
			  AND EXISTS (
				SELECT 1
				FROM jsonb_array_elements(form.fields_json) field
				WHERE field->>'mapTo'=$3
			  )
			RETURNING form.id, COALESCE(form.revision, 1) AS revision,
			          form.is_active, jsonb_array_length(form.fields_json) AS field_count
		)
		INSERT INTO audit_events (
			organization_id, actor_user_id, event_type, entity_type,
			entity_id, summary, metadata_json
		)
		SELECT $1, $2, 'lead_form.mapping_revised', 'lead_capture_form',
		       updated.id, $4,
		       jsonb_build_object(
				 'revision', updated.revision,
				 'previousRevision', updated.revision-1,
				 'isActive', updated.is_active,
				 'fieldCount', updated.field_count,
				 'customFieldKey', $5::text
			   )
		FROM updated
	`, organizationID, actorUserID, "custom:"+fieldKey, summary, fieldKey)
	if err != nil {
		return fmt.Errorf("advance mapped lead form revisions: %w", err)
	}
	return nil
}
