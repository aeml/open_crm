-- open-crm-deploy: expand
-- Make public lead capture explicit, replay-safe, and less attractive to blind
-- submission bots without introducing a hosted-only CAPTCHA dependency.

ALTER TABLE lead_capture_forms
  ADD COLUMN consent_text TEXT DEFAULT 'I agree to be contacted about this request.';

ALTER TABLE lead_capture_forms
  ADD CONSTRAINT lead_capture_forms_consent_text_check
  CHECK (
    consent_text IS NOT NULL
    AND length(trim(consent_text)) BETWEEN 1 AND 1000
  ) NOT VALID;

ALTER TABLE lead_capture_forms
  VALIDATE CONSTRAINT lead_capture_forms_consent_text_check;

ALTER TABLE lead_capture_submissions
  ADD COLUMN consent_text_snapshot TEXT,
  ADD COLUMN consented_at TIMESTAMPTZ;

ALTER TABLE lead_capture_submissions
  ADD CONSTRAINT lead_capture_submissions_consent_evidence_check
  CHECK (
    (consent_text_snapshot IS NULL AND consented_at IS NULL)
    OR (
      consent_text_snapshot IS NOT NULL
      AND length(trim(consent_text_snapshot)) BETWEEN 1 AND 1000
      AND consented_at IS NOT NULL
    )
  ) NOT VALID;

ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_consent_evidence_check;

CREATE TABLE lead_capture_submission_challenges (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL,
  form_id BIGINT NOT NULL,
  token_digest TEXT NOT NULL UNIQUE,
  consent_text_snapshot TEXT NOT NULL,
  request_digest TEXT,
  submission_id BIGINT,
  issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  not_before TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  CONSTRAINT lead_capture_submission_challenges_form_fk
    FOREIGN KEY (organization_id, form_id)
    REFERENCES lead_capture_forms(organization_id, id)
    ON DELETE CASCADE,
  CONSTRAINT lead_capture_submission_challenges_token_digest_check
    CHECK (token_digest ~ '^[0-9a-f]{64}$'),
  CONSTRAINT lead_capture_submission_challenges_request_digest_check
    CHECK (request_digest IS NULL OR request_digest ~ '^[0-9a-f]{64}$'),
  CONSTRAINT lead_capture_submission_challenges_consent_text_check
    CHECK (length(trim(consent_text_snapshot)) BETWEEN 1 AND 1000),
  CONSTRAINT lead_capture_submission_challenges_window_check
    CHECK (issued_at < not_before AND not_before < expires_at),
  CONSTRAINT lead_capture_submission_challenges_consumption_check
    CHECK (
      (consumed_at IS NULL AND request_digest IS NULL AND submission_id IS NULL)
      OR
      (consumed_at IS NOT NULL AND request_digest IS NOT NULL AND submission_id IS NOT NULL)
    )
);

CREATE INDEX idx_lead_capture_submission_challenges_cleanup
  ON lead_capture_submission_challenges(expires_at, id);

CREATE INDEX idx_lead_capture_submission_challenges_org_form
  ON lead_capture_submission_challenges(organization_id, form_id, issued_at DESC);
