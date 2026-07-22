-- open-crm-deploy: expand
-- Bound reusable email-template and snippet management without hiding legacy
-- rows. Revisions let writers bind edits and deletes to the exact definition
-- they reviewed; existing definitions begin at revision 1.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

ALTER TABLE email_templates
  ADD COLUMN revision INTEGER DEFAULT 1;

UPDATE email_templates SET revision = 1 WHERE revision IS NULL;

ALTER TABLE email_templates
  ADD CONSTRAINT email_templates_revision_positive
  CHECK (revision IS NOT NULL AND revision > 0) NOT VALID;

ALTER TABLE email_templates
  VALIDATE CONSTRAINT email_templates_revision_positive;

ALTER TABLE email_snippets
  ADD COLUMN revision INTEGER DEFAULT 1;

UPDATE email_snippets SET revision = 1 WHERE revision IS NULL;

ALTER TABLE email_snippets
  ADD CONSTRAINT email_snippets_revision_positive
  CHECK (revision IS NOT NULL AND revision > 0) NOT VALID;

ALTER TABLE email_snippets
  VALIDATE CONSTRAINT email_snippets_revision_positive;

-- PostgreSQL does not use the tenant-scoped lower(name) uniqueness proof to
-- eliminate the immutable-ID tie-breaker sort. These exact management indexes
-- keep both normal bounded catalogs and legacy overflow on an index-backed
-- page plan.
CREATE INDEX IF NOT EXISTS idx_email_templates_org_name_id
  ON email_templates (organization_id, lower(name), id);

CREATE INDEX IF NOT EXISTS idx_email_snippets_org_name_id
  ON email_snippets (organization_id, lower(name), id);
