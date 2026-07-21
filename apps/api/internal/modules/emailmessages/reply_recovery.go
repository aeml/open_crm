package emailmessages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	defaultReplyRecoveryBatch    = 100
	maxReplyRecoveryBatch        = 1000
	defaultReplyRecoveryInterval = time.Minute
)

type ReplyRecoverySummary struct {
	MarkedUncertain int64
}

type ReplyOperationalStats struct {
	Sending      int64
	StaleSending int64
	Uncertain    int64
}

type ReplyRecoveryObserver interface {
	ObserveEmailReplyRecovery(outcome string, markedUncertain int64)
}

// RecoverStaleReplies converts a bounded set of abandoned send claims into an
// explicit operator-resolvable state. It never retries a provider call.
func (s *Service) RecoverStaleReplies(ctx context.Context, batchSize int) (ReplyRecoverySummary, error) {
	if s == nil || s.pool == nil {
		return ReplyRecoverySummary{}, fmt.Errorf("email messages service not configured")
	}
	if batchSize == 0 {
		batchSize = defaultReplyRecoveryBatch
	}
	if batchSize < 0 || batchSize > maxReplyRecoveryBatch {
		return ReplyRecoverySummary{}, ErrInvalidInput
	}
	var marked int64
	err := s.pool.QueryRow(ctx, `
		WITH candidates AS MATERIALIZED (
		  SELECT id
		  FROM email_reply_requests
		  WHERE status='sending' AND claimed_at <= $1
		  ORDER BY claimed_at,id
		  LIMIT $2
		  FOR UPDATE SKIP LOCKED
		), recovered AS (
		  UPDATE email_reply_requests request
		  SET status='uncertain',
		      last_error='The mailbox provider outcome is unknown after an interrupted send.',
		      finalized_at=$3,updated_at=$3
		  FROM candidates
		  WHERE request.id=candidates.id AND request.status='sending'
		  RETURNING request.id
		)
		SELECT COUNT(*) FROM recovered
	`, s.now().UTC().Add(-staleReplyClaimAfter), batchSize, s.now().UTC()).Scan(&marked)
	if err != nil {
		return ReplyRecoverySummary{}, fmt.Errorf("recover stale email replies: %w", err)
	}
	return ReplyRecoverySummary{MarkedUncertain: marked}, nil
}

func (s *Service) ReplyOperationalStats(ctx context.Context) (ReplyOperationalStats, error) {
	if s == nil || s.pool == nil {
		return ReplyOperationalStats{}, fmt.Errorf("email messages service not configured")
	}
	var stats ReplyOperationalStats
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status='sending'),
		       COUNT(*) FILTER (WHERE status='sending' AND claimed_at <= $1),
		       COUNT(*) FILTER (WHERE status='uncertain')
		FROM email_reply_requests
	`, s.now().UTC().Add(-staleReplyClaimAfter)).Scan(&stats.Sending, &stats.StaleSending, &stats.Uncertain)
	if err != nil {
		return ReplyOperationalStats{}, fmt.Errorf("collect email reply operational stats: %w", err)
	}
	return stats, nil
}

func (s *Service) RunReplyRecoveryScheduler(ctx context.Context, logger *slog.Logger, interval time.Duration, observer ReplyRecoveryObserver) {
	if s == nil || s.pool == nil {
		return
	}
	if interval <= 0 {
		interval = defaultReplyRecoveryInterval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.RecoverStaleReplies(ctx, defaultReplyRecoveryBatch)
			outcome := "success"
			if err != nil {
				outcome = "error"
			}
			if observer != nil {
				observer.ObserveEmailReplyRecovery(outcome, summary.MarkedUncertain)
			}
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Error("email reply recovery failed", "error", err)
			} else if summary.MarkedUncertain > 0 && logger != nil {
				logger.Warn("stale email replies require operator resolution", "marked_uncertain", summary.MarkedUncertain)
			}
			timer.Reset(interval)
		}
	}
}
