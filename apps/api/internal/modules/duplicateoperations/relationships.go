package duplicateoperations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func moveRelationships(ctx context.Context, tx pgx.Tx, input MergeInput) (map[string]int, error) {
	counts := map[string]int{}
	move := func(name, query string, args ...any) error {
		result, err := tx.Exec(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("move duplicate %s: %w", name, err)
		}
		if affected := int(result.RowsAffected()); affected > 0 {
			counts[name] += affected
		}
		return nil
	}
	orgID, sourceID, targetID, entityType := input.OrganizationID, input.SourceEntityID, input.TargetEntityID, input.EntityType

	genericMoves := []struct {
		name  string
		query string
	}{
		{"notes", `UPDATE notes SET entity_id=$3 WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2`},
		{"tasks", `UPDATE tasks SET entity_id=$3,updated_at=NOW() WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2`},
		{"activities", `UPDATE activities SET entity_id=$3 WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2`},
		{"meetings", `UPDATE calendar_events SET entity_id=$3,updated_at=NOW() WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2`},
		{"calls", `UPDATE call_logs SET entity_id=$3,updated_at=NOW() WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2`},
		{"smsMessages", `UPDATE sms_messages SET entity_id=$3 WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2`},
		{"smsSuppressions", `UPDATE sms_suppressions SET entity_id=$3,updated_at=NOW() WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2`},
		{"emailMessages", `UPDATE email_messages SET entity_id=$3 WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2`},
		{"notifications", `UPDATE notifications SET entity_id=$3 WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2`},
	}
	for _, item := range genericMoves {
		if err := move(item.name, item.query, orgID, sourceID, targetID, entityType); err != nil {
			return nil, err
		}
	}

	if err := consolidateFollowers(ctx, tx, input, counts); err != nil {
		return nil, err
	}
	if err := consolidateEmailLinks(ctx, tx, input, counts); err != nil {
		return nil, err
	}
	if entityType == "contact" {
		if err := move("deals", `UPDATE deals SET primary_contact_id=$3,updated_at=NOW() WHERE organization_id=$1 AND primary_contact_id=$2`, orgID, sourceID, targetID); err != nil {
			return nil, err
		}
		if err := move("leadSubmissions", `UPDATE lead_capture_submissions SET contact_id=$3 WHERE organization_id=$1 AND contact_id=$2`, orgID, sourceID, targetID); err != nil {
			return nil, err
		}
		if err := consolidateSequenceEnrollments(ctx, tx, input, counts); err != nil {
			return nil, err
		}
		if err := consolidateContactCompanyLinks(ctx, tx, input, counts); err != nil {
			return nil, err
		}
	} else {
		if err := move("deals", `UPDATE deals SET company_id=$3,updated_at=NOW() WHERE organization_id=$1 AND company_id=$2`, orgID, sourceID, targetID); err != nil {
			return nil, err
		}
		if err := consolidateCompanyContactLinks(ctx, tx, input, counts); err != nil {
			return nil, err
		}
	}
	return counts, nil
}

func consolidateFollowers(ctx context.Context, tx pgx.Tx, input MergeInput, counts map[string]int) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO record_followers (organization_id,entity_type,entity_id,user_id,created_by_user_id,created_at)
		SELECT organization_id,entity_type,$3,user_id,created_by_user_id,created_at
		FROM record_followers WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2
		ON CONFLICT (organization_id,entity_type,entity_id,user_id) DO NOTHING
	`, input.OrganizationID, input.SourceEntityID, input.TargetEntityID, input.EntityType); err != nil {
		return fmt.Errorf("consolidate duplicate followers: %w", err)
	}
	result, err := tx.Exec(ctx, `DELETE FROM record_followers WHERE organization_id=$1 AND entity_type=$3 AND entity_id=$2`, input.OrganizationID, input.SourceEntityID, input.EntityType)
	if err != nil {
		return fmt.Errorf("remove duplicate follower links: %w", err)
	}
	if affected := int(result.RowsAffected()); affected > 0 {
		counts["followers"] = affected
	}
	return nil
}

func consolidateEmailLinks(ctx context.Context, tx pgx.Tx, input MergeInput, counts map[string]int) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_message_entity_links (organization_id,email_message_id,entity_type,entity_id,created_at)
		SELECT organization_id,email_message_id,entity_type,$3,created_at
		FROM email_message_entity_links WHERE organization_id=$1 AND entity_type=$4 AND entity_id=$2
		ON CONFLICT (email_message_id,entity_type,entity_id) DO NOTHING
	`, input.OrganizationID, input.SourceEntityID, input.TargetEntityID, input.EntityType); err != nil {
		return fmt.Errorf("consolidate duplicate email links: %w", err)
	}
	result, err := tx.Exec(ctx, `DELETE FROM email_message_entity_links WHERE organization_id=$1 AND entity_type=$3 AND entity_id=$2`, input.OrganizationID, input.SourceEntityID, input.EntityType)
	if err != nil {
		return fmt.Errorf("remove duplicate email links: %w", err)
	}
	if affected := int(result.RowsAffected()); affected > 0 {
		counts["emailLinks"] = affected
	}
	return nil
}

func consolidateSequenceEnrollments(ctx context.Context, tx pgx.Tx, input MergeInput, counts map[string]int) error {
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM email_sequence_enrollments WHERE organization_id=$1 AND contact_id=$2`, input.OrganizationID, input.SourceEntityID).Scan(&total); err != nil {
		return fmt.Errorf("count duplicate sequence enrollments: %w", err)
	}
	if total == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE email_sequence_enrollments source SET status='cancelled',next_send_at=NULL,cancelled_at=COALESCE(cancelled_at,NOW()),updated_at=NOW()
		WHERE source.organization_id=$1 AND source.contact_id=$2 AND source.status IN ('active','paused')
		  AND EXISTS (SELECT 1 FROM email_sequence_enrollments target WHERE target.organization_id=$1 AND target.contact_id=$3 AND target.sequence_id=source.sequence_id AND target.status IN ('active','paused'))
	`, input.OrganizationID, input.SourceEntityID, input.TargetEntityID); err != nil {
		return fmt.Errorf("resolve duplicate active sequence enrollments: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE email_sequence_enrollments SET contact_id=$3,updated_at=NOW() WHERE organization_id=$1 AND contact_id=$2`, input.OrganizationID, input.SourceEntityID, input.TargetEntityID); err != nil {
		return fmt.Errorf("move duplicate sequence enrollments: %w", err)
	}
	counts["sequenceEnrollments"] = total
	return nil
}

func consolidateContactCompanyLinks(ctx context.Context, tx pgx.Tx, input MergeInput, counts map[string]int) error {
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM contact_company_links WHERE organization_id=$1 AND contact_id=$2`, input.OrganizationID, input.SourceEntityID).Scan(&total); err != nil {
		return fmt.Errorf("count duplicate client links: %w", err)
	}
	if total == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		WITH source_links AS MATERIALIZED (
		  SELECT company_id,relationship_title,is_primary FROM contact_company_links WHERE organization_id=$1 AND contact_id=$2
		), demoted AS (
		  UPDATE contact_company_links SET is_primary=FALSE WHERE organization_id=$1 AND contact_id=$2 AND is_primary RETURNING 1
		), sync AS (SELECT count(*) FROM demoted)
		INSERT INTO contact_company_links (organization_id,contact_id,company_id,relationship_title,is_primary)
		SELECT $1,$3,source_links.company_id,source_links.relationship_title,source_links.is_primary FROM source_links CROSS JOIN sync
		ON CONFLICT (organization_id,contact_id,company_id) DO UPDATE SET
		  relationship_title=COALESCE(NULLIF(contact_company_links.relationship_title,''),EXCLUDED.relationship_title),
		  is_primary=contact_company_links.is_primary OR EXCLUDED.is_primary
	`, input.OrganizationID, input.SourceEntityID, input.TargetEntityID); err != nil {
		return fmt.Errorf("consolidate duplicate client links: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM contact_company_links WHERE organization_id=$1 AND contact_id=$2`, input.OrganizationID, input.SourceEntityID); err != nil {
		return fmt.Errorf("remove duplicate client links: %w", err)
	}
	counts["clientLinks"] = total
	return nil
}

func consolidateCompanyContactLinks(ctx context.Context, tx pgx.Tx, input MergeInput, counts map[string]int) error {
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM contact_company_links WHERE organization_id=$1 AND company_id=$2`, input.OrganizationID, input.SourceEntityID).Scan(&total); err != nil {
		return fmt.Errorf("count duplicate contact links: %w", err)
	}
	if total == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		WITH source_links AS MATERIALIZED (
		  SELECT contact_id,relationship_title,is_primary FROM contact_company_links WHERE organization_id=$1 AND company_id=$2
		)
		INSERT INTO contact_company_links (organization_id,contact_id,company_id,relationship_title,is_primary)
		SELECT $1,source_links.contact_id,$3,source_links.relationship_title,
		       source_links.is_primary AND NOT EXISTS (SELECT 1 FROM contact_company_links WHERE organization_id=$1 AND company_id=$3 AND is_primary)
		FROM source_links
		ON CONFLICT (organization_id,contact_id,company_id) DO UPDATE SET
		  relationship_title=COALESCE(NULLIF(contact_company_links.relationship_title,''),EXCLUDED.relationship_title),
		  is_primary=contact_company_links.is_primary OR EXCLUDED.is_primary
	`, input.OrganizationID, input.SourceEntityID, input.TargetEntityID); err != nil {
		return fmt.Errorf("consolidate duplicate contact links: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM contact_company_links WHERE organization_id=$1 AND company_id=$2`, input.OrganizationID, input.SourceEntityID); err != nil {
		return fmt.Errorf("remove duplicate contact links: %w", err)
	}
	counts["contactLinks"] = total
	return nil
}
