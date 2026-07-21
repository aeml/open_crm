-- open-crm-deploy: expand
-- Retain the immutable planned execution time for durable workflow runs.
-- Existing runs were immediate, so their original start/creation time is the
-- truthful schedule when this expand-only column is introduced.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE workflow_automation_runs
  ADD COLUMN scheduled_at TIMESTAMPTZ;

ALTER TABLE workflow_automation_runs
  ALTER COLUMN scheduled_at SET DEFAULT NOW();

UPDATE workflow_automation_runs
SET scheduled_at = COALESCE(started_at, created_at);

ALTER TABLE workflow_automation_runs
  ADD CONSTRAINT workflow_automation_runs_scheduled_at_present
  CHECK (scheduled_at IS NOT NULL) NOT VALID;

ALTER TABLE workflow_automation_runs
  VALIDATE CONSTRAINT workflow_automation_runs_scheduled_at_present;

CREATE INDEX idx_workflow_automation_runs_active_scheduled
  ON workflow_automation_runs(status, scheduled_at)
  WHERE status IN ('queued', 'running');
