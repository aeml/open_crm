-- open-crm-deploy: expand
-- Provider-API reconciliation evidence. Durable attempts and retries live in
-- background_jobs; these columns expose the latest tenant-level outcome
-- without copying provider payloads or credentials into application state.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS billing_last_reconciliation_attempt_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS billing_last_reconciled_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS billing_last_reconciliation_error TEXT;

CREATE INDEX IF NOT EXISTS idx_organizations_billing_reconciliation_due
  ON organizations(billing_last_reconciled_at NULLS FIRST, id)
  WHERE billing_provider = 'stripe'
    AND stripe_customer_id IS NOT NULL
    AND stripe_subscription_id IS NOT NULL;
