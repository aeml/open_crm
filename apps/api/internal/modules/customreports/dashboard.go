package customreports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	MaxDashboardWidgets     = 6
	dashboardWidgetPageSize = 12
)

var ErrDashboardRevisionConflict = errors.New("shared report dashboard revision conflict")

type DashboardWidgetInput struct {
	ReportDefinitionID int64  `json:"reportDefinitionId"`
	Width              string `json:"width"`
}

type DashboardInput struct {
	Revision int64                  `json:"revision"`
	Widgets  []DashboardWidgetInput `json:"widgets"`
}

type DashboardWidget struct {
	ID                 int64      `json:"id"`
	ReportDefinitionID int64      `json:"reportDefinitionId"`
	Position           int        `json:"position"`
	Width              string     `json:"width"`
	Definition         Definition `json:"definition"`
}

type Dashboard struct {
	ID        int64             `json:"id"`
	Revision  int64             `json:"revision"`
	Widgets   []DashboardWidget `json:"widgets"`
	UpdatedAt time.Time         `json:"updatedAt,omitempty"`
}

type DashboardWidgetExecution struct {
	Position   int        `json:"position"`
	Width      string     `json:"width"`
	Definition Definition `json:"definition"`
	Result     Execution  `json:"result"`
}

type DashboardExecution struct {
	Revision    int64                      `json:"revision"`
	GeneratedAt time.Time                  `json:"generatedAt"`
	Widgets     []DashboardWidgetExecution `json:"widgets"`
}

func (s *Service) GetDashboard(ctx context.Context, organizationID int64) (Dashboard, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return Dashboard{}, fmt.Errorf("custom reports service not configured")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Dashboard{}, fmt.Errorf("begin shared report dashboard read: %w", err)
	}
	defer tx.Rollback(ctx)
	dashboard, err := loadDashboard(ctx, tx, organizationID)
	if err != nil {
		return Dashboard{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Dashboard{}, fmt.Errorf("commit shared report dashboard read: %w", err)
	}
	return dashboard, nil
}

func (s *Service) UpdateDashboard(ctx context.Context, organizationID, actorUserID int64, input DashboardInput) (Dashboard, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || actorUserID <= 0 {
		return Dashboard{}, fmt.Errorf("custom reports service not configured")
	}
	input, err := normalizeDashboardInput(input)
	if err != nil {
		return Dashboard{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Dashboard{}, fmt.Errorf("begin shared report dashboard update: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("shared-report-dashboard:%d", organizationID)); err != nil {
		return Dashboard{}, fmt.Errorf("lock shared report dashboard writer: %w", err)
	}
	if err := requireActiveReportWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Dashboard{}, err
	}

	current, err := loadDashboardForUpdate(ctx, tx, organizationID)
	if err != nil {
		return Dashboard{}, err
	}
	if input.Revision != current.Revision {
		return Dashboard{}, ErrDashboardRevisionConflict
	}

	for _, widget := range input.Widgets {
		definition, _, err := loadExecutableDefinitionForShare(ctx, tx, organizationID, widget.ReportDefinitionID)
		if err != nil {
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInactive) || errors.Is(err, ErrUnsupportedVisualization) {
				return Dashboard{}, ErrInvalidInput
			}
			return Dashboard{}, err
		}
		if definition.VisualizationType != "bar" || definition.VisualizationContract != groupedBarContract {
			return Dashboard{}, ErrInvalidInput
		}
	}
	if dashboardMatchesInput(current, input) {
		if err := tx.Commit(ctx); err != nil {
			return Dashboard{}, fmt.Errorf("commit unchanged shared report dashboard: %w", err)
		}
		return current, nil
	}

	dashboardID := current.ID
	nextRevision := current.Revision + 1
	if dashboardID == 0 {
		err = tx.QueryRow(ctx, `
			INSERT INTO custom_report_dashboards (organization_id,revision,updated_by_user_id)
			VALUES ($1,$2,$3)
			RETURNING id
		`, organizationID, nextRevision, actorUserID).Scan(&dashboardID)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE custom_report_dashboards
			SET revision=$3,updated_by_user_id=$4,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2
		`, organizationID, dashboardID, nextRevision, actorUserID)
	}
	if err != nil {
		return Dashboard{}, fmt.Errorf("save shared report dashboard: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM custom_report_dashboard_widgets WHERE organization_id=$1 AND dashboard_id=$2`, organizationID, dashboardID); err != nil {
		return Dashboard{}, fmt.Errorf("replace shared report dashboard widgets: %w", err)
	}
	for position, widget := range input.Widgets {
		if _, err := tx.Exec(ctx, `
			INSERT INTO custom_report_dashboard_widgets (organization_id,dashboard_id,report_definition_id,position,width)
			VALUES ($1,$2,$3,$4,$5)
		`, organizationID, dashboardID, widget.ReportDefinitionID, position, widget.Width); err != nil {
			return Dashboard{}, fmt.Errorf("insert shared report dashboard widget: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'report_dashboard.updated','report_dashboard',$3,'Updated shared report dashboard',
			jsonb_build_object('revision',$4::bigint,'widgetCount',$5::int))
	`, organizationID, actorUserID, dashboardID, nextRevision, len(input.Widgets)); err != nil {
		return Dashboard{}, fmt.Errorf("record shared report dashboard audit: %w", err)
	}
	saved, err := loadDashboard(ctx, tx, organizationID)
	if err != nil {
		return Dashboard{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Dashboard{}, fmt.Errorf("commit shared report dashboard update: %w", err)
	}
	return saved, nil
}

func (s *Service) ExecuteDashboard(ctx context.Context, organizationID int64) (DashboardExecution, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return DashboardExecution{}, fmt.Errorf("custom reports service not configured")
	}
	queryCtx, cancel := context.WithTimeout(ctx, executionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return DashboardExecution{}, dashboardExecutionError(queryCtx, "begin shared report dashboard snapshot", err)
	}
	defer tx.Rollback(queryCtx)
	dashboard, err := loadDashboard(queryCtx, tx, organizationID)
	if err != nil {
		return DashboardExecution{}, dashboardExecutionError(queryCtx, "load shared report dashboard snapshot", err)
	}
	generatedAt := time.Now().UTC()
	result := DashboardExecution{Revision: dashboard.Revision, GeneratedAt: generatedAt, Widgets: []DashboardWidgetExecution{}}
	for _, widget := range dashboard.Widgets {
		definition := widget.Definition
		if !definition.IsActive {
			return DashboardExecution{}, ErrInactive
		}
		if definition.VisualizationType != "bar" || definition.VisualizationContract != groupedBarContract {
			return DashboardExecution{}, ErrUnsupportedVisualization
		}
		input := normalizeInput(Input{
			Name:                  definition.Name,
			Description:           definition.Description,
			SourceType:            definition.SourceType,
			VisualizationType:     definition.VisualizationType,
			VisualizationContract: definition.VisualizationContract,
			Columns:               definition.Columns,
			Filters:               definition.Filters,
			GroupBy:               definition.GroupBy,
			Aggregation:           definition.Aggregation,
		})
		if err := validateInput(input); err != nil {
			return DashboardExecution{}, ErrInvalidInput
		}
		statement, args, columns, err := buildExecutionStatementWindow(organizationID, input, dashboardWidgetPageSize+1, 0)
		if err != nil {
			return DashboardExecution{}, err
		}
		rows, err := queryExecutionRows(queryCtx, tx, statement, args, columns)
		if err != nil {
			return DashboardExecution{}, dashboardExecutionError(queryCtx, "execute shared report dashboard widget", err)
		}
		hasMore := len(rows) > dashboardWidgetPageSize
		if hasMore {
			rows = rows[:dashboardWidgetPageSize]
		}
		result.Widgets = append(result.Widgets, DashboardWidgetExecution{
			Position:   widget.Position,
			Width:      widget.Width,
			Definition: definition,
			Result: Execution{
				DefinitionID:          definition.ID,
				DefinitionName:        definition.Name,
				SourceType:            definition.SourceType,
				VisualizationType:     definition.VisualizationType,
				VisualizationContract: definition.VisualizationContract,
				Columns:               columns,
				Rows:                  rows,
				Page:                  1,
				PageSize:              dashboardWidgetPageSize,
				HasMore:               hasMore,
				GeneratedAt:           generatedAt,
			},
		})
	}
	if err := tx.Commit(queryCtx); err != nil {
		return DashboardExecution{}, dashboardExecutionError(queryCtx, "commit shared report dashboard snapshot", err)
	}
	return result, nil
}

func normalizeDashboardInput(input DashboardInput) (DashboardInput, error) {
	if input.Revision < 0 || input.Widgets == nil || len(input.Widgets) > MaxDashboardWidgets {
		return DashboardInput{}, ErrInvalidInput
	}
	seen := make(map[int64]bool, len(input.Widgets))
	for index := range input.Widgets {
		widget := &input.Widgets[index]
		if widget.ReportDefinitionID <= 0 || seen[widget.ReportDefinitionID] {
			return DashboardInput{}, ErrInvalidInput
		}
		seen[widget.ReportDefinitionID] = true
		if widget.Width == "" {
			widget.Width = "half"
		}
		if widget.Width != "half" && widget.Width != "full" {
			return DashboardInput{}, ErrInvalidInput
		}
	}
	return input, nil
}

func loadDashboardForUpdate(ctx context.Context, tx pgx.Tx, organizationID int64) (Dashboard, error) {
	var dashboard Dashboard
	err := tx.QueryRow(ctx, `
		SELECT id,revision,updated_at
		FROM custom_report_dashboards
		WHERE organization_id=$1
		FOR UPDATE
	`, organizationID).Scan(&dashboard.ID, &dashboard.Revision, &dashboard.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Dashboard{Widgets: []DashboardWidget{}}, nil
	}
	if err != nil {
		return Dashboard{}, fmt.Errorf("lock shared report dashboard: %w", err)
	}
	dashboard.Widgets, err = loadDashboardWidgets(ctx, tx, organizationID, dashboard.ID)
	return dashboard, err
}

func loadDashboard(ctx context.Context, query executionQuerier, organizationID int64) (Dashboard, error) {
	var dashboard Dashboard
	err := query.QueryRow(ctx, `
		SELECT id,revision,updated_at
		FROM custom_report_dashboards
		WHERE organization_id=$1
	`, organizationID).Scan(&dashboard.ID, &dashboard.Revision, &dashboard.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Dashboard{Widgets: []DashboardWidget{}}, nil
	}
	if err != nil {
		return Dashboard{}, fmt.Errorf("load shared report dashboard: %w", err)
	}
	dashboard.Widgets, err = loadDashboardWidgets(ctx, query, organizationID, dashboard.ID)
	return dashboard, err
}

func loadDashboardWidgets(ctx context.Context, query executionQuerier, organizationID, dashboardID int64) ([]DashboardWidget, error) {
	rows, err := query.Query(ctx, `
		SELECT w.id,w.report_definition_id,w.position,w.width,
			d.id,d.name,d.description,d.source_type,d.visualization_type,COALESCE(d.visualization_contract,''),
			d.columns_json,d.filters_json,d.group_by,d.aggregation_json,d.is_active,d.created_at,d.updated_at
		FROM custom_report_dashboard_widgets w
		JOIN custom_report_definitions d
		  ON d.organization_id=w.organization_id AND d.id=w.report_definition_id
		WHERE w.organization_id=$1 AND w.dashboard_id=$2
		ORDER BY w.position,w.id
	`, organizationID, dashboardID)
	if err != nil {
		return nil, fmt.Errorf("list shared report dashboard widgets: %w", err)
	}
	defer rows.Close()
	widgets := make([]DashboardWidget, 0, MaxDashboardWidgets)
	for rows.Next() {
		var widget DashboardWidget
		var columnsJSON, filtersJSON, aggregationJSON []byte
		if err := rows.Scan(
			&widget.ID, &widget.ReportDefinitionID, &widget.Position, &widget.Width,
			&widget.Definition.ID, &widget.Definition.Name, &widget.Definition.Description,
			&widget.Definition.SourceType, &widget.Definition.VisualizationType, &widget.Definition.VisualizationContract,
			&columnsJSON, &filtersJSON, &widget.Definition.GroupBy, &aggregationJSON,
			&widget.Definition.IsActive, &widget.Definition.CreatedAt, &widget.Definition.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan shared report dashboard widget: %w", err)
		}
		if err := decodeDefinitionJSON(&widget.Definition, columnsJSON, filtersJSON, aggregationJSON); err != nil {
			return nil, err
		}
		widgets = append(widgets, widget)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shared report dashboard widgets: %w", err)
	}
	return widgets, nil
}

func loadExecutableDefinitionForShare(ctx context.Context, tx pgx.Tx, organizationID, definitionID int64) (Definition, Input, error) {
	definition, err := scanDefinition(tx.QueryRow(ctx, definitionSelect+`
		WHERE organization_id=$1 AND id=$2
		FOR SHARE
	`, organizationID, definitionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Definition{}, Input{}, ErrNotFound
		}
		return Definition{}, Input{}, fmt.Errorf("lock shared report dashboard definition: %w", err)
	}
	if !definition.IsActive {
		return Definition{}, Input{}, ErrInactive
	}
	if !isExecutableVisualization(definition) {
		return Definition{}, Input{}, ErrUnsupportedVisualization
	}
	input := normalizeInput(Input{
		Name: definition.Name, Description: definition.Description, SourceType: definition.SourceType,
		VisualizationType: definition.VisualizationType, VisualizationContract: definition.VisualizationContract,
		Columns: definition.Columns, Filters: definition.Filters, GroupBy: definition.GroupBy, Aggregation: definition.Aggregation,
	})
	if err := validateInput(input); err != nil {
		return Definition{}, Input{}, ErrInvalidInput
	}
	return definition, input, nil
}

func dashboardMatchesInput(current Dashboard, input DashboardInput) bool {
	if len(current.Widgets) != len(input.Widgets) {
		return false
	}
	for index, widget := range current.Widgets {
		if widget.Position != index || widget.ReportDefinitionID != input.Widgets[index].ReportDefinitionID || widget.Width != input.Widgets[index].Width {
			return false
		}
	}
	return true
}

func dashboardExecutionError(ctx context.Context, operation string, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrQueryTimeout
	}
	return fmt.Errorf("%s: %w", operation, err)
}
