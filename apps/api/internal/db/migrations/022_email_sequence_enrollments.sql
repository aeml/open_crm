-- 1.1.8 Email sequence enrollment foundation: contacts can be enrolled into
-- sequence definitions and carry scheduler state. A worker will consume this
-- state in a later slice; this migration does not send automated emails.

CREATE TABLE email_sequence_enrollments (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  sequence_id BIGINT NOT NULL REFERENCES email_sequences(id) ON DELETE CASCADE,
  contact_id BIGINT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
  enrolled_by_user_id BIGINT REFERENCES users(id),
  status TEXT NOT NULL DEFAULT 'active',
  current_step_order INTEGER NOT NULL DEFAULT 1,
  next_send_at TIMESTAMPTZ,
  last_sent_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT email_sequence_enrollments_status_check CHECK (status IN ('active', 'paused', 'completed', 'cancelled')),
  CONSTRAINT email_sequence_enrollments_step_check CHECK (current_step_order > 0)
);

CREATE UNIQUE INDEX idx_email_sequence_enrollments_active_contact
  ON email_sequence_enrollments(organization_id, sequence_id, contact_id)
  WHERE status IN ('active', 'paused');

CREATE INDEX idx_email_sequence_enrollments_contact
  ON email_sequence_enrollments(organization_id, contact_id, created_at DESC);

CREATE INDEX idx_email_sequence_enrollments_due
  ON email_sequence_enrollments(organization_id, status, next_send_at)
  WHERE status = 'active' AND next_send_at IS NOT NULL;
