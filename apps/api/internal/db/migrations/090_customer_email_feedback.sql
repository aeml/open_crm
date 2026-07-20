-- open-crm-deploy: expand
-- Retain bounded, machine-readable DSN/ARF evidence from connected mailboxes
-- and attach terminal delivery outcomes without rewriting provider acceptance.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE email_messages
  ADD COLUMN delivery_outcome TEXT DEFAULT '',
  ADD COLUMN delivery_outcome_at TIMESTAMPTZ,
  ADD COLUMN delivery_feedback_email_message_id BIGINT;

ALTER TABLE email_messages
  ADD CONSTRAINT email_messages_delivery_outcome_check CHECK (
    delivery_outcome IS NOT NULL AND delivery_outcome IN ('', 'bounced', 'complaint')
  ) NOT VALID,
  ADD CONSTRAINT email_messages_delivery_outcome_shape_check CHECK (
    (delivery_outcome = '' AND delivery_outcome_at IS NULL AND delivery_feedback_email_message_id IS NULL)
    OR (delivery_outcome IN ('bounced', 'complaint') AND delivery_outcome_at IS NOT NULL AND delivery_feedback_email_message_id IS NOT NULL)
  ) NOT VALID,
  ADD CONSTRAINT email_messages_delivery_feedback_message_fk
    FOREIGN KEY (delivery_feedback_email_message_id) REFERENCES email_messages(id) NOT VALID;

ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_delivery_outcome_check;
ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_delivery_outcome_shape_check;
ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_delivery_feedback_message_fk;

CREATE INDEX idx_email_messages_org_delivery_outcome
  ON email_messages(organization_id, delivery_outcome, delivery_outcome_at DESC)
  WHERE delivery_outcome <> '';

CREATE UNIQUE INDEX idx_email_messages_org_mailbox_outbound_rfc_message
  ON email_messages(organization_id, mailbox_user_id, rfc_message_id)
  WHERE direction = 'outbound' AND status = 'sent' AND mailbox_user_id IS NOT NULL AND rfc_message_id <> '';

ALTER TABLE email_sequence_deliveries
  ADD COLUMN delivery_outcome TEXT DEFAULT '',
  ADD COLUMN delivery_outcome_at TIMESTAMPTZ,
  ADD COLUMN delivery_feedback_email_message_id BIGINT,
  ADD COLUMN delivery_feedback_status_code TEXT DEFAULT '';

ALTER TABLE email_sequence_deliveries
  ADD CONSTRAINT email_sequence_deliveries_delivery_outcome_check CHECK (
    delivery_outcome IS NOT NULL AND delivery_outcome IN ('', 'bounced', 'complaint')
  ) NOT VALID,
  ADD CONSTRAINT email_sequence_deliveries_delivery_outcome_shape_check CHECK (
    (delivery_outcome = '' AND delivery_outcome_at IS NULL AND delivery_feedback_email_message_id IS NULL AND delivery_feedback_status_code = '')
    OR (delivery_outcome IN ('bounced', 'complaint') AND delivery_outcome_at IS NOT NULL AND delivery_feedback_email_message_id IS NOT NULL AND delivery_feedback_status_code IS NOT NULL AND CHAR_LENGTH(delivery_feedback_status_code) BETWEEN 1 AND 100)
  ) NOT VALID,
  ADD CONSTRAINT email_sequence_deliveries_delivery_feedback_message_fk
    FOREIGN KEY (delivery_feedback_email_message_id) REFERENCES email_messages(id) NOT VALID;

ALTER TABLE email_sequence_deliveries VALIDATE CONSTRAINT email_sequence_deliveries_delivery_outcome_check;
ALTER TABLE email_sequence_deliveries VALIDATE CONSTRAINT email_sequence_deliveries_delivery_outcome_shape_check;
ALTER TABLE email_sequence_deliveries VALIDATE CONSTRAINT email_sequence_deliveries_delivery_feedback_message_fk;

CREATE INDEX idx_email_sequence_deliveries_org_delivery_outcome
  ON email_sequence_deliveries(organization_id, delivery_outcome, delivery_outcome_at DESC)
  WHERE delivery_outcome <> '';

CREATE TABLE customer_email_feedback_events (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  mailbox_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  inbound_email_message_id BIGINT NOT NULL REFERENCES email_messages(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  feedback_type TEXT NOT NULL,
  original_rfc_message_id TEXT NOT NULL DEFAULT '',
  recipient_email TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL DEFAULT '',
  status_code TEXT NOT NULL DEFAULT '',
  applied BOOLEAN NOT NULL DEFAULT FALSE,
  sequence_delivery_id BIGINT REFERENCES email_sequence_deliveries(id) ON DELETE SET NULL,
  outbound_email_message_id BIGINT REFERENCES email_messages(id) ON DELETE SET NULL,
  received_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT customer_email_feedback_provider_check CHECK (provider IN ('imap', 'google', 'microsoft')),
  CONSTRAINT customer_email_feedback_type_check CHECK (feedback_type IN ('bounce', 'complaint')),
  CONSTRAINT customer_email_feedback_message_id_check CHECK (original_rfc_message_id = '' OR CHAR_LENGTH(original_rfc_message_id) BETWEEN 3 AND 500),
  CONSTRAINT customer_email_feedback_recipient_check CHECK (recipient_email = '' OR CHAR_LENGTH(recipient_email) <= 320),
  CONSTRAINT customer_email_feedback_action_check CHECK (CHAR_LENGTH(action) BETWEEN 1 AND 100),
  CONSTRAINT customer_email_feedback_status_check CHECK (CHAR_LENGTH(status_code) BETWEEN 1 AND 100),
  CONSTRAINT customer_email_feedback_event_unique UNIQUE (
    organization_id, inbound_email_message_id, feedback_type, original_rfc_message_id, recipient_email
  )
);

CREATE INDEX idx_customer_email_feedback_recent
  ON customer_email_feedback_events(received_at DESC, id DESC);

CREATE INDEX idx_customer_email_feedback_unapplied
  ON customer_email_feedback_events(received_at DESC, id DESC)
  WHERE applied = FALSE;
