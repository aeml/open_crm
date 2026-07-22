-- open-crm-deploy: expand
-- Bind saved-view changes to the exact personal definition the user reviewed
-- and provide an exact index-backed management order.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

ALTER TABLE saved_views
  ADD COLUMN revision INTEGER DEFAULT 1;

UPDATE saved_views SET revision = 1 WHERE revision IS NULL;

ALTER TABLE saved_views
  ADD CONSTRAINT saved_views_revision_positive
  CHECK (revision IS NOT NULL AND revision > 0) NOT VALID;

ALTER TABLE saved_views
  VALIDATE CONSTRAINT saved_views_revision_positive;

CREATE INDEX IF NOT EXISTS idx_saved_views_management
  ON saved_views (organization_id, user_id, entity_type, is_default DESC, lower(name), id);
