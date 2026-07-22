-- open-crm-deploy: expand
-- Company detail and recipient/relationship selection page linked contacts by
-- tenant and company. The contact ID tie-breaker keeps the link lookup bounded
-- before the joined contact-name ordering is applied.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_contact_company_links_org_company_contact
  ON contact_company_links(organization_id, company_id, contact_id);
