package customreports

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultExecutionPageSize = 50
	maxExecutionPage         = 100
	maxExecutionPageSize     = 100
	executionTimeout         = 5 * time.Second
)

var decimalFilterPattern = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)$`)

type ExecuteQuery struct {
	Page     int
	PageSize int
}

type ResultColumn struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	DataType string `json:"dataType"`
}

type ResultRow struct {
	Values map[string]*string `json:"values"`
}

type Execution struct {
	DefinitionID   int64          `json:"definitionId"`
	DefinitionName string         `json:"definitionName"`
	SourceType     string         `json:"sourceType"`
	Columns        []ResultColumn `json:"columns"`
	Rows           []ResultRow    `json:"rows"`
	Page           int            `json:"page"`
	PageSize       int            `json:"pageSize"`
	HasMore        bool           `json:"hasMore"`
	GeneratedAt    time.Time      `json:"generatedAt"`
}

type reportFieldSpec struct {
	Expression string
	Label      string
	DataType   string
}

type reportSourceSpec struct {
	From       string
	TenantExpr string
	ArchiveSQL string
	OrderSQL   string
	Fields     map[string]reportFieldSpec
}

func (s *Service) Execute(ctx context.Context, organizationID, definitionID int64, query ExecuteQuery) (Execution, error) {
	if s == nil || s.pool == nil {
		return Execution{}, fmt.Errorf("custom reports service not configured")
	}
	query, err := normalizeExecuteQuery(query)
	if err != nil {
		return Execution{}, err
	}
	definition, err := scanDefinition(s.pool.QueryRow(ctx, definitionSelect+`
		WHERE organization_id = $1 AND id = $2
	`, organizationID, definitionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Execution{}, ErrNotFound
		}
		return Execution{}, fmt.Errorf("load custom report definition: %w", err)
	}
	if !definition.IsActive {
		return Execution{}, ErrInactive
	}
	if definition.VisualizationType != "table" {
		return Execution{}, ErrUnsupportedVisualization
	}
	definitionInput := normalizeInput(Input{
		Name:              definition.Name,
		Description:       definition.Description,
		SourceType:        definition.SourceType,
		VisualizationType: definition.VisualizationType,
		Columns:           definition.Columns,
		Filters:           definition.Filters,
		GroupBy:           definition.GroupBy,
		Aggregation:       definition.Aggregation,
	})
	if err := validateInput(definitionInput); err != nil {
		return Execution{}, ErrInvalidInput
	}

	statement, args, columns, err := buildExecutionStatement(organizationID, definitionInput, query)
	if err != nil {
		return Execution{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, executionTimeout)
	defer cancel()
	rows, err := s.pool.Query(queryCtx, statement, args...)
	if err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return Execution{}, ErrQueryTimeout
		}
		return Execution{}, fmt.Errorf("execute custom report: %w", err)
	}
	defer rows.Close()

	resultRows := make([]ResultRow, 0, query.PageSize+1)
	for rows.Next() {
		cells := make([]pgtype.Text, len(columns))
		destinations := make([]any, len(columns))
		for index := range cells {
			destinations[index] = &cells[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return Execution{}, fmt.Errorf("scan custom report result: %w", err)
		}
		values := make(map[string]*string, len(columns))
		for index, column := range columns {
			if !cells[index].Valid {
				values[column.Key] = nil
				continue
			}
			value := cells[index].String
			values[column.Key] = &value
		}
		resultRows = append(resultRows, ResultRow{Values: values})
	}
	if err := rows.Err(); err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return Execution{}, ErrQueryTimeout
		}
		return Execution{}, fmt.Errorf("iterate custom report results: %w", err)
	}
	hasMore := len(resultRows) > query.PageSize
	if hasMore {
		resultRows = resultRows[:query.PageSize]
	}
	return Execution{
		DefinitionID:   definition.ID,
		DefinitionName: definition.Name,
		SourceType:     definition.SourceType,
		Columns:        columns,
		Rows:           resultRows,
		Page:           query.Page,
		PageSize:       query.PageSize,
		HasMore:        hasMore,
		GeneratedAt:    time.Now().UTC(),
	}, nil
}

func normalizeExecuteQuery(query ExecuteQuery) (ExecuteQuery, error) {
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = defaultExecutionPageSize
	}
	if query.Page < 1 || query.Page > maxExecutionPage || query.PageSize < 1 || query.PageSize > maxExecutionPageSize {
		return ExecuteQuery{}, ErrInvalidQuery
	}
	return query, nil
}

func buildExecutionStatement(organizationID int64, input Input, query ExecuteQuery) (string, []any, []ResultColumn, error) {
	source, ok := reportExecutionSources[input.SourceType]
	if !ok {
		return "", nil, nil, ErrInvalidInput
	}
	args := []any{organizationID}
	where := []string{source.TenantExpr + " = $1", source.ArchiveSQL}
	for _, filter := range input.Filters {
		clause, value, hasValue, err := executionFilterClause(source, filter, len(args)+1)
		if err != nil {
			return "", nil, nil, err
		}
		where = append(where, clause)
		if hasValue {
			args = append(args, value)
		}
	}

	selects, columns, orderSQL, err := executionSelect(source, input)
	if err != nil {
		return "", nil, nil, err
	}
	limitPosition := len(args) + 1
	offsetPosition := len(args) + 2
	args = append(args, query.PageSize+1, (query.Page-1)*query.PageSize)
	statement := `SELECT ` + strings.Join(selects, ", ") + `
		FROM ` + source.From + `
		WHERE ` + strings.Join(where, " AND ")
	if input.Aggregation.Function != "none" && input.GroupBy != "" {
		statement += `
		GROUP BY ` + source.Fields[input.GroupBy].Expression
	}
	if orderSQL != "" {
		statement += `
		ORDER BY ` + orderSQL
	}
	statement += fmt.Sprintf("\n\t\tLIMIT $%d OFFSET $%d", limitPosition, offsetPosition)
	return statement, args, columns, nil
}

func executionSelect(source reportSourceSpec, input Input) ([]string, []ResultColumn, string, error) {
	if input.Aggregation.Function == "none" {
		selects := make([]string, 0, len(input.Columns))
		columns := make([]ResultColumn, 0, len(input.Columns))
		for _, key := range input.Columns {
			field, ok := source.Fields[key]
			if !ok {
				return nil, nil, "", ErrInvalidInput
			}
			selects = append(selects, displayExpression(field))
			columns = append(columns, ResultColumn{Key: key, Label: field.Label, DataType: field.DataType})
		}
		return selects, columns, source.OrderSQL, nil
	}

	aggregateExpression, aggregateColumn, err := executionAggregation(source, input.Aggregation)
	if err != nil {
		return nil, nil, "", err
	}
	if input.GroupBy == "" {
		return []string{aggregateExpression + "::text"}, []ResultColumn{aggregateColumn}, "", nil
	}
	groupField, ok := source.Fields[input.GroupBy]
	if !ok {
		return nil, nil, "", ErrInvalidInput
	}
	selects := []string{displayExpression(groupField), aggregateExpression + "::text"}
	columns := []ResultColumn{{Key: input.GroupBy, Label: groupField.Label, DataType: groupField.DataType}, aggregateColumn}
	return selects, columns, groupField.Expression + ` ASC NULLS LAST`, nil
}

func executionAggregation(source reportSourceSpec, aggregation Aggregation) (string, ResultColumn, error) {
	if aggregation.Function == "count" && aggregation.Field == "" {
		return "COUNT(*)", ResultColumn{Key: "recordCount", Label: "Record count", DataType: "integer"}, nil
	}
	field, ok := source.Fields[aggregation.Field]
	if !ok {
		return "", ResultColumn{}, ErrInvalidInput
	}
	function := strings.ToUpper(aggregation.Function)
	key := aggregation.Function + upperFirst(aggregation.Field)
	label := strings.ToUpper(aggregation.Function) + " " + field.Label
	dataType := field.DataType
	if aggregation.Function == "count" {
		function = "COUNT"
		dataType = "integer"
	}
	return function + "(" + field.Expression + ")", ResultColumn{Key: key, Label: label, DataType: dataType}, nil
}

func displayExpression(field reportFieldSpec) string {
	switch field.DataType {
	case "date":
		return "TO_CHAR(" + field.Expression + ", 'YYYY-MM-DD')"
	case "timestamp":
		return "TO_CHAR(" + field.Expression + ` AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`
	default:
		return "(" + field.Expression + ")::text"
	}
}

func executionFilterClause(source reportSourceSpec, filter Filter, argumentPosition int) (string, any, bool, error) {
	field, ok := source.Fields[filter.Field]
	if !ok || validateFilterValueByType(field.DataType, filter) != nil {
		return "", nil, false, ErrInvalidInput
	}
	if filter.Operator == "exists" {
		if field.DataType == "text" {
			return "NULLIF(BTRIM(COALESCE(" + field.Expression + ", '')), '') IS NOT NULL", nil, false, nil
		}
		return field.Expression + " IS NOT NULL", nil, false, nil
	}
	parameter := fmt.Sprintf("$%d", argumentPosition)
	switch field.DataType {
	case "text":
		switch filter.Operator {
		case "equals":
			return "LOWER(COALESCE(" + field.Expression + ", '')) = LOWER(" + parameter + ")", filter.Value, true, nil
		case "notEquals":
			return "LOWER(COALESCE(" + field.Expression + ", '')) <> LOWER(" + parameter + ")", filter.Value, true, nil
		case "contains":
			return "COALESCE(" + field.Expression + ", '') ILIKE " + parameter + ` ESCAPE E'\\'`, escapeLike(filter.Value), true, nil
		}
	case "integer":
		return field.Expression + comparisonOperator(filter.Operator) + parameter + "::bigint", filter.Value, true, nil
	case "numeric":
		return field.Expression + comparisonOperator(filter.Operator) + parameter + "::numeric", filter.Value, true, nil
	case "date":
		return field.Expression + comparisonOperator(filter.Operator) + parameter + "::date", filter.Value, true, nil
	case "timestamp":
		return field.Expression + comparisonOperator(filter.Operator) + parameter + "::timestamptz", filter.Value, true, nil
	}
	return "", nil, false, ErrInvalidInput
}

func comparisonOperator(operator string) string {
	switch operator {
	case "equals":
		return " = "
	case "notEquals":
		return " <> "
	case "greaterThan", "after":
		return " > "
	case "lessThan", "before":
		return " < "
	default:
		return ""
	}
}

func validateFilterValue(sourceType string, filter Filter) error {
	source, ok := reportExecutionSources[sourceType]
	if !ok {
		return ErrInvalidInput
	}
	field, ok := source.Fields[filter.Field]
	if !ok {
		return ErrInvalidInput
	}
	return validateFilterValueByType(field.DataType, filter)
}

func validateFilterValueByType(dataType string, filter Filter) error {
	if filter.Operator == "exists" {
		return nil
	}
	if len(filter.Value) > 256 {
		return ErrInvalidInput
	}
	switch dataType {
	case "text":
		if filter.Operator != "equals" && filter.Operator != "notEquals" && filter.Operator != "contains" {
			return ErrInvalidInput
		}
	case "integer":
		if filter.Operator != "equals" && filter.Operator != "notEquals" && filter.Operator != "greaterThan" && filter.Operator != "lessThan" {
			return ErrInvalidInput
		}
		if _, err := strconv.ParseInt(filter.Value, 10, 64); err != nil {
			return ErrInvalidInput
		}
	case "numeric":
		if filter.Operator != "equals" && filter.Operator != "notEquals" && filter.Operator != "greaterThan" && filter.Operator != "lessThan" {
			return ErrInvalidInput
		}
		if !decimalFilterPattern.MatchString(filter.Value) {
			return ErrInvalidInput
		}
	case "date":
		if filter.Operator != "equals" && filter.Operator != "notEquals" && filter.Operator != "before" && filter.Operator != "after" {
			return ErrInvalidInput
		}
		if _, err := time.Parse("2006-01-02", filter.Value); err != nil {
			return ErrInvalidInput
		}
	case "timestamp":
		if filter.Operator != "equals" && filter.Operator != "notEquals" && filter.Operator != "before" && filter.Operator != "after" {
			return ErrInvalidInput
		}
		if _, err := time.Parse(time.RFC3339, filter.Value); err != nil {
			if _, dateErr := time.Parse("2006-01-02", filter.Value); dateErr != nil {
				return ErrInvalidInput
			}
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return "%" + value + "%"
}

func upperFirst(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

var reportExecutionSources = map[string]reportSourceSpec{
	"contacts": {
		From: "contacts c", TenantExpr: "c.organization_id", ArchiveSQL: "c.archived_at IS NULL", OrderSQL: "c.updated_at DESC, c.id DESC",
		Fields: map[string]reportFieldSpec{
			"id": {Expression: "c.id", Label: "Contact ID", DataType: "integer"}, "firstName": {Expression: "c.first_name", Label: "First name", DataType: "text"},
			"lastName": {Expression: "c.last_name", Label: "Last name", DataType: "text"}, "email": {Expression: "c.email", Label: "Email", DataType: "text"},
			"phone": {Expression: "c.phone", Label: "Phone", DataType: "text"}, "status": {Expression: "c.status", Label: "Status", DataType: "text"},
			"ownerUserId": {Expression: "c.owner_user_id", Label: "Owner user ID", DataType: "integer"}, "leadSource": {Expression: "c.lead_source", Label: "Lead source", DataType: "text"},
			"leadScore": {Expression: "c.lead_score", Label: "Lead score", DataType: "integer"}, "createdAt": {Expression: "c.created_at", Label: "Created at", DataType: "timestamp"},
			"updatedAt": {Expression: "c.updated_at", Label: "Updated at", DataType: "timestamp"},
		},
	},
	"companies": {
		From: "companies co", TenantExpr: "co.organization_id", ArchiveSQL: "co.archived_at IS NULL", OrderSQL: "co.updated_at DESC, co.id DESC",
		Fields: map[string]reportFieldSpec{
			"id": {Expression: "co.id", Label: "Company ID", DataType: "integer"}, "name": {Expression: "co.name", Label: "Name", DataType: "text"},
			"clientType": {Expression: "co.client_type", Label: "Client type", DataType: "text"}, "industry": {Expression: "co.industry", Label: "Industry", DataType: "text"},
			"status": {Expression: "co.status", Label: "Status", DataType: "text"}, "city": {Expression: "co.city", Label: "City", DataType: "text"},
			"state": {Expression: "co.state", Label: "State", DataType: "text"}, "country": {Expression: "co.country", Label: "Country", DataType: "text"},
			"createdAt": {Expression: "co.created_at", Label: "Created at", DataType: "timestamp"}, "updatedAt": {Expression: "co.updated_at", Label: "Updated at", DataType: "timestamp"},
		},
	},
	"deals": {
		From: "deals d JOIN deal_stages ds ON ds.organization_id = d.organization_id AND ds.id = d.stage_id", TenantExpr: "d.organization_id", ArchiveSQL: "d.archived_at IS NULL", OrderSQL: "d.updated_at DESC, d.id DESC",
		Fields: map[string]reportFieldSpec{
			"id": {Expression: "d.id", Label: "Deal ID", DataType: "integer"}, "name": {Expression: "d.name", Label: "Deal name", DataType: "text"},
			"stageName": {Expression: "ds.name", Label: "Stage", DataType: "text"}, "status": {Expression: "d.status", Label: "Status", DataType: "text"},
			"valueAmount": {Expression: "d.value_amount", Label: "Value amount", DataType: "numeric"}, "valueCurrency": {Expression: "d.value_currency", Label: "Currency", DataType: "text"},
			"ownerUserId": {Expression: "d.owner_user_id", Label: "Owner user ID", DataType: "integer"}, "expectedCloseDate": {Expression: "d.expected_close_date", Label: "Expected close", DataType: "date"},
			"createdAt": {Expression: "d.created_at", Label: "Created at", DataType: "timestamp"}, "updatedAt": {Expression: "d.updated_at", Label: "Updated at", DataType: "timestamp"},
		},
	},
	"tasks": {
		From: "tasks t", TenantExpr: "t.organization_id", ArchiveSQL: "t.archived_at IS NULL", OrderSQL: "t.updated_at DESC, t.id DESC",
		Fields: map[string]reportFieldSpec{
			"id": {Expression: "t.id", Label: "Task ID", DataType: "integer"}, "title": {Expression: "t.title", Label: "Title", DataType: "text"},
			"status": {Expression: "t.status", Label: "Status", DataType: "text"}, "entityType": {Expression: "t.entity_type", Label: "Related record type", DataType: "text"},
			"assignedToUserId": {Expression: "t.assigned_to_user_id", Label: "Assignee user ID", DataType: "integer"}, "dueAt": {Expression: "t.due_at", Label: "Due at", DataType: "timestamp"},
			"completedAt": {Expression: "t.completed_at", Label: "Completed at", DataType: "timestamp"}, "createdAt": {Expression: "t.created_at", Label: "Created at", DataType: "timestamp"},
			"updatedAt": {Expression: "t.updated_at", Label: "Updated at", DataType: "timestamp"},
		},
	},
}
