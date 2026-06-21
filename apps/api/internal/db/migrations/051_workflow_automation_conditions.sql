-- 1.5.2 Workflow automation condition/branching foundation.
-- Adds an all/any condition group to each automation definition. Execution and
-- multi-branch action routing remain future slices.

ALTER TABLE workflow_automations
  ADD COLUMN condition_logic TEXT NOT NULL DEFAULT 'all',
  ADD COLUMN conditions_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD CONSTRAINT workflow_automations_condition_logic_check CHECK (condition_logic IN ('all', 'any')),
  ADD CONSTRAINT workflow_automations_conditions_json_array_check CHECK (jsonb_typeof(conditions_json) = 'array');
