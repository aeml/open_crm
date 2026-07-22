-- open-crm-deploy: expand
-- Retain the exact same-tenant user selected by the first reviewed
-- trigger-capable workflow action. Historical outcomes and rolling old writers
-- keep nullable/default values until they execute this explicit contract.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE workflow_automation_action_outcomes
  ADD COLUMN assigned_user_id BIGINT,
  ADD COLUMN assignment_changed BOOLEAN DEFAULT FALSE,
  ADD CONSTRAINT workflow_action_outcomes_assignment_shape_check
    CHECK (
      assignment_changed IS NOT NULL
      AND
      (
        (
          action_type = 'assign_owner'
          AND status = 'succeeded'
          AND assigned_user_id IS NOT NULL
        )
        OR
        (
          action_type = 'assign_owner'
          AND status <> 'succeeded'
          AND assigned_user_id IS NULL
          AND assignment_changed = FALSE
        )
        OR
        (
          action_type <> 'assign_owner'
          AND assigned_user_id IS NULL
          AND assignment_changed = FALSE
        )
      )
    ) NOT VALID,
  ADD CONSTRAINT workflow_action_outcomes_assigned_membership_fk
    FOREIGN KEY (organization_id, assigned_user_id)
    REFERENCES organization_memberships(organization_id, user_id)
    NOT VALID;

ALTER TABLE workflow_automation_action_outcomes
  VALIDATE CONSTRAINT workflow_action_outcomes_assignment_shape_check;

ALTER TABLE workflow_automation_action_outcomes
  VALIDATE CONSTRAINT workflow_action_outcomes_assigned_membership_fk;
