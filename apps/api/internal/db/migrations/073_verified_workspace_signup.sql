-- open-crm-deploy: expand
-- Verified self-serve provisioning. Existing users are trusted as already
-- verified so this additive migration does not invalidate live sessions.

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS email_verification_token_hash TEXT,
  ADD COLUMN IF NOT EXISTS email_verification_expires_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS email_verification_sent_at TIMESTAMPTZ;

UPDATE users
SET email_verified_at = COALESCE(email_verified_at, created_at, NOW())
WHERE email_verified_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_verification_token_hash
  ON users(email_verification_token_hash)
  WHERE email_verification_token_hash IS NOT NULL;

ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS trial_started_at TIMESTAMPTZ;

UPDATE organizations
SET trial_started_at = trial_ends_at - INTERVAL '14 days'
WHERE subscription_status = 'trialing'
  AND trial_ends_at IS NOT NULL
  AND trial_started_at IS NULL;

CREATE TABLE IF NOT EXISTS workspace_bootstrap_requests (
  id BIGSERIAL PRIMARY KEY,
  idempotency_key_hash TEXT NOT NULL UNIQUE,
  request_sha256 TEXT NOT NULL,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT workspace_bootstrap_key_hash_check CHECK (CHAR_LENGTH(idempotency_key_hash) = 64),
  CONSTRAINT workspace_bootstrap_request_hash_check CHECK (CHAR_LENGTH(request_sha256) = 64),
  CONSTRAINT workspace_bootstrap_user_unique UNIQUE (user_id)
);

CREATE INDEX IF NOT EXISTS idx_workspace_bootstrap_requests_org_created
  ON workspace_bootstrap_requests(organization_id, created_at DESC, id DESC);
