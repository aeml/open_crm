-- open-crm-deploy: expand
-- Retain tenant-bound output evidence for the reviewed deal-to-sequence action.
-- Nullable columns and defaults preserve rolling old writers and historical
-- action outcomes. The runtime fills the complete shape only after the
-- enrollment and its first durable delivery job commit with the deal event.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

CREATE UNIQUE INDEX idx_email_sequence_enrollments_org_id_unique
  ON email_sequence_enrollments(organization_id, id);

CREATE UNIQUE INDEX idx_email_sequence_enrollments_workflow_output
  ON email_sequence_enrollments(organization_id, id, sequence_id, contact_id);

CREATE UNIQUE INDEX idx_contacts_org_id_unique
  ON contacts(organization_id, id);

ALTER TABLE workflow_automation_action_outcomes
  ADD COLUMN sequence_id BIGINT,
  ADD COLUMN sequence_enrollment_id BIGINT,
  ADD COLUMN sequence_contact_id BIGINT,
  ADD COLUMN sequence_enrollment_created BOOLEAN DEFAULT FALSE,
  ADD CONSTRAINT workflow_action_outcomes_sequence_shape_check
    CHECK (
      sequence_enrollment_created IS NOT NULL
      AND
      (
        (
          action_type = 'add_to_sequence'
          AND status = 'succeeded'
          AND sequence_id IS NOT NULL
          AND sequence_enrollment_id IS NOT NULL
          AND sequence_contact_id IS NOT NULL
        )
        OR
        (
          action_type = 'add_to_sequence'
          AND status <> 'succeeded'
          AND sequence_id IS NULL
          AND sequence_enrollment_id IS NULL
          AND sequence_contact_id IS NULL
          AND sequence_enrollment_created = FALSE
        )
        OR
        (
          action_type <> 'add_to_sequence'
          AND sequence_id IS NULL
          AND sequence_enrollment_id IS NULL
          AND sequence_contact_id IS NULL
          AND sequence_enrollment_created = FALSE
        )
      )
    ) NOT VALID,
  ADD CONSTRAINT workflow_action_outcomes_sequence_fk
    FOREIGN KEY (organization_id, sequence_id)
    REFERENCES email_sequences(organization_id, id)
    NOT VALID,
  ADD CONSTRAINT workflow_action_outcomes_sequence_enrollment_fk
    FOREIGN KEY (
      organization_id, sequence_enrollment_id, sequence_id, sequence_contact_id
    )
    REFERENCES email_sequence_enrollments(
      organization_id, id, sequence_id, contact_id
    )
    NOT VALID,
  ADD CONSTRAINT workflow_action_outcomes_sequence_contact_fk
    FOREIGN KEY (organization_id, sequence_contact_id)
    REFERENCES contacts(organization_id, id)
    NOT VALID;

ALTER TABLE workflow_automation_action_outcomes
  VALIDATE CONSTRAINT workflow_action_outcomes_sequence_shape_check;

ALTER TABLE workflow_automation_action_outcomes
  VALIDATE CONSTRAINT workflow_action_outcomes_sequence_fk;

ALTER TABLE workflow_automation_action_outcomes
  VALIDATE CONSTRAINT workflow_action_outcomes_sequence_enrollment_fk;

ALTER TABLE workflow_automation_action_outcomes
  VALIDATE CONSTRAINT workflow_action_outcomes_sequence_contact_fk;
