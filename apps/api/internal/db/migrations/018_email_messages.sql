-- 1.1.4 Email outbox/log: durable record of every customer-facing email sent
-- through the CRM (via a user's mailbox), scoped to an organization and linked
-- to the CRM record it concerns. Foundation for delivery status and tracking.

CREATE TABLE email_messages (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  to_email TEXT NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'sent',
  error TEXT NOT NULL DEFAULT '',
  entity_type TEXT NOT NULL DEFAULT '',
  entity_id BIGINT,
  sent_by_user_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT email_messages_status_check CHECK (status IN ('sent', 'failed'))
);

CREATE INDEX idx_email_messages_org_created ON email_messages(organization_id, created_at DESC);
CREATE INDEX idx_email_messages_entity ON email_messages(organization_id, entity_type, entity_id, created_at DESC);
