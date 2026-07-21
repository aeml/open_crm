-- Bound aggregate durable-workflow health queries without tenant or record labels.

CREATE INDEX idx_workflow_automation_runs_active_created
  ON workflow_automation_runs(status, created_at)
  WHERE status IN ('queued', 'running');

CREATE INDEX idx_workflow_automation_runs_terminal_recent
  ON workflow_automation_runs(status, completed_at DESC)
  WHERE status IN ('failed', 'skipped');
