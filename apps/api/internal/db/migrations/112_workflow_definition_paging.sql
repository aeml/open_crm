-- open-crm-deploy: expand
-- Support the exact active-first management order used by bounded workflow
-- definition pages without changing historical definition or execution state.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_workflow_automations_org_management_page
  ON workflow_automations(
    organization_id,
    is_active DESC,
    position ASC,
    updated_at DESC,
    id DESC
  );
