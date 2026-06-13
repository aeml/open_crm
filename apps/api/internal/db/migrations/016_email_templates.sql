-- 1.1.5 Email templates: reusable, organization-scoped message templates with
-- merge fields (e.g. {{first_name}}) for one-to-one and bulk email.

CREATE TABLE email_templates (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_email_templates_org_name ON email_templates(organization_id, lower(name));
CREATE INDEX idx_email_templates_org ON email_templates(organization_id, lower(name));
