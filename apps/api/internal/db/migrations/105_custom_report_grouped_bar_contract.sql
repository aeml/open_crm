-- open-crm-deploy: expand
-- Makes grouped-bar execution opt-in without activating historical metadata-only rows.

SET lock_timeout = '5s';
SET statement_timeout = '30s';

ALTER TABLE custom_report_definitions
  ADD COLUMN visualization_contract TEXT DEFAULT '';

ALTER TABLE custom_report_definitions
  ADD CONSTRAINT custom_report_definitions_visualization_contract_check
  CHECK (
    COALESCE(visualization_contract, '') = ''
    OR (visualization_type = 'bar' AND visualization_contract = 'grouped_bar_v1')
  ) NOT VALID;

ALTER TABLE custom_report_definitions
  VALIDATE CONSTRAINT custom_report_definitions_visualization_contract_check;
