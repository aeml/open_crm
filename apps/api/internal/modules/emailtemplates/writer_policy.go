package emailtemplates

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func lockDefinitionWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2 AND membership_status='active'
		FOR UPDATE
	`, organizationID, actorUserID).Scan(&role)
	allowed := role == "owner" || role == "admin" || role == "member"
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !allowed) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock email definition writer: %w", err)
	}
	lockKey := fmt.Sprintf("email-template-definition-capacity:%d", organizationID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return fmt.Errorf("lock email definition capacity: %w", err)
	}
	return nil
}

func requireDefinitionCapacity(ctx context.Context, tx pgx.Tx, organizationID int64, kind string) error {
	var limit int
	var limitErr error
	var count int
	var err error
	switch kind {
	case "template":
		limit, limitErr = MaxStoredTemplates, ErrTemplateLimit
		err = tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM email_templates WHERE organization_id=$1`, organizationID).Scan(&count)
	case "snippet":
		limit, limitErr = MaxStoredSnippets, ErrSnippetLimit
		err = tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM email_snippets WHERE organization_id=$1`, organizationID).Scan(&count)
	default:
		return fmt.Errorf("unsupported email definition kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("count email %ss: %w", kind, err)
	}
	if count >= limit {
		return limitErr
	}
	return nil
}

func auditDefinition(ctx context.Context, tx pgx.Tx, organizationID, actorUserID, entityID int64, name string, revision int, kind, action string) error {
	if (kind != "template" && kind != "snippet") || (action != "created" && action != "updated" && action != "deleted") {
		return fmt.Errorf("unsupported email definition audit %s %s", kind, action)
	}
	eventType := "email_" + kind + "." + action
	entityType := "email_" + kind
	summary := map[string]string{
		"created": "Created email " + kind,
		"updated": "Updated email " + kind,
		"deleted": "Deleted email " + kind,
	}[action]
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,$3,$4,$5,$6 || ' ' || $7,jsonb_build_object('revision',$8::int))
	`, organizationID, actorUserID, eventType, entityType, entityID, summary, name, revision); err != nil {
		return fmt.Errorf("audit email %s %s: %w", kind, action, err)
	}
	return nil
}
