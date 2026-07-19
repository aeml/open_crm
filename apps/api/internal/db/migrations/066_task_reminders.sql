-- open-crm-deploy: expand
-- Task reminders are durable, versioned against the task scheduling fields,
-- and use the shared job queue. Superseded reminders remain as history while
-- their pending jobs are completed as safe no-ops.

ALTER TABLE tasks
  ADD COLUMN reminder_version INTEGER DEFAULT 0
    CHECK (reminder_version >= 0);

CREATE TABLE task_reminders (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  reminder_type TEXT NOT NULL,
  reminder_version INTEGER NOT NULL,
  due_at TIMESTAMPTZ NOT NULL,
  remind_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  delivered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT task_reminders_type_check CHECK (reminder_type IN ('due_soon', 'overdue')),
  CONSTRAINT task_reminders_version_check CHECK (reminder_version >= 0),
  CONSTRAINT task_reminders_status_check CHECK (status IN ('pending', 'sent', 'skipped')),
  UNIQUE (organization_id, task_id, reminder_version, reminder_type)
);

CREATE INDEX idx_task_reminders_due
  ON task_reminders(status, remind_at, id)
  WHERE status = 'pending';

CREATE INDEX idx_task_reminders_org_task
  ON task_reminders(organization_id, task_id, reminder_version, status);

CREATE INDEX idx_task_reminders_org_user
  ON task_reminders(organization_id, user_id, status, remind_at);

-- Existing future tasks receive a due-soon and overdue reminder. Existing
-- overdue tasks receive only the overdue reminder so enabling the feature does
-- not create two immediate notifications for the same task.
INSERT INTO task_reminders (
  organization_id, task_id, user_id, reminder_type, reminder_version, due_at, remind_at
)
SELECT organization_id, id, assigned_to_user_id, 'due_soon', COALESCE(reminder_version, 0),
       due_at, GREATEST(NOW(), due_at - INTERVAL '24 hours')
FROM tasks
WHERE archived_at IS NULL AND status = 'open' AND assigned_to_user_id IS NOT NULL
  AND due_at > NOW()
ON CONFLICT DO NOTHING;

INSERT INTO task_reminders (
  organization_id, task_id, user_id, reminder_type, reminder_version, due_at, remind_at
)
SELECT organization_id, id, assigned_to_user_id, 'overdue', COALESCE(reminder_version, 0),
       due_at, GREATEST(NOW(), due_at)
FROM tasks
WHERE archived_at IS NULL AND status = 'open' AND assigned_to_user_id IS NOT NULL
  AND due_at IS NOT NULL
ON CONFLICT DO NOTHING;

INSERT INTO background_jobs (
  organization_id, job_type, idempotency_key, payload_json, max_attempts, run_at
)
SELECT organization_id, 'task.reminder', 'task-reminder:' || id::text,
       jsonb_build_object('reminderId', id::text), 5, remind_at
FROM task_reminders
WHERE status = 'pending'
ON CONFLICT (organization_id, job_type, idempotency_key) DO NOTHING;
