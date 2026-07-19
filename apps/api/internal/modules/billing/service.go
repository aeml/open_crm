package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	platformauth "github.com/aeml/open_crm/apps/api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrInvalidPlan is returned when a requested plan key is not in the catalog.
var ErrInvalidPlan = errors.New("invalid plan")

// ErrLimitReached is returned when creating a resource would exceed the
// organization's plan limit for that resource.
var ErrLimitReached = errors.New("plan limit reached")

// ErrSubscriptionInactive is returned when an organization's subscription does
// not permit new writes (canceled, or an expired trial).
var ErrSubscriptionInactive = errors.New("subscription inactive")

var (
	ErrBillingUnavailable   = errors.New("hosted billing unavailable")
	ErrBillingConflict      = errors.New("billing idempotency conflict")
	ErrBillingCustomerUnset = errors.New("billing customer not established")
	ErrBillingCustomerSet   = errors.New("billing customer already established")
	ErrBillingInProgress    = errors.New("billing checkout already in progress")
	ErrBillingForbidden     = errors.New("billing actor is not an active administrator")
)

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
	Status                 string     `json:"status"`
	TrialEndsAt            *time.Time `json:"trialEndsAt,omitempty"`
	TrialDaysLeft          int        `json:"trialDaysLeft"`
	InTrial                bool       `json:"inTrial"`
	Provider               string     `json:"provider"`
	ProviderStatus         string     `json:"providerStatus,omitempty"`
	CurrentPeriodEnd       *time.Time `json:"currentPeriodEnd,omitempty"`
	CancelAtPeriodEnd      bool       `json:"cancelAtPeriodEnd"`
	CustomerEstablished    bool       `json:"customerEstablished"`
	PortalAvailable        bool       `json:"portalAvailable"`
	CheckoutAvailablePlans []string   `json:"checkoutAvailablePlans"`
	LastReconciledAt       *time.Time `json:"lastReconciledAt,omitempty"`
	ReconciliationStale    bool       `json:"reconciliationStale"`
	Suspended              bool       `json:"suspended"`
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

const planLookupSQL = `
	SELECT plan, subscription_status, trial_ends_at,
	       COALESCE(billing_provider, ''), COALESCE(billing_provider_status, ''),
	       subscription_current_period_end, COALESCE(subscription_cancel_at_period_end, FALSE),
	       COALESCE(stripe_customer_id, ''), COALESCE(stripe_subscription_id, ''),
	       billing_last_reconciled_at
	FROM organizations WHERE id = $1
`

const usageCountsSQL = `
	SELECT
		(SELECT COUNT(*) FROM organization_memberships WHERE organization_id = $1 AND COALESCE(membership_status, 'active') = 'active'),
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
	var providerName, providerStatus, customerID, subscriptionID string
	var currentPeriodEnd, lastReconciledAt *time.Time
	var cancelAtPeriodEnd bool
	if err := s.pool.QueryRow(ctx, planLookupSQL, organizationID).Scan(
		&planKey, &subStatus, &trialEndsAt, &providerName, &providerStatus,
		&currentPeriodEnd, &cancelAtPeriodEnd, &customerID, &subscriptionID, &lastReconciledAt,
	); err != nil {
		return Entitlements{}, fmt.Errorf("load organization plan: %w", err)
	}

	var seats, contacts, deals int
	if err := s.pool.QueryRow(ctx, usageCountsSQL, organizationID).Scan(&seats, &contacts, &deals); err != nil {
		return Entitlements{}, fmt.Errorf("load usage counts: %w", err)
	}

	plan := PlanByKey(planKey)
	subscription := buildSubscription(subStatus, trialEndsAt)
	if providerName == "" {
		providerName = s.ProviderName()
	}
	subscription.Provider = providerName
	subscription.ProviderStatus = providerStatus
	subscription.Suspended = providerSuspendsWrites(providerStatus)
	subscription.CurrentPeriodEnd = currentPeriodEnd
	subscription.CancelAtPeriodEnd = cancelAtPeriodEnd
	subscription.CustomerEstablished = customerID != ""
	subscription.PortalAvailable = s.provider.PortalAvailable() && providerName == "stripe" && customerID != ""
	subscription.LastReconciledAt = lastReconciledAt
	subscription.ReconciliationStale = providerName == "stripe" && subscriptionID != "" &&
		(lastReconciledAt == nil || time.Now().UTC().After(lastReconciledAt.Add(2*reconciliationFreshness)))
	if customerID == "" {
		for _, candidate := range Catalog() {
			if s.provider.CheckoutAvailable(candidate.Key) {
				subscription.CheckoutAvailablePlans = append(subscription.CheckoutAvailablePlans, candidate.Key)
			}
		}
	}
	return Entitlements{
		Plan:         plan,
		Features:     plan.Features,
		Subscription: subscription,
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
	var providerName, providerStatus, customerID, subscriptionID string
	var currentPeriodEnd, lastReconciledAt *time.Time
	var cancelAtPeriodEnd bool
	if err := s.pool.QueryRow(ctx, planLookupSQL, organizationID).Scan(
		&currentPlan, &currentStatus, &trialEndsAt, &providerName, &providerStatus,
		&currentPeriodEnd, &cancelAtPeriodEnd, &customerID, &subscriptionID, &lastReconciledAt,
	); err != nil {
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

type CheckoutInput struct {
	OrganizationID int64
	ActorUserID    int64
	Plan           string
	IdempotencyKey string
}

func (s *Service) CreateCheckoutSession(ctx context.Context, input CheckoutInput) (HostedSession, error) {
	if s == nil || s.pool == nil || s.provider == nil {
		return HostedSession{}, ErrBillingUnavailable
	}
	input.Plan = strings.ToLower(strings.TrimSpace(input.Plan))
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.OrganizationID <= 0 || input.ActorUserID <= 0 ||
		!ValidPlanKey(input.Plan) || input.Plan == "free" ||
		len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 200 {
		return HostedSession{}, ErrInvalidPlan
	}
	if !s.provider.CheckoutAvailable(input.Plan) {
		return HostedSession{}, ErrBillingUnavailable
	}
	requestHash, err := checkoutRequestHash(input)
	if err != nil {
		return HostedSession{}, fmt.Errorf("hash checkout request: %w", err)
	}
	keyHash := hashBillingValue(input.IdempotencyKey)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return HostedSession{}, fmt.Errorf("begin checkout request: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("billing-checkout:%d:%s", input.OrganizationID, keyHash)); err != nil {
		return HostedSession{}, fmt.Errorf("lock checkout request: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("billing-checkout-org:%d", input.OrganizationID)); err != nil {
		return HostedSession{}, fmt.Errorf("lock checkout organization: %w", err)
	}
	var actorEmail, customerID, subscriptionID string
	if err := tx.QueryRow(ctx, `
		SELECT LOWER(users.email), COALESCE(organization.stripe_customer_id, ''), COALESCE(organization.stripe_subscription_id, '')
		FROM organizations organization
		JOIN organization_memberships membership
		  ON membership.organization_id=organization.id AND membership.user_id=$2
		JOIN users ON users.id=membership.user_id
		WHERE organization.id=$1
		  AND membership.role IN ('owner', 'admin')
		  AND COALESCE(membership.membership_status, 'active')='active'
	`, input.OrganizationID, input.ActorUserID).Scan(&actorEmail, &customerID, &subscriptionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return HostedSession{}, ErrBillingForbidden
		}
		return HostedSession{}, fmt.Errorf("load checkout organization: %w", err)
	}
	if customerID != "" || subscriptionID != "" {
		return HostedSession{}, ErrBillingCustomerSet
	}
	var storedHash, storedID, storedURL string
	var storedExpiresAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT request_sha256, COALESCE(provider_session_id, ''), COALESCE(checkout_url, ''), checkout_expires_at
		FROM billing_checkout_requests WHERE organization_id=$2 AND idempotency_key_hash=$1 FOR UPDATE
	`, keyHash, input.OrganizationID).Scan(&storedHash, &storedID, &storedURL, &storedExpiresAt)
	if err == nil {
		if storedHash != requestHash {
			return HostedSession{}, ErrBillingConflict
		}
		if storedID != "" && storedURL != "" {
			if err := tx.Commit(ctx); err != nil {
				return HostedSession{}, fmt.Errorf("commit checkout replay: %w", err)
			}
			return HostedSession{ID: storedID, URL: storedURL, ExpiresAt: unixTime(storedExpiresAt)}, nil
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return HostedSession{}, fmt.Errorf("load checkout request: %w", err)
	} else {
		var existing HostedSession
		var existingStatus string
		var existingExpiresAt time.Time
		existingErr := tx.QueryRow(ctx, `
			SELECT status, COALESCE(provider_session_id, ''), COALESCE(checkout_url, ''), COALESCE(checkout_expires_at, NOW())
			FROM billing_checkout_requests
			WHERE organization_id=$1 AND plan=$2 AND provider=$3
			  AND (status='creating' OR (status='created' AND checkout_expires_at > NOW()))
			ORDER BY created_at DESC, id DESC LIMIT 1
		`, input.OrganizationID, input.Plan, s.provider.Name()).Scan(&existingStatus, &existing.ID, &existing.URL, &existingExpiresAt)
		if existingErr == nil {
			if existingStatus == "creating" {
				return HostedSession{}, ErrBillingInProgress
			}
			existing.ExpiresAt = existingExpiresAt.Unix()
			if _, err := tx.Exec(ctx, `
				INSERT INTO billing_checkout_requests (
					organization_id, actor_user_id, idempotency_key_hash, request_sha256, plan, provider,
					provider_session_id, checkout_url, checkout_expires_at, status
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'created')
			`, input.OrganizationID, input.ActorUserID, keyHash, requestHash, input.Plan, s.provider.Name(), existing.ID, existing.URL, existingExpiresAt); err != nil {
				return HostedSession{}, fmt.Errorf("persist checkout session reuse: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return HostedSession{}, fmt.Errorf("commit checkout session reuse: %w", err)
			}
			return existing, nil
		}
		if !errors.Is(existingErr, pgx.ErrNoRows) {
			return HostedSession{}, fmt.Errorf("load active checkout session: %w", existingErr)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO billing_checkout_requests (
				organization_id, actor_user_id, idempotency_key_hash, request_sha256, plan, provider
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, input.OrganizationID, input.ActorUserID, keyHash, requestHash, input.Plan, s.provider.Name()); err != nil {
			return HostedSession{}, fmt.Errorf("persist checkout request: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return HostedSession{}, fmt.Errorf("commit checkout request: %w", err)
	}

	session, providerErr := s.provider.CreateCheckoutSession(ctx, CheckoutRequest{
		OrganizationID: input.OrganizationID,
		ActorUserID:    input.ActorUserID,
		Email:          actorEmail,
		Plan:           input.Plan,
		CustomerID:     customerID,
		IdempotencyKey: "opencrm_checkout_" + hashBillingValue(fmt.Sprintf("%d:%s", input.OrganizationID, keyHash)),
	})
	if providerErr != nil {
		_, _ = s.pool.Exec(ctx, `
			UPDATE billing_checkout_requests
			SET status='failed', last_error=$2, updated_at=NOW()
			WHERE organization_id=$3 AND idempotency_key_hash=$1
		`, keyHash, boundedBillingError(providerErr), input.OrganizationID)
		return HostedSession{}, fmt.Errorf("create hosted checkout: %w", providerErr)
	}
	if session.ExpiresAt <= 0 {
		session.ExpiresAt = time.Now().Add(24 * time.Hour).Unix()
	}
	expiresAt := time.Unix(session.ExpiresAt, 0).UTC()
	if _, err := s.pool.Exec(ctx, `
		UPDATE billing_checkout_requests
		SET provider_session_id=$2, checkout_url=$3, checkout_expires_at=$5,
		    status=CASE WHEN EXISTS (
		      SELECT 1 FROM organizations organization
		      WHERE organization.id=billing_checkout_requests.organization_id
		        AND organization.stripe_customer_id IS NOT NULL
		    ) THEN 'completed' ELSE 'created' END,
		    last_error=NULL, updated_at=NOW()
		WHERE organization_id=$6 AND idempotency_key_hash=$1 AND request_sha256=$4
	`, keyHash, session.ID, session.URL, requestHash, expiresAt, input.OrganizationID); err != nil {
		return HostedSession{}, fmt.Errorf("record hosted checkout: %w", err)
	}
	return session, nil
}

func unixTime(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.Unix()
}

func (s *Service) CreatePortalSession(ctx context.Context, organizationID int64) (HostedSession, error) {
	if s == nil || s.pool == nil || s.provider == nil || s.provider.Name() != "stripe" || !s.provider.PortalAvailable() {
		return HostedSession{}, ErrBillingUnavailable
	}
	var customerID string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(stripe_customer_id, '') FROM organizations WHERE id=$1`, organizationID).Scan(&customerID); err != nil {
		return HostedSession{}, fmt.Errorf("load billing customer: %w", err)
	}
	if customerID == "" {
		return HostedSession{}, ErrBillingCustomerUnset
	}
	token, err := platformauth.NewSessionToken()
	if err != nil {
		return HostedSession{}, fmt.Errorf("generate portal idempotency token: %w", err)
	}
	return s.provider.CreatePortalSession(ctx, PortalRequest{
		OrganizationID: organizationID,
		CustomerID:     customerID,
		IdempotencyKey: "opencrm_portal_" + hashBillingValue(token),
	})
}

func checkoutRequestHash(input CheckoutInput) (string, error) {
	payload, err := json.Marshal(struct {
		OrganizationID int64  `json:"organizationId"`
		ActorUserID    int64  `json:"actorUserId"`
		Plan           string `json:"plan"`
	}{input.OrganizationID, input.ActorUserID, input.Plan})
	if err != nil {
		return "", err
	}
	return hashBillingValue(string(payload)), nil
}

func hashBillingValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func boundedBillingError(err error) string {
	value := strings.TrimSpace(err.Error())
	if len(value) > 1000 {
		return value[:1000]
	}
	return value
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

const subscriptionLookupSQL = `SELECT subscription_status, trial_ends_at, COALESCE(billing_provider_status, '') FROM organizations WHERE id = $1`

// EnforceWritable returns ErrSubscriptionInactive when the organization's
// subscription does not permit new writes: a canceled subscription, or a trial
// whose period has ended. Active, past-due (grace), and in-period trials pass.
func (s *Service) EnforceWritable(ctx context.Context, organizationID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("billing service not configured")
	}

	var status string
	var trialEndsAt *time.Time
	var providerStatus string
	if err := s.pool.QueryRow(ctx, subscriptionLookupSQL, organizationID).Scan(&status, &trialEndsAt, &providerStatus); err != nil {
		return fmt.Errorf("load subscription status: %w", err)
	}
	return checkWritable(status, trialEndsAt, providerStatus)
}

// CheckWritable is the shared pure write-permission decision for a subscription
// state. Tenant-facing services with public write paths use it after locking
// the owning organization so those paths cannot bypass hosted suspension.
func CheckWritable(status string, trialEndsAt *time.Time, providerStatus ...string) error {
	if len(providerStatus) > 0 && providerSuspendsWrites(providerStatus[0]) {
		return ErrSubscriptionInactive
	}
	switch status {
	case "canceled":
		return ErrSubscriptionInactive
	case "trialing":
		if trialEndsAt != nil && time.Now().After(*trialEndsAt) {
			return ErrSubscriptionInactive
		}
		return nil
	default:
		// active, past_due (grace period), or unknown statuses remain writable.
		return nil
	}
}

func checkWritable(status string, trialEndsAt *time.Time, providerStatus ...string) error {
	return CheckWritable(status, trialEndsAt, providerStatus...)
}

func providerSuspendsWrites(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "incomplete", "unpaid", "paused", "incomplete_expired":
		return true
	default:
		return false
	}
}
