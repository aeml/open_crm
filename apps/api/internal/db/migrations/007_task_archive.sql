ALTER TABLE tasks
ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_tasks_org_archived_status_due ON tasks (organization_id, archived_at, status, due_at);
