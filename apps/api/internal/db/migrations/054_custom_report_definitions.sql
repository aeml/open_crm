-- 1.6.1 Custom report builder definition foundation.
-- Stores report builder metadata before analytics query execution exists.

CREATE TABLE custom_report_definitions (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  source_type TEXT NOT NULL,
  columns_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  filters_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  group_by TEXT NOT NULL DEFAULT '',
  aggregation_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT custom_report_definitions_name_check CHECK (length(trim(name)) > 0),
  CONSTRAINT custom_report_definitions_source_type_check CHECK (source_type IN ('contacts', 'companies', 'deals', 'tasks')),
  CONSTRAINT custom_report_definitions_columns_json_array_check CHECK (jsonb_typeof(columns_json) = 'array'),
  CONSTRAINT custom_report_definitions_filters_json_array_check CHECK (jsonb_typeof(filters_json) = 'array'),
  CONSTRAINT custom_report_definitions_aggregation_json_object_check CHECK (jsonb_typeof(aggregation_json) = 'object')
);

CREATE UNIQUE INDEX idx_custom_report_definitions_org_name_unique
  ON custom_report_definitions(organization_id, lower(name));

CREATE INDEX idx_custom_report_definitions_org_active
  ON custom_report_definitions(organization_id, is_active, updated_at DESC, id DESC);

CREATE INDEX idx_custom_report_definitions_org_source
  ON custom_report_definitions(organization_id, source_type, updated_at DESC, id DESC);
