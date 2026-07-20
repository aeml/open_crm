-- open-crm-deploy: expand
-- Each effective owner transition advances a durable generation. Notification
-- idempotency keys use it so transaction retries and unchanged saves are quiet,
-- while assigning a deal away and back still creates a new useful event.

ALTER TABLE deals
  ADD COLUMN owner_assignment_version INTEGER DEFAULT 0
    CONSTRAINT deals_owner_assignment_version_check
    CHECK (owner_assignment_version >= 0);
