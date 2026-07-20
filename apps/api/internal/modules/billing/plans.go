package billing

import "strings"

// Feature keys are reserved for an approved hosted feature policy. The current
// catalog deliberately returns no feature grants: a name in this list must not
// become a commercial promise before API, UI, and worker enforcement agree.
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

// Plan describes a billing tier: its identity, non-authoritative pricing hint,
// numeric limits, and any approved feature grants. Stripe Checkout remains the
// price authority. Plans are defined in code (not the database) so entitlement
// logic stays explicit and testable.
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

// CanCreateMore reports whether one more unit fits within the usage's limit.
func CanCreateMore(usage LimitUsage) bool {
	if usage.Unlimited {
		return true
	}
	return usage.Used < usage.Limit
}

// defaultPlanKey is assigned to organizations that have no recognized plan.
const defaultPlanKey = "free"

var catalog = []Plan{
	{
		Key:             "free",
		Name:            "Free",
		Description:     "A small hosted workspace with bounded CRM capacity.",
		MonthlyPriceUSD: 0,
		SeatLimit:       2,
		ContactLimit:    500,
		DealLimit:       250,
		Features:        []string{},
	},
	{
		Key:             "starter",
		Name:            "Starter",
		Description:     "Higher hosted capacity for a growing service team.",
		MonthlyPriceUSD: 19,
		SeatLimit:       5,
		ContactLimit:    2500,
		DealLimit:       2500,
		Features:        []string{},
	},
	{
		Key:             "pro",
		Name:            "Pro",
		Description:     "Expanded hosted capacity for an established team.",
		MonthlyPriceUSD: 49,
		SeatLimit:       25,
		ContactLimit:    50000,
		DealLimit:       50000,
		Features:        []string{},
	},
	{
		Key:             "enterprise",
		Name:            "Enterprise",
		Description:     "Custom hosted capacity requiring an operator agreement.",
		MonthlyPriceUSD: 0, // contact sales
		SeatLimit:       Unlimited,
		ContactLimit:    Unlimited,
		DealLimit:       Unlimited,
		Features:        []string{},
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
