-- open-crm-deploy: expand
-- The focused notification center uses an immutable id tie-breaker for equal
-- event times. The existing recipient-scoped partial unread index continues to
-- serve the exact backlog count and acknowledgement updates.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX IF NOT EXISTS idx_notifications_user_created_id
  ON notifications(organization_id, user_id, created_at DESC, id DESC);
