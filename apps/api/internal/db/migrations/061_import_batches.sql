-- open-crm-deploy: expand
-- Import batches retain operational metadata and row outcomes, but never retain
-- uploaded CSV contents. Imported entity ids make safe, audited rollback possible.

CREATE TABLE import_batches (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id),
  entity_type TEXT NOT NULL
    CONSTRAINT import_batches_entity_type_check
    CHECK (entity_type IN ('contacts', 'companies')),
  original_filename TEXT NOT NULL,
  idempotency_key TEXT NOT NULL
    CONSTRAINT import_batches_idempotency_key_check
    CHECK (length(trim(idempotency_key)) BETWEEN 8 AND 200),
  source_sha256 TEXT NOT NULL
    CONSTRAINT import_batches_source_sha256_check
    CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
  mapping_json JSONB NOT NULL DEFAULT '{}'::jsonb
    CONSTRAINT import_batches_mapping_json_object_check
    CHECK (jsonb_typeof(mapping_json) = 'object'),
  status TEXT NOT NULL DEFAULT 'processing'
    CONSTRAINT import_batches_status_check
    CHECK (status IN ('processing', 'completed', 'completed_with_errors', 'rolled_back', 'partially_rolled_back', 'failed')),
  total_rows INTEGER NOT NULL DEFAULT 0
    CONSTRAINT import_batches_total_rows_check CHECK (total_rows >= 0),
  processed_rows INTEGER NOT NULL DEFAULT 0
    CONSTRAINT import_batches_processed_rows_check CHECK (processed_rows >= 0 AND processed_rows <= total_rows),
  success_rows INTEGER NOT NULL DEFAULT 0
    CONSTRAINT import_batches_success_rows_check CHECK (success_rows >= 0 AND success_rows <= processed_rows),
  error_rows INTEGER NOT NULL DEFAULT 0
    CONSTRAINT import_batches_error_rows_check CHECK (error_rows >= 0 AND error_rows <= processed_rows),
  rolled_back_rows INTEGER NOT NULL DEFAULT 0
    CONSTRAINT import_batches_rolled_back_rows_check CHECK (rolled_back_rows >= 0 AND rolled_back_rows <= success_rows),
  rollback_skipped_rows INTEGER NOT NULL DEFAULT 0
    CONSTRAINT import_batches_rollback_skipped_rows_check CHECK (rollback_skipped_rows >= 0 AND rollback_skipped_rows <= success_rows),
  failure_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMPTZ,
  rolled_back_at TIMESTAMPTZ,
  UNIQUE (organization_id, idempotency_key),
  UNIQUE (organization_id, id)
);

CREATE TABLE import_batch_rows (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  import_batch_id BIGINT NOT NULL,
  row_number INTEGER NOT NULL
    CONSTRAINT import_batch_rows_row_number_check CHECK (row_number >= 2),
  status TEXT NOT NULL
    CONSTRAINT import_batch_rows_status_check
    CHECK (status IN ('imported', 'error', 'rolled_back', 'rollback_skipped')),
  entity_id BIGINT,
  imported_entity_updated_at TIMESTAMPTZ,
  errors_json JSONB NOT NULL DEFAULT '[]'::jsonb
    CONSTRAINT import_batch_rows_errors_json_array_check
    CHECK (jsonb_typeof(errors_json) = 'array'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (organization_id, import_batch_id, row_number),
  FOREIGN KEY (organization_id, import_batch_id)
    REFERENCES import_batches(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_import_batches_org_created
  ON import_batches(organization_id, created_at DESC, id DESC);
CREATE INDEX idx_import_batches_org_status
  ON import_batches(organization_id, status, updated_at DESC);
CREATE INDEX idx_import_batch_rows_errors
  ON import_batch_rows(organization_id, import_batch_id, row_number)
  WHERE status = 'error';
CREATE INDEX idx_import_batch_rows_entity
  ON import_batch_rows(organization_id, entity_id)
  WHERE entity_id IS NOT NULL;

-- Duplicate checks remain tenant scoped and normalized. These non-unique
-- indexes accelerate import review without prematurely choosing merge policy.
CREATE INDEX idx_contacts_org_active_email_dedupe
  ON contacts(organization_id, lower(email))
  WHERE archived_at IS NULL AND email IS NOT NULL;
CREATE INDEX idx_contacts_org_active_phone_dedupe
  ON contacts(organization_id, regexp_replace(phone, '[^0-9]', '', 'g'))
  WHERE archived_at IS NULL AND phone IS NOT NULL;
CREATE INDEX idx_contacts_org_active_identity_dedupe
  ON contacts(organization_id, lower(first_name), lower(last_name), COALESCE(NULLIF(lower(email), ''), '__empty__'))
  WHERE archived_at IS NULL;
CREATE INDEX idx_companies_org_active_name_dedupe
  ON companies(organization_id, lower(name))
  WHERE archived_at IS NULL;
CREATE INDEX idx_companies_org_active_phone_dedupe
  ON companies(organization_id, regexp_replace(phone, '[^0-9]', '', 'g'))
  WHERE archived_at IS NULL AND phone IS NOT NULL;
CREATE INDEX idx_companies_org_active_website_dedupe
  ON companies(organization_id, lower(website))
  WHERE archived_at IS NULL AND website IS NOT NULL;
