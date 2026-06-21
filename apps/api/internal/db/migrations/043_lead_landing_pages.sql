-- 1.4.2 Hosted landing pages foundation.

ALTER TABLE lead_capture_forms
  ADD CONSTRAINT lead_capture_forms_org_id_unique UNIQUE (organization_id, id);

CREATE TABLE lead_landing_pages (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  public_id TEXT NOT NULL UNIQUE,
  lead_capture_form_id BIGINT NOT NULL,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  title TEXT NOT NULL,
  subtitle TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL DEFAULT '',
  cta_label TEXT NOT NULL DEFAULT 'Submit',
  theme TEXT NOT NULL DEFAULT 'light',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT lead_landing_pages_form_org_fk FOREIGN KEY (organization_id, lead_capture_form_id) REFERENCES lead_capture_forms(organization_id, id) ON DELETE CASCADE,
  CONSTRAINT lead_landing_pages_name_check CHECK (length(trim(name)) > 0),
  CONSTRAINT lead_landing_pages_slug_check CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  CONSTRAINT lead_landing_pages_title_check CHECK (length(trim(title)) > 0),
  CONSTRAINT lead_landing_pages_cta_label_check CHECK (length(trim(cta_label)) > 0),
  CONSTRAINT lead_landing_pages_theme_check CHECK (theme IN ('light', 'blue', 'dark'))
);

CREATE UNIQUE INDEX idx_lead_landing_pages_slug_unique
  ON lead_landing_pages(slug);

CREATE INDEX idx_lead_landing_pages_org_active
  ON lead_landing_pages(organization_id, is_active, updated_at DESC, id DESC);

CREATE INDEX idx_lead_landing_pages_org_form
  ON lead_landing_pages(organization_id, lead_capture_form_id);
