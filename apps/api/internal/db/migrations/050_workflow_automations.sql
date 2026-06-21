-- 1.5.1 Workflow automation trigger model foundation.
-- Stores admin-defined automation triggers without executing actions yet.

CREATE TABLE workflow_automations (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  trigger_type TEXT NOT NULL,
  target_entity_type TEXT NOT NULL,
  trigger_config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_active BOOLEAN NOT NULL DEFAULT FALSE,
  position INTEGER NOT NULL DEFAULT 0,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT workflow_automations_name_check CHECK (length(trim(name)) > 0),
  CONSTRAINT workflow_automations_trigger_type_check CHECK (trigger_type IN ('record_created', 'record_updated', 'stage_changed', 'date_reached', 'form_submitted', 'inbound_email', 'webhook')),
  CONSTRAINT workflow_automations_target_entity_type_check CHECK (target_entity_type IN ('contact', 'company', 'deal', 'task', 'lead_form', 'email_message', 'webhook')),
  CONSTRAINT workflow_automations_trigger_config_json_object_check CHECK (jsonb_typeof(trigger_config_json) = 'object'),
  CONSTRAINT workflow_automations_position_check CHECK (position >= 0)
);

CREATE UNIQUE INDEX idx_workflow_automations_org_name_unique
  ON workflow_automations(organization_id, lower(name));

CREATE INDEX idx_workflow_automations_org_active_position
  ON workflow_automations(organization_id, is_active, position, updated_at DESC, id DESC);

CREATE INDEX idx_workflow_automations_org_trigger
  ON workflow_automations(organization_id, trigger_type, target_entity_type);
