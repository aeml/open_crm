-- open-crm-deploy: expand
-- Make public landing-page and website-widget administration revision safe.

SET lock_timeout = '5s';
SET statement_timeout = '30s';

ALTER TABLE lead_landing_pages
  ADD COLUMN revision INTEGER DEFAULT 1;

ALTER TABLE lead_landing_pages
  ADD CONSTRAINT lead_landing_pages_revision_positive
  CHECK (revision IS NOT NULL AND revision > 0) NOT VALID;

ALTER TABLE lead_landing_pages
  VALIDATE CONSTRAINT lead_landing_pages_revision_positive;

ALTER TABLE lead_chat_widgets
  ADD COLUMN revision INTEGER DEFAULT 1;

ALTER TABLE lead_chat_widgets
  ADD CONSTRAINT lead_chat_widgets_revision_positive
  CHECK (revision IS NOT NULL AND revision > 0) NOT VALID;

ALTER TABLE lead_chat_widgets
  VALIDATE CONSTRAINT lead_chat_widgets_revision_positive;

RESET statement_timeout;
RESET lock_timeout;
