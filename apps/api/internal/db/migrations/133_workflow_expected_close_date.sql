-- open-crm-deploy: expand
-- Retain exact before/after evidence for the reviewed deal expected-close-date
-- workflow action. Nullable columns and the false default preserve historical
-- rows and rolling old writers until they execute the explicit v1 contract.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE workflow_automation_action_outcomes
  ADD COLUMN updated_field_name TEXT,
  ADD COLUMN previous_date_value DATE,
  ADD COLUMN current_date_value DATE,
  ADD COLUMN field_value_changed BOOLEAN DEFAULT FALSE,
  ADD CONSTRAINT workflow_action_outcomes_expected_close_shape_check
    CHECK (
      field_value_changed IS NOT NULL
      AND
      (
        (
          action_type = 'update_field'
          AND status = 'succeeded'
          AND updated_field_name = 'expectedCloseDate'
          AND current_date_value IS NOT NULL
          AND field_value_changed = (previous_date_value IS DISTINCT FROM current_date_value)
        )
        OR
        (
          action_type = 'update_field'
          AND status <> 'succeeded'
          AND updated_field_name IS NULL
          AND previous_date_value IS NULL
          AND current_date_value IS NULL
          AND field_value_changed = FALSE
        )
        OR
        (
          action_type <> 'update_field'
          AND updated_field_name IS NULL
          AND previous_date_value IS NULL
          AND current_date_value IS NULL
          AND field_value_changed = FALSE
        )
      )
    ) NOT VALID;

ALTER TABLE workflow_automation_action_outcomes
  VALIDATE CONSTRAINT workflow_action_outcomes_expected_close_shape_check;
