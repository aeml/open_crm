-- 1.1.8 Email sequences foundation: reusable, organization-scoped cadence
-- definitions with ordered steps. Enrollment, scheduling, and reply detection
-- are added in later slices.

CREATE TABLE email_sequences (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft',
  created_by_user_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT email_sequences_status_check CHECK (status IN ('draft', 'active', 'paused'))
);

CREATE UNIQUE INDEX idx_email_sequences_org_name ON email_sequences(organization_id, lower(name));
CREATE INDEX idx_email_sequences_org_status ON email_sequences(organization_id, status, lower(name));

CREATE TABLE email_sequence_steps (
  id BIGSERIAL PRIMARY KEY,
  sequence_id BIGINT NOT NULL REFERENCES email_sequences(id) ON DELETE CASCADE,
  step_order INTEGER NOT NULL,
  delay_days INTEGER NOT NULL DEFAULT 0,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT email_sequence_steps_order_check CHECK (step_order > 0),
  CONSTRAINT email_sequence_steps_delay_check CHECK (delay_days >= 0),
  CONSTRAINT email_sequence_steps_unique_order UNIQUE (sequence_id, step_order)
);

CREATE INDEX idx_email_sequence_steps_sequence ON email_sequence_steps(sequence_id, step_order);
