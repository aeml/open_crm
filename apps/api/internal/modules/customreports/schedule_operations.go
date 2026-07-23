package customreports

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	defaultArtifactCleanupInterval = time.Hour
	defaultArtifactCleanupLimit    = 100
	maxArtifactCleanupLimit        = 1000
)

type ScheduledDeliveryStats struct {
	ActiveSchedules  int64
	ActiveRuns       int64
	Uncertain        int64
	Failed24h        int64
	OldestOverdueAge int64
}

func (s *Service) ResolveRecipientDelivery(ctx context.Context, organizationID, actorUserID, deliveryID int64, input DeliveryResolutionInput) (DeliveryRun, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || actorUserID <= 0 || deliveryID <= 0 {
		return DeliveryRun{}, ErrInvalidInput
	}
	input.Resolution = strings.ToLower(strings.TrimSpace(input.Resolution))
	if input.Resolution != "confirmed_sent" && input.Resolution != "retry" {
		return DeliveryRun{}, ErrInvalidInput
	}
	if input.Resolution == "retry" && !s.deliveryAvailable() {
		return DeliveryRun{}, ErrDeliveryNotConfigured
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeliveryRun{}, fmt.Errorf("begin scheduled report resolution: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveReportAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return DeliveryRun{}, err
	}
	var runID, scheduleID, recipientUserID, scheduleRevision, currentRevision int64
	var status string
	var scheduleActive, recipientActive bool
	var artifact []byte
	var artifactExpiresAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT delivery.delivery_run_id,run.schedule_id,delivery.recipient_user_id,delivery.status,
		       run.schedule_revision,schedule.revision,schedule.is_active,
		       membership.membership_status='active' AND schedule_recipient.id IS NOT NULL,
		       run.artifact,run.artifact_expires_at
		FROM custom_report_recipient_deliveries delivery
		JOIN custom_report_delivery_runs run ON run.organization_id=delivery.organization_id AND run.id=delivery.delivery_run_id
		JOIN custom_report_schedules schedule ON schedule.organization_id=run.organization_id AND schedule.id=run.schedule_id
		JOIN organization_memberships membership ON membership.organization_id=delivery.organization_id AND membership.user_id=delivery.recipient_user_id
		LEFT JOIN custom_report_schedule_recipients schedule_recipient ON schedule_recipient.organization_id=delivery.organization_id AND schedule_recipient.schedule_id=run.schedule_id AND schedule_recipient.recipient_user_id=delivery.recipient_user_id
		WHERE delivery.organization_id=$1 AND delivery.id=$2
		FOR UPDATE OF delivery,run,schedule
	`, organizationID, deliveryID).Scan(&runID, &scheduleID, &recipientUserID, &status, &scheduleRevision, &currentRevision, &scheduleActive, &recipientActive, &artifact, &artifactExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeliveryRun{}, ErrNotFound
	}
	if err != nil {
		return DeliveryRun{}, fmt.Errorf("load scheduled report resolution: %w", err)
	}
	if input.Resolution == "confirmed_sent" {
		if status != "uncertain" {
			return DeliveryRun{}, ErrDeliveryNotRecoverable
		}
		if _, err := tx.Exec(ctx, `
			UPDATE custom_report_recipient_deliveries
			SET status='accepted',accepted_at=COALESCE(accepted_at,NOW()),last_error='',resolved_at=NOW(),resolved_by_user_id=$3,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2
		`, organizationID, deliveryID, actorUserID); err != nil {
			return DeliveryRun{}, err
		}
		if err := reopenAndFinalizeDeliveryRun(ctx, tx, organizationID, runID); err != nil {
			return DeliveryRun{}, err
		}
	} else {
		if status != "failed" && status != "uncertain" {
			return DeliveryRun{}, ErrDeliveryNotRecoverable
		}
		if status == "uncertain" && !input.ConfirmDuplicateRisk {
			return DeliveryRun{}, ErrInvalidInput
		}
		if !scheduleActive || scheduleRevision != currentRevision || !recipientActive || len(artifact) == 0 || artifactExpiresAt == nil || !artifactExpiresAt.After(s.currentTime()) {
			return DeliveryRun{}, ErrDeliveryNotRecoverable
		}
		if _, err := tx.Exec(ctx, `
			UPDATE custom_report_recipient_deliveries
			SET status='pending',provider_message_id='',last_error='',accepted_at=NULL,resolved_at=NOW(),resolved_by_user_id=$3,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2
		`, organizationID, deliveryID, actorUserID); err != nil {
			return DeliveryRun{}, err
		}
		var generation int
		if err := tx.QueryRow(ctx, `
			UPDATE custom_report_delivery_runs
			SET status='sending',recovery_generation=recovery_generation+1,last_error='',completed_at=NULL,updated_at=NOW()
			WHERE organization_id=$1 AND id=$2
			RETURNING recovery_generation
		`, organizationID, runID).Scan(&generation); err != nil {
			return DeliveryRun{}, err
		}
		key := "report-delivery:" + strconv.FormatInt(runID, 10) + ":recovery:" + strconv.Itoa(generation)
		if _, err := tx.Exec(ctx, `
			INSERT INTO background_jobs(organization_id,job_type,idempotency_key,payload_json,max_attempts,run_at)
			VALUES ($1,$2,$3,jsonb_build_object('deliveryRunId',$4::text),3,NOW())
		`, organizationID, ScheduledDeliveryJobType, key, strconv.FormatInt(runID, 10)); err != nil {
			return DeliveryRun{}, fmt.Errorf("enqueue scheduled report recovery: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events(organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'report_schedule.delivery_resolved','report_recipient_delivery',$3,'Resolved scheduled saved-report delivery',jsonb_build_object('deliveryRunId',$4::bigint,'scheduleId',$5::bigint,'recipientUserId',$6::bigint,'resolution',$7::text,'duplicateRiskConfirmed',$8::boolean))
	`, organizationID, actorUserID, deliveryID, runID, scheduleID, recipientUserID, input.Resolution, input.ConfirmDuplicateRisk); err != nil {
		return DeliveryRun{}, fmt.Errorf("audit scheduled report resolution: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return DeliveryRun{}, fmt.Errorf("commit scheduled report resolution: %w", err)
	}
	return deliveryRunResult(ctx, s.pool, organizationID, runID)
}

func reopenAndFinalizeDeliveryRun(ctx context.Context, tx pgx.Tx, organizationID, runID int64) error {
	if _, err := tx.Exec(ctx, `UPDATE custom_report_delivery_runs SET status='sending',completed_at=NULL,updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, runID); err != nil {
		return err
	}
	return finalizeDeliveryRun(ctx, tx, organizationID, runID)
}

func (s *Service) CleanupExpiredDeliveryArtifacts(ctx context.Context, limit int) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("custom reports service not configured")
	}
	if limit == 0 {
		limit = defaultArtifactCleanupLimit
	}
	if limit < 1 || limit > maxArtifactCleanupLimit {
		return 0, ErrInvalidInput
	}
	result, err := s.pool.Exec(ctx, `
		WITH candidates AS (
			SELECT id FROM custom_report_delivery_runs
			WHERE artifact IS NOT NULL AND artifact_expires_at <= $1
			  AND status IN ('succeeded','partial','failed','canceled')
			ORDER BY artifact_expires_at,id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE custom_report_delivery_runs run
		SET artifact=NULL,filename='',content_sha256='',byte_size=0,artifact_expires_at=NULL,updated_at=NOW()
		FROM candidates WHERE run.id=candidates.id
	`, s.currentTime(), limit)
	if err != nil {
		return 0, fmt.Errorf("clean scheduled report artifacts: %w", err)
	}
	return result.RowsAffected(), nil
}

func (s *Service) RunDeliveryArtifactCleanupScheduler(ctx context.Context, logger *slog.Logger, interval time.Duration) {
	if s == nil || s.pool == nil {
		return
	}
	if interval <= 0 {
		interval = defaultArtifactCleanupInterval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			purged, err := s.CleanupExpiredDeliveryArtifacts(ctx, 0)
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Error("scheduled report artifact cleanup failed", "error", err)
			} else if purged > 0 && logger != nil {
				logger.Info("scheduled report artifacts removed", "count", purged)
			}
			timer.Reset(interval)
		}
	}
}

func (s *Service) ScheduledDeliveryOperationalStats(ctx context.Context) (ScheduledDeliveryStats, error) {
	if s == nil || s.pool == nil {
		return ScheduledDeliveryStats{}, fmt.Errorf("custom reports service not configured")
	}
	var stats ScheduledDeliveryStats
	err := s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM custom_report_schedules WHERE is_active),
		  (SELECT COUNT(*) FROM custom_report_delivery_runs WHERE status IN ('pending','sending')),
		  (SELECT COUNT(*) FROM custom_report_recipient_deliveries WHERE status='uncertain'),
		  (SELECT COUNT(*) FROM custom_report_recipient_deliveries WHERE status='failed' AND updated_at >= NOW()-INTERVAL '24 hours'),
		  COALESCE((SELECT EXTRACT(EPOCH FROM GREATEST(NOW()-MIN(next_run_at),INTERVAL '0 seconds'))::bigint FROM custom_report_schedules WHERE is_active AND next_run_at<NOW()),0)
	`).Scan(&stats.ActiveSchedules, &stats.ActiveRuns, &stats.Uncertain, &stats.Failed24h, &stats.OldestOverdueAge)
	if err != nil {
		return ScheduledDeliveryStats{}, fmt.Errorf("load scheduled report operational stats: %w", err)
	}
	return stats, nil
}
