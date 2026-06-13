-- 1.0.3 Plan tiers: associate each organization with a billing plan.
-- Plans are defined in application code (billing.Catalog); this column records
-- the active plan key for entitlement and limit enforcement.

ALTER TABLE organizations
  ADD COLUMN plan TEXT NOT NULL DEFAULT 'free';

ALTER TABLE organizations
  ADD CONSTRAINT organizations_plan_check
  CHECK (plan IN ('free', 'starter', 'pro', 'enterprise'));

CREATE INDEX idx_organizations_plan ON organizations(plan);
