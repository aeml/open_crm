-- open-crm-deploy: expand
-- Keep the fixed operational dashboard's two bounded/selective reads from
-- sorting or scanning unrelated tenant history. Deal, task, membership,
-- exchange-rate, quota, and client-review paths already have matching indexes;
-- tenant rollups still deliberately inspect the tenant's active working set.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX IF NOT EXISTS idx_activities_dashboard_recent
  ON activities (organization_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_contacts_dashboard_recent
  ON contacts (organization_id, created_at DESC)
  WHERE archived_at IS NULL;
