-- open-crm-deploy: expand
-- Retain the exact successful action that caused a nested workflow event and
-- bound the causal chain before any trigger-capable action is promoted. The
-- same migration adds bounded result evidence for the first reviewed
-- non-task action: an in-app teammate notification.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE workflow_automation_runs
  ADD COLUMN causation_run_id BIGINT,
  ADD COLUMN causation_action_position INTEGER,
  ADD COLUMN causal_depth INTEGER DEFAULT 0,
  ADD CONSTRAINT workflow_automation_runs_causality_shape_check
    CHECK (
      causal_depth IS NOT NULL
      AND (
      (causation_run_id IS NULL AND causation_action_position IS NULL AND causal_depth = 0)
      OR
      (causation_run_id IS NOT NULL
       AND causation_action_position BETWEEN 1 AND 25
       AND causal_depth BETWEEN 1 AND 9)
      )
    ) NOT VALID;

ALTER TABLE workflow_automation_runs
  ADD CONSTRAINT workflow_automation_runs_causation_action_fk
    FOREIGN KEY (organization_id, causation_run_id, causation_action_position)
    REFERENCES workflow_automation_action_outcomes(organization_id, run_id, action_position)
    ON DELETE CASCADE
    NOT VALID;

ALTER TABLE workflow_automation_runs
  VALIDATE CONSTRAINT workflow_automation_runs_causality_shape_check;

ALTER TABLE workflow_automation_runs
  VALIDATE CONSTRAINT workflow_automation_runs_causation_action_fk;

CREATE INDEX idx_workflow_automation_runs_org_causation
  ON workflow_automation_runs(organization_id, causation_run_id, causation_action_position)
  WHERE causation_run_id IS NOT NULL;

ALTER TABLE workflow_automation_action_outcomes
  ADD COLUMN notification_count INTEGER DEFAULT 0,
  ADD CONSTRAINT workflow_action_outcomes_notification_count_check
    CHECK (notification_count IS NOT NULL AND notification_count BETWEEN 0 AND 50) NOT VALID;

ALTER TABLE workflow_automation_action_outcomes
  VALIDATE CONSTRAINT workflow_action_outcomes_notification_count_check;
