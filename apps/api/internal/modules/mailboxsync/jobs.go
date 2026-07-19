package mailboxsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

const (
	MailboxSyncJobType       = "mailbox.sync"
	mailboxSchedulerInterval = time.Minute
)

var ErrInvalidJobPayload = errors.New("invalid mailbox sync job payload")

type jobQueue interface {
	Enqueue(context.Context, modulejobs.EnqueueInput) (modulejobs.Job, error)
}

type ScheduleSummary struct {
	Due       int
	Scheduled int
}

func (s *Service) ScheduleDueJobs(ctx context.Context, queue jobQueue, limit int) (ScheduleSummary, error) {
	if !s.Configured() || queue == nil {
		return ScheduleSummary{}, ErrNotConfigured
	}
	targetsStore, ok := s.accounts.(syncTargetStore)
	if !ok {
		return ScheduleSummary{}, ErrNotConfigured
	}
	if limit <= 0 || limit > 100 {
		limit = defaultBatchLimit
	}
	targets, err := targetsStore.ListSyncTargets(ctx, limit)
	if err != nil {
		return ScheduleSummary{}, err
	}
	summary := ScheduleSummary{Due: len(targets)}
	for _, target := range targets {
		if target.OrganizationID <= 0 || target.UserID <= 0 || target.DueAt.IsZero() {
			return summary, fmt.Errorf("%w: invalid mailbox sync target", ErrInvalidJobPayload)
		}
		_, err := queue.Enqueue(ctx, modulejobs.EnqueueInput{
			OrganizationID: target.OrganizationID,
			Type:           MailboxSyncJobType,
			IdempotencyKey: fmt.Sprintf("user:%d:due:%s", target.UserID, target.DueAt.UTC().Format(time.RFC3339Nano)),
			Payload:        map[string]any{"userId": strconv.FormatInt(target.UserID, 10)},
			MaxAttempts:    5,
		})
		if err != nil {
			return summary, fmt.Errorf("schedule mailbox sync job: %w", err)
		}
		summary.Scheduled++
	}
	return summary, nil
}

func (s *Service) HandleJob(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
	value, ok := job.Payload["userId"].(string)
	if !ok || job.OrganizationID <= 0 {
		return nil, ErrInvalidJobPayload
	}
	userID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || userID <= 0 {
		return nil, ErrInvalidJobPayload
	}
	result, err := s.SyncUser(ctx, job.OrganizationID, userID)
	if err != nil {
		return nil, err
	}
	if result.Status == "error" {
		return nil, fmt.Errorf("mailbox sync did not complete: %s", result.Error)
	}
	return map[string]any{"status": result.Status, "imported": result.Imported, "userId": value}, nil
}

func (s *Service) RunJobScheduler(ctx context.Context, queue jobQueue, logger *slog.Logger, interval time.Duration, limit int) {
	if !s.Configured() || queue == nil {
		return
	}
	if interval <= 0 {
		interval = mailboxSchedulerInterval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.ScheduleDueJobs(ctx, queue, limit)
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Warn("mailbox sync job scheduling failed", "error", err)
			} else if summary.Due > 0 && logger != nil {
				logger.Info("mailbox sync jobs scheduled", "due", summary.Due, "scheduled", summary.Scheduled)
			}
			timer.Reset(interval)
		}
	}
}
