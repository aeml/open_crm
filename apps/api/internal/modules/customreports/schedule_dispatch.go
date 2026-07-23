package customreports

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	"github.com/jackc/pgx/v5"
)

const (
	defaultScheduleDiscoveryLimit    = 50
	maxScheduleDiscoveryLimit        = 100
	defaultScheduleDiscoveryInterval = time.Minute
)

func (s *Service) EnqueueDueDeliveries(ctx context.Context, limit int) (int, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("custom reports service not configured")
	}
	if limit == 0 {
		limit = defaultScheduleDiscoveryLimit
	}
	if limit < 1 || limit > maxScheduleDiscoveryLimit {
		return 0, ErrInvalidInput
	}
	now := s.currentTime()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin scheduled report discovery: %w", err)
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, scheduleSelect+`
		WHERE schedule.is_active AND schedule.next_run_at IS NOT NULL AND schedule.next_run_at <= $1
		ORDER BY schedule.next_run_at,schedule.id
		LIMIT $2
		FOR UPDATE OF schedule SKIP LOCKED
	`, now, limit)
	if err != nil {
		return 0, fmt.Errorf("discover due report schedules: %w", err)
	}
	due := []ReportSchedule{}
	for rows.Next() {
		schedule, scanErr := scanSchedule(rows)
		if scanErr != nil {
			rows.Close()
			return 0, scanErr
		}
		due = append(due, schedule)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate due report schedules: %w", err)
	}
	if len(due) > 0 && !s.deliveryAvailable() {
		return 0, ErrDeliveryNotConfigured
	}
	enqueued := 0
	for _, schedule := range due {
		if schedule.NextRunAt == nil {
			continue
		}
		var runID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO custom_report_delivery_runs(
				organization_id,schedule_id,report_definition_id,schedule_revision,scheduled_for
			) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (organization_id,schedule_id,scheduled_for) DO NOTHING
			RETURNING id
		`, schedule.OrganizationID, schedule.ID, schedule.ReportDefinitionID, schedule.Revision, *schedule.NextRunAt).Scan(&runID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("create scheduled report occurrence: %w", err)
		}
		if err == nil {
			if _, err := tx.Exec(ctx, `
				INSERT INTO custom_report_recipient_deliveries(organization_id,delivery_run_id,recipient_user_id)
				SELECT recipient.organization_id,$1,recipient.recipient_user_id
				FROM custom_report_schedule_recipients recipient
				JOIN organization_memberships membership
				  ON membership.organization_id=recipient.organization_id
				 AND membership.user_id=recipient.recipient_user_id
				WHERE recipient.organization_id=$2 AND recipient.schedule_id=$3 AND membership.membership_status='active'
				ON CONFLICT (organization_id,delivery_run_id,recipient_user_id) DO NOTHING
			`, runID, schedule.OrganizationID, schedule.ID); err != nil {
				return 0, fmt.Errorf("capture scheduled report recipients: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO background_jobs(organization_id,job_type,idempotency_key,payload_json,max_attempts,run_at)
				SELECT organization_id,$3,$4,jsonb_build_object('deliveryRunId',$5::text),3,$6
				FROM custom_report_delivery_runs WHERE organization_id=$1 AND id=$2
			`, schedule.OrganizationID, runID, ScheduledDeliveryJobType, "report-delivery:"+strconv.FormatInt(runID, 10)+":initial", strconv.FormatInt(runID, 10), now); err != nil {
				return 0, fmt.Errorf("enqueue scheduled report delivery: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO audit_events(organization_id,event_type,entity_type,entity_id,summary,metadata_json)
				SELECT organization_id,'report_schedule.queued','report_delivery_run',$2,'Queued scheduled saved-report delivery',
				       jsonb_build_object('scheduleId',schedule_id,'reportDefinitionId',report_definition_id,'scheduleRevision',schedule_revision,'scheduledFor',scheduled_for)
				FROM custom_report_delivery_runs WHERE organization_id=$1 AND id=$2
			`, schedule.OrganizationID, runID); err != nil {
				return 0, fmt.Errorf("audit scheduled report occurrence: %w", err)
			}
			enqueued++
		}
		next := nextRunFromSchedule(schedule, now)
		if _, err := tx.Exec(ctx, `UPDATE custom_report_schedules SET next_run_at=$3,updated_at=updated_at WHERE organization_id=$1 AND id=$2`, schedule.OrganizationID, schedule.ID, next); err != nil {
			return 0, fmt.Errorf("advance report schedule: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit scheduled report discovery: %w", err)
	}
	return enqueued, nil
}

func (s *Service) RunDeliveryScheduler(ctx context.Context, logger *slog.Logger, interval time.Duration) {
	if s == nil || s.pool == nil {
		return
	}
	if interval <= 0 {
		interval = defaultScheduleDiscoveryInterval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			enqueued, err := s.EnqueueDueDeliveries(ctx, 0)
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Error("scheduled report discovery failed", "error", err)
			} else if enqueued > 0 && logger != nil {
				logger.Info("scheduled report deliveries queued", "count", enqueued)
			}
			timer.Reset(interval)
		}
	}
}

func ScheduledDeliveryPermanentFailure(err error) bool {
	return errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInactive) ||
		errors.Is(err, ErrUnsupportedVisualization) || errors.Is(err, ErrTooManyRows) ||
		errors.Is(err, ErrScheduledArtifactTooLarge) || errors.Is(err, ErrDeliveryNotConfigured)
}

func ScheduledDeliveryDeferred(err error) bool {
	return errors.Is(err, ErrDeliveryInProgress)
}

func deliveryRunID(job modulejobs.Job) (int64, error) {
	value, ok := job.Payload["deliveryRunId"]
	if !ok {
		return 0, ErrInvalidInput
	}
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case float64:
		text = strconv.FormatInt(int64(typed), 10)
	default:
		return 0, ErrInvalidInput
	}
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrInvalidInput
	}
	return id, nil
}
