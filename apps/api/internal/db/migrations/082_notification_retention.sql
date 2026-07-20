-- open-crm-deploy: expand
-- Notification cleanup and aggregate health queries run across tenants. These
-- indexes keep each bounded pass and protected metrics scrape independent of
-- tenant count without exposing tenant or recipient identity.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX IF NOT EXISTS idx_notifications_retention_read
  ON notifications(read_at ASC, id ASC)
  WHERE read_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_notifications_retention_unread
  ON notifications(created_at ASC, id ASC)
  WHERE read_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_notifications_operational_created
  ON notifications(created_at ASC)
  INCLUDE (organization_id, user_id, event_type);
