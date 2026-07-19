-- open-crm-deploy: expand
-- Auditable sequence delivery state. SMTP has no universal idempotency key, so
-- a delivery that may have crossed the provider boundary is made explicit and
-- cannot be retried automatically.

CREATE TABLE email_sequence_deliveries (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  enrollment_id BIGINT NOT NULL REFERENCES email_sequence_enrollments(id) ON DELETE CASCADE,
  step_order INTEGER NOT NULL,
  recipient_email TEXT NOT NULL,
  subject TEXT NOT NULL,
  text_body TEXT NOT NULL,
  html_body TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'queued',
  last_error TEXT NOT NULL DEFAULT '',
  attempt_started_at TIMESTAMPTZ,
  finalized_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT email_sequence_deliveries_step_check CHECK (step_order > 0),
  CONSTRAINT email_sequence_deliveries_recipient_check CHECK (BTRIM(recipient_email) <> ''),
  CONSTRAINT email_sequence_deliveries_subject_check CHECK (BTRIM(subject) <> ''),
  CONSTRAINT email_sequence_deliveries_body_check CHECK (BTRIM(text_body) <> ''),
  CONSTRAINT email_sequence_deliveries_status_check CHECK (status IN ('queued', 'sending', 'sent', 'suppressed', 'uncertain')),
  CONSTRAINT email_sequence_deliveries_attempt_state_check CHECK (
    (status = 'queued' AND attempt_started_at IS NULL AND finalized_at IS NULL)
    OR (status = 'sending' AND attempt_started_at IS NOT NULL AND finalized_at IS NULL)
    OR (status IN ('sent', 'suppressed', 'uncertain') AND finalized_at IS NOT NULL)
  ),
  UNIQUE (organization_id, enrollment_id, step_order)
);

CREATE INDEX idx_email_sequence_deliveries_org_status
  ON email_sequence_deliveries(organization_id, status, updated_at DESC, id DESC);

CREATE INDEX idx_email_sequence_deliveries_enrollment
  ON email_sequence_deliveries(organization_id, enrollment_id, step_order);

INSERT INTO background_jobs (organization_id, job_type, idempotency_key, payload_json, max_attempts, run_at)
SELECT e.organization_id,
       'email_sequence.send',
       'enrollment:' || e.id::text || ':step:' || e.current_step_order::text,
       jsonb_build_object('enrollmentId', e.id::text, 'stepOrder', e.current_step_order::text),
       5,
       e.next_send_at
FROM email_sequence_enrollments e
JOIN email_sequences seq ON seq.id = e.sequence_id AND seq.organization_id = e.organization_id AND seq.status = 'active'
WHERE e.status = 'active'
  AND e.next_send_at IS NOT NULL
  AND e.enrolled_by_user_id IS NOT NULL
ON CONFLICT (organization_id, job_type, idempotency_key) DO NOTHING;
