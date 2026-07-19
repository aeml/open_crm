-- open-crm-deploy: expand
-- Membership lifecycle is organization-scoped so access can be disabled without
-- deleting the user or historical actor/creator references. The column remains
-- nullable during the expand window; all application reads treat NULL as active,
-- and both existing and future rows are populated by this migration/default.

ALTER TABLE organization_memberships
  ADD COLUMN membership_status TEXT DEFAULT 'active'
    CONSTRAINT organization_memberships_status_check
    CHECK (membership_status IN ('active', 'disabled')),
  ADD COLUMN status_changed_at TIMESTAMPTZ,
  ADD COLUMN status_changed_by_user_id BIGINT REFERENCES users(id);

UPDATE organization_memberships
SET membership_status = 'active'
WHERE membership_status IS NULL;

CREATE INDEX idx_organization_memberships_org_status
  ON organization_memberships(organization_id, membership_status, role, user_id);
