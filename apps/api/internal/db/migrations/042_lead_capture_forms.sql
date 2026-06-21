-- 1.4.1 Embeddable lead capture forms foundation.

CREATE TABLE lead_capture_forms (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  public_id TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  fields_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  success_message TEXT NOT NULL DEFAULT 'Thanks. We will be in touch soon.',
  source_label TEXT NOT NULL DEFAULT 'Lead capture form',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT lead_capture_forms_name_check CHECK (length(trim(name)) > 0),
  CONSTRAINT lead_capture_forms_slug_check CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  CONSTRAINT lead_capture_forms_title_check CHECK (length(trim(title)) > 0),
  CONSTRAINT lead_capture_forms_success_message_check CHECK (length(trim(success_message)) > 0),
  CONSTRAINT lead_capture_forms_source_label_check CHECK (length(trim(source_label)) > 0),
  CONSTRAINT lead_capture_forms_fields_json_array_check CHECK (jsonb_typeof(fields_json) = 'array')
);

CREATE UNIQUE INDEX idx_lead_capture_forms_org_slug_unique
  ON lead_capture_forms(organization_id, slug);

CREATE INDEX idx_lead_capture_forms_org_active
  ON lead_capture_forms(organization_id, is_active, updated_at DESC, id DESC);

CREATE TABLE lead_capture_submissions (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  form_id BIGINT NOT NULL REFERENCES lead_capture_forms(id) ON DELETE CASCADE,
  contact_id BIGINT REFERENCES contacts(id) ON DELETE SET NULL,
  payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  source_url TEXT NOT NULL DEFAULT '',
  remote_addr TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT lead_capture_submissions_payload_json_object_check CHECK (jsonb_typeof(payload_json) = 'object')
);

CREATE INDEX idx_lead_capture_submissions_org_form_created
  ON lead_capture_submissions(organization_id, form_id, created_at DESC, id DESC);

CREATE INDEX idx_lead_capture_submissions_contact
  ON lead_capture_submissions(organization_id, contact_id)
  WHERE contact_id IS NOT NULL;
