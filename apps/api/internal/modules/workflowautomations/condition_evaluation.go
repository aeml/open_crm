package workflowautomations

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func validateCondition(target string, condition Condition) error {
	if !isAllowedConditionField(target, condition.Field) || !isAllowedConditionOperator(condition.Operator) {
		return ErrInvalidInput
	}
	if condition.Operator != "exists" && condition.Value == "" {
		return ErrInvalidInput
	}
	if isNumericConditionOperator(condition.Operator) {
		if _, err := strconv.ParseFloat(condition.Value, 64); err != nil {
			return ErrInvalidInput
		}
	}
	return nil
}

func isAllowedConditionOperator(operator string) bool {
	switch operator {
	case "equals", "notEquals", "contains", "exists", "greaterThan", "lessThan":
		return true
	default:
		return false
	}
}

func isNumericConditionOperator(operator string) bool {
	switch operator {
	case "greaterThan", "lessThan":
		return true
	default:
		return false
	}
}

func isAllowedConditionField(target, field string) bool {
	switch target {
	case "contact":
		switch field {
		case "firstName", "lastName", "email", "phone", "status", "ownerUserId", "leadSource", "utmSource", "utmMedium", "utmCampaign", "jobTitle", "city", "state", "country", "leadScore", "leadGrade":
			return true
		}
	case "company":
		switch field {
		case "name", "clientType", "industry", "phone", "website", "status", "city", "state", "country":
			return true
		}
	case "deal":
		switch field {
		case "name", "stageId", "stageName", "status", "valueAmount", "valueCurrency", "ownerUserId", "companyId", "primaryContactId", "expectedCloseDate":
			return true
		}
	case "task":
		switch field {
		case "title", "status", "entityType", "entityId", "assignedToUserId", "dueAt":
			return true
		}
	case "lead_form":
		switch field {
		case "formId", "formPublicId", "sourceUrl", "leadSource", "utmSource", "utmMedium", "utmCampaign":
			return true
		}
	case "email_message":
		switch field {
		case "fromEmail", "toEmail", "subject", "direction", "status":
			return true
		}
	case "webhook":
		switch field {
		case "event", "source", "payloadType":
			return true
		}
	}
	return false
}

func EvaluateConditions(logic string, conditions []Condition, fields map[string]any) bool {
	logic = strings.TrimSpace(strings.ToLower(logic))
	if logic == "" {
		logic = "all"
	}
	if len(conditions) == 0 {
		return true
	}

	if logic == "any" {
		for _, condition := range conditions {
			if conditionMatches(condition, fields) {
				return true
			}
		}
		return false
	}

	for _, condition := range conditions {
		if !conditionMatches(condition, fields) {
			return false
		}
	}
	return true
}

func conditionMatches(condition Condition, fields map[string]any) bool {
	actual, ok := fields[condition.Field]
	if condition.Operator == "exists" {
		return ok && valueExists(actual)
	}
	if !ok {
		return false
	}

	actualText := valueToString(actual)
	switch condition.Operator {
	case "equals":
		return strings.EqualFold(actualText, condition.Value)
	case "notEquals":
		return !strings.EqualFold(actualText, condition.Value)
	case "contains":
		return strings.Contains(strings.ToLower(actualText), strings.ToLower(condition.Value))
	case "greaterThan", "lessThan":
		actualNumber, actualOK := valueToFloat(actual)
		expectedNumber, expectedErr := strconv.ParseFloat(condition.Value, 64)
		if !actualOK || expectedErr != nil {
			return false
		}
		if condition.Operator == "greaterThan" {
			return actualNumber > expectedNumber
		}
		return actualNumber < expectedNumber
	default:
		return false
	}
}

func valueExists(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func valueToString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func valueToFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		parsed, err := strconv.ParseFloat(fmt.Sprint(value), 64)
		return parsed, err == nil
	}
}
