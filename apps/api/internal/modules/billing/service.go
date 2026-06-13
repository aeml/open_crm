package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrInvalidPlan is returned when a requested plan key is not in the catalog.
var ErrInvalidPlan = errors.New("invalid plan")

// ErrLimitReached is returned when creating a resource would exceed the
// organization's plan limit for that resource.
var ErrLimitReached = errors.New("plan limit reached")

// Metered resource keys for limit enforcement.
const (
	ResourceContacts = "contacts"
	ResourceDeals    = "deals"
	ResourceSeats    = "seats"
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

// Subscription describes an organization's billing lifecycle state.
type Subscription struct {
	Status        string     `json:"status"`
	TrialEndsAt   *time.Time `json:"trialEndsAt,omitempty"`
	TrialDaysLeft int        `json:"trialDaysLeft"`
	InTrial       bool       `json:"inTrial"`
}

// Entitlements is the resolved billing state for an organization: its active
// plan, the features it unlocks, and current usage against each metered limit.
type Entitlements struct {
	Plan         Plan         `json:"plan"`
	Features     []string     `json:"features"`
	Subscription Subscription `json:"subscription"`
	Seats        LimitUsage   `json:"seats"`
	Contacts     LimitUsage   `json:"contacts"`
	Deals        LimitUsage   `json:"deals"`
}

// Service resolves plan entitlements and usage for organizations.
type Service struct {
	pool     *pgxpool.Pool
	provider Provider
}

func NewService(pool *pgxpool.Pool, provider Provider) *Service {
	if provider == nil {
		provider = FakeProvider{}
	}
	return &Service{pool: pool, provider: provider}
}

// ProviderName reports the active billing provider for diagnostics.
func (s *Service) ProviderName() string {
	if s == nil || s.provider == nil {
		return "none"
	}
	return s.provider.Name()
}

const planLookupSQL = `SELECT plan, subscription_status, trial_ends_at FROM organizations WHERE id = $1`

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
	var subStatus string
	var trialEndsAt *time.Time
	if err := s.pool.QueryRow(ctx, planLookupSQL, organizationID).Scan(&planKey, &subStatus, &trialEndsAt); err != nil {
		return Entitlements{}, fmt.Errorf("load organization plan: %w", err)
	}

	var seats, contacts, deals int
	if err := s.pool.QueryRow(ctx, usageCountsSQL, organizationID).Scan(&seats, &contacts, &deals); err != nil {
		return Entitlements{}, fmt.Errorf("load usage counts: %w", err)
	}

	plan := PlanByKey(planKey)
	return Entitlements{
		Plan:         plan,
		Features:     plan.Features,
		Subscription: buildSubscription(subStatus, trialEndsAt),
		Seats:        newLimitUsage(seats, plan.SeatLimit),
		Contacts:     newLimitUsage(contacts, plan.ContactLimit),
		Deals:        newLimitUsage(deals, plan.DealLimit),
	}, nil
}

// buildSubscription derives display-friendly trial state from the stored
// status and trial end timestamp.
func buildSubscription(status string, trialEndsAt *time.Time) Subscription {
	sub := Subscription{Status: status, TrialEndsAt: trialEndsAt}
	if status == "trialing" && trialEndsAt != nil {
		remaining := time.Until(*trialEndsAt)
		if remaining > 0 {
			sub.InTrial = true
			sub.TrialDaysLeft = int(remaining.Hours()/24) + 1
		}
	}
	return sub
}

// ChangePlan transitions an organization to a new plan. It validates the
// target plan, asks the billing provider to provision the subscription, then
// persists the new plan and returns refreshed entitlements. The actual payment
// processing is delegated to the configured Provider (fake by default).
func (s *Service) ChangePlan(ctx context.Context, organizationID int64, planKey string) (Entitlements, error) {
	if s == nil || s.pool == nil {
		return Entitlements{}, fmt.Errorf("billing service not configured")
	}

	planKey = strings.TrimSpace(strings.ToLower(planKey))
	if !ValidPlanKey(planKey) {
		return Entitlements{}, ErrInvalidPlan
	}

	var currentPlan, currentStatus string
	var trialEndsAt *time.Time
	if err := s.pool.QueryRow(ctx, planLookupSQL, organizationID).Scan(&currentPlan, &currentStatus, &trialEndsAt); err != nil {
		return Entitlements{}, fmt.Errorf("load organization plan: %w", err)
	}

	if _, err := s.provider.ChangeSubscription(ctx, ChangeRequest{
		OrganizationID: organizationID,
		FromPlan:       currentPlan,
		ToPlan:         planKey,
	}); err != nil {
		return Entitlements{}, fmt.Errorf("provider change subscription: %w", err)
	}

	// A successful plan change activates the subscription and ends any trial.
	if _, err := s.pool.Exec(ctx, `UPDATE organizations SET plan = $1, subscription_status = 'active', trial_ends_at = NULL WHERE id = $2`, planKey, organizationID); err != nil {
		return Entitlements{}, fmt.Errorf("persist organization plan: %w", err)
	}

	return s.Entitlements(ctx, organizationID)
}

// EnforceCanCreate returns ErrLimitReached if adding one more of the given
// metered resource would exceed the organization's plan limit. Unlimited
// plans and unmetered resources always pass.
func (s *Service) EnforceCanCreate(ctx context.Context, organizationID int64, resource string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("billing service not configured")
	}

	entitlements, err := s.Entitlements(ctx, organizationID)
	if err != nil {
		return err
	}

	var usage LimitUsage
	switch resource {
	case ResourceContacts:
		usage = entitlements.Contacts
	case ResourceDeals:
		usage = entitlements.Deals
	case ResourceSeats:
		usage = entitlements.Seats
	default:
		return nil
	}

	if !CanCreateMore(usage) {
		return ErrLimitReached
	}
	return nil
}
