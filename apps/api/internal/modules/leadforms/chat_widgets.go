package leadforms

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ListChatWidgetsByOrganization returns one stable bounded management page and
// its exact status-filtered total from the same tenant snapshot.
func (s *Service) ListChatWidgetsByOrganization(ctx context.Context, organizationID int64, query LeadSurfaceListQuery) (ChatWidgetListPage, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return ChatWidgetListPage{}, fmt.Errorf("lead forms service not configured")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	query, page, err := normalizeLeadSurfaceListQuery(query)
	if err != nil {
		return ChatWidgetListPage{}, err
	}
	filter := leadSurfaceStatusFilter(query.Status, "w")
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ChatWidgetListPage{}, fmt.Errorf("begin lead chat widget list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result := ChatWidgetListPage{Widgets: []ChatWidget{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_chat_widgets w WHERE w.organization_id=$1`+filter, organizationID).Scan(&result.Total); err != nil {
		return ChatWidgetListPage{}, fmt.Errorf("count lead chat widgets: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT w.id,w.name,w.public_id,w.title,w.welcome_message,w.prompt_label,w.cta_label,w.theme,w.position,
		       w.lead_capture_form_id,f.name,f.public_id,w.is_active,COALESCE(w.revision,1),w.created_at,w.updated_at
		FROM lead_chat_widgets w
		JOIN lead_capture_forms f ON f.organization_id=w.organization_id AND f.id=w.lead_capture_form_id
		WHERE w.organization_id=$1`+filter+`
		ORDER BY w.is_active DESC,w.updated_at DESC,w.id DESC
		LIMIT $2 OFFSET $3
	`, organizationID, page.Size, page.Offset)
	if err != nil {
		return ChatWidgetListPage{}, fmt.Errorf("list lead chat widgets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		entry, scanErr := scanChatWidget(rows)
		if scanErr != nil {
			return ChatWidgetListPage{}, scanErr
		}
		result.Widgets = append(result.Widgets, entry)
	}
	if err := rows.Err(); err != nil {
		return ChatWidgetListPage{}, fmt.Errorf("iterate lead chat widgets: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return ChatWidgetListPage{}, fmt.Errorf("commit lead chat widget list: %w", err)
	}
	return result, nil
}

func (s *Service) CreateChatWidget(ctx context.Context, organizationID, actorUserID int64, input ChatWidgetInput) (ChatWidget, error) {
	if s == nil || s.pool == nil {
		return ChatWidget{}, fmt.Errorf("lead forms service not configured")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	input = normalizeChatWidgetInput(input)
	if err := validateChatWidgetInput(input); err != nil {
		return ChatWidget{}, err
	}
	publicID, err := newChatWidgetPublicID()
	if err != nil {
		return ChatWidget{}, err
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return ChatWidget{}, fmt.Errorf("begin lead chat widget create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireLeadFormAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return ChatWidget{}, err
	}
	if err := requireLeadSurfaceForm(ctx, tx, organizationID, input.LeadCaptureFormID, isActive); err != nil {
		return ChatWidget{}, err
	}

	widget, err := scanChatWidget(tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO lead_chat_widgets (
				organization_id,public_id,lead_capture_form_id,name,title,welcome_message,
				prompt_label,cta_label,theme,position,is_active,created_by_user_id,updated_by_user_id
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
			RETURNING *
		)
		SELECT w.id,w.name,w.public_id,w.title,w.welcome_message,w.prompt_label,w.cta_label,w.theme,w.position,
		       w.lead_capture_form_id,f.name,f.public_id,w.is_active,COALESCE(w.revision,1),w.created_at,w.updated_at
		FROM inserted w
		JOIN lead_capture_forms f ON f.organization_id=w.organization_id AND f.id=w.lead_capture_form_id
	`, organizationID, publicID, input.LeadCaptureFormID, input.Name, input.Title, input.WelcomeMessage, input.PromptLabel, input.CTALabel, input.Theme, input.Position, isActive, actorUserID))
	if err != nil {
		return ChatWidget{}, mapChatWidgetSaveError(err)
	}
	if err := auditChatWidgetDefinition(ctx, tx, organizationID, actorUserID, widget, "lead_chat_widget.created", "Created lead chat widget", 0); err != nil {
		return ChatWidget{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChatWidget{}, fmt.Errorf("commit lead chat widget create: %w", err)
	}
	return widget, nil
}

func (s *Service) UpdateChatWidget(ctx context.Context, organizationID, widgetID, actorUserID int64, input ChatWidgetInput) (ChatWidget, error) {
	if s == nil || s.pool == nil {
		return ChatWidget{}, fmt.Errorf("lead forms service not configured")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	input = normalizeChatWidgetInput(input)
	if input.Revision <= 0 {
		return ChatWidget{}, ErrInvalidWidget
	}
	if err := validateChatWidgetInput(input); err != nil {
		return ChatWidget{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return ChatWidget{}, fmt.Errorf("begin lead chat widget update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireLeadFormAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return ChatWidget{}, err
	}
	var currentRevision int
	var currentActive bool
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(revision,1),is_active
		FROM lead_chat_widgets
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, widgetID).Scan(&currentRevision, &currentActive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChatWidget{}, ErrNotFound
		}
		return ChatWidget{}, fmt.Errorf("lock lead chat widget: %w", err)
	}
	if input.Revision != currentRevision {
		return ChatWidget{}, ErrStaleWidget
	}
	targetActive := currentActive
	if input.IsActive != nil {
		targetActive = *input.IsActive
	}
	if err := requireLeadSurfaceForm(ctx, tx, organizationID, input.LeadCaptureFormID, targetActive); err != nil {
		return ChatWidget{}, err
	}
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	widget, err := scanChatWidget(tx.QueryRow(ctx, `
		WITH updated AS (
			UPDATE lead_chat_widgets
			SET lead_capture_form_id=$3,name=$4,title=$5,welcome_message=$6,prompt_label=$7,
			    cta_label=$8,theme=$9,position=$10,is_active=COALESCE($11::boolean,is_active),
			    updated_by_user_id=$12,revision=revision+1,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2 AND revision=$13
			RETURNING *
		)
		SELECT w.id,w.name,w.public_id,w.title,w.welcome_message,w.prompt_label,w.cta_label,w.theme,w.position,
		       w.lead_capture_form_id,f.name,f.public_id,w.is_active,COALESCE(w.revision,1),w.created_at,w.updated_at
		FROM updated w
		JOIN lead_capture_forms f ON f.organization_id=w.organization_id AND f.id=w.lead_capture_form_id
	`, organizationID, widgetID, input.LeadCaptureFormID, input.Name, input.Title, input.WelcomeMessage, input.PromptLabel, input.CTALabel, input.Theme, input.Position, isActive, actorUserID, currentRevision))
	if err != nil {
		return ChatWidget{}, mapChatWidgetSaveError(err)
	}
	if err := auditChatWidgetDefinition(ctx, tx, organizationID, actorUserID, widget, "lead_chat_widget.updated", "Updated lead chat widget", currentRevision); err != nil {
		return ChatWidget{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChatWidget{}, fmt.Errorf("commit lead chat widget update: %w", err)
	}
	return widget, nil
}
