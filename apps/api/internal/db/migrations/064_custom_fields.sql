-- open-crm-deploy: expand

CREATE TABLE custom_field_definitions (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id),
  entity_type TEXT NOT NULL
    CONSTRAINT custom_field_definitions_entity_type_check
    CHECK (entity_type IN ('contact', 'company')),
  field_key TEXT NOT NULL
    CONSTRAINT custom_field_definitions_key_check
    CHECK (field_key ~ '^[a-z][a-z0-9_]{1,39}$'),
  label TEXT NOT NULL
    CONSTRAINT custom_field_definitions_label_check
    CHECK (length(trim(label)) BETWEEN 1 AND 100),
  data_type TEXT NOT NULL
    CONSTRAINT custom_field_definitions_data_type_check
    CHECK (data_type IN ('text', 'number', 'date', 'boolean', 'select')),
  options_json JSONB NOT NULL DEFAULT '[]'::jsonb
    CONSTRAINT custom_field_definitions_options_array_check
    CHECK (jsonb_typeof(options_json) = 'array'),
  is_required BOOLEAN NOT NULL DEFAULT FALSE,
  show_in_list BOOLEAN NOT NULL DEFAULT FALSE,
  position INTEGER NOT NULL DEFAULT 0
    CONSTRAINT custom_field_definitions_position_check
    CHECK (position BETWEEN 0 AND 1000),
  archived_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (organization_id, entity_type, field_key),
  UNIQUE (organization_id, id)
);

CREATE UNIQUE INDEX idx_custom_field_definitions_org_active_label
  ON custom_field_definitions(organization_id, entity_type, lower(label))
  WHERE archived_at IS NULL;

CREATE INDEX idx_custom_field_definitions_org_entity_position
  ON custom_field_definitions(organization_id, entity_type, position, id)
  WHERE archived_at IS NULL;

ALTER TABLE contacts
  ADD COLUMN custom_fields JSONB DEFAULT '{}'::jsonb
  CONSTRAINT contacts_custom_fields_object_check
  CHECK (custom_fields IS NULL OR jsonb_typeof(custom_fields) = 'object');

ALTER TABLE companies
  ADD COLUMN custom_fields JSONB DEFAULT '{}'::jsonb
  CONSTRAINT companies_custom_fields_object_check
  CHECK (custom_fields IS NULL OR jsonb_typeof(custom_fields) = 'object');

CREATE INDEX idx_contacts_custom_fields_gin
  ON contacts USING GIN(custom_fields jsonb_path_ops);

CREATE INDEX idx_companies_custom_fields_gin
  ON companies USING GIN(custom_fields jsonb_path_ops);

CREATE FUNCTION mergeCustomFields(source_fields JSONB, target_fields JSONB, selected_source_fields TEXT[])
RETURNS JSONB
LANGUAGE SQL
IMMUTABLE
AS $$
  SELECT
    ((source_fields || target_fields) - COALESCE(array_agg(substring(field_name FROM 8)), ARRAY[]::TEXT[]))
    || COALESCE(
      jsonb_object_agg(substring(field_name FROM 8), source_fields -> substring(field_name FROM 8))
        FILTER (WHERE source_fields ? substring(field_name FROM 8)),
      '{}'::jsonb
    )
  FROM unnest(selected_source_fields) field_name
  WHERE field_name LIKE 'custom:%'
$$;
