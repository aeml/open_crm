-- open-crm-deploy: expand
-- Bound aggregate durable-workflow health queries without tenant or record labels.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

CREATE INDEX idx_workflow_automation_runs_active_created
  ON workflow_automation_runs(status, created_at)
  WHERE status IN ('queued', 'running');

CREATE INDEX idx_workflow_automation_runs_terminal_recent
  ON workflow_automation_runs(status, completed_at DESC)
  WHERE status IN ('failed', 'skipped');
