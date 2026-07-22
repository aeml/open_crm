// Package customreports stores self-service report builder definitions.
package customreports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName            = errors.New("custom report definition name already exists")
	ErrForbidden                = errors.New("custom report definition access forbidden")
	ErrInactive                 = errors.New("custom report definition is inactive")
	ErrInvalidInput             = errors.New("invalid custom report definition")
	ErrInvalidQuery             = errors.New("invalid custom report query")
	ErrNotFound                 = errors.New("custom report definition not found")
	ErrQueryTimeout             = errors.New("custom report query timed out")
	ErrTooManyRows              = errors.New("custom report export exceeds the 10,000-row synchronous limit; narrow the saved filters")
	ErrUnsupportedVisualization = errors.New("custom report visualization is not executable")
)

type Definition struct {
	ID                    int64       `json:"id"`
	Name                  string      `json:"name"`
	Description           string      `json:"description"`
	SourceType            string      `json:"sourceType"`
	VisualizationType     string      `json:"visualizationType"`
	VisualizationContract string      `json:"visualizationContract"`
	Columns               []string    `json:"columns"`
	Filters               []Filter    `json:"filters"`
	GroupBy               string      `json:"groupBy"`
	Aggregation           Aggregation `json:"aggregation"`
	IsActive              bool        `json:"isActive"`
	CreatedAt             time.Time   `json:"createdAt"`
	UpdatedAt             time.Time   `json:"updatedAt"`
}

type Filter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type Aggregation struct {
	Function string `json:"function"`
	Field    string `json:"field"`
}

type Input struct {
	Name                  string      `json:"name"`
	Description           string      `json:"description"`
	SourceType            string      `json:"sourceType"`
	VisualizationType     string      `json:"visualizationType"`
	VisualizationContract string      `json:"visualizationContract"`
	Columns               []string    `json:"columns"`
	Filters               []Filter    `json:"filters"`
	GroupBy               string      `json:"groupBy"`
	Aggregation           Aggregation `json:"aggregation"`
	IsActive              *bool       `json:"isActive"`
}

type ListQuery struct {
	Page     int
	PageSize int
}

type ListPage struct {
	Definitions []Definition `json:"definitions"`
	Page        int          `json:"page"`
	PageSize    int          `json:"pageSize"`
	Total       int          `json:"total"`
}

type Service struct {
	pool *pgxpool.Pool
}

const DefaultDefinitionListPageSize = 50

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (ListPage, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return ListPage{}, fmt.Errorf("custom reports service not configured")
	}
	page, err := platformpagination.Normalize(query.Page, query.PageSize, DefaultDefinitionListPageSize)
	if err != nil {
		return ListPage{}, ErrInvalidInput
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ListPage{}, fmt.Errorf("begin custom report definition list: %w", err)
	}
	defer tx.Rollback(ctx)

	result := ListPage{Definitions: []Definition{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM custom_report_definitions WHERE organization_id=$1`, organizationID).Scan(&result.Total); err != nil {
		return ListPage{}, fmt.Errorf("count custom report definitions: %w", err)
	}

	rows, err := tx.Query(ctx, definitionSelect+`
		WHERE organization_id = $1
		ORDER BY is_active DESC, updated_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, organizationID, page.Size, page.Offset)
	if err != nil {
		return ListPage{}, fmt.Errorf("list custom report definitions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		definition, err := scanDefinition(rows)
		if err != nil {
			return ListPage{}, err
		}
		result.Definitions = append(result.Definitions, definition)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, fmt.Errorf("iterate custom report definitions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ListPage{}, fmt.Errorf("commit custom report definition list: %w", err)
	}
	return result, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDefinition(row rowScanner) (Definition, error) {
	var definition Definition
	var columnsJSON []byte
	var filtersJSON []byte
	var aggregationJSON []byte
	if err := row.Scan(&definition.ID, &definition.Name, &definition.Description, &definition.SourceType, &definition.VisualizationType, &definition.VisualizationContract, &columnsJSON, &filtersJSON, &definition.GroupBy, &aggregationJSON, &definition.IsActive, &definition.CreatedAt, &definition.UpdatedAt); err != nil {
		return Definition{}, err
	}
	if len(columnsJSON) > 0 {
		if err := json.Unmarshal(columnsJSON, &definition.Columns); err != nil {
			return Definition{}, fmt.Errorf("decode custom report columns: %w", err)
		}
	}
	if len(filtersJSON) > 0 {
		if err := json.Unmarshal(filtersJSON, &definition.Filters); err != nil {
			return Definition{}, fmt.Errorf("decode custom report filters: %w", err)
		}
	}
	if len(aggregationJSON) > 0 {
		if err := json.Unmarshal(aggregationJSON, &definition.Aggregation); err != nil {
			return Definition{}, fmt.Errorf("decode custom report aggregation: %w", err)
		}
	}
	if definition.Columns == nil {
		definition.Columns = []string{}
	}
	if definition.Filters == nil {
		definition.Filters = []Filter{}
	}
	definition.Aggregation = normalizeAggregation(definition.Aggregation)
	return definition, nil
}

func encodeDefinitionJSON(input Input) ([]byte, []byte, []byte, error) {
	columnsJSON, err := json.Marshal(input.Columns)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode custom report columns: %w", err)
	}
	filtersJSON, err := json.Marshal(input.Filters)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode custom report filters: %w", err)
	}
	aggregationJSON, err := json.Marshal(input.Aggregation)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode custom report aggregation: %w", err)
	}
	return columnsJSON, filtersJSON, aggregationJSON, nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.SourceType = strings.ToLower(strings.TrimSpace(input.SourceType))
	input.VisualizationType = normalizeVisualizationType(input.VisualizationType)
	input.VisualizationContract = strings.ToLower(strings.TrimSpace(input.VisualizationContract))
	input.Columns = normalizeColumns(input.Columns)
	input.Filters = normalizeFilters(input.Filters)
	input.GroupBy = strings.TrimSpace(input.GroupBy)
	input.Aggregation = normalizeAggregation(input.Aggregation)
	return input
}

func normalizeColumns(columns []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(columns))
	for _, column := range columns {
		column = strings.TrimSpace(column)
		if column == "" || seen[column] {
			continue
		}
		seen[column] = true
		normalized = append(normalized, column)
	}
	return normalized
}

func normalizeFilters(filters []Filter) []Filter {
	normalized := make([]Filter, 0, len(filters))
	for _, filter := range filters {
		field := strings.TrimSpace(filter.Field)
		operator := normalizeOperator(filter.Operator)
		value := strings.TrimSpace(filter.Value)
		if field == "" || operator == "" {
			continue
		}
		normalized = append(normalized, Filter{Field: field, Operator: operator, Value: value})
	}
	return normalized
}

func normalizeAggregation(aggregation Aggregation) Aggregation {
	function := strings.ToLower(strings.TrimSpace(aggregation.Function))
	field := strings.TrimSpace(aggregation.Field)
	if function == "" {
		function = "count"
	}
	if function == "average" {
		function = "avg"
	}
	if function == "none" {
		field = ""
	}
	return Aggregation{Function: function, Field: field}
}

func normalizeVisualizationType(visualizationType string) string {
	visualizationType = strings.ToLower(strings.TrimSpace(visualizationType))
	if visualizationType == "" {
		return "table"
	}
	return visualizationType
}

func normalizeOperator(operator string) string {
	operator = strings.TrimSpace(operator)
	switch strings.ToLower(strings.ReplaceAll(operator, "_", "")) {
	case "equals":
		return "equals"
	case "notequals":
		return "notEquals"
	case "contains":
		return "contains"
	case "exists":
		return "exists"
	case "greaterthan":
		return "greaterThan"
	case "lessthan":
		return "lessThan"
	case "before":
		return "before"
	case "after":
		return "after"
	default:
		return operator
	}
}

func validateInput(input Input) error {
	if input.Name == "" || len(input.Name) > 120 || len(input.Description) > 1000 || !isAllowedSource(input.SourceType) || !isAllowedVisualizationType(input.VisualizationType) || len(input.Columns) > 20 || len(input.Filters) > 20 {
		return ErrInvalidInput
	}
	for _, column := range input.Columns {
		if !isAllowedField(input.SourceType, column) {
			return ErrInvalidInput
		}
	}
	for _, filter := range input.Filters {
		if !isAllowedField(input.SourceType, filter.Field) || !isAllowedOperator(filter.Operator) {
			return ErrInvalidInput
		}
		if filter.Operator != "exists" && filter.Value == "" {
			return ErrInvalidInput
		}
		if err := validateFilterValue(input.SourceType, filter); err != nil {
			return err
		}
	}
	if input.GroupBy != "" && !isAllowedField(input.SourceType, input.GroupBy) {
		return ErrInvalidInput
	}
	if input.GroupBy != "" && input.Aggregation.Function == "none" {
		return ErrInvalidInput
	}
	if err := validateAggregation(input.SourceType, input.Aggregation); err != nil {
		return err
	}
	return validateVisualizationInput(input)
}

func validateAggregation(sourceType string, aggregation Aggregation) error {
	switch aggregation.Function {
	case "none":
		if aggregation.Field != "" {
			return ErrInvalidInput
		}
	case "count":
		if aggregation.Field != "" && !isAllowedField(sourceType, aggregation.Field) {
			return ErrInvalidInput
		}
	case "sum", "avg":
		if !isNumericField(sourceType, aggregation.Field) {
			return ErrInvalidInput
		}
	case "min", "max":
		if aggregation.Field == "" || !isAllowedField(sourceType, aggregation.Field) {
			return ErrInvalidInput
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func isAllowedSource(sourceType string) bool {
	_, ok := reportFieldsBySource[sourceType]
	return ok
}

func isAllowedVisualizationType(visualizationType string) bool {
	switch visualizationType {
	case "table", "bar", "line", "funnel", "pie", "kpi":
		return true
	default:
		return false
	}
}

func isAllowedField(sourceType, field string) bool {
	for _, allowed := range reportFieldsBySource[sourceType] {
		if field == allowed {
			return true
		}
	}
	return false
}

func isNumericField(sourceType, field string) bool {
	for _, allowed := range numericReportFieldsBySource[sourceType] {
		if field == allowed {
			return true
		}
	}
	return false
}

func isAllowedOperator(operator string) bool {
	switch operator {
	case "equals", "notEquals", "contains", "exists", "greaterThan", "lessThan", "before", "after":
		return true
	default:
		return false
	}
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrDuplicateName
		case "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save custom report definition: %w", err)
}

const definitionReturningColumns = `id, name, description, source_type, visualization_type, COALESCE(visualization_contract, '') AS visualization_contract, columns_json, filters_json, group_by, aggregation_json, is_active, created_at, updated_at`

const definitionSelect = `
	SELECT ` + definitionReturningColumns + `
	FROM custom_report_definitions
`

var reportFieldsBySource = map[string][]string{
	"contacts":  {"id", "firstName", "lastName", "email", "phone", "status", "ownerUserId", "leadSource", "leadScore", "createdAt", "updatedAt"},
	"companies": {"id", "name", "clientType", "industry", "status", "city", "state", "country", "createdAt", "updatedAt"},
	"deals":     {"id", "name", "stageName", "status", "valueAmount", "valueCurrency", "ownerUserId", "expectedCloseDate", "createdAt", "updatedAt"},
	"tasks":     {"id", "title", "status", "entityType", "assignedToUserId", "dueAt", "completedAt", "createdAt", "updatedAt"},
}

var numericReportFieldsBySource = map[string][]string{
	"contacts": {"leadScore"},
	"deals":    {"valueAmount"},
}
