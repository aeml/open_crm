-- open-crm-deploy: expand
-- One-time, expiring account-recovery state. Only token digests are persisted;
-- raw reset tokens exist only in the delivery request and recipient link.

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS password_reset_token_hash TEXT,
  ADD COLUMN IF NOT EXISTS password_reset_expires_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS password_reset_requested_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS password_reset_delivery_status TEXT,
  ADD COLUMN IF NOT EXISTS password_reset_delivery_attempted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS password_reset_consumed_at TIMESTAMPTZ;

ALTER TABLE users
  ADD CONSTRAINT users_password_reset_token_hash_check
    CHECK (password_reset_token_hash IS NULL OR CHAR_LENGTH(password_reset_token_hash) = 64) NOT VALID;

ALTER TABLE users
  VALIDATE CONSTRAINT users_password_reset_token_hash_check;

ALTER TABLE users
  ADD CONSTRAINT users_password_reset_delivery_status_check
    CHECK (password_reset_delivery_status IS NULL OR password_reset_delivery_status IN ('pending', 'sent', 'failed', 'consumed')) NOT VALID;

ALTER TABLE users
  VALIDATE CONSTRAINT users_password_reset_delivery_status_check;

ALTER TABLE users
  ADD CONSTRAINT users_password_reset_state_check
    CHECK (
      (
        password_reset_token_hash IS NULL
        AND password_reset_expires_at IS NULL
        AND (
          (
            password_reset_delivery_status IS NULL
            AND password_reset_requested_at IS NULL
            AND password_reset_delivery_attempted_at IS NULL
            AND password_reset_consumed_at IS NULL
          )
          OR
          (
            password_reset_delivery_status = 'consumed'
            AND password_reset_requested_at IS NOT NULL
            AND password_reset_consumed_at IS NOT NULL
          )
        )
      )
      OR
      (
        password_reset_token_hash IS NOT NULL
        AND password_reset_expires_at IS NOT NULL
        AND password_reset_requested_at IS NOT NULL
        AND password_reset_delivery_status IN ('pending', 'sent', 'failed')
        AND password_reset_consumed_at IS NULL
        AND (
          (password_reset_delivery_status = 'pending' AND password_reset_delivery_attempted_at IS NULL)
          OR
          (password_reset_delivery_status IN ('sent', 'failed') AND password_reset_delivery_attempted_at IS NOT NULL)
        )
      )
    ) NOT VALID;

ALTER TABLE users
  VALIDATE CONSTRAINT users_password_reset_state_check;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_password_reset_token_hash
  ON users(password_reset_token_hash)
  WHERE password_reset_token_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_users_password_reset_delivery_health
  ON users(password_reset_delivery_status, password_reset_requested_at)
  WHERE password_reset_delivery_status IN ('pending', 'failed');
