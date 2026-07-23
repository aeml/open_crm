package leadscoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) EvaluateContact(ctx context.Context, organizationID, contactID, actorUserID int64) (Evaluation, error) {
	if s == nil || s.pool == nil {
		return Evaluation{}, fmt.Errorf("lead scoring service not configured")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Evaluation{}, fmt.Errorf("begin lead scoring transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockEvaluationWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Evaluation{}, err
	}
	contact, err := loadContact(ctx, tx, organizationID, contactID, true)
	if err != nil {
		return Evaluation{}, err
	}
	rules, err := activeRules(ctx, tx, organizationID)
	if err != nil {
		return Evaluation{}, err
	}

	rawScore := 0
	matchedRules := make([]MatchedRule, 0)
	assignedToUserID := int64(0)
	assignedToUserName := ""
	for _, rule := range rules {
		if !matchesRule(rule, contact) {
			continue
		}
		rawScore += rule.ScoreDelta
		matchedRule := MatchedRule{
			ID:               rule.ID,
			Name:             rule.Name,
			ScoreDelta:       rule.ScoreDelta,
			AssignToUserID:   rule.AssignToUserID,
			AssignToUserName: rule.AssignToUserName,
		}
		if rule.AssignToUserID > 0 {
			if err := ensureAssignee(ctx, tx, organizationID, rule.AssignToUserID); errors.Is(err, ErrInvalidAssignee) {
				matchedRule.AssignToUserID = 0
				matchedRule.AssignToUserName = ""
			} else if err != nil {
				return Evaluation{}, err
			} else if contact.OwnerUserID == 0 && assignedToUserID == 0 {
				assignedToUserID = rule.AssignToUserID
				assignedToUserName = rule.AssignToUserName
			}
		}
		matchedRules = append(matchedRules, matchedRule)
	}

	score := clampScore(rawScore)
	grade := gradeForScore(score)
	breakdownJSON, err := json.Marshal(matchedRules)
	if err != nil {
		return Evaluation{}, fmt.Errorf("encode lead score breakdown: %w", err)
	}
	metadataJSON, err := json.Marshal(map[string]any{
		"score":              score,
		"grade":              grade,
		"matchedRules":       matchedRules,
		"assignedToUserId":   assignedToUserID,
		"assignedToUserName": assignedToUserName,
	})
	if err != nil {
		return Evaluation{}, fmt.Errorf("encode lead score activity metadata: %w", err)
	}

	updated, err := tx.Exec(ctx, `
		UPDATE contacts
		SET lead_score = $3,
		    lead_grade = $4,
		    lead_scored_at = NOW(),
		    lead_score_breakdown = $5::jsonb,
		    owner_user_id = COALESCE(owner_user_id, NULLIF($6, 0)),
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, contactID, score, grade, string(breakdownJSON), assignedToUserID)
	if err != nil {
		return Evaluation{}, fmt.Errorf("update contact lead score: %w", err)
	}
	if updated.RowsAffected() == 0 {
		return Evaluation{}, ErrNotFound
	}
	summary := fmt.Sprintf("Lead scored: %d points", score)
	if grade != "" {
		summary += " (" + grade + ")"
	}
	if assignedToUserName != "" {
		summary += " and routed to " + assignedToUserName
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary, metadata_json)
		VALUES ($1, 'contact', $2, $3, 'lead.scored', $4, $5::jsonb)
	`, organizationID, contactID, actorUserID, summary, string(metadataJSON)); err != nil {
		return Evaluation{}, fmt.Errorf("insert lead score activity: %w", err)
	}
	contact, err = loadContact(ctx, tx, organizationID, contactID, false)
	if err != nil {
		return Evaluation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Evaluation{}, fmt.Errorf("commit lead scoring transaction: %w", err)
	}
	return Evaluation{
		Contact:            contact,
		Score:              score,
		Grade:              grade,
		MatchedRules:       matchedRules,
		AssignedToUserID:   assignedToUserID,
		AssignedToUserName: assignedToUserName,
	}, nil
}

func activeRules(ctx context.Context, query ruleQuerier, organizationID int64) ([]Rule, error) {
	rows, err := query.Query(ctx, ruleSelect+`
		WHERE r.organization_id = $1 AND r.is_active = TRUE
		ORDER BY r.position ASC, r.id ASC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list active lead scoring rules: %w", err)
	}
	defer rows.Close()

	rules := make([]Rule, 0)
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active lead scoring rules: %w", err)
	}
	return rules, nil
}

func loadContact(ctx context.Context, query ruleQuerier, organizationID, contactID int64, lock bool) (modulecontacts.Summary, error) {
	var contact modulecontacts.Summary
	var scoredAt pgtype.Timestamptz
	lockClause := ""
	if lock {
		lockClause = " FOR UPDATE OF co"
	}
	if err := query.QueryRow(ctx, `
		SELECT co.id, co.first_name, co.last_name,
			COALESCE(co.email, ''), COALESCE(co.phone, ''),
			COALESCE(co.address_line1, ''), COALESCE(co.address_line2, ''),
			COALESCE(co.city, ''), COALESCE(co.state, ''),
			COALESCE(co.postal_code, ''), COALESCE(co.country, ''),
			COALESCE(co.job_title, ''), COALESCE(co.status, ''), co.is_client,
			COALESCE(co.owner_user_id, 0),
			COALESCE(NULLIF(TRIM(COALESCE(ou.first_name, '') || ' ' || COALESCE(ou.last_name, '')), ''), COALESCE(ou.email, '')),
			COALESCE(co.lead_source, ''), COALESCE(co.first_source_url, ''),
			COALESCE(co.utm_source, ''), COALESCE(co.utm_medium, ''),
			COALESCE(co.utm_campaign, ''), COALESCE(co.utm_term, ''), COALESCE(co.utm_content, ''),
			co.lead_score, COALESCE(co.lead_grade, ''), co.lead_scored_at
		FROM contacts co
		LEFT JOIN users ou ON ou.id = co.owner_user_id
		WHERE co.organization_id = $1 AND co.id = $2 AND co.archived_at IS NULL
	`+lockClause, organizationID, contactID).Scan(
		&contact.ID, &contact.FirstName, &contact.LastName,
		&contact.Email, &contact.Phone,
		&contact.AddressLine1, &contact.AddressLine2,
		&contact.City, &contact.State, &contact.PostalCode, &contact.Country,
		&contact.JobTitle, &contact.Status, &contact.IsClient,
		&contact.OwnerUserID, &contact.OwnerUserName,
		&contact.LeadSource, &contact.FirstSourceURL,
		&contact.UTMSource, &contact.UTMMedium,
		&contact.UTMCampaign, &contact.UTMTerm, &contact.UTMContent,
		&contact.LeadScore, &contact.LeadGrade, &scoredAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return modulecontacts.Summary{}, ErrNotFound
		}
		return modulecontacts.Summary{}, fmt.Errorf("load lead scoring contact: %w", err)
	}
	if scoredAt.Valid {
		value := scoredAt.Time
		contact.LeadScoredAt = &value
	}
	return contact, nil
}
