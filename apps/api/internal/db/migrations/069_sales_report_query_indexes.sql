-- open-crm-deploy: expand
-- Sales activity reports bound work by tenant and date, then optionally by
-- actor. Keep those access paths aligned with the report's exact action set;
-- unrelated audit history must not make a sales read scan the tenant ledger.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_activities_sales_report_org_created
  ON activities(organization_id, created_at DESC)
  INCLUDE (actor_user_id, action)
  WHERE action IN ('note.created', 'task.created', 'task.automated', 'task.completed');

CREATE INDEX idx_activities_sales_report_org_actor_created
  ON activities(organization_id, actor_user_id, created_at DESC)
  INCLUDE (action)
  WHERE action IN ('note.created', 'task.created', 'task.automated', 'task.completed');
