-- open-crm-deploy: expand
-- Support the immutable newest-first cursor used by tenant-scoped sequence
-- enrollment history without changing delivery or enrollment state.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_email_sequence_enrollments_org_sequence_created
  ON email_sequence_enrollments(
    organization_id,
    sequence_id,
    created_at DESC,
    id DESC
  );
