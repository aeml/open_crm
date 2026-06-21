-- 1.5.3 Workflow automation action library foundation.
-- Stores ordered action definitions without executing them yet.

ALTER TABLE workflow_automations
  ADD COLUMN actions_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD CONSTRAINT workflow_automations_actions_json_array_check CHECK (jsonb_typeof(actions_json) = 'array');
