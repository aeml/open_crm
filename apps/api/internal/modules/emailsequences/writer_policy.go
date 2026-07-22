package emailsequences

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func lockSequenceWriter(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, requireAdmin bool) error {
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role
		FROM organization_memberships
		WHERE organization_id=$1 AND user_id=$2 AND membership_status='active'
		FOR UPDATE
	`, organizationID, actorUserID).Scan(&role)
	allowed := role == "owner" || role == "admin" || (!requireAdmin && role == "member")
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !allowed) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock email sequence writer: %w", err)
	}
	lockKey := fmt.Sprintf("email-sequence-active-capacity:%d", organizationID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return fmt.Errorf("lock email sequence capacity: %w", err)
	}
	return nil
}

func requireActiveSequenceCapacity(ctx context.Context, tx pgx.Tx, organizationID int64) error {
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM email_sequences WHERE organization_id=$1 AND status='active'`, organizationID).Scan(&count); err != nil {
		return fmt.Errorf("count active email sequences: %w", err)
	}
	if count >= MaxActiveSequences {
		return ErrActiveLimit
	}
	return nil
}

func auditSequence(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, sequence Sequence, stepCount int, action string) error {
	summaries := map[string]string{
		"created":  "Created email sequence",
		"updated":  "Updated email sequence",
		"deleted":  "Deleted email sequence",
		"approved": "Approved and activated email sequence",
		"paused":   "Paused email sequence",
	}
	summary, ok := summaries[action]
	if !ok {
		return fmt.Errorf("unsupported email sequence audit action %q", action)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,$3,'email_sequence',$4,$5 || ' ' || $6,
		        jsonb_build_object('revision',$7::int,'status',$8::text,'steps',$9::int))
	`, organizationID, actorUserID, "email_sequence."+action, sequence.ID, summary, sequence.Name,
		sequence.Revision, sequence.Status, stepCount); err != nil {
		return fmt.Errorf("audit email sequence %s: %w", action, err)
	}
	return nil
}
