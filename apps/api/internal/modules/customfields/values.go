package customfields

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const maxValuesBytes = 16 * 1024

type definitionQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func LoadDefinitions(ctx context.Context, querier definitionQuerier, organizationID int64, entityType string, includeArchived bool) ([]Definition, error) {
	entityType = normalizeEntityType(entityType)
	if organizationID <= 0 || entityType == "" {
		return nil, fmt.Errorf("%w: organization and entity type are required", ErrInvalidInput)
	}
	archivedFilter := " AND archived_at IS NULL"
	if includeArchived {
		archivedFilter = ""
	}
	rows, err := querier.Query(ctx, `
		SELECT id,entity_type,field_key,label,data_type,options_json,is_required,show_in_list,position,revision,created_by_user_id,created_at,updated_at,archived_at
		FROM custom_field_definitions
		WHERE organization_id=$1 AND entity_type=$2`+archivedFilter+`
		ORDER BY position,id
	`, organizationID, entityType)
	if err != nil {
		return nil, fmt.Errorf("list custom field definitions: %w", err)
	}
	defer rows.Close()
	definitions := []Definition{}
	for rows.Next() {
		definition, err := scanDefinition(rows)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom field definitions: %w", err)
	}
	return definitions, nil
}

func DecodeValues(raw []byte) (Values, error) {
	values := Values{}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return values, nil
	}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode custom fields: %w", err)
	}
	if values == nil {
		values = Values{}
	}
	return values, nil
}

func EncodeValues(values Values) ([]byte, error) {
	if values == nil {
		values = Values{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode custom fields: %w", err)
	}
	if len(encoded) > maxValuesBytes {
		return nil, fmt.Errorf("%w: custom field values exceed %d bytes", ErrInvalidInput, maxValuesBytes)
	}
	return encoded, nil
}

func NormalizeValues(ctx context.Context, querier definitionQuerier, organizationID int64, entityType string, submitted, existing Values) (Values, error) {
	definitions, err := LoadDefinitions(ctx, querier, organizationID, entityType, false)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.FieldKey] = definition
	}
	result := Values{}
	for key, value := range existing {
		result[key] = append(json.RawMessage(nil), value...)
	}
	for key, value := range submitted {
		definition, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("%w: unknown or archived custom field %q", ErrInvalidInput, key)
		}
		normalized, present, err := normalizeValue(definition, value)
		if err != nil {
			return nil, err
		}
		if present {
			result[key] = normalized
		} else {
			delete(result, key)
		}
	}
	for _, definition := range definitions {
		value, present := result[definition.FieldKey]
		if !present {
			if definition.Required {
				return nil, fmt.Errorf("%w: %s is required", ErrInvalidInput, definition.Label)
			}
			continue
		}
		normalized, present, err := normalizeValue(definition, value)
		if err != nil {
			return nil, err
		}
		if !present {
			delete(result, definition.FieldKey)
			if definition.Required {
				return nil, fmt.Errorf("%w: %s is required", ErrInvalidInput, definition.Label)
			}
		} else {
			result[definition.FieldKey] = normalized
		}
	}
	if _, err := EncodeValues(result); err != nil {
		return nil, err
	}
	return result, nil
}

func RawValueFromString(definition Definition, value string) (json.RawMessage, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return json.RawMessage("null"), nil
	}
	var input any = value
	switch definition.DataType {
	case "number":
		number, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return nil, fmt.Errorf("%w: %s must be a number", ErrInvalidInput, definition.Label)
		}
		input = json.Number(value)
	case "boolean":
		boolean, err := strconv.ParseBool(strings.ToLower(value))
		if err != nil {
			return nil, fmt.Errorf("%w: %s must be true or false", ErrInvalidInput, definition.Label)
		}
		input = boolean
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("%w: %s has an invalid value", ErrInvalidInput, definition.Label)
	}
	normalized, _, err := normalizeValue(definition, encoded)
	return normalized, err
}

func FormatValue(definition Definition, value json.RawMessage) string {
	normalized, present, err := normalizeValue(definition, value)
	if err != nil || !present {
		return ""
	}
	if definition.DataType == "boolean" {
		var boolean bool
		_ = json.Unmarshal(normalized, &boolean)
		return strconv.FormatBool(boolean)
	}
	if definition.DataType == "number" {
		return string(normalized)
	}
	var text string
	_ = json.Unmarshal(normalized, &text)
	return text
}

func normalizeValue(definition Definition, raw json.RawMessage) (json.RawMessage, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, false, nil
	}
	fail := func(message string) (json.RawMessage, bool, error) {
		return nil, false, fmt.Errorf("%w: %s %s", ErrInvalidInput, definition.Label, message)
	}
	switch definition.DataType {
	case "text", "date", "select":
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fail("must be text")
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, false, nil
		}
		if definition.DataType == "text" && len(value) > 500 {
			return fail("must be 500 characters or fewer")
		}
		if definition.DataType == "date" {
			parsed, err := time.Parse("2006-01-02", value)
			if err != nil || parsed.Format("2006-01-02") != value {
				return fail("must be a date in YYYY-MM-DD format")
			}
		}
		if definition.DataType == "select" && !contains(definition.Options, value) {
			return fail("must be one of the configured options")
		}
		encoded, _ := json.Marshal(value)
		return encoded, true, nil
	case "number":
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return fail("must be a number")
		}
		number, ok := value.(json.Number)
		if !ok {
			return fail("must be a number")
		}
		if _, err := strconv.ParseFloat(number.String(), 64); err != nil {
			return fail("must be a finite number")
		}
		return json.RawMessage(number.String()), true, nil
	case "boolean":
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return fail("must be true or false")
		}
		if value {
			return json.RawMessage("true"), true, nil
		}
		return json.RawMessage("false"), true, nil
	default:
		return fail("has an unsupported type")
	}
}

func normalizeEntityType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value != "contact" && value != "company" {
		return ""
	}
	return value
}

var fieldKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

func normalizeFieldKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
