-- open-crm-deploy: expand
-- Durable, self-service workspace portability artifacts. The generated bundle
-- lives in PostgreSQL so every API instance can serve it; artifact bytes expire
-- independently from the retained request/audit metadata.

CREATE TABLE IF NOT EXISTS workspace_exports (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  requested_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  idempotency_key_hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  filename TEXT NOT NULL DEFAULT '',
  content_sha256 TEXT NOT NULL DEFAULT '',
  byte_size BIGINT NOT NULL DEFAULT 0,
  dataset_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
  artifact BYTEA,
  last_error TEXT NOT NULL DEFAULT '',
  completed_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  downloaded_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT workspace_exports_key_hash_check CHECK (CHAR_LENGTH(idempotency_key_hash) = 64),
  CONSTRAINT workspace_exports_status_check CHECK (status IN ('pending', 'processing', 'ready', 'failed', 'expired')),
  CONSTRAINT workspace_exports_filename_length_check CHECK (CHAR_LENGTH(filename) <= 255),
  CONSTRAINT workspace_exports_sha256_check CHECK (content_sha256 = '' OR CHAR_LENGTH(content_sha256) = 64),
  CONSTRAINT workspace_exports_byte_size_check CHECK (byte_size >= 0),
  CONSTRAINT workspace_exports_dataset_counts_check CHECK (jsonb_typeof(dataset_counts) = 'object'),
  CONSTRAINT workspace_exports_ready_shape_check CHECK (
    status <> 'ready' OR (
      artifact IS NOT NULL AND filename <> '' AND content_sha256 <> '' AND
      byte_size > 0 AND completed_at IS NOT NULL AND expires_at IS NOT NULL
    )
  ),
  CONSTRAINT workspace_exports_expired_shape_check CHECK (status <> 'expired' OR artifact IS NULL),
  CONSTRAINT workspace_exports_membership_fk
    FOREIGN KEY (organization_id, requested_by_user_id)
    REFERENCES organization_memberships(organization_id, user_id),
  CONSTRAINT workspace_exports_org_key_unique UNIQUE (organization_id, idempotency_key_hash)
);

CREATE INDEX IF NOT EXISTS idx_workspace_exports_org_created
  ON workspace_exports(organization_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_workspace_exports_expiry
  ON workspace_exports(expires_at, id)
  WHERE status = 'ready' AND artifact IS NOT NULL;
