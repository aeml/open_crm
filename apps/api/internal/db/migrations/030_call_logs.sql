-- 1.2.1 Click-to-call and call logging foundation.

CREATE TABLE call_logs (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL,
  entity_id BIGINT NOT NULL,
  direction TEXT NOT NULL DEFAULT 'outbound',
  phone_number TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'initiated',
  disposition TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  provider_name TEXT NOT NULL DEFAULT '',
  provider_call_id TEXT NOT NULL DEFAULT '',
  created_by_user_id BIGINT NOT NULL REFERENCES users(id),
  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT call_logs_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal')),
  CONSTRAINT call_logs_direction_check CHECK (direction IN ('outbound', 'inbound')),
  CONSTRAINT call_logs_status_check CHECK (status IN ('initiated', 'completed', 'failed'))
);

CREATE INDEX idx_call_logs_org_entity_created
  ON call_logs(organization_id, entity_type, entity_id, created_at DESC, id DESC);

CREATE INDEX idx_call_logs_org_created
  ON call_logs(organization_id, created_at DESC, id DESC);
