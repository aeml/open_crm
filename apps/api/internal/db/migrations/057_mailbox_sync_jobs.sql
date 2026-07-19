-- open-crm-deploy: expand
-- Stable mailbox scheduling cycles. A failed queue attempt must not create a
-- new idempotency key merely because sync_status/updated_at changed.

ALTER TABLE user_email_accounts
  ADD COLUMN next_sync_at TIMESTAMPTZ;

UPDATE user_email_accounts
SET next_sync_at = COALESCE(last_sync_at + INTERVAL '15 minutes', updated_at)
WHERE sync_enabled = TRUE;

CREATE INDEX idx_user_email_accounts_next_sync
  ON user_email_accounts(next_sync_at ASC, organization_id, user_id)
  WHERE sync_enabled = TRUE AND next_sync_at IS NOT NULL;
