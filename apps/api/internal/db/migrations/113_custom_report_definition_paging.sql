-- open-crm-deploy: expand
-- Support the exact active-first management order used by bounded saved-report
-- definition pages without changing historical definitions or report results.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_custom_report_definitions_org_management_page
  ON custom_report_definitions(
    organization_id,
    is_active DESC,
    updated_at DESC,
    id DESC
  );
