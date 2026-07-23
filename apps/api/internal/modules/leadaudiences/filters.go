package leadaudiences

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Filters = normalizeFilters(input.Filters)
	return input
}

func normalizeFilters(filters map[string]string) map[string]string {
	normalized := make(map[string]string, len(filters))
	for key, value := range filters {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		switch key {
		case "q", "leadSource", "utmSource", "utmMedium", "utmCampaign":
			normalized[key] = value
		case "status", "hasEmail", "hasPhone":
			normalized[key] = strings.ToLower(value)
		default:
			normalized[key] = value
		}
	}
	return normalized
}

func validateInput(input Input) error {
	if input.Name == "" || utf8.RuneCountInString(input.Name) > MaxAudienceNameLength || utf8.RuneCountInString(input.Description) > MaxAudienceDescription {
		return ErrInvalidInput
	}
	return validateFilters(input.Filters)
}

func validateFilters(filters map[string]string) error {
	for key, value := range filters {
		switch key {
		case "q":
			if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > MaxAudienceQueryLength {
				return ErrInvalidInput
			}
		case "leadSource", "utmSource", "utmMedium", "utmCampaign":
			if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > MaxAudienceFilterLength {
				return ErrInvalidInput
			}
		case "status":
			if !isAllowedStatus(value) {
				return ErrInvalidInput
			}
		case "hasEmail", "hasPhone":
			if _, err := strconv.ParseBool(value); err != nil {
				return ErrInvalidInput
			}
		default:
			return ErrInvalidInput
		}
	}
	return nil
}

func isAllowedStatus(status string) bool {
	switch status {
	case "lead", "prospect", "customer":
		return true
	default:
		return false
	}
}

func buildMemberFilter(organizationID int64, filters map[string]string) (string, []any, error) {
	filters = normalizeFilters(filters)
	if err := validateFilters(filters); err != nil {
		return "", nil, err
	}
	args := []any{organizationID}
	clauses := []string{"c.organization_id = $1", "c.archived_at IS NULL"}
	if value := filters["q"]; value != "" {
		args = append(args, "%"+value+"%")
		arg := len(args)
		clauses = append(clauses, fmt.Sprintf(`(
			c.first_name ILIKE $%[1]d OR
			c.last_name ILIKE $%[1]d OR
			(c.first_name || ' ' || c.last_name) ILIKE $%[1]d OR
			COALESCE(c.email, '') ILIKE $%[1]d OR
			COALESCE(c.phone, '') ILIKE $%[1]d OR
			COALESCE(c.job_title, '') ILIKE $%[1]d OR
			COALESCE(c.lead_source, '') ILIKE $%[1]d OR
			COALESCE(c.utm_source, '') ILIKE $%[1]d OR
			COALESCE(c.utm_medium, '') ILIKE $%[1]d OR
			COALESCE(c.utm_campaign, '') ILIKE $%[1]d
		)`, arg))
	}
	if value := filters["status"]; value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("COALESCE(c.status, '') = $%d", len(args)))
	}
	for key, column := range map[string]string{
		"leadSource":  "c.lead_source",
		"utmSource":   "c.utm_source",
		"utmMedium":   "c.utm_medium",
		"utmCampaign": "c.utm_campaign",
	} {
		if value := filters[key]; value != "" {
			args = append(args, value)
			clauses = append(clauses, fmt.Sprintf("lower(COALESCE(%s, '')) = lower($%d)", column, len(args)))
		}
	}
	for key, column := range map[string]string{
		"hasEmail": "c.email",
		"hasPhone": "c.phone",
	} {
		if value := filters[key]; value != "" {
			wantValue, err := strconv.ParseBool(value)
			if err != nil {
				return "", nil, ErrInvalidInput
			}
			operator := "<>"
			if !wantValue {
				operator = "="
			}
			clauses = append(clauses, fmt.Sprintf("COALESCE(%s, '') %s ''", column, operator))
		}
	}
	return "WHERE " + strings.Join(clauses, " AND "), args, nil
}
