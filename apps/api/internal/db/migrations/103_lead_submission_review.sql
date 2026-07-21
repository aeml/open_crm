-- open-crm-deploy: expand
-- Reversible operator review for public lead submissions. The exact contact
-- archive timestamp binds recovery to the quarantine action instead of
-- restoring an independently archived row. The request ledger retains only
-- digests and bounded effect counts so delayed retries remain harmless.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE lead_capture_submissions
  ADD COLUMN review_status TEXT,
  ADD COLUMN review_version INTEGER,
  ADD COLUMN review_note TEXT,
  ADD COLUMN reviewed_at TIMESTAMPTZ,
  ADD COLUMN reviewed_by_user_id BIGINT,
  ADD COLUMN quarantined_contact_at TIMESTAMPTZ;

ALTER TABLE lead_capture_submissions
  ALTER COLUMN review_status SET DEFAULT 'unreviewed',
  ALTER COLUMN review_version SET DEFAULT 0;

UPDATE lead_capture_submissions
SET review_status = 'unreviewed', review_version = 0
WHERE review_status IS NULL OR review_version IS NULL;

ALTER TABLE lead_capture_submissions
  ADD CONSTRAINT lead_capture_submissions_review_status_present
    CHECK (review_status IS NOT NULL) NOT VALID,
  ADD CONSTRAINT lead_capture_submissions_review_version_present
    CHECK (review_version IS NOT NULL) NOT VALID,
  ADD CONSTRAINT lead_capture_submissions_review_status_check
    CHECK (review_status IN ('unreviewed', 'legitimate', 'spam')) NOT VALID,
  ADD CONSTRAINT lead_capture_submissions_review_version_check
    CHECK (review_version >= 0) NOT VALID,
  ADD CONSTRAINT lead_capture_submissions_review_note_check
    CHECK (review_note IS NULL OR CHAR_LENGTH(review_note) <= 500) NOT VALID,
  ADD CONSTRAINT lead_capture_submissions_review_evidence_check
    CHECK (
      (review_status = 'unreviewed' AND reviewed_at IS NULL AND reviewed_by_user_id IS NULL)
      OR
      (review_status IN ('legitimate', 'spam') AND reviewed_at IS NOT NULL)
    ) NOT VALID,
  ADD CONSTRAINT lead_capture_submissions_quarantine_state_check
    CHECK (quarantined_contact_at IS NULL OR review_status = 'spam') NOT VALID,
  ADD CONSTRAINT lead_capture_submissions_reviewer_membership_fk
    FOREIGN KEY (organization_id, reviewed_by_user_id)
    REFERENCES organization_memberships(organization_id, user_id)
    ON DELETE SET NULL (reviewed_by_user_id) NOT VALID;

ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_review_status_present;
ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_review_version_present;
ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_review_status_check;
ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_review_version_check;
ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_review_note_check;
ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_review_evidence_check;
ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_quarantine_state_check;
ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_reviewer_membership_fk;

CREATE UNIQUE INDEX idx_lead_capture_submissions_org_id_unique
  ON lead_capture_submissions(organization_id, id);

CREATE TABLE lead_capture_submission_review_requests (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  submission_id BIGINT NOT NULL,
  key_digest TEXT NOT NULL,
  request_sha256 TEXT NOT NULL,
  result_review_version INTEGER NOT NULL,
  cancelled_runs INTEGER NOT NULL DEFAULT 0,
  recovered_runs INTEGER NOT NULL DEFAULT 0,
  completed_runs INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT lead_capture_submission_review_requests_submission_fk
    FOREIGN KEY (organization_id, submission_id)
    REFERENCES lead_capture_submissions(organization_id, id) ON DELETE CASCADE,
  CONSTRAINT lead_capture_submission_review_requests_key_check
    CHECK (CHAR_LENGTH(key_digest) = 64),
  CONSTRAINT lead_capture_submission_review_requests_request_check
    CHECK (CHAR_LENGTH(request_sha256) = 64),
  CONSTRAINT lead_capture_submission_review_requests_version_check
    CHECK (result_review_version >= 0),
  CONSTRAINT lead_capture_submission_review_requests_effects_check
    CHECK (cancelled_runs >= 0 AND recovered_runs >= 0 AND completed_runs >= 0),
  CONSTRAINT lead_capture_submission_review_requests_org_key_unique
    UNIQUE (organization_id, submission_id, key_digest)
);

CREATE INDEX idx_lead_capture_submissions_org_review_created
  ON lead_capture_submissions(organization_id, review_status, created_at DESC, id DESC);

CREATE INDEX idx_lead_capture_submissions_review_created
  ON lead_capture_submissions(review_status, created_at);
