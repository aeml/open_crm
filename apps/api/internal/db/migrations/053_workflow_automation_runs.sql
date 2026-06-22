-- 1.5.7 Workflow automation run history foundation.
-- Stores execution attempts and idempotency keys before the executor exists.

CREATE TABLE workflow_automation_runs (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  automation_id BIGINT NOT NULL REFERENCES workflow_automations(id) ON DELETE CASCADE,
  automation_name TEXT NOT NULL,
  trigger_type TEXT NOT NULL,
  target_entity_type TEXT NOT NULL,
  target_entity_id BIGINT,
  trigger_event_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'running',
  trigger_payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  condition_result BOOLEAN,
  actions_total INTEGER NOT NULL DEFAULT 0,
  actions_completed INTEGER NOT NULL DEFAULT 0,
  retry_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT workflow_automation_runs_automation_name_check CHECK (length(trim(automation_name)) > 0),
  CONSTRAINT workflow_automation_runs_trigger_type_check CHECK (trigger_type IN ('record_created', 'record_updated', 'stage_changed', 'date_reached', 'form_submitted', 'inbound_email', 'webhook')),
  CONSTRAINT workflow_automation_runs_target_entity_type_check CHECK (target_entity_type IN ('contact', 'company', 'deal', 'task', 'lead_form', 'email_message', 'webhook')),
  CONSTRAINT workflow_automation_runs_target_entity_id_check CHECK (target_entity_id IS NULL OR target_entity_id > 0),
  CONSTRAINT workflow_automation_runs_event_key_check CHECK (length(trim(trigger_event_key)) > 0),
  CONSTRAINT workflow_automation_runs_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'skipped', 'cancelled')),
  CONSTRAINT workflow_automation_runs_trigger_payload_json_object_check CHECK (jsonb_typeof(trigger_payload_json) = 'object'),
  CONSTRAINT workflow_automation_runs_actions_total_check CHECK (actions_total >= 0),
  CONSTRAINT workflow_automation_runs_actions_completed_check CHECK (actions_completed >= 0),
  CONSTRAINT workflow_automation_runs_retry_count_check CHECK (retry_count >= 0),
  CONSTRAINT workflow_automation_runs_action_progress_check CHECK (actions_completed <= actions_total),
  CONSTRAINT workflow_automation_runs_terminal_completed_check CHECK (
    (status IN ('succeeded', 'failed', 'skipped', 'cancelled') AND completed_at IS NOT NULL)
    OR (status IN ('queued', 'running') AND completed_at IS NULL)
  )
);

CREATE UNIQUE INDEX idx_workflow_automation_runs_org_automation_event_unique
  ON workflow_automation_runs(organization_id, automation_id, trigger_event_key);

CREATE INDEX idx_workflow_automation_runs_org_created
  ON workflow_automation_runs(organization_id, created_at DESC, id DESC);

CREATE INDEX idx_workflow_automation_runs_org_status
  ON workflow_automation_runs(organization_id, status, created_at DESC, id DESC);

CREATE INDEX idx_workflow_automation_runs_org_automation_created
  ON workflow_automation_runs(organization_id, automation_id, created_at DESC, id DESC);
