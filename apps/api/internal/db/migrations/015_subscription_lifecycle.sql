-- 1.0.6 / 1.0.9 Subscription lifecycle: track each organization's billing
-- status and trial period. New organizations start a time-limited trial.

ALTER TABLE organizations
  ADD COLUMN subscription_status TEXT NOT NULL DEFAULT 'trialing';

ALTER TABLE organizations
  ADD CONSTRAINT organizations_subscription_status_check
  CHECK (subscription_status IN ('trialing', 'active', 'past_due', 'canceled'));

ALTER TABLE organizations
  ADD COLUMN trial_ends_at TIMESTAMPTZ;

-- Existing organizations are treated as active (not in trial).
UPDATE organizations SET subscription_status = 'active' WHERE subscription_status = 'trialing';
