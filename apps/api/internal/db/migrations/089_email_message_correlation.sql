-- open-crm-deploy: expand
-- Preserve provider/RFC correlation evidence so inbound replies can stop a
-- sequence only when they reference an accepted delivery. Historical rows
-- remain unqualified rather than being guessed from subject or timestamps.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE email_sequence_deliveries
  ADD COLUMN rfc_message_id TEXT DEFAULT '',
  ADD COLUMN provider_message_id TEXT DEFAULT '',
  ADD COLUMN provider_thread_id TEXT DEFAULT '';

ALTER TABLE email_sequence_deliveries
  ADD CONSTRAINT email_sequence_deliveries_rfc_message_id_check CHECK (
    rfc_message_id IS NOT NULL AND (rfc_message_id = '' OR CHAR_LENGTH(rfc_message_id) BETWEEN 3 AND 500)
  ) NOT VALID,
  ADD CONSTRAINT email_sequence_deliveries_provider_message_id_check CHECK (
    provider_message_id IS NOT NULL AND CHAR_LENGTH(provider_message_id) <= 500
  ) NOT VALID,
  ADD CONSTRAINT email_sequence_deliveries_provider_thread_id_check CHECK (
    provider_thread_id IS NOT NULL AND CHAR_LENGTH(provider_thread_id) <= 500
  ) NOT VALID;

ALTER TABLE email_sequence_deliveries VALIDATE CONSTRAINT email_sequence_deliveries_rfc_message_id_check;
ALTER TABLE email_sequence_deliveries VALIDATE CONSTRAINT email_sequence_deliveries_provider_message_id_check;
ALTER TABLE email_sequence_deliveries VALIDATE CONSTRAINT email_sequence_deliveries_provider_thread_id_check;

CREATE UNIQUE INDEX idx_email_sequence_deliveries_org_rfc_message_id
  ON email_sequence_deliveries(organization_id, rfc_message_id)
  WHERE rfc_message_id <> '';

CREATE INDEX idx_email_sequence_deliveries_org_provider_thread
  ON email_sequence_deliveries(organization_id, provider_thread_id)
  WHERE provider_thread_id <> '';

ALTER TABLE email_messages
  ADD COLUMN rfc_message_id TEXT DEFAULT '',
  ADD COLUMN in_reply_to TEXT DEFAULT '',
  ADD COLUMN reference_message_ids TEXT[] DEFAULT '{}'::TEXT[];

ALTER TABLE email_messages
  ADD CONSTRAINT email_messages_rfc_message_id_check CHECK (
    rfc_message_id IS NOT NULL AND (rfc_message_id = '' OR CHAR_LENGTH(rfc_message_id) BETWEEN 3 AND 500)
  ) NOT VALID,
  ADD CONSTRAINT email_messages_in_reply_to_check CHECK (
    in_reply_to IS NOT NULL AND (in_reply_to = '' OR CHAR_LENGTH(in_reply_to) BETWEEN 3 AND 500)
  ) NOT VALID,
  ADD CONSTRAINT email_messages_reference_message_ids_check CHECK (
    reference_message_ids IS NOT NULL AND (
    CARDINALITY(reference_message_ids) <= 50
    AND ARRAY_POSITION(reference_message_ids, NULL) IS NULL
    AND CHAR_LENGTH(ARRAY_TO_STRING(reference_message_ids, '')) <= 25000
    )
  ) NOT VALID;

ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_rfc_message_id_check;
ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_in_reply_to_check;
ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_reference_message_ids_check;
