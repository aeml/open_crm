package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	defaultReadRetention     = 90 * 24 * time.Hour
	defaultUnreadRetention   = 365 * 24 * time.Hour
	defaultRetentionBatch    = 500
	maxRetentionBatch        = 5000
	defaultRetentionInterval = time.Hour
)

var ErrInvalidRetentionPolicy = errors.New("invalid notification retention policy")

// RetentionPolicy bounds notification history while giving unread work a much
// longer recovery window than acknowledged items.
type RetentionPolicy struct {
	ReadFor   time.Duration
	UnreadFor time.Duration
	BatchSize int
}

type RetentionSummary struct {
	ReadDeleted   int64
	UnreadDeleted int64
}

type RetentionObserver interface {
	ObserveNotificationRetention(outcome string, readDeleted, unreadDeleted int64)
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		ReadFor:   defaultReadRetention,
		UnreadFor: defaultUnreadRetention,
		BatchSize: defaultRetentionBatch,
	}
}

// ApplyRetention removes one bounded batch from each lifecycle class. The
// separate SKIP LOCKED selections let multiple API instances make progress
// without contending on the same rows.
func (s *Service) ApplyRetention(ctx context.Context, policy RetentionPolicy) (RetentionSummary, error) {
	if s == nil || s.pool == nil {
		return RetentionSummary{}, fmt.Errorf("notifications service not configured")
	}
	policy, err := normalizeRetentionPolicy(policy)
	if err != nil {
		return RetentionSummary{}, err
	}
	now := s.now().UTC()
	summary := RetentionSummary{}

	read, err := s.pool.Exec(ctx, `
		WITH candidates AS (
			SELECT id
			FROM notifications
			WHERE read_at IS NOT NULL AND read_at < $1
			ORDER BY read_at ASC, id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM notifications notification
		USING candidates
		WHERE notification.id = candidates.id
	`, now.Add(-policy.ReadFor), policy.BatchSize)
	if err != nil {
		return summary, fmt.Errorf("delete retained read notifications: %w", err)
	}
	summary.ReadDeleted = read.RowsAffected()

	unread, err := s.pool.Exec(ctx, `
		WITH candidates AS (
			SELECT id
			FROM notifications
			WHERE read_at IS NULL AND created_at < $1
			ORDER BY created_at ASC, id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM notifications notification
		USING candidates
		WHERE notification.id = candidates.id
	`, now.Add(-policy.UnreadFor), policy.BatchSize)
	if err != nil {
		return summary, fmt.Errorf("delete retained unread notifications: %w", err)
	}
	summary.UnreadDeleted = unread.RowsAffected()
	return summary, nil
}

func (s *Service) RunRetentionScheduler(ctx context.Context, logger *slog.Logger, policy RetentionPolicy, interval time.Duration, observer RetentionObserver) {
	if s == nil || s.pool == nil {
		return
	}
	if interval <= 0 {
		interval = defaultRetentionInterval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.ApplyRetention(ctx, policy)
			outcome := "success"
			if err != nil {
				outcome = "error"
			}
			if observer != nil {
				observer.ObserveNotificationRetention(outcome, summary.ReadDeleted, summary.UnreadDeleted)
			}
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Error("notification retention failed", "error", err, "read_deleted", summary.ReadDeleted, "unread_deleted", summary.UnreadDeleted)
			} else if (summary.ReadDeleted > 0 || summary.UnreadDeleted > 0) && logger != nil {
				logger.Info("notification retention completed", "read_deleted", summary.ReadDeleted, "unread_deleted", summary.UnreadDeleted)
			}
			timer.Reset(interval)
		}
	}
}

func normalizeRetentionPolicy(policy RetentionPolicy) (RetentionPolicy, error) {
	if policy.ReadFor == 0 {
		policy.ReadFor = defaultReadRetention
	}
	if policy.UnreadFor == 0 {
		policy.UnreadFor = defaultUnreadRetention
	}
	if policy.BatchSize == 0 {
		policy.BatchSize = defaultRetentionBatch
	}
	if policy.ReadFor < 0 || policy.UnreadFor < policy.ReadFor || policy.BatchSize < 0 || policy.BatchSize > maxRetentionBatch {
		return RetentionPolicy{}, ErrInvalidRetentionPolicy
	}
	return policy, nil
}
