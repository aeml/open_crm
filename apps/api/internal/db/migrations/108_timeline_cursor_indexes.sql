-- open-crm-deploy: expand
-- Notes and record activity use stable (created_at, id) keyset cursors. Include
-- the id tie-breaker in each tenant/entity access path so large histories do
-- not require an in-memory sort for equal timestamps.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_notes_org_entity_created_id
  ON notes(organization_id, entity_type, entity_id, created_at DESC, id DESC);

CREATE INDEX idx_activities_org_entity_created_id
  ON activities(organization_id, entity_type, entity_id, created_at DESC, id DESC);
