-- open-crm-deploy: expand
-- Bulk operations retain only reversible CRM state, never contact, client,
-- deal, or task content. The applied version makes rollback refuse to replace
-- a teammate's later edit.

CREATE TABLE bulk_operations (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id),
  entity_type TEXT NOT NULL
    CONSTRAINT bulk_operations_entity_type_check
    CHECK (entity_type IN ('contact', 'company', 'deal', 'task')),
  action TEXT NOT NULL
    CONSTRAINT bulk_operations_action_check
    CHECK (action IN ('archive', 'reassign', 'set_status')),
  action_value TEXT,
  target_user_id BIGINT REFERENCES users(id),
  idempotency_key TEXT NOT NULL
    CONSTRAINT bulk_operations_idempotency_key_check
    CHECK (length(trim(idempotency_key)) BETWEEN 8 AND 200),
  request_sha256 TEXT NOT NULL
    CONSTRAINT bulk_operations_request_sha256_check
    CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL DEFAULT 'completed'
    CONSTRAINT bulk_operations_status_check
    CHECK (status IN ('completed', 'rolled_back', 'partially_rolled_back')),
  target_count INTEGER NOT NULL
    CONSTRAINT bulk_operations_target_count_check
    CHECK (target_count BETWEEN 1 AND 100),
  changed_count INTEGER NOT NULL DEFAULT 0
    CONSTRAINT bulk_operations_changed_count_check
    CHECK (changed_count >= 0 AND changed_count <= target_count),
  rolled_back_count INTEGER NOT NULL DEFAULT 0
    CONSTRAINT bulk_operations_rolled_back_count_check
    CHECK (rolled_back_count >= 0 AND rolled_back_count <= changed_count),
  rollback_skipped_count INTEGER NOT NULL DEFAULT 0
    CONSTRAINT bulk_operations_rollback_skipped_count_check
    CHECK (rollback_skipped_count >= 0 AND rollback_skipped_count <= changed_count),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  rolled_back_at TIMESTAMPTZ,
  UNIQUE (organization_id, idempotency_key),
  UNIQUE (organization_id, id)
);

CREATE TABLE bulk_operation_rows (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  bulk_operation_id BIGINT NOT NULL,
  entity_id BIGINT NOT NULL,
  before_owner_user_id BIGINT REFERENCES users(id),
  before_status TEXT,
  before_archived_at TIMESTAMPTZ,
  before_completed_at TIMESTAMPTZ,
  applied_entity_updated_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL DEFAULT 'applied'
    CONSTRAINT bulk_operation_rows_status_check
    CHECK (status IN ('applied', 'rolled_back', 'rollback_skipped')),
  rollback_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (organization_id, bulk_operation_id, entity_id),
  FOREIGN KEY (organization_id, bulk_operation_id)
    REFERENCES bulk_operations(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_bulk_operations_org_created
  ON bulk_operations(organization_id, created_at DESC, id DESC);
CREATE INDEX idx_bulk_operations_org_entity_created
  ON bulk_operations(organization_id, entity_type, created_at DESC, id DESC);
CREATE INDEX idx_bulk_operation_rows_entity
  ON bulk_operation_rows(organization_id, entity_id, created_at DESC);
CREATE INDEX idx_bulk_operation_rows_rollback
  ON bulk_operation_rows(organization_id, bulk_operation_id, id)
  WHERE status = 'applied';
