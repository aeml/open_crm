-- open-crm-deploy: expand
-- Require an explicit, revision-bound approval before an email sequence can
-- enroll contacts or send. Existing active definitions are paused so the new
-- application must deliberately approve them before delivery resumes.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE email_sequences
  ADD COLUMN revision INTEGER DEFAULT 1,
  ADD COLUMN approved_revision INTEGER,
  ADD COLUMN approved_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  ADD COLUMN approved_at TIMESTAMPTZ;

UPDATE email_sequences
SET status = 'paused',
    updated_at = NOW()
WHERE status = 'active';

ALTER TABLE email_sequences
  ADD CONSTRAINT email_sequences_revision_check CHECK (revision IS NOT NULL AND revision > 0) NOT VALID,
  ADD CONSTRAINT email_sequences_approval_revision_check CHECK (approved_revision IS NULL OR (approved_revision > 0 AND approved_revision <= revision)) NOT VALID,
  ADD CONSTRAINT email_sequences_approval_state_check CHECK (
    (approved_revision IS NULL AND approved_at IS NULL)
    OR (approved_revision IS NOT NULL AND approved_at IS NOT NULL)
  ) NOT VALID;

ALTER TABLE email_sequences VALIDATE CONSTRAINT email_sequences_revision_check;
ALTER TABLE email_sequences VALIDATE CONSTRAINT email_sequences_approval_revision_check;
ALTER TABLE email_sequences VALIDATE CONSTRAINT email_sequences_approval_state_check;
