-- open-crm-deploy: expand
-- Reconcile existing won deals into the same client model used by new wins.
-- An explicit deal company takes precedence; only company-less wins promote an
-- individual primary contact. Historical rows are not assigned a fabricated
-- actor or activity entry.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_deals_org_company_active_updated
  ON deals(organization_id, company_id, updated_at DESC, id DESC)
  WHERE company_id IS NOT NULL AND archived_at IS NULL;

CREATE INDEX idx_deals_org_primary_contact_active_updated
  ON deals(organization_id, primary_contact_id, updated_at DESC, id DESC)
  WHERE primary_contact_id IS NOT NULL AND archived_at IS NULL;

UPDATE companies company
SET status='customer',updated_at=NOW()
WHERE company.archived_at IS NULL
  AND COALESCE(company.status,'')<>'customer'
  AND EXISTS (
    SELECT 1
    FROM deals deal
    WHERE deal.organization_id=company.organization_id
      AND deal.company_id=company.id
      AND deal.status='won'
      AND deal.archived_at IS NULL
  );

UPDATE contacts contact
SET status='customer',is_client=TRUE,updated_at=NOW()
WHERE contact.archived_at IS NULL
  AND (COALESCE(contact.status,'')<>'customer' OR contact.is_client=FALSE)
  AND EXISTS (
    SELECT 1
    FROM deals deal
    WHERE deal.organization_id=contact.organization_id
      AND deal.company_id IS NULL
      AND deal.primary_contact_id=contact.id
      AND deal.status='won'
      AND deal.archived_at IS NULL
  );
