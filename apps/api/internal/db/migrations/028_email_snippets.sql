-- 1.1.5 Email snippets: reusable, organization-scoped body fragments
-- inserted into one-to-one email drafts and templates.

CREATE TABLE email_snippets (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_email_snippets_org_name ON email_snippets(organization_id, lower(name));
CREATE INDEX idx_email_snippets_org ON email_snippets(organization_id, lower(name));
