package billing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LimitUsage pairs a numeric limit with current usage for a metered resource.
type LimitUsage struct {
	Used      int  `json:"used"`
	Limit     int  `json:"limit"`
	Unlimited bool `json:"unlimited"`
	Exceeded  bool `json:"exceeded"`
}

func newLimitUsage(used, limit int) LimitUsage {
	return LimitUsage{
		Used:      used,
		Limit:     limit,
		Unlimited: limit == Unlimited,
		Exceeded:  !WithinLimit(used, limit),
	}
}

// Entitlements is the resolved billing state for an organization: its active
// plan, the features it unlocks, and current usage against each metered limit.
type Entitlements struct {
	Plan     Plan       `json:"plan"`
	Features []string   `json:"features"`
	Seats    LimitUsage `json:"seats"`
	Contacts LimitUsage `json:"contacts"`
	Deals    LimitUsage `json:"deals"`
}

// Service resolves plan entitlements and usage for organizations.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

const planLookupSQL = `SELECT plan FROM organizations WHERE id = $1`

const usageCountsSQL = `
	SELECT
		(SELECT COUNT(*) FROM organization_memberships WHERE organization_id = $1),
		(SELECT COUNT(*) FROM contacts WHERE organization_id = $1 AND archived_at IS NULL),
		(SELECT COUNT(*) FROM deals WHERE organization_id = $1 AND archived_at IS NULL)
`

// Entitlements loads the organization's plan and computes live usage against
// its limits. Usage counts exclude archived records.
func (s *Service) Entitlements(ctx context.Context, organizationID int64) (Entitlements, error) {
	if s == nil || s.pool == nil {
		return Entitlements{}, fmt.Errorf("billing service not configured")
	}

	var planKey string
	if err := s.pool.QueryRow(ctx, planLookupSQL, organizationID).Scan(&planKey); err != nil {
		return Entitlements{}, fmt.Errorf("load organization plan: %w", err)
	}

	var seats, contacts, deals int
	if err := s.pool.QueryRow(ctx, usageCountsSQL, organizationID).Scan(&seats, &contacts, &deals); err != nil {
		return Entitlements{}, fmt.Errorf("load usage counts: %w", err)
	}

	plan := PlanByKey(planKey)
	return Entitlements{
		Plan:     plan,
		Features: plan.Features,
		Seats:    newLimitUsage(seats, plan.SeatLimit),
		Contacts: newLimitUsage(contacts, plan.ContactLimit),
		Deals:    newLimitUsage(deals, plan.DealLimit),
	}, nil
}
