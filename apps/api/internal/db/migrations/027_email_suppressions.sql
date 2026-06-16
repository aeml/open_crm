-- 1.1.7 Email suppression/unsubscribe foundation. Suppressions are
-- organization-scoped so each tenant controls its own recipient opt-outs.

CREATE TABLE email_suppressions (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  email TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT 'unsubscribed',
  source TEXT NOT NULL DEFAULT '',
  created_by_user_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT email_suppressions_reason_check CHECK (reason IN ('unsubscribed', 'manual', 'bounce', 'complaint')),
  CONSTRAINT email_suppressions_email_nonempty CHECK (email <> '')
);

CREATE UNIQUE INDEX idx_email_suppressions_org_email
  ON email_suppressions(organization_id, email);

CREATE INDEX idx_email_suppressions_org_created
  ON email_suppressions(organization_id, created_at DESC);
