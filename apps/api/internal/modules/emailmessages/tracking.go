package emailmessages

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	defaultTrackingRetentionBatch    = 500
	maxTrackingRetentionBatch        = 5000
	defaultTrackingRetentionInterval = time.Hour
)

type TrackingRetentionSummary struct {
	MessagesPurged int64
}

type TrackingRetentionObserver interface {
	ObserveEmailTrackingRetention(outcome string, messagesPurged int64)
}

func validEmailTrackingToken(token string) bool {
	token = strings.TrimSpace(token)
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(decoded) == 32
}

// MarkOpenedByToken records an approximate open only while the sender-authorized
// collection window is active. Invalid, unknown, and expired tokens remain
// indistinguishable to the public pixel handler.
func (s *Service) MarkOpenedByToken(ctx context.Context, token string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email messages service not configured")
	}
	if !validEmailTrackingToken(token) {
		return nil
	}
	now := s.now().UTC()
	_, err := s.pool.Exec(ctx, `
		UPDATE email_messages
		SET open_count = LEAST(open_count, 2147483646) + 1,
		    first_opened_at = COALESCE(first_opened_at, $2),
		    last_opened_at = $2
		WHERE tracking_token = $1
		  AND engagement_tracking_enabled = TRUE
		  AND engagement_tracking_purged_at IS NULL
		  AND engagement_tracking_expires_at > $2
	`, strings.TrimSpace(token), now)
	if err != nil {
		return fmt.Errorf("mark email opened: %w", err)
	}
	return nil
}

// MarkClickedByToken always resolves a retained, validated redirect target, but
// increments observations only during the authorized collection window. This
// keeps links usable after tracking expires without extending collection.
func (s *Service) MarkClickedByToken(ctx context.Context, token string) (string, error) {
	if s == nil || s.pool == nil {
		return "", fmt.Errorf("email messages service not configured")
	}
	if !validEmailTrackingToken(token) {
		return "", ErrNotFound
	}
	now := s.now().UTC()
	var targetURL string
	err := s.pool.QueryRow(ctx, `
		WITH resolved AS MATERIALIZED (
			SELECT link.id, link.email_message_id, link.target_url,
			       message.engagement_tracking_enabled
			         AND message.engagement_tracking_purged_at IS NULL
			         AND message.engagement_tracking_expires_at > $2 AS collect
			FROM email_message_links link
			JOIN email_messages message ON message.id = link.email_message_id
			WHERE link.click_token = $1
			FOR UPDATE OF message
		), updated_link AS (
			UPDATE email_message_links link
			SET click_count = LEAST(link.click_count, 2147483646) + 1,
			    first_clicked_at = COALESCE(link.first_clicked_at, $2),
			    last_clicked_at = $2
			FROM resolved
			WHERE link.id = resolved.id AND resolved.collect
			RETURNING link.id
		), updated_message AS (
			UPDATE email_messages message
			SET click_count = LEAST(message.click_count, 2147483646) + 1,
			    first_clicked_at = COALESCE(message.first_clicked_at, $2),
			    last_clicked_at = $2
			FROM resolved
			WHERE message.id = resolved.email_message_id AND resolved.collect
			RETURNING message.id
		)
		SELECT target_url FROM resolved
	`, strings.TrimSpace(token), now).Scan(&targetURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("mark email clicked: %w", err)
	}
	return targetURL, nil
}

// ApplyTrackingRetention scrubs one bounded batch of expired engagement
// observations. Click tokens and validated destinations remain so links in old
// customer email continue to work without recording another event.
func (s *Service) ApplyTrackingRetention(ctx context.Context, batchSize int) (TrackingRetentionSummary, error) {
	if s == nil || s.pool == nil {
		return TrackingRetentionSummary{}, fmt.Errorf("email messages service not configured")
	}
	if batchSize == 0 {
		batchSize = defaultTrackingRetentionBatch
	}
	if batchSize < 0 || batchSize > maxTrackingRetentionBatch {
		return TrackingRetentionSummary{}, ErrInvalidInput
	}
	now := s.now().UTC()
	var purged int64
	err := s.pool.QueryRow(ctx, `
		WITH candidates AS MATERIALIZED (
			SELECT id
			FROM email_messages
			WHERE engagement_tracking_enabled = TRUE
			  AND engagement_tracking_purged_at IS NULL
			  AND engagement_tracking_expires_at <= $1
			ORDER BY engagement_tracking_expires_at, id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		), scrubbed_links AS (
			UPDATE email_message_links link
			SET click_count = 0, first_clicked_at = NULL, last_clicked_at = NULL
			FROM candidates
			WHERE link.email_message_id = candidates.id
			RETURNING link.id
		), scrubbed_messages AS (
			UPDATE email_messages message
			SET tracking_token = NULL,
			    open_count = 0, first_opened_at = NULL, last_opened_at = NULL,
			    click_count = 0, first_clicked_at = NULL, last_clicked_at = NULL,
			    engagement_tracking_purged_at = $1
			FROM candidates
			WHERE message.id = candidates.id
			RETURNING message.id
		)
		SELECT COUNT(*) FROM scrubbed_messages
	`, now, batchSize).Scan(&purged)
	if err != nil {
		return TrackingRetentionSummary{}, fmt.Errorf("purge expired email engagement tracking: %w", err)
	}
	return TrackingRetentionSummary{MessagesPurged: purged}, nil
}

func (s *Service) RunTrackingRetentionScheduler(ctx context.Context, logger *slog.Logger, interval time.Duration, observer TrackingRetentionObserver) {
	if s == nil || s.pool == nil {
		return
	}
	if interval <= 0 {
		interval = defaultTrackingRetentionInterval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.ApplyTrackingRetention(ctx, defaultTrackingRetentionBatch)
			outcome := "success"
			if err != nil {
				outcome = "error"
			}
			if observer != nil {
				observer.ObserveEmailTrackingRetention(outcome, summary.MessagesPurged)
			}
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Error("email tracking retention failed", "error", err, "messages_purged", summary.MessagesPurged)
			} else if summary.MessagesPurged > 0 && logger != nil {
				logger.Info("email tracking retention completed", "messages_purged", summary.MessagesPurged)
			}
			timer.Reset(interval)
		}
	}
}
