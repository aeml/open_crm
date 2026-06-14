-- 1.1.4 Email engagement tracking: add an unguessable token per sent CRM
-- email and aggregate open counts from a tracking pixel endpoint.

ALTER TABLE email_messages
  ADD COLUMN tracking_token TEXT,
  ADD COLUMN open_count INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN first_opened_at TIMESTAMPTZ,
  ADD COLUMN last_opened_at TIMESTAMPTZ;

CREATE UNIQUE INDEX idx_email_messages_tracking_token ON email_messages(tracking_token) WHERE tracking_token IS NOT NULL;
