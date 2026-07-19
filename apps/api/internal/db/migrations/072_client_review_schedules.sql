-- open-crm-deploy: expand
-- Persist one task-backed review or renewal obligation per active client.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_org_id_unique
  ON tasks(organization_id, id);

CREATE TABLE IF NOT EXISTS client_review_schedules (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL CHECK (entity_type IN ('contact', 'company')),
  entity_id BIGINT NOT NULL,
  review_type TEXT NOT NULL CHECK (review_type IN ('review', 'renewal')),
  next_review_at TIMESTAMPTZ NOT NULL,
  cadence_months INTEGER NOT NULL DEFAULT 0 CHECK (cadence_months IN (0, 1, 3, 6, 12)),
  current_task_id BIGINT NOT NULL,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT client_review_schedules_entity_unique UNIQUE (organization_id, entity_type, entity_id),
  CONSTRAINT client_review_schedules_task_unique UNIQUE (organization_id, current_task_id),
  CONSTRAINT client_review_schedules_task_org_fk FOREIGN KEY (organization_id, current_task_id)
    REFERENCES tasks(organization_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_client_review_schedules_org_active_due
  ON client_review_schedules(organization_id, next_review_at, entity_type, entity_id)
  WHERE completed_at IS NULL;
