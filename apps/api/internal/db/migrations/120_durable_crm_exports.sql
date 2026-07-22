-- open-crm-deploy: expand
-- Durable filtered CRM CSV artifacts. Small exports keep their synchronous
-- endpoints; this ledger lets larger jobs finish independently from an HTTP
-- request and keeps recovery metadata after the short-lived bytes expire.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

CREATE TABLE IF NOT EXISTS crm_exports (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  requested_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  idempotency_key_hash TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  criteria_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending',
  progress_rows INTEGER NOT NULL DEFAULT 0,
  row_count INTEGER NOT NULL DEFAULT 0,
  filename TEXT NOT NULL DEFAULT '',
  content_sha256 TEXT NOT NULL DEFAULT '',
  byte_size BIGINT NOT NULL DEFAULT 0,
  artifact BYTEA,
  last_error TEXT NOT NULL DEFAULT '',
  completed_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  downloaded_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT crm_exports_key_hash_check CHECK (CHAR_LENGTH(idempotency_key_hash) = 64),
  CONSTRAINT crm_exports_resource_check CHECK (resource_type IN ('contacts','companies','deals','tasks')),
  CONSTRAINT crm_exports_criteria_check CHECK (jsonb_typeof(criteria_json) = 'object'),
  CONSTRAINT crm_exports_status_check CHECK (status IN ('pending','processing','ready','failed','expired')),
  CONSTRAINT crm_exports_progress_check CHECK (progress_rows >= 0 AND row_count >= 0),
  CONSTRAINT crm_exports_filename_check CHECK (CHAR_LENGTH(filename) <= 255),
  CONSTRAINT crm_exports_sha_check CHECK (content_sha256 = '' OR CHAR_LENGTH(content_sha256) = 64),
  CONSTRAINT crm_exports_size_check CHECK (byte_size >= 0 AND (artifact IS NULL OR octet_length(artifact) <= 52428800)),
  CONSTRAINT crm_exports_ready_shape_check CHECK (
    status <> 'ready' OR (
      artifact IS NOT NULL AND filename <> '' AND content_sha256 <> '' AND
      byte_size > 0 AND completed_at IS NOT NULL AND expires_at IS NOT NULL
    )
  ),
  CONSTRAINT crm_exports_expired_shape_check CHECK (status <> 'expired' OR artifact IS NULL),
  CONSTRAINT crm_exports_membership_fk
    FOREIGN KEY (organization_id, requested_by_user_id)
    REFERENCES organization_memberships(organization_id, user_id),
  CONSTRAINT crm_exports_org_key_unique UNIQUE (organization_id, idempotency_key_hash)
);

CREATE INDEX IF NOT EXISTS idx_crm_exports_org_created
  ON crm_exports(organization_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_crm_exports_expiry
  ON crm_exports(expires_at, id)
  WHERE status = 'ready' AND artifact IS NOT NULL;
