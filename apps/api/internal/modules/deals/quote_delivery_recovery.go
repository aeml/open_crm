package deals

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	defaultQuoteDeliveryRecoveryBatch    = 100
	maxQuoteDeliveryRecoveryBatch        = 1000
	defaultQuoteDeliveryRecoveryInterval = time.Minute
)

type QuoteDeliveryRecoverySummary struct {
	MarkedUncertain int64
}

type QuoteDeliveryOperationalStats struct {
	Sending      int64
	StaleSending int64
	Uncertain    int64
}

type QuoteDeliveryRecoveryObserver interface {
	ObserveQuoteDeliveryRecovery(outcome string, markedUncertain int64)
}

// RecoverStaleQuoteDeliveries turns abandoned provider claims into an
// operator-resolvable state. It never retries a mailbox provider call.
func (s *Service) RecoverStaleQuoteDeliveries(ctx context.Context, batchSize int) (QuoteDeliveryRecoverySummary, error) {
	if s == nil || s.pool == nil {
		return QuoteDeliveryRecoverySummary{}, fmt.Errorf("deals service not configured")
	}
	if batchSize == 0 {
		batchSize = defaultQuoteDeliveryRecoveryBatch
	}
	if batchSize < 0 || batchSize > maxQuoteDeliveryRecoveryBatch {
		return QuoteDeliveryRecoverySummary{}, ErrQuoteDeliveryInvalid
	}
	now := s.clock().UTC()
	var marked int64
	err := s.pool.QueryRow(ctx, `
		WITH candidates AS MATERIALIZED (
		  SELECT id
		  FROM deal_quote_deliveries
		  WHERE status='sending' AND claimed_at <= $1
		  ORDER BY claimed_at,id
		  LIMIT $2
		  FOR UPDATE SKIP LOCKED
		), recovered AS (
		  UPDATE deal_quote_deliveries delivery
		  SET status='uncertain',
		      last_error='The mailbox provider outcome is unknown after an interrupted send.',
		      finalized_at=$3,updated_at=$3
		  FROM candidates
		  WHERE delivery.id=candidates.id AND delivery.status='sending'
		  RETURNING delivery.id
		)
		SELECT COUNT(*) FROM recovered
	`, now.Add(-staleQuoteDeliveryClaimAfter), batchSize, now).Scan(&marked)
	if err != nil {
		return QuoteDeliveryRecoverySummary{}, fmt.Errorf("recover stale quote deliveries: %w", err)
	}
	return QuoteDeliveryRecoverySummary{MarkedUncertain: marked}, nil
}

func (s *Service) QuoteDeliveryOperationalStats(ctx context.Context) (QuoteDeliveryOperationalStats, error) {
	if s == nil || s.pool == nil {
		return QuoteDeliveryOperationalStats{}, fmt.Errorf("deals service not configured")
	}
	var stats QuoteDeliveryOperationalStats
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status='sending'),
		       COUNT(*) FILTER (WHERE status='sending' AND claimed_at <= $1),
		       COUNT(*) FILTER (WHERE status='uncertain')
		FROM deal_quote_deliveries
	`, s.clock().UTC().Add(-staleQuoteDeliveryClaimAfter)).Scan(&stats.Sending, &stats.StaleSending, &stats.Uncertain)
	if err != nil {
		return QuoteDeliveryOperationalStats{}, fmt.Errorf("collect quote delivery operational stats: %w", err)
	}
	return stats, nil
}

func (s *Service) RunQuoteDeliveryRecoveryScheduler(ctx context.Context, logger *slog.Logger, interval time.Duration, observer QuoteDeliveryRecoveryObserver) {
	if s == nil || s.pool == nil {
		return
	}
	if interval <= 0 {
		interval = defaultQuoteDeliveryRecoveryInterval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.RecoverStaleQuoteDeliveries(ctx, defaultQuoteDeliveryRecoveryBatch)
			outcome := "success"
			if err != nil {
				outcome = "error"
			}
			if observer != nil {
				observer.ObserveQuoteDeliveryRecovery(outcome, summary.MarkedUncertain)
			}
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Error("quote delivery recovery failed", "error", err)
			} else if summary.MarkedUncertain > 0 && logger != nil {
				logger.Warn("stale quote deliveries require operator resolution", "marked_uncertain", summary.MarkedUncertain)
			}
			timer.Reset(interval)
		}
	}
}
