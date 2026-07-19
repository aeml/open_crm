package customfields

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func ValidateFilter(ctx context.Context, querier definitionQuerier, organizationID int64, entityType string, filter Filter) (NormalizedFilter, error) {
	filter.FieldKey = normalizeFieldKey(filter.FieldKey)
	filter.Operator = strings.ToLower(strings.TrimSpace(filter.Operator))
	filter.Value = strings.TrimSpace(filter.Value)
	if filter.FieldKey == "" {
		return NormalizedFilter{}, nil
	}
	definitions, err := LoadDefinitions(ctx, querier, organizationID, entityType, false)
	if err != nil {
		return NormalizedFilter{}, err
	}
	var definition Definition
	for _, candidate := range definitions {
		if candidate.FieldKey == filter.FieldKey {
			definition = candidate
			break
		}
	}
	if definition.ID == 0 {
		return NormalizedFilter{}, fmt.Errorf("%w: unknown custom field filter", ErrInvalidInput)
	}
	allowed := map[string][]string{
		"text": {"eq", "contains"}, "select": {"eq"}, "number": {"eq", "gte", "lte"},
		"date": {"eq", "before", "after"}, "boolean": {"eq"},
	}
	if !contains(allowed[definition.DataType], filter.Operator) {
		return NormalizedFilter{}, fmt.Errorf("%w: unsupported operator for %s", ErrInvalidInput, definition.Label)
	}
	raw, err := RawValueFromString(definition, filter.Value)
	if err != nil || string(raw) == "null" {
		if err != nil {
			return NormalizedFilter{}, err
		}
		return NormalizedFilter{}, fmt.Errorf("%w: a custom field filter value is required", ErrInvalidInput)
	}
	result := NormalizedFilter{Definition: definition, Operator: filter.Operator, Value: FormatValue(definition, raw)}
	if definition.DataType == "boolean" {
		result.Boolean, _ = strconv.ParseBool(result.Value)
	}
	return result, nil
}

func AppendFilterSQL(alias string, args []any, filter NormalizedFilter) (string, []any) {
	if filter.Definition.ID == 0 {
		return "", args
	}
	keyPosition := len(args) + 1
	args = append(args, filter.Definition.FieldKey)
	valuePosition := len(args) + 1
	operator := "="
	switch filter.Operator {
	case "gte":
		operator = ">="
	case "lte":
		operator = "<="
	case "after":
		operator = ">"
	case "before":
		operator = "<"
	}
	column := fmt.Sprintf("%s.custom_fields", alias)
	switch filter.Definition.DataType {
	case "text":
		value := filter.Value
		if filter.Operator == "contains" {
			value = "%" + value + "%"
			args = append(args, value)
			return fmt.Sprintf(" AND %s ->> $%d ILIKE $%d", column, keyPosition, valuePosition), args
		}
		args = append(args, value)
		return fmt.Sprintf(" AND %s ->> $%d = $%d", column, keyPosition, valuePosition), args
	case "select", "date":
		args = append(args, filter.Value)
		return fmt.Sprintf(" AND %s ->> $%d %s $%d", column, keyPosition, operator, valuePosition), args
	case "number":
		args = append(args, filter.Value)
		return fmt.Sprintf(" AND jsonb_typeof(%s -> $%d) = 'number' AND (%s ->> $%d)::numeric %s $%d::numeric", column, keyPosition, column, keyPosition, operator, valuePosition), args
	case "boolean":
		args = append(args, filter.Boolean)
		return fmt.Sprintf(" AND %s -> $%d = to_jsonb($%d::boolean)", column, keyPosition, valuePosition), args
	default:
		return "", args
	}
}
