-- 1.1.3 Email privacy controls. Synced inbound mailbox messages can be linked
-- to CRM records, but should not expose a user's private inbox to all members.

ALTER TABLE email_messages
  ADD COLUMN visibility TEXT NOT NULL DEFAULT 'shared';

UPDATE email_messages
SET visibility = 'private'
WHERE direction = 'inbound';

UPDATE email_messages
SET visibility = 'shared'
WHERE direction <> 'inbound';

ALTER TABLE email_messages
  ADD CONSTRAINT email_messages_visibility_check CHECK (visibility IN ('shared', 'private'));

CREATE INDEX idx_email_messages_visibility
  ON email_messages(organization_id, visibility, mailbox_user_id, sent_by_user_id);
