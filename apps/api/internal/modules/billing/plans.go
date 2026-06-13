package billing

import "strings"

// Feature keys gate plan-restricted capabilities. They are referenced by both
// the API (entitlement checks) and the frontend (UI affordances).
const (
	FeatureSavedViews        = "saved_views"
	FeatureCSVImport         = "csv_import"
	FeatureCSVExport         = "csv_export"
	FeatureEmailSync         = "email_sync"
	FeatureAutomation        = "automation"
	FeatureCustomFields      = "custom_fields"
	FeatureAPIAccess         = "api_access"
	FeatureAdvancedReporting = "advanced_reporting"
	FeatureSSO               = "sso"
)

// Unlimited marks a numeric limit as having no ceiling.
const Unlimited = -1

// Plan describes a billing tier: its identity, pricing hint, numeric limits,
// and the set of features it unlocks. Plans are defined in code (not the
// database) so entitlement logic stays explicit and testable.
type Plan struct {
	Key             string   `json:"key"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	MonthlyPriceUSD int      `json:"monthlyPriceUsd"`
	SeatLimit       int      `json:"seatLimit"`
	ContactLimit    int      `json:"contactLimit"`
	DealLimit       int      `json:"dealLimit"`
	Features        []string `json:"features"`
}

// HasFeature reports whether the plan unlocks the given feature key.
func (p Plan) HasFeature(feature string) bool {
	for _, f := range p.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// WithinLimit reports whether a usage count is permitted by a limit value.
// Unlimited always passes. Otherwise usage must be at or below the limit.
func WithinLimit(used, limit int) bool {
	if limit == Unlimited {
		return true
	}
	return used <= limit
}

// defaultPlanKey is assigned to organizations that have no recognized plan.
const defaultPlanKey = "free"

var catalog = []Plan{
	{
		Key:             "free",
		Name:            "Free",
		Description:     "Get started with core CRM for a small team.",
		MonthlyPriceUSD: 0,
		SeatLimit:       2,
		ContactLimit:    500,
		DealLimit:       250,
		Features: []string{
			FeatureSavedViews,
			FeatureCSVExport,
		},
	},
	{
		Key:             "starter",
		Name:            "Starter",
		Description:     "For growing teams that need import and email.",
		MonthlyPriceUSD: 19,
		SeatLimit:       5,
		ContactLimit:    2500,
		DealLimit:       2500,
		Features: []string{
			FeatureSavedViews,
			FeatureCSVExport,
			FeatureCSVImport,
			FeatureEmailSync,
		},
	},
	{
		Key:             "pro",
		Name:            "Pro",
		Description:     "Automation, custom fields, and API for scaling sales orgs.",
		MonthlyPriceUSD: 49,
		SeatLimit:       25,
		ContactLimit:    50000,
		DealLimit:       50000,
		Features: []string{
			FeatureSavedViews,
			FeatureCSVExport,
			FeatureCSVImport,
			FeatureEmailSync,
			FeatureAutomation,
			FeatureCustomFields,
			FeatureAPIAccess,
			FeatureAdvancedReporting,
		},
	},
	{
		Key:             "enterprise",
		Name:            "Enterprise",
		Description:     "Unlimited scale with SSO and enterprise controls.",
		MonthlyPriceUSD: 0, // contact sales
		SeatLimit:       Unlimited,
		ContactLimit:    Unlimited,
		DealLimit:       Unlimited,
		Features: []string{
			FeatureSavedViews,
			FeatureCSVExport,
			FeatureCSVImport,
			FeatureEmailSync,
			FeatureAutomation,
			FeatureCustomFields,
			FeatureAPIAccess,
			FeatureAdvancedReporting,
			FeatureSSO,
		},
	},
}

// Catalog returns all plans in display order.
func Catalog() []Plan {
	out := make([]Plan, len(catalog))
	copy(out, catalog)
	return out
}

// ValidPlanKey reports whether the key names a real plan in the catalog.
func ValidPlanKey(key string) bool {
	key = strings.TrimSpace(strings.ToLower(key))
	for _, p := range catalog {
		if p.Key == key {
			return true
		}
	}
	return false
}

// PlanByKey resolves a plan key to its definition. Unknown or empty keys
// resolve to the default (free) plan so entitlement checks never panic.
func PlanByKey(key string) Plan {
	key = strings.TrimSpace(strings.ToLower(key))
	for _, p := range catalog {
		if p.Key == key {
			return p
		}
	}
	for _, p := range catalog {
		if p.Key == defaultPlanKey {
			return p
		}
	}
	return catalog[0]
}
