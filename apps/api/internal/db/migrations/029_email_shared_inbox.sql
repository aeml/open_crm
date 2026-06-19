-- 1.1.9 Shared team inbox foundation. Shared inbound mailbox messages can be
-- assigned and closed without exposing private inbound messages by default.

ALTER TABLE email_messages
  ADD COLUMN shared_inbox_status TEXT NOT NULL DEFAULT 'open',
  ADD COLUMN shared_inbox_assigned_to_user_id BIGINT REFERENCES users(id),
  ADD COLUMN shared_inbox_updated_at TIMESTAMPTZ;

ALTER TABLE email_messages
  ADD CONSTRAINT email_messages_shared_inbox_status_check CHECK (shared_inbox_status IN ('open', 'closed'));

CREATE INDEX idx_email_messages_shared_inbox
  ON email_messages(organization_id, shared_inbox_status, received_at DESC, created_at DESC)
  WHERE direction = 'inbound' AND visibility = 'shared';

CREATE INDEX idx_email_messages_shared_inbox_assignee
  ON email_messages(organization_id, shared_inbox_assigned_to_user_id, shared_inbox_status)
  WHERE direction = 'inbound' AND visibility = 'shared' AND shared_inbox_assigned_to_user_id IS NOT NULL;
