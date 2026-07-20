-- open-crm-deploy: expand
-- Classify future sequence completions without guessing how historical rows
-- finished. Reply evidence points to the retained inbound mailbox message.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE email_sequence_enrollments
  ADD COLUMN completion_reason TEXT,
  ADD COLUMN replied_at TIMESTAMPTZ,
  ADD COLUMN reply_email_message_id BIGINT;

ALTER TABLE email_sequence_enrollments
  ADD CONSTRAINT email_sequence_enrollments_completion_reason_check CHECK (
    completion_reason IS NULL
    OR (status = 'completed' AND completion_reason IN ('finished', 'replied', 'suppressed'))
  ) NOT VALID,
  ADD CONSTRAINT email_sequence_enrollments_reply_shape_check CHECK (
    (completion_reason = 'replied' AND replied_at IS NOT NULL AND reply_email_message_id IS NOT NULL)
    OR (completion_reason IS DISTINCT FROM 'replied' AND replied_at IS NULL AND reply_email_message_id IS NULL)
  ) NOT VALID,
  ADD CONSTRAINT email_sequence_enrollments_reply_message_fk
    FOREIGN KEY (reply_email_message_id) REFERENCES email_messages(id) NOT VALID;

ALTER TABLE email_sequence_enrollments VALIDATE CONSTRAINT email_sequence_enrollments_completion_reason_check;
ALTER TABLE email_sequence_enrollments VALIDATE CONSTRAINT email_sequence_enrollments_reply_shape_check;
ALTER TABLE email_sequence_enrollments VALIDATE CONSTRAINT email_sequence_enrollments_reply_message_fk;
