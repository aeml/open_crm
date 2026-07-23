-- open-crm-deploy: expand

SET lock_timeout = '5s';
SET statement_timeout = '30s';

ALTER TABLE custom_field_definitions
  ADD COLUMN revision INTEGER DEFAULT 1;

ALTER TABLE custom_field_definitions
  ADD CONSTRAINT custom_field_definitions_revision_positive
  CHECK (revision IS NOT NULL AND revision > 0) NOT VALID;

ALTER TABLE custom_field_definitions
  VALIDATE CONSTRAINT custom_field_definitions_revision_positive;

RESET statement_timeout;
RESET lock_timeout;
