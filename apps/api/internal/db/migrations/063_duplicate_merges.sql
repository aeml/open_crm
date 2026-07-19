-- open-crm-deploy: expand

CREATE TABLE duplicate_merge_operations (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id),
  entity_type TEXT NOT NULL
    CONSTRAINT duplicate_merge_operations_entity_type_check
    CHECK (entity_type IN ('contact', 'company')),
  source_entity_id BIGINT NOT NULL,
  target_entity_id BIGINT NOT NULL,
  source_fields JSONB NOT NULL DEFAULT '[]'::jsonb
    CONSTRAINT duplicate_merge_operations_source_fields_check
    CHECK (jsonb_typeof(source_fields) = 'array'),
  relationship_counts JSONB NOT NULL DEFAULT '{}'::jsonb
    CONSTRAINT duplicate_merge_operations_relationship_counts_check
    CHECK (jsonb_typeof(relationship_counts) = 'object'),
  idempotency_key TEXT NOT NULL
    CONSTRAINT duplicate_merge_operations_idempotency_key_check
    CHECK (length(trim(idempotency_key)) BETWEEN 8 AND 200),
  request_sha256 TEXT NOT NULL
    CONSTRAINT duplicate_merge_operations_request_sha256_check
    CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  source_updated_at TIMESTAMPTZ NOT NULL,
  target_updated_at TIMESTAMPTZ NOT NULL,
  target_applied_updated_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT duplicate_merge_operations_distinct_records_check
    CHECK (source_entity_id > 0 AND target_entity_id > 0 AND source_entity_id <> target_entity_id),
  UNIQUE (organization_id, idempotency_key)
);

CREATE UNIQUE INDEX idx_duplicate_merge_operations_source
  ON duplicate_merge_operations(organization_id, entity_type, source_entity_id);

CREATE INDEX idx_duplicate_merge_operations_history
  ON duplicate_merge_operations(organization_id, entity_type, created_at DESC, id DESC);
