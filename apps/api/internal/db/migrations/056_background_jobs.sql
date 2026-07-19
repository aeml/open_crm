-- open-crm-deploy: expand
-- Durable tenant-scoped work queue with leased claims, bounded retries, dead
-- letters, and operator replay. Idempotency keys are stable for the lifetime of
-- a job, including after success, so a repeated enqueue cannot repeat an effect.

CREATE TABLE background_jobs (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  job_type TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending',
  priority INTEGER NOT NULL DEFAULT 0,
  attempts INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 5,
  run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  locked_at TIMESTAMPTZ,
  locked_by TEXT,
  lock_token TEXT,
  lease_expires_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT '',
  result_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT background_jobs_type_check CHECK (BTRIM(job_type) <> '' AND CHAR_LENGTH(job_type) <= 100),
  CONSTRAINT background_jobs_idempotency_key_check CHECK (BTRIM(idempotency_key) <> '' AND CHAR_LENGTH(idempotency_key) <= 255),
  CONSTRAINT background_jobs_payload_json_object_check CHECK (jsonb_typeof(payload_json) = 'object'),
  CONSTRAINT background_jobs_status_check CHECK (status IN ('pending', 'running', 'retryable', 'succeeded', 'dead')),
  CONSTRAINT background_jobs_priority_check CHECK (priority BETWEEN -100 AND 100),
  CONSTRAINT background_jobs_attempts_check CHECK (attempts >= 0 AND attempts <= max_attempts),
  CONSTRAINT background_jobs_max_attempts_check CHECK (max_attempts BETWEEN 1 AND 25),
  CONSTRAINT background_jobs_result_json_object_check CHECK (jsonb_typeof(result_json) = 'object'),
  CONSTRAINT background_jobs_lock_state_check CHECK (
    (status = 'running' AND locked_at IS NOT NULL AND locked_by IS NOT NULL AND lock_token IS NOT NULL AND lease_expires_at IS NOT NULL)
    OR
    (status <> 'running' AND locked_at IS NULL AND locked_by IS NULL AND lock_token IS NULL AND lease_expires_at IS NULL)
  ),
  CONSTRAINT background_jobs_completion_state_check CHECK (
    (status = 'succeeded' AND completed_at IS NOT NULL)
    OR
    (status <> 'succeeded' AND completed_at IS NULL)
  ),
  UNIQUE (organization_id, job_type, idempotency_key)
);

CREATE INDEX idx_background_jobs_claim
  ON background_jobs(priority DESC, run_at ASC, id ASC)
  WHERE status IN ('pending', 'retryable');

CREATE INDEX idx_background_jobs_expired_leases
  ON background_jobs(lease_expires_at ASC, id ASC)
  WHERE status = 'running';

CREATE INDEX idx_background_jobs_org_status_created
  ON background_jobs(organization_id, status, created_at DESC, id DESC);

CREATE INDEX idx_background_jobs_type_status_run
  ON background_jobs(job_type, status, run_at ASC, id ASC);

-- Reminders created before this migration become durable jobs during the same
-- transaction. New reminders enqueue their job when the event is scheduled.
INSERT INTO background_jobs (organization_id, job_type, idempotency_key, payload_json, max_attempts, run_at)
SELECT organization_id,
       'calendar.reminder',
       'reminder:' || id::text,
       jsonb_build_object('reminderId', id::text),
       5,
       remind_at
FROM calendar_event_reminders
WHERE status = 'pending'
ON CONFLICT (organization_id, job_type, idempotency_key) DO NOTHING;
