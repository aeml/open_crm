package leadforms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	"github.com/jackc/pgx/v5"
)

const customFieldMappingPrefix = "custom:"

var customFieldMappingKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

type mappingQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type FieldMappingSnapshot struct {
	FormFieldKey string `json:"formFieldKey"`
	Destination  string `json:"destination"`
	DataType     string `json:"dataType"`
}

type preparedLeadContact struct {
	Contact             contactInput
	Payload             map[string]string
	CustomFieldsJSON    []byte
	MappingSnapshotJSON []byte
}

func prepareLeadContact(ctx context.Context, querier mappingQuerier, organizationID int64, form Form, values map[string]string) (preparedLeadContact, error) {
	contact, payload, err := contactInputFromSubmission(form, values)
	if err != nil {
		return preparedLeadContact{}, err
	}
	customValues, snapshot, err := prepareContactCustomFields(ctx, querier, organizationID, form.Fields, payload)
	if err != nil {
		return preparedLeadContact{}, err
	}
	customFieldsJSON, err := encodeCustomFieldValues(customValues)
	if err != nil {
		return preparedLeadContact{}, err
	}
	mappingSnapshotJSON, err := encodeFieldMappingSnapshot(snapshot)
	if err != nil {
		return preparedLeadContact{}, err
	}
	return preparedLeadContact{
		Contact:             contact,
		Payload:             payload,
		CustomFieldsJSON:    customFieldsJSON,
		MappingSnapshotJSON: mappingSnapshotJSON,
	}, nil
}

func hydrateFormFields(ctx context.Context, querier mappingQuerier, organizationID int64, fields []Field, requireComplete bool) ([]Field, error) {
	if !hasCustomFieldMapping(fields) && !requireComplete {
		return cloneFields(fields), nil
	}
	definitions, err := modulecustomfields.LoadDefinitions(ctx, querier, organizationID, "contact", false)
	if err != nil {
		return nil, fmt.Errorf("load lead form contact custom fields: %w", err)
	}
	return hydrateFormFieldsWithDefinitions(fields, definitions, requireComplete)
}

func hydrateFormList(ctx context.Context, querier mappingQuerier, organizationID int64, forms []Form) error {
	needsDefinitions := false
	for _, form := range forms {
		if hasCustomFieldMapping(form.Fields) {
			needsDefinitions = true
			break
		}
	}
	if !needsDefinitions {
		return nil
	}
	definitions, err := modulecustomfields.LoadDefinitions(ctx, querier, organizationID, "contact", false)
	if err != nil {
		return fmt.Errorf("load lead form contact custom fields: %w", err)
	}
	for index := range forms {
		forms[index].Fields, err = hydrateFormFieldsWithDefinitions(forms[index].Fields, definitions, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func hydrateFormFieldsWithDefinitions(fields []Field, definitions []modulecustomfields.Definition, requireComplete bool) ([]Field, error) {
	byKey := make(map[string]modulecustomfields.Definition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.FieldKey] = definition
	}

	result := cloneFields(fields)
	mapped := make(map[string]bool, len(result))
	for index := range result {
		key, custom := customFieldKey(result[index].MapTo)
		if !custom {
			continue
		}
		definition, ok := byKey[key]
		if !ok {
			if requireComplete {
				return nil, fmt.Errorf("%w: contact custom field %q is missing or archived", ErrInvalidMapping, key)
			}
			continue
		}
		mapped[key] = true
		result[index].FieldType = publicFieldType(definition, result[index].FieldType)
		result[index].Options = append([]string(nil), definition.Options...)
		if definition.Required {
			result[index].Required = true
		}
	}
	if requireComplete {
		for _, definition := range definitions {
			if definition.Required && !mapped[definition.FieldKey] {
				return nil, fmt.Errorf("%w: required contact custom field %q is not mapped", ErrInvalidMapping, definition.FieldKey)
			}
		}
	}
	return result, nil
}

func prepareContactCustomFields(ctx context.Context, querier mappingQuerier, organizationID int64, fields []Field, values map[string]string) (modulecustomfields.Values, []FieldMappingSnapshot, error) {
	definitions, err := modulecustomfields.LoadDefinitions(ctx, querier, organizationID, "contact", false)
	if err != nil {
		return nil, nil, fmt.Errorf("load lead submission custom fields: %w", err)
	}
	byKey := make(map[string]modulecustomfields.Definition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.FieldKey] = definition
	}

	submitted := modulecustomfields.Values{}
	snapshot := make([]FieldMappingSnapshot, 0, len(fields))
	mapped := make(map[string]bool, len(fields))
	for _, field := range fields {
		key, custom := customFieldKey(field.MapTo)
		if !custom {
			if field.MapTo != "" {
				snapshot = append(snapshot, FieldMappingSnapshot{FormFieldKey: field.Key, Destination: field.MapTo, DataType: coreMappingDataType(field.MapTo)})
			}
			continue
		}
		definition, ok := byKey[key]
		if !ok {
			return nil, nil, fmt.Errorf("%w: mapped contact custom field %q is unavailable", ErrFormUnavailable, key)
		}
		mapped[key] = true
		raw, err := modulecustomfields.RawValueFromString(definition, values[field.Key])
		if err != nil {
			if errors.Is(err, modulecustomfields.ErrInvalidInput) {
				return nil, nil, ErrInvalidSubmission
			}
			return nil, nil, err
		}
		submitted[key] = raw
		snapshot = append(snapshot, FieldMappingSnapshot{FormFieldKey: field.Key, Destination: field.MapTo, DataType: definition.DataType})
	}
	for _, definition := range definitions {
		if definition.Required && !mapped[definition.FieldKey] {
			return nil, nil, fmt.Errorf("%w: required contact custom field %q is not mapped", ErrFormUnavailable, definition.FieldKey)
		}
	}
	normalized, err := modulecustomfields.NormalizeValues(ctx, querier, organizationID, "contact", submitted, nil)
	if err != nil {
		if errors.Is(err, modulecustomfields.ErrInvalidInput) {
			return nil, nil, ErrInvalidSubmission
		}
		return nil, nil, err
	}
	return normalized, snapshot, nil
}

func encodeCustomFieldValues(values modulecustomfields.Values) ([]byte, error) {
	encoded, err := modulecustomfields.EncodeValues(values)
	if err != nil {
		return nil, fmt.Errorf("encode lead contact custom fields: %w", err)
	}
	return encoded, nil
}

func encodeFieldMappingSnapshot(snapshot []FieldMappingSnapshot) ([]byte, error) {
	if snapshot == nil {
		snapshot = []FieldMappingSnapshot{}
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode lead field mapping snapshot: %w", err)
	}
	return encoded, nil
}

func hasCustomFieldMapping(fields []Field) bool {
	for _, field := range fields {
		if _, ok := customFieldKey(field.MapTo); ok {
			return true
		}
	}
	return false
}

func customFieldKey(mapping string) (string, bool) {
	if !strings.HasPrefix(mapping, customFieldMappingPrefix) {
		return "", false
	}
	key := strings.TrimPrefix(mapping, customFieldMappingPrefix)
	return key, customFieldMappingKeyPattern.MatchString(key)
}

func publicFieldType(definition modulecustomfields.Definition, requested string) string {
	switch definition.DataType {
	case "text":
		if requested == "textarea" {
			return "textarea"
		}
		return "text"
	case "number", "date", "select":
		return definition.DataType
	case "boolean":
		return "boolean"
	default:
		return "text"
	}
}

func coreMappingDataType(mapping string) string {
	if mapping == "email" {
		return "email"
	}
	if mapping == "phone" {
		return "telephone"
	}
	return "text"
}

func cloneFields(fields []Field) []Field {
	result := make([]Field, len(fields))
	for index := range fields {
		result[index] = fields[index]
		result[index].Options = append([]string(nil), fields[index].Options...)
	}
	return result
}
