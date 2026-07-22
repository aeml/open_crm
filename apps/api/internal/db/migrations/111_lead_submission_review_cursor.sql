-- open-crm-deploy: expand
-- Complete the immutable creation-key order used by the operator lead-review
-- queue for unfiltered and combined form/status continuation.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_lead_capture_submissions_org_created
  ON lead_capture_submissions(organization_id, created_at DESC, id DESC);

CREATE INDEX idx_lead_capture_submissions_org_form_review_created
  ON lead_capture_submissions(
    organization_id,
    form_id,
    review_status,
    created_at DESC,
    id DESC
  );
