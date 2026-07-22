package emailmessages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	defaultRecordDeliveryRecoveryBatch    = 100
	maxRecordDeliveryRecoveryBatch        = 1000
	defaultRecordDeliveryRecoveryInterval = time.Minute
)

type RecordDeliveryRecoverySummary struct {
	MarkedUncertain int64
}

type RecordDeliveryOperationalStats struct {
	Sending      int64
	StaleSending int64
	Uncertain    int64
}

type RecordDeliveryRecoveryObserver interface {
	ObserveRecordEmailDeliveryRecovery(outcome string, markedUncertain int64)
}

func (s *Service) RecoverStaleRecordDeliveries(ctx context.Context, batchSize int) (RecordDeliveryRecoverySummary, error) {
	if s == nil || s.pool == nil {
		return RecordDeliveryRecoverySummary{}, fmt.Errorf("email messages service not configured")
	}
	if batchSize == 0 {
		batchSize = defaultRecordDeliveryRecoveryBatch
	}
	if batchSize < 0 || batchSize > maxRecordDeliveryRecoveryBatch {
		return RecordDeliveryRecoverySummary{}, ErrInvalidInput
	}
	now := s.now().UTC()
	var marked int64
	if err := s.pool.QueryRow(ctx, `
		WITH candidates AS MATERIALIZED (
		  SELECT id
		  FROM record_email_deliveries
		  WHERE status='sending' AND claimed_at <= $1
		  ORDER BY claimed_at,id
		  LIMIT $2
		  FOR UPDATE SKIP LOCKED
		), recovered AS (
		  UPDATE record_email_deliveries delivery
		  SET status='uncertain',
		      last_error='The mailbox provider outcome is unknown after an interrupted send.',
		      finalized_at=$3,updated_at=$3
		  FROM candidates
		  WHERE delivery.id=candidates.id AND delivery.status='sending'
		  RETURNING delivery.id
		)
		SELECT COUNT(*) FROM recovered
	`, now.Add(-staleRecordDeliveryClaimAfter), batchSize, now).Scan(&marked); err != nil {
		return RecordDeliveryRecoverySummary{}, fmt.Errorf("recover stale record email deliveries: %w", err)
	}
	return RecordDeliveryRecoverySummary{MarkedUncertain: marked}, nil
}

func (s *Service) RecordDeliveryOperationalStats(ctx context.Context) (RecordDeliveryOperationalStats, error) {
	if s == nil || s.pool == nil {
		return RecordDeliveryOperationalStats{}, fmt.Errorf("email messages service not configured")
	}
	var stats RecordDeliveryOperationalStats
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status='sending'),
		       COUNT(*) FILTER (WHERE status='sending' AND claimed_at <= $1),
		       COUNT(*) FILTER (WHERE status='uncertain')
		FROM record_email_deliveries
	`, s.now().UTC().Add(-staleRecordDeliveryClaimAfter)).Scan(&stats.Sending, &stats.StaleSending, &stats.Uncertain); err != nil {
		return RecordDeliveryOperationalStats{}, fmt.Errorf("collect record email delivery operational stats: %w", err)
	}
	return stats, nil
}

func (s *Service) RunRecordDeliveryRecoveryScheduler(ctx context.Context, logger *slog.Logger, interval time.Duration, observer RecordDeliveryRecoveryObserver) {
	if s == nil || s.pool == nil {
		return
	}
	if interval <= 0 {
		interval = defaultRecordDeliveryRecoveryInterval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.RecoverStaleRecordDeliveries(ctx, defaultRecordDeliveryRecoveryBatch)
			outcome := "success"
			if err != nil {
				outcome = "error"
			}
			if observer != nil {
				observer.ObserveRecordEmailDeliveryRecovery(outcome, summary.MarkedUncertain)
			}
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Error("record email delivery recovery failed", "error", err)
			} else if summary.MarkedUncertain > 0 && logger != nil {
				logger.Warn("stale record email deliveries require operator resolution", "marked_uncertain", summary.MarkedUncertain)
			}
			timer.Reset(interval)
		}
	}
}
