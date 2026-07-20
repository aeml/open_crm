-- open-crm-deploy: expand
-- Short-lived, tenant-scoped reservations close the check-then-insert race for
-- the hosted seat/contact/deal limits. Domain writes consume a reservation in
-- the same transaction as the capacity-increasing effect; abandoned rows
-- expire and are ignored so a crashed request cannot strand quota forever.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE TABLE IF NOT EXISTS billing_capacity_reservations (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  resource TEXT NOT NULL,
  amount INTEGER NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT billing_capacity_reservations_resource_check
    CHECK (resource IN ('seats', 'contacts', 'deals')),
  CONSTRAINT billing_capacity_reservations_amount_check
    CHECK (amount BETWEEN 1 AND 1000),
  CONSTRAINT billing_capacity_reservations_expiry_check
    CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS idx_billing_capacity_reservations_org_resource_expiry
  ON billing_capacity_reservations(organization_id, resource, expires_at, id);
