-- open-crm-deploy: expand
-- Correlate Postmark bounce/complaint callbacks to the exact system-email
-- attempt without persisting the raw one-time credential or recipient address.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE users
  ADD COLUMN email_verification_delivery_status TEXT,
  ADD COLUMN email_verification_delivery_key_hash TEXT,
  ADD COLUMN email_verification_provider_message_id TEXT,
  ADD COLUMN password_setup_delivery_status TEXT,
  ADD COLUMN password_setup_delivery_key_hash TEXT,
  ADD COLUMN password_setup_provider_message_id TEXT,
  ADD COLUMN password_reset_delivery_key_hash TEXT,
  ADD COLUMN password_reset_provider_message_id TEXT,
  ADD COLUMN system_email_suppressed_at TIMESTAMPTZ,
  ADD COLUMN system_email_suppression_reason TEXT;

ALTER TABLE users
  ADD CONSTRAINT users_email_verification_delivery_status_check
    CHECK (email_verification_delivery_status IS NULL OR email_verification_delivery_status IN ('pending', 'sent', 'failed', 'bounced', 'complaint')) NOT VALID,
  ADD CONSTRAINT users_password_setup_delivery_status_check
    CHECK (password_setup_delivery_status IS NULL OR password_setup_delivery_status IN ('pending', 'sent', 'failed', 'bounced', 'complaint')) NOT VALID,
  ADD CONSTRAINT users_email_verification_delivery_key_hash_check
    CHECK (email_verification_delivery_key_hash IS NULL OR CHAR_LENGTH(email_verification_delivery_key_hash) = 64) NOT VALID,
  ADD CONSTRAINT users_password_setup_delivery_key_hash_check
    CHECK (password_setup_delivery_key_hash IS NULL OR CHAR_LENGTH(password_setup_delivery_key_hash) = 64) NOT VALID,
  ADD CONSTRAINT users_password_reset_delivery_key_hash_check
    CHECK (password_reset_delivery_key_hash IS NULL OR CHAR_LENGTH(password_reset_delivery_key_hash) = 64) NOT VALID,
  ADD CONSTRAINT users_email_verification_provider_message_id_check
    CHECK (email_verification_provider_message_id IS NULL OR CHAR_LENGTH(email_verification_provider_message_id) BETWEEN 1 AND 200) NOT VALID,
  ADD CONSTRAINT users_password_setup_provider_message_id_check
    CHECK (password_setup_provider_message_id IS NULL OR CHAR_LENGTH(password_setup_provider_message_id) BETWEEN 1 AND 200) NOT VALID,
  ADD CONSTRAINT users_password_reset_provider_message_id_check
    CHECK (password_reset_provider_message_id IS NULL OR CHAR_LENGTH(password_reset_provider_message_id) BETWEEN 1 AND 200) NOT VALID,
  ADD CONSTRAINT users_system_email_suppression_check
    CHECK (
      (system_email_suppressed_at IS NULL AND system_email_suppression_reason IS NULL)
      OR (system_email_suppressed_at IS NOT NULL AND system_email_suppression_reason = 'complaint')
    ) NOT VALID;

ALTER TABLE users VALIDATE CONSTRAINT users_email_verification_delivery_status_check;
ALTER TABLE users VALIDATE CONSTRAINT users_password_setup_delivery_status_check;
ALTER TABLE users VALIDATE CONSTRAINT users_email_verification_delivery_key_hash_check;
ALTER TABLE users VALIDATE CONSTRAINT users_password_setup_delivery_key_hash_check;
ALTER TABLE users VALIDATE CONSTRAINT users_password_reset_delivery_key_hash_check;
ALTER TABLE users VALIDATE CONSTRAINT users_email_verification_provider_message_id_check;
ALTER TABLE users VALIDATE CONSTRAINT users_password_setup_provider_message_id_check;
ALTER TABLE users VALIDATE CONSTRAINT users_password_reset_provider_message_id_check;
ALTER TABLE users VALIDATE CONSTRAINT users_system_email_suppression_check;

CREATE UNIQUE INDEX idx_users_email_verification_delivery_key
  ON users(email_verification_delivery_key_hash)
  WHERE email_verification_delivery_key_hash IS NOT NULL;

CREATE UNIQUE INDEX idx_users_password_setup_delivery_key
  ON users(password_setup_delivery_key_hash)
  WHERE password_setup_delivery_key_hash IS NOT NULL;

CREATE UNIQUE INDEX idx_users_password_reset_delivery_key
  ON users(password_reset_delivery_key_hash)
  WHERE password_reset_delivery_key_hash IS NOT NULL;

CREATE TABLE system_email_feedback_events (
  id BIGSERIAL PRIMARY KEY,
  provider TEXT NOT NULL,
  record_type TEXT NOT NULL,
  provider_event_id BIGINT NOT NULL,
  provider_message_id TEXT NOT NULL,
  payload_sha256 TEXT NOT NULL,
  purpose TEXT NOT NULL,
  organization_id BIGINT REFERENCES organizations(id) ON DELETE CASCADE,
  user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  event_at TIMESTAMPTZ NOT NULL,
  bounce_type TEXT NOT NULL DEFAULT '',
  inactive BOOLEAN NOT NULL DEFAULT FALSE,
  can_activate BOOLEAN NOT NULL DEFAULT FALSE,
  applied BOOLEAN NOT NULL DEFAULT FALSE,
  received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT system_email_feedback_provider_check CHECK (provider = 'postmark'),
  CONSTRAINT system_email_feedback_record_type_check CHECK (record_type IN ('bounce', 'complaint')),
  CONSTRAINT system_email_feedback_purpose_check CHECK (purpose IN ('workspace_verification', 'user_invitation', 'password_reset')),
  CONSTRAINT system_email_feedback_event_id_check CHECK (provider_event_id > 0),
  CONSTRAINT system_email_feedback_message_id_check CHECK (CHAR_LENGTH(provider_message_id) BETWEEN 1 AND 200),
  CONSTRAINT system_email_feedback_payload_hash_check CHECK (CHAR_LENGTH(payload_sha256) = 64),
  CONSTRAINT system_email_feedback_bounce_type_check CHECK (CHAR_LENGTH(bounce_type) <= 100),
  CONSTRAINT system_email_feedback_provider_event_unique UNIQUE (provider, record_type, provider_event_id)
);

CREATE INDEX idx_system_email_feedback_recent
  ON system_email_feedback_events(received_at DESC, id DESC);

CREATE INDEX idx_system_email_feedback_unapplied
  ON system_email_feedback_events(received_at DESC, id DESC)
  WHERE applied = FALSE;
