package orgprofile

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Detail struct {
	OrganizationID int64             `json:"organizationId"`
	BusinessType   string            `json:"businessType"`
	DisplayName    string            `json:"displayName"`
	Modules        []string          `json:"modules"`
	Labels         map[string]string `json:"labels"`
}

type UpdateInput struct {
	BusinessType string `json:"businessType"`
}

type profileDefinition struct {
	DisplayName string
	Modules     []string
	Labels      map[string]string
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) GetByOrganizationID(ctx context.Context, organizationID int64) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("org profile service not configured")
	}

	var businessType string
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(business_type, ''), 'general')
		FROM organizations
		WHERE id = $1
	`, organizationID).Scan(&businessType); err != nil {
		return Detail{}, fmt.Errorf("get organization business type: %w", err)
	}

	return BuildDetailForBusinessType(organizationID, businessType)
}

func (s *Service) UpdateByOrganizationID(ctx context.Context, organizationID, _ int64, input UpdateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("org profile service not configured")
	}

	businessType := normalizeBusinessType(input.BusinessType)
	if _, ok := profileDefinitions()[businessType]; !ok {
		return Detail{}, fmt.Errorf("unsupported business type")
	}

	if _, err := s.pool.Exec(ctx, `
		UPDATE organizations
		SET business_type = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, organizationID, businessType); err != nil {
		return Detail{}, fmt.Errorf("update organization business type: %w", err)
	}

	return BuildDetailForBusinessType(organizationID, businessType)
}

func BuildDetailForBusinessType(organizationID int64, businessType string) (Detail, error) {
	businessType = normalizeBusinessType(businessType)
	definition, ok := profileDefinitions()[businessType]
	if !ok {
		return Detail{}, fmt.Errorf("unsupported business type")
	}

	modules := make([]string, 0, len(definition.Modules))
	modules = append(modules, definition.Modules...)
	labels := make(map[string]string, len(definition.Labels))
	for key, value := range definition.Labels {
		labels[key] = value
	}

	return Detail{
		OrganizationID: organizationID,
		BusinessType:   businessType,
		DisplayName:    definition.DisplayName,
		Modules:        modules,
		Labels:         labels,
	}, nil
}

func normalizeBusinessType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "general"
	}
	return value
}

func profileDefinitions() map[string]profileDefinition {
	return map[string]profileDefinition{
		"general": {
			DisplayName: "General CRM",
			Modules:     []string{"contacts", "companies", "deals", "tasks"},
			Labels: map[string]string{
				"contacts":  "Contacts",
				"companies": "Companies",
				"deals":     "Deals",
				"tasks":     "Tasks",
			},
		},
		"services": {
			DisplayName: "Services",
			Modules:     []string{"contacts", "companies", "deals", "tasks", "projects"},
			Labels: map[string]string{
				"contacts":  "Contacts",
				"companies": "Clients",
				"deals":     "Engagements",
				"tasks":     "Service Tasks",
			},
		},
		"product-sales": {
			DisplayName: "Product Sales",
			Modules:     []string{"contacts", "companies", "deals", "tasks", "catalog"},
			Labels: map[string]string{
				"contacts":  "Contacts",
				"companies": "Accounts",
				"deals":     "Opportunities",
				"tasks":     "Follow-ups",
			},
		},
		"construction-services": {
			DisplayName: "Construction Services",
			Modules:     []string{"contacts", "companies", "deals", "tasks", "estimates"},
			Labels: map[string]string{
				"contacts":  "Contacts",
				"companies": "Clients",
				"deals":     "Jobs",
				"tasks":     "Site Tasks",
			},
		},
	}
}
