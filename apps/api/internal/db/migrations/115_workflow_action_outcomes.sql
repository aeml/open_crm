-- open-crm-deploy: expand
-- Retain one immutable, tenant-bound lifecycle row for every supported workflow
-- task action. This makes run inspection independent of later definition edits
-- and gives future action families a typed execution-evidence boundary.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

CREATE UNIQUE INDEX idx_workflow_automation_runs_org_id_unique
  ON workflow_automation_runs(organization_id, id);

CREATE TABLE workflow_automation_action_outcomes (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL,
  run_id BIGINT NOT NULL,
  action_position INTEGER NOT NULL,
  action_type TEXT NOT NULL,
  action_label TEXT NOT NULL,
  status TEXT NOT NULL,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  scheduled_at TIMESTAMPTZ NOT NULL,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  task_id BIGINT,
  task_due_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT workflow_action_outcomes_run_fk
    FOREIGN KEY (organization_id, run_id)
    REFERENCES workflow_automation_runs(organization_id, id)
    ON DELETE CASCADE,
  CONSTRAINT workflow_action_outcomes_position_check
    CHECK (action_position BETWEEN 1 AND 25),
  CONSTRAINT workflow_action_outcomes_type_check
    CHECK (length(trim(action_type)) BETWEEN 1 AND 64),
  CONSTRAINT workflow_action_outcomes_label_check
    CHECK (length(trim(action_label)) BETWEEN 1 AND 200),
  CONSTRAINT workflow_action_outcomes_status_check
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'skipped', 'cancelled')),
  CONSTRAINT workflow_action_outcomes_attempt_count_check
    CHECK (attempt_count >= 0),
  CONSTRAINT workflow_action_outcomes_task_id_check
    CHECK (task_id IS NULL OR task_id > 0),
  CONSTRAINT workflow_action_outcomes_task_due_check
    CHECK (task_due_at IS NULL OR task_id IS NOT NULL),
  CONSTRAINT workflow_action_outcomes_last_error_check
    CHECK (length(last_error) <= 2000),
  CONSTRAINT workflow_action_outcomes_terminal_completed_check
    CHECK (
      (status IN ('succeeded', 'failed', 'skipped', 'cancelled') AND completed_at IS NOT NULL)
      OR (status IN ('queued', 'running') AND completed_at IS NULL)
    )
);

CREATE UNIQUE INDEX idx_workflow_action_outcomes_org_run_position_unique
  ON workflow_automation_action_outcomes(organization_id, run_id, action_position);

-- Older runs did not retain immutable action labels. Backfill the exact lead
-- snapshot where it exists and otherwise use an explicit historical label;
-- never infer a mutable current definition as past execution evidence.
INSERT INTO workflow_automation_action_outcomes (
  organization_id, run_id, action_position, action_type, action_label, status,
  attempt_count, scheduled_at, started_at, completed_at, task_id, task_due_at,
  last_error, created_at, updated_at
)
SELECT run.organization_id,
       run.id,
       action_position,
       CASE
         WHEN run.target_entity_type IN ('deal', 'lead_form') THEN 'create_task'
         ELSE 'historical_action'
       END,
       CASE
         WHEN run.target_entity_type = 'lead_form'
           THEN COALESCE(
             NULLIF(trim(run.trigger_payload_json #>> '{definition,action,config,title}'), ''),
             'Historical task action'
           )
         ELSE 'Historical action ' || action_position::text
       END,
       run.status,
       CASE
         WHEN run.status = 'succeeded' THEN GREATEST(run.retry_count + 1, 1)
         WHEN run.status IN ('running', 'failed') THEN GREATEST(run.retry_count + 1, 1)
         ELSE run.retry_count
       END,
       COALESCE(run.scheduled_at, run.created_at),
       CASE WHEN run.status = 'queued' THEN NULL ELSE COALESCE(run.started_at, run.created_at) END,
       CASE
         WHEN run.status IN ('succeeded', 'failed', 'skipped', 'cancelled')
           THEN COALESCE(run.completed_at, run.updated_at)
         ELSE NULL
       END,
       task.id,
       task.due_at,
       LEFT(COALESCE(
         NULLIF(run.last_error, ''),
         NULLIF(run.trigger_payload_json->>'skipReason', ''),
         NULLIF(run.trigger_payload_json->>'terminalReason', ''),
         ''
       ), 2000),
       run.created_at,
       run.updated_at
FROM workflow_automation_runs run
CROSS JOIN LATERAL generate_series(1, run.actions_total) action_position
CROSS JOIN LATERAL (
  SELECT CASE
    WHEN run.target_entity_type = 'lead_form'
      AND length(run.trigger_payload_json->>'createdTaskId') BETWEEN 1 AND 18
      AND (run.trigger_payload_json->>'createdTaskId') ~ '^[1-9][0-9]*$'
      THEN (run.trigger_payload_json->>'createdTaskId')::bigint
    WHEN run.target_entity_type = 'deal'
      AND jsonb_typeof(run.trigger_payload_json->'taskIds') = 'array'
      AND jsonb_array_length(run.trigger_payload_json->'taskIds') >= action_position
      AND length(run.trigger_payload_json->'taskIds'->>(action_position - 1)) BETWEEN 1 AND 18
      AND (run.trigger_payload_json->'taskIds'->>(action_position - 1)) ~ '^[1-9][0-9]*$'
      THEN (run.trigger_payload_json->'taskIds'->>(action_position - 1))::bigint
    ELSE NULL
  END AS task_id
) retained
LEFT JOIN tasks task
  ON task.organization_id = run.organization_id
 AND task.id = retained.task_id;
