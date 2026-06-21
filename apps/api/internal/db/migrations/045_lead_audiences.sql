-- 1.4.4 List segmentation and dynamic/saved audiences foundation.

CREATE TABLE lead_audiences (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  filters_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT lead_audiences_name_check CHECK (length(trim(name)) > 0),
  CONSTRAINT lead_audiences_filters_json_object_check CHECK (jsonb_typeof(filters_json) = 'object')
);

CREATE UNIQUE INDEX idx_lead_audiences_org_name_unique
  ON lead_audiences(organization_id, lower(name));

CREATE INDEX idx_lead_audiences_org_active
  ON lead_audiences(organization_id, is_active, updated_at DESC, id DESC);
