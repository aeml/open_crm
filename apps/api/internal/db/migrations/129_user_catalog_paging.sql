-- open-crm-deploy: expand
-- Bound retained team-member administration without changing membership state.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_organization_memberships_org_status_user
  ON organization_memberships(
    organization_id,
    (COALESCE(membership_status, 'active')),
    user_id
  );
