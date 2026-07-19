package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultJobTimeout   = 4 * time.Minute
	maxRetryDelay       = time.Hour
)

type Handler func(context.Context, Job) (map[string]any, error)

type queueStore interface {
	Claim(context.Context, string, []string, int, time.Duration) ([]Job, error)
	Complete(context.Context, Job, map[string]any) (Job, error)
	Defer(context.Context, Job, error, time.Time) (Job, error)
	Fail(context.Context, Job, error, time.Time) (Job, error)
	DeadLetter(context.Context, Job, error) (Job, error)
}

type Observer interface {
	ObserveJob(jobType, outcome string)
}

type permanentError struct {
	err error
}

type deferredError struct {
	err     error
	retryAt time.Time
}

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

// Deferred returns a worker result that safely releases a claimed job until
// retryAt without consuming an attempt. Use it when policy or another expected
// external state temporarily prevents execution rather than when work failed.
func Deferred(err error, retryAt time.Time) error {
	if err == nil {
		return nil
	}
	return deferredError{err: err, retryAt: retryAt}
}

func (e deferredError) Error() string { return e.err.Error() }
func (e deferredError) Unwrap() error { return e.err }

type Worker struct {
	store        queueStore
	handlers     map[string]Handler
	jobTypes     []string
	workerID     string
	logger       *slog.Logger
	pollInterval time.Duration
	lease        time.Duration
	jobTimeout   time.Duration
	now          func() time.Time
	observer     Observer
}

type RunSummary struct {
	Claimed   int
	Succeeded int
	Deferred  int
	Retried   int
	Dead      int
}

func NewWorker(store queueStore, handlers map[string]Handler, workerID string, logger *slog.Logger, observers ...Observer) *Worker {
	types := make([]string, 0, len(handlers))
	for jobType := range handlers {
		if strings.TrimSpace(jobType) != "" {
			types = append(types, jobType)
		}
	}
	sort.Strings(types)
	worker := &Worker{
		store:        store,
		handlers:     handlers,
		jobTypes:     types,
		workerID:     strings.TrimSpace(workerID),
		logger:       logger,
		pollInterval: defaultPollInterval,
		lease:        defaultLease,
		jobTimeout:   defaultJobTimeout,
		now:          time.Now,
	}
	if len(observers) > 0 {
		worker.observer = observers[0]
	}
	return worker
}

func (w *Worker) RunOnce(ctx context.Context) (RunSummary, error) {
	if w == nil || w.store == nil || w.workerID == "" || len(w.jobTypes) == 0 {
		return RunSummary{}, ErrInvalidInput
	}
	claimed, err := w.store.Claim(ctx, w.workerID, w.jobTypes, 1, w.lease)
	if err != nil {
		w.observe("_worker", "cycle_error")
		return RunSummary{}, err
	}
	summary := RunSummary{Claimed: len(claimed)}
	for _, job := range claimed {
		outcome, err := w.execute(ctx, job)
		if err != nil {
			w.observe("_worker", "cycle_error")
			return summary, err
		}
		w.observe(job.Type, outcome)
		switch outcome {
		case "succeeded":
			summary.Succeeded++
		case "deferred":
			summary.Deferred++
		case "retryable":
			summary.Retried++
		case "dead":
			summary.Dead++
		}
	}
	return summary, nil
}

func (w *Worker) Run(ctx context.Context) {
	if w == nil || w.store == nil || w.workerID == "" || len(w.jobTypes) == 0 {
		return
	}
	interval := w.pollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := w.RunOnce(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				w.log(slog.LevelWarn, "background job worker cycle failed", "error", err)
			} else if summary.Claimed > 0 {
				w.log(slog.LevelInfo, "background job worker cycle completed", "claimed", summary.Claimed, "succeeded", summary.Succeeded, "deferred", summary.Deferred, "retried", summary.Retried, "dead", summary.Dead)
			}
			timer.Reset(interval)
		}
	}
}

func (w *Worker) execute(ctx context.Context, job Job) (outcome string, err error) {
	handler := w.handlers[job.Type]
	if handler == nil {
		_, finishErr := w.store.DeadLetter(ctx, job, fmt.Errorf("no handler registered for job type %q", job.Type))
		return "dead", finishErr
	}
	timeout := w.jobTimeout
	if timeout <= 0 || timeout >= w.lease {
		timeout = defaultJobTimeout
	}
	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, handlerErr := callHandler(jobCtx, handler, job)
	if handlerErr == nil {
		if _, err := w.store.Complete(ctx, job, result); err != nil {
			return "", err
		}
		return "succeeded", nil
	}
	if ctx.Err() != nil {
		// Leave the lease intact. Another worker can reclaim it after a graceful
		// shutdown interrupted the handler.
		return "", ctx.Err()
	}
	var deferred deferredError
	if errors.As(handlerErr, &deferred) {
		updated, err := w.store.Defer(ctx, job, handlerErr, deferred.retryAt)
		if err != nil {
			return "", err
		}
		if updated.Status == "dead" {
			return "dead", nil
		}
		return "deferred", nil
	}
	var permanent permanentError
	if errors.As(handlerErr, &permanent) {
		if _, err := w.store.DeadLetter(ctx, job, handlerErr); err != nil {
			return "", err
		}
		return "dead", nil
	}
	retryAt := w.now().Add(retryDelay(job.Attempts))
	updated, err := w.store.Fail(ctx, job, handlerErr, retryAt)
	if err != nil {
		return "", err
	}
	if updated.Status == "dead" {
		return "dead", nil
	}
	return "retryable", nil
}

func callHandler(ctx context.Context, handler Handler, job Job) (result map[string]any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("background job handler panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	return handler(ctx, job)
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	minutes := math.Pow(2, float64(attempt-1))
	delay := time.Duration(minutes) * time.Minute
	if delay > maxRetryDelay {
		return maxRetryDelay
	}
	return delay
}

func (w *Worker) log(level slog.Level, message string, args ...any) {
	if w.logger != nil {
		w.logger.Log(context.Background(), level, message, args...)
	}
}

func (w *Worker) observe(jobType, outcome string) {
	if w.observer != nil {
		w.observer.ObserveJob(jobType, outcome)
	}
}
