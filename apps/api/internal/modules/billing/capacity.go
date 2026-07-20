package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

const maxCapacityReservation = 1000

var (
	ErrInvalidCapacityResource    = errors.New("invalid capacity resource")
	ErrCapacityReservationExpired = errors.New("capacity reservation expired")
	ErrCapacityUnavailable        = errors.New("capacity enforcement unavailable")
)

// CapacityReservation is a short-lived claim against one hosted plan limit.
// An ID of zero is an unmanaged or unlimited no-op reservation.
type CapacityReservation struct {
	ID             int64
	OrganizationID int64
	Resource       string
	Amount         int
	ExpiresAt      time.Time
}

func (r CapacityReservation) Enforced() bool { return r.ID > 0 }

// CapacityManager is the narrow transaction seam used by every operation that
// can increase a currently enforced hosted capacity.
type CapacityManager interface {
	ReserveCapacity(context.Context, int64, string, int) (CapacityReservation, error)
	ConsumeCapacity(context.Context, pgx.Tx, CapacityReservation) error
	CancelCapacity(context.Context, CapacityReservation) error
}

// ReserveCapacity is the nil-safe entry point for domain services. Tests and
// self-contained module uses that do not configure billing preserve the
// historical unrestricted behavior.
func ReserveCapacity(ctx context.Context, manager CapacityManager, organizationID int64, resource string, amount int) (CapacityReservation, error) {
	if manager == nil {
		return CapacityReservation{}, nil
	}
	return manager.ReserveCapacity(ctx, organizationID, resource, amount)
}

// ConsumeCapacity is the matching nil-safe transaction hook.
func ConsumeCapacity(ctx context.Context, manager CapacityManager, tx pgx.Tx, reservation CapacityReservation) error {
	if manager == nil || !reservation.Enforced() {
		return nil
	}
	return manager.ConsumeCapacity(ctx, tx, reservation)
}

// LockCapacityEffect establishes the same tenant-first lock order for every
// transaction that will consume an enforced claim. Besides closing snapshot
// gaps, this avoids deadlocks with public paths that already lock subscription
// state before inserting a record.
func LockCapacityEffect(ctx context.Context, tx pgx.Tx, reservation CapacityReservation) error {
	if !reservation.Enforced() {
		return nil
	}
	if tx == nil || reservation.OrganizationID <= 0 {
		return ErrInvalidCapacityResource
	}
	var organizationID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, reservation.OrganizationID).Scan(&organizationID); err != nil {
		return fmt.Errorf("%w: lock capacity effect organization: %v", ErrCapacityUnavailable, err)
	}
	return nil
}

// ReserveCapacity serializes reservations per tenant and counts both durable
// records and unexpired concurrent claims. Self-hosted and unlimited plans get
// a no-op reservation and remain unrestricted.
func (s *Service) ReserveCapacity(ctx context.Context, organizationID int64, resource string, amount int) (CapacityReservation, error) {
	if s == nil || s.pool == nil {
		return CapacityReservation{}, fmt.Errorf("%w: billing service not configured", ErrCapacityUnavailable)
	}
	if organizationID <= 0 || amount <= 0 || amount > maxCapacityReservation {
		return CapacityReservation{}, ErrInvalidCapacityResource
	}
	if !validCapacityResource(resource) {
		return CapacityReservation{}, ErrInvalidCapacityResource
	}
	if !s.Hosted() {
		return CapacityReservation{}, nil
	}

	// The organization row is the per-tenant serialization point. Read
	// committed ensures a waiter observes the preceding reservation after the
	// lock is released instead of surfacing an avoidable serialization abort.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CapacityReservation{}, fmt.Errorf("%w: begin capacity reservation: %v", ErrCapacityUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var planKey string
	if err := tx.QueryRow(ctx, `SELECT plan FROM organizations WHERE id=$1 FOR UPDATE`, organizationID).Scan(&planKey); err != nil {
		return CapacityReservation{}, fmt.Errorf("%w: lock capacity organization: %v", ErrCapacityUnavailable, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM billing_capacity_reservations WHERE organization_id=$1 AND resource=$2 AND expires_at <= NOW()`, organizationID, resource); err != nil {
		return CapacityReservation{}, fmt.Errorf("%w: expire stale capacity reservations: %v", ErrCapacityUnavailable, err)
	}

	used, err := capacityUsage(ctx, tx, organizationID, resource)
	if err != nil {
		return CapacityReservation{}, fmt.Errorf("%w: %v", ErrCapacityUnavailable, err)
	}
	var reserved int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount),0)::bigint
		FROM billing_capacity_reservations
		WHERE organization_id=$1 AND resource=$2 AND expires_at > NOW()
	`, organizationID, resource).Scan(&reserved); err != nil {
		return CapacityReservation{}, fmt.Errorf("%w: load active capacity reservations: %v", ErrCapacityUnavailable, err)
	}
	limit := capacityLimit(PlanByKey(planKey), resource)
	if limit == Unlimited {
		if err := tx.Commit(ctx); err != nil {
			return CapacityReservation{}, fmt.Errorf("%w: commit unlimited capacity check: %v", ErrCapacityUnavailable, err)
		}
		return CapacityReservation{}, nil
	}
	if used+reserved+int64(amount) > int64(limit) {
		return CapacityReservation{}, ErrLimitReached
	}

	reservation := CapacityReservation{OrganizationID: organizationID, Resource: resource, Amount: amount}
	if err := tx.QueryRow(ctx, `
		INSERT INTO billing_capacity_reservations (organization_id,resource,amount,expires_at)
		VALUES ($1,$2,$3,NOW()+INTERVAL '10 minutes')
		RETURNING id,expires_at
	`, organizationID, resource, amount).Scan(&reservation.ID, &reservation.ExpiresAt); err != nil {
		return CapacityReservation{}, fmt.Errorf("%w: insert capacity reservation: %v", ErrCapacityUnavailable, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CapacityReservation{}, fmt.Errorf("%w: commit capacity reservation: %v", ErrCapacityUnavailable, err)
	}
	return reservation, nil
}

// ConsumeCapacity removes a live reservation inside the same transaction as
// its capacity-increasing effect. An expired or missing reservation rolls the
// effect back because its concurrency guarantee is no longer valid.
func (s *Service) ConsumeCapacity(ctx context.Context, tx pgx.Tx, reservation CapacityReservation) error {
	if !reservation.Enforced() {
		return nil
	}
	if s == nil || tx == nil || reservation.OrganizationID <= 0 || !validCapacityResource(reservation.Resource) || reservation.Amount <= 0 {
		return ErrInvalidCapacityResource
	}
	// Hold the same tenant serialization lock used by ReserveCapacity until the
	// domain effect and reservation deletion commit together. A waiter can
	// therefore observe either the claim or the newly committed record, never a
	// gap between two separate COUNT snapshots.
	if err := LockCapacityEffect(ctx, tx, reservation); err != nil {
		return err
	}
	var amount int
	err := tx.QueryRow(ctx, `
		DELETE FROM billing_capacity_reservations
		WHERE id=$1 AND organization_id=$2 AND resource=$3 AND expires_at > NOW()
		RETURNING amount
	`, reservation.ID, reservation.OrganizationID, reservation.Resource).Scan(&amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCapacityReservationExpired
	}
	if err != nil {
		return fmt.Errorf("%w: consume capacity reservation: %v", ErrCapacityUnavailable, err)
	}
	if amount != reservation.Amount {
		return ErrInvalidCapacityResource
	}
	return nil
}

// CancelCapacity releases a reservation after validation or a domain write
// fails. It is idempotent because a successfully consumed row is already gone.
func (s *Service) CancelCapacity(ctx context.Context, reservation CapacityReservation) error {
	if !reservation.Enforced() {
		return nil
	}
	if s == nil || s.pool == nil {
		return fmt.Errorf("%w: billing service not configured", ErrCapacityUnavailable)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM billing_capacity_reservations WHERE id=$1 AND organization_id=$2 AND resource=$3`, reservation.ID, reservation.OrganizationID, reservation.Resource); err != nil {
		return fmt.Errorf("%w: cancel capacity reservation: %v", ErrCapacityUnavailable, err)
	}
	return nil
}

// CancelReservation is a bounded best-effort failure-path cleanup. The durable
// expiry remains the crash-recovery guarantee if the database is unavailable.
func CancelReservation(manager CapacityManager, reservation CapacityReservation) {
	if manager == nil || !reservation.Enforced() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := manager.CancelCapacity(ctx, reservation); err != nil {
		log.Printf("billing capacity reservation cleanup failed: organization_id=%d resource=%s reservation_id=%d err=%v", reservation.OrganizationID, reservation.Resource, reservation.ID, err)
	}
}

func capacityUsage(ctx context.Context, tx pgx.Tx, organizationID int64, resource string) (int64, error) {
	queries := map[string]string{
		ResourceSeats:    `SELECT COUNT(*) FROM organization_memberships WHERE organization_id=$1 AND COALESCE(membership_status,'active')='active'`,
		ResourceContacts: `SELECT COUNT(*) FROM contacts WHERE organization_id=$1 AND archived_at IS NULL`,
		ResourceDeals:    `SELECT COUNT(*) FROM deals WHERE organization_id=$1 AND archived_at IS NULL`,
	}
	var used int64
	if err := tx.QueryRow(ctx, queries[resource], organizationID).Scan(&used); err != nil {
		return 0, fmt.Errorf("load %s capacity usage: %w", resource, err)
	}
	return used, nil
}

func capacityLimit(plan Plan, resource string) int {
	switch resource {
	case ResourceSeats:
		return plan.SeatLimit
	case ResourceContacts:
		return plan.ContactLimit
	case ResourceDeals:
		return plan.DealLimit
	default:
		return 0
	}
}

func validCapacityResource(resource string) bool {
	return resource == ResourceSeats || resource == ResourceContacts || resource == ResourceDeals
}
