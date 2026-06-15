-- 1.1.2 Inbound mailbox storage foundation. Extend the existing email log so
-- synced mailbox messages can live beside CRM-sent messages without changing
-- provider ingestion yet.

ALTER TABLE email_messages
  DROP CONSTRAINT email_messages_status_check,
  ADD COLUMN direction TEXT NOT NULL DEFAULT 'outbound',
  ADD COLUMN from_email TEXT NOT NULL DEFAULT '',
  ADD COLUMN mailbox_user_id BIGINT REFERENCES users(id),
  ADD COLUMN provider_message_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN provider_thread_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN received_at TIMESTAMPTZ;

UPDATE email_messages
SET mailbox_user_id = sent_by_user_id
WHERE mailbox_user_id IS NULL AND sent_by_user_id IS NOT NULL;

ALTER TABLE email_messages
  ADD CONSTRAINT email_messages_direction_check CHECK (direction IN ('outbound', 'inbound')),
  ADD CONSTRAINT email_messages_status_check CHECK (status IN ('sent', 'failed', 'received'));

CREATE INDEX idx_email_messages_mailbox_user ON email_messages(organization_id, mailbox_user_id, received_at DESC, created_at DESC) WHERE mailbox_user_id IS NOT NULL;
CREATE UNIQUE INDEX idx_email_messages_provider_message ON email_messages(organization_id, mailbox_user_id, provider_message_id) WHERE provider_message_id <> '';
