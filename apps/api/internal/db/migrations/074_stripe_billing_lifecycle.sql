-- open-crm-deploy: expand
-- Durable hosted-billing references, checkout idempotency, signed webhook
-- receipts, and invoice reconciliation. Provider state remains nullable so
-- self-hosted/fake deployments retain their existing behavior.

ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS billing_provider TEXT,
  ADD COLUMN IF NOT EXISTS billing_provider_status TEXT,
  ADD COLUMN IF NOT EXISTS stripe_customer_id TEXT,
  ADD COLUMN IF NOT EXISTS stripe_subscription_id TEXT,
  ADD COLUMN IF NOT EXISTS subscription_current_period_end TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS subscription_cancel_at_period_end BOOLEAN DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS billing_last_event_created BIGINT,
  ADD COLUMN IF NOT EXISTS billing_last_event_id TEXT,
  ADD COLUMN IF NOT EXISTS billing_last_invoice_event_created BIGINT,
  ADD COLUMN IF NOT EXISTS billing_last_invoice_event_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_organizations_stripe_customer_unique
  ON organizations(stripe_customer_id)
  WHERE stripe_customer_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_organizations_stripe_subscription_unique
  ON organizations(stripe_subscription_id)
  WHERE stripe_subscription_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS billing_checkout_requests (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  actor_user_id BIGINT NOT NULL REFERENCES users(id),
  idempotency_key_hash TEXT NOT NULL,
  request_sha256 TEXT NOT NULL,
  plan TEXT NOT NULL CHECK (plan IN ('starter', 'pro', 'enterprise')),
  provider TEXT NOT NULL,
  provider_session_id TEXT,
  checkout_url TEXT,
  checkout_expires_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'creating' CHECK (status IN ('creating', 'created', 'completed', 'failed', 'expired')),
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT billing_checkout_requests_membership_fk
    FOREIGN KEY (organization_id, actor_user_id)
    REFERENCES organization_memberships(organization_id, user_id),
  CONSTRAINT billing_checkout_requests_org_key_unique
    UNIQUE (organization_id, idempotency_key_hash)
);

CREATE INDEX IF NOT EXISTS idx_billing_checkout_requests_org_created
  ON billing_checkout_requests(organization_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS billing_webhook_events (
  id BIGSERIAL PRIMARY KEY,
  provider TEXT NOT NULL,
  provider_event_id TEXT NOT NULL UNIQUE,
  event_type TEXT NOT NULL,
  provider_created BIGINT NOT NULL,
  livemode BOOLEAN NOT NULL,
  payload_sha256 TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'processing' CHECK (status IN ('processing', 'processed', 'ignored', 'failed')),
  attempt_count INTEGER NOT NULL DEFAULT 1,
  organization_id BIGINT REFERENCES organizations(id) ON DELETE SET NULL,
  last_error TEXT,
  processed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_billing_webhook_events_status_created
  ON billing_webhook_events(status, created_at, id);

CREATE TABLE IF NOT EXISTS billing_invoices (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  provider_invoice_id TEXT NOT NULL UNIQUE,
  provider_subscription_id TEXT,
  status TEXT NOT NULL,
  currency TEXT,
  amount_due BIGINT NOT NULL DEFAULT 0,
  amount_paid BIGINT NOT NULL DEFAULT 0,
  hosted_invoice_url TEXT,
  invoice_pdf_url TEXT,
  attempted BOOLEAN NOT NULL DEFAULT FALSE,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  next_payment_attempt TIMESTAMPTZ,
  paid_at TIMESTAMPTZ,
  provider_created_at TIMESTAMPTZ,
  last_event_created BIGINT NOT NULL,
  last_event_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_billing_invoices_org_created
  ON billing_invoices(organization_id, provider_created_at DESC, id DESC);
