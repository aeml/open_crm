package leadscoring

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Rule, error) {
	if s == nil || s.pool == nil {
		return Rule{}, fmt.Errorf("lead scoring service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Rule{}, err
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Rule{}, fmt.Errorf("begin lead scoring rule create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockRuleWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Rule{}, err
	}
	if err := ensureAssignee(ctx, tx, organizationID, input.AssignToUserID); err != nil {
		return Rule{}, err
	}
	var ruleCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_scoring_rules WHERE organization_id=$1`, organizationID).Scan(&ruleCount); err != nil {
		return Rule{}, fmt.Errorf("count lead scoring rules: %w", err)
	}
	if ruleCount >= MaxRulesPerOrganization {
		return Rule{}, ErrRuleLimit
	}

	var ruleID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO lead_scoring_rules (organization_id, name, description, field, operator, value, score_delta, assign_to_user_id, is_active, position, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, 0), $9, $10, $11, $11)
		RETURNING id
	`, organizationID, input.Name, input.Description, input.Field, input.Operator, input.Value, input.ScoreDelta, input.AssignToUserID, isActive, input.Position, actorUserID).Scan(&ruleID); err != nil {
		return Rule{}, mapSaveError(err)
	}
	rule, err := getByID(ctx, tx, organizationID, ruleID)
	if err != nil {
		return Rule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Rule{}, fmt.Errorf("commit lead scoring rule create: %w", err)
	}
	return rule, nil
}

func (s *Service) Update(ctx context.Context, organizationID, ruleID, actorUserID int64, input Input) (Rule, error) {
	if s == nil || s.pool == nil {
		return Rule{}, fmt.Errorf("lead scoring service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Rule{}, err
	}
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Rule{}, fmt.Errorf("begin lead scoring rule update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockRuleWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Rule{}, err
	}
	var existingID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM lead_scoring_rules WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, ruleID).Scan(&existingID); errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, ErrNotFound
	} else if err != nil {
		return Rule{}, fmt.Errorf("lock lead scoring rule: %w", err)
	}
	if err := ensureAssignee(ctx, tx, organizationID, input.AssignToUserID); err != nil {
		return Rule{}, err
	}

	updated, err := tx.Exec(ctx, `
		UPDATE lead_scoring_rules
		SET name = $3,
		    description = $4,
		    field = $5,
		    operator = $6,
		    value = $7,
		    score_delta = $8,
		    assign_to_user_id = NULLIF($9, 0),
		    is_active = COALESCE($10::boolean, is_active),
		    position = $11,
		    updated_by_user_id = $12,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
	`, organizationID, ruleID, input.Name, input.Description, input.Field, input.Operator, input.Value, input.ScoreDelta, input.AssignToUserID, isActive, input.Position, actorUserID)
	if err != nil {
		return Rule{}, mapSaveError(err)
	}
	if updated.RowsAffected() == 0 {
		return Rule{}, ErrNotFound
	}
	rule, err := getByID(ctx, tx, organizationID, ruleID)
	if err != nil {
		return Rule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Rule{}, fmt.Errorf("commit lead scoring rule update: %w", err)
	}
	return rule, nil
}

func getByID(ctx context.Context, query ruleQuerier, organizationID, ruleID int64) (Rule, error) {
	rule, err := scanRule(query.QueryRow(ctx, ruleSelect+`
		WHERE r.organization_id = $1 AND r.id = $2
	`, organizationID, ruleID))
	if err != nil {
		return Rule{}, mapSaveError(err)
	}
	return rule, nil
}
