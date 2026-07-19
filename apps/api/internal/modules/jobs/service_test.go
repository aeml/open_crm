package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeEnqueueInputDefaultsDurabilityControls(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	input := normalizeEnqueueInput(EnqueueInput{
		OrganizationID: 42,
		Type:           " mailbox.sync ",
		IdempotencyKey: " mailbox:7:2026-07-19T12:00 ",
	}, now)
	if input.Type != "mailbox.sync" || input.IdempotencyKey != "mailbox:7:2026-07-19T12:00" || input.MaxAttempts != defaultMaxAttempts || !input.RunAt.Equal(now) || input.Payload == nil {
		t.Fatalf("unexpected normalized job input: %#v", input)
	}
	if !validEnqueueInput(input) {
		t.Fatal("expected normalized job input to be valid")
	}
}

func TestNormalizeJobTypesDropsInvalidAndDuplicateValues(t *testing.T) {
	long := strings.Repeat("x", 101)
	types := normalizeJobTypes([]string{" mailbox.sync ", "", "mailbox.sync", long, "calendar.reminder"})
	if len(types) != 2 || types[0] != "mailbox.sync" || types[1] != "calendar.reminder" {
		t.Fatalf("unexpected normalized job types: %#v", types)
	}
}

func TestRetryDelayIsBounded(t *testing.T) {
	if retryDelay(1) != time.Minute || retryDelay(2) != 2*time.Minute || retryDelay(20) != maxRetryDelay {
		t.Fatal("expected exponential retry delay capped at one hour")
	}
}

type fakeQueueStore struct {
	claimed      []Job
	completed    int
	failed       int
	dead         int
	failureState string
}

type fakeJobObserver struct {
	jobType string
	outcome string
}

func (o *fakeJobObserver) ObserveJob(jobType, outcome string) {
	o.jobType = jobType
	o.outcome = outcome
}

func (f *fakeQueueStore) Claim(context.Context, string, []string, int, time.Duration) ([]Job, error) {
	return f.claimed, nil
}

func (f *fakeQueueStore) Complete(_ context.Context, _ Job, _ map[string]any) (Job, error) {
	f.completed++
	return Job{Status: "succeeded"}, nil
}

func (f *fakeQueueStore) Fail(_ context.Context, _ Job, _ error, _ time.Time) (Job, error) {
	f.failed++
	status := f.failureState
	if status == "" {
		status = "retryable"
	}
	return Job{Status: status}, nil
}

func (f *fakeQueueStore) DeadLetter(_ context.Context, _ Job, _ error) (Job, error) {
	f.dead++
	return Job{Status: "dead"}, nil
}

func TestWorkerCompletesSuccessfulJob(t *testing.T) {
	store := &fakeQueueStore{claimed: []Job{{ID: 1, Type: "test", Attempts: 1, LockToken: "claim"}}}
	observer := &fakeJobObserver{}
	worker := NewWorker(store, map[string]Handler{"test": func(context.Context, Job) (map[string]any, error) {
		return map[string]any{"worked": true}, nil
	}}, "worker-1", nil, observer)

	summary, err := worker.RunOnce(context.Background())
	if err != nil || summary.Succeeded != 1 || store.completed != 1 || store.failed != 0 || store.dead != 0 {
		t.Fatalf("unexpected successful worker result: summary=%#v store=%#v err=%v", summary, store, err)
	}
	if observer.jobType != "test" || observer.outcome != "succeeded" {
		t.Fatalf("unexpected observed job outcome: %+v", observer)
	}
}

func TestWorkerRetriesOrdinaryFailureAndDeadLettersPermanentFailure(t *testing.T) {
	for _, test := range []struct {
		name        string
		handlerErr  error
		expectRetry int
		expectDead  int
	}{
		{name: "retryable", handlerErr: errors.New("provider unavailable"), expectRetry: 1},
		{name: "permanent", handlerErr: Permanent(errors.New("invalid payload")), expectDead: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeQueueStore{claimed: []Job{{ID: 1, Type: "test", Attempts: 1, LockToken: "claim"}}}
			worker := NewWorker(store, map[string]Handler{"test": func(context.Context, Job) (map[string]any, error) {
				return nil, test.handlerErr
			}}, "worker-1", nil)
			summary, err := worker.RunOnce(context.Background())
			if err != nil || store.failed != test.expectRetry || store.dead != test.expectDead || summary.Retried != test.expectRetry || summary.Dead != test.expectDead {
				t.Fatalf("unexpected failure result: summary=%#v store=%#v err=%v", summary, store, err)
			}
		})
	}
}

func TestWorkerRecoversPanicsAsRetryableFailures(t *testing.T) {
	store := &fakeQueueStore{claimed: []Job{{ID: 1, Type: "test", Attempts: 1, LockToken: "claim"}}}
	worker := NewWorker(store, map[string]Handler{"test": func(context.Context, Job) (map[string]any, error) {
		panic("boom")
	}}, "worker-1", nil)
	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("expected recovered panic to become a retry, got %v", err)
	}
	if store.failed != 1 {
		t.Fatalf("expected recovered panic to fail the job once, got %#v", store)
	}
}
