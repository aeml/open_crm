-- open-crm-deploy: expand
-- Add one bounded, revision-safe scheduled CSV delivery per saved report. A due
-- occurrence captures one exact artifact and one recipient set before any
-- provider effect; per-recipient state makes acceptance, retryable failure, and
-- ambiguous delivery independently recoverable without cross-tenant joins.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE TABLE custom_report_schedules (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  report_definition_id BIGINT NOT NULL,
  revision BIGINT NOT NULL DEFAULT 1,
  cadence TEXT NOT NULL,
  weekday_utc SMALLINT,
  hour_utc SMALLINT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  next_run_at TIMESTAMPTZ,
  created_by_user_id BIGINT NOT NULL,
  updated_by_user_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT custom_report_schedules_revision_positive CHECK (revision > 0),
  CONSTRAINT custom_report_schedules_cadence_check CHECK (cadence IN ('daily', 'weekly')),
  CONSTRAINT custom_report_schedules_hour_check CHECK (hour_utc BETWEEN 0 AND 23),
  CONSTRAINT custom_report_schedules_weekday_check CHECK (
    (cadence = 'daily' AND weekday_utc IS NULL)
    OR (cadence = 'weekly' AND weekday_utc BETWEEN 0 AND 6)
  ),
  CONSTRAINT custom_report_schedules_org_id_unique UNIQUE (organization_id, id),
  CONSTRAINT custom_report_schedules_definition_unique UNIQUE (organization_id, report_definition_id),
  CONSTRAINT custom_report_schedules_definition_fk
    FOREIGN KEY (organization_id, report_definition_id)
    REFERENCES custom_report_definitions(organization_id, id),
  CONSTRAINT custom_report_schedules_creator_membership_fk
    FOREIGN KEY (organization_id, created_by_user_id)
    REFERENCES organization_memberships(organization_id, user_id),
  CONSTRAINT custom_report_schedules_updater_membership_fk
    FOREIGN KEY (organization_id, updated_by_user_id)
    REFERENCES organization_memberships(organization_id, user_id)
);

CREATE TABLE custom_report_schedule_recipients (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL,
  schedule_id BIGINT NOT NULL,
  recipient_user_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT custom_report_schedule_recipients_schedule_fk
    FOREIGN KEY (organization_id, schedule_id)
    REFERENCES custom_report_schedules(organization_id, id)
    ON DELETE CASCADE,
  CONSTRAINT custom_report_schedule_recipients_membership_fk
    FOREIGN KEY (organization_id, recipient_user_id)
    REFERENCES organization_memberships(organization_id, user_id),
  CONSTRAINT custom_report_schedule_recipients_unique
    UNIQUE (organization_id, schedule_id, recipient_user_id)
);

CREATE TABLE custom_report_delivery_runs (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL,
  schedule_id BIGINT NOT NULL,
  report_definition_id BIGINT NOT NULL,
  schedule_revision BIGINT NOT NULL,
  scheduled_for TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  filename TEXT NOT NULL DEFAULT '',
  content_sha256 TEXT NOT NULL DEFAULT '',
  byte_size BIGINT NOT NULL DEFAULT 0,
  row_count INTEGER NOT NULL DEFAULT 0,
  artifact BYTEA,
  artifact_expires_at TIMESTAMPTZ,
  recovery_generation INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT custom_report_delivery_runs_status_check
    CHECK (status IN ('pending', 'sending', 'succeeded', 'partial', 'failed', 'canceled')),
  CONSTRAINT custom_report_delivery_runs_revision_positive CHECK (schedule_revision > 0),
  CONSTRAINT custom_report_delivery_runs_artifact_shape CHECK (
    (artifact IS NULL AND filename = '' AND content_sha256 = '' AND byte_size = 0 AND artifact_expires_at IS NULL)
    OR
    (artifact IS NOT NULL AND filename <> '' AND content_sha256 ~ '^[0-9a-f]{64}$' AND byte_size = octet_length(artifact) AND byte_size > 0 AND artifact_expires_at IS NOT NULL)
  ),
  CONSTRAINT custom_report_delivery_runs_row_count_nonnegative CHECK (row_count >= 0),
  CONSTRAINT custom_report_delivery_runs_recovery_nonnegative CHECK (recovery_generation >= 0),
  CONSTRAINT custom_report_delivery_runs_org_id_unique UNIQUE (organization_id, id),
  CONSTRAINT custom_report_delivery_runs_occurrence_unique UNIQUE (organization_id, schedule_id, scheduled_for),
  CONSTRAINT custom_report_delivery_runs_schedule_fk
    FOREIGN KEY (organization_id, schedule_id)
    REFERENCES custom_report_schedules(organization_id, id),
  CONSTRAINT custom_report_delivery_runs_definition_fk
    FOREIGN KEY (organization_id, report_definition_id)
    REFERENCES custom_report_definitions(organization_id, id)
);

CREATE TABLE custom_report_recipient_deliveries (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL,
  delivery_run_id BIGINT NOT NULL,
  recipient_user_id BIGINT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  attempt_count INTEGER NOT NULL DEFAULT 0,
  provider_message_id TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  attempted_at TIMESTAMPTZ,
  accepted_at TIMESTAMPTZ,
  resolved_at TIMESTAMPTZ,
  resolved_by_user_id BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT custom_report_recipient_deliveries_status_check
    CHECK (status IN ('pending', 'sending', 'accepted', 'uncertain', 'failed', 'skipped')),
  CONSTRAINT custom_report_recipient_deliveries_attempt_count_check CHECK (attempt_count BETWEEN 0 AND 25),
  CONSTRAINT custom_report_recipient_deliveries_acceptance_check CHECK (
    (status = 'accepted' AND accepted_at IS NOT NULL)
    OR (status <> 'accepted')
  ),
  CONSTRAINT custom_report_recipient_deliveries_resolution_check CHECK (
    (resolved_by_user_id IS NULL AND resolved_at IS NULL)
    OR (resolved_by_user_id IS NOT NULL AND resolved_at IS NOT NULL)
  ),
  CONSTRAINT custom_report_recipient_deliveries_run_fk
    FOREIGN KEY (organization_id, delivery_run_id)
    REFERENCES custom_report_delivery_runs(organization_id, id)
    ON DELETE CASCADE,
  CONSTRAINT custom_report_recipient_deliveries_membership_fk
    FOREIGN KEY (organization_id, recipient_user_id)
    REFERENCES organization_memberships(organization_id, user_id),
  CONSTRAINT custom_report_recipient_deliveries_resolver_membership_fk
    FOREIGN KEY (organization_id, resolved_by_user_id)
    REFERENCES organization_memberships(organization_id, user_id),
  CONSTRAINT custom_report_recipient_deliveries_unique
    UNIQUE (organization_id, delivery_run_id, recipient_user_id)
);

CREATE INDEX idx_custom_report_schedules_due
  ON custom_report_schedules(next_run_at, id)
  WHERE is_active AND next_run_at IS NOT NULL;

CREATE INDEX idx_custom_report_delivery_runs_history
  ON custom_report_delivery_runs(organization_id, created_at DESC, id DESC);

CREATE INDEX idx_custom_report_delivery_runs_artifact_expiry
  ON custom_report_delivery_runs(artifact_expires_at, id)
  WHERE artifact IS NOT NULL;

CREATE INDEX idx_custom_report_recipient_deliveries_recovery
  ON custom_report_recipient_deliveries(status, attempted_at, id)
  WHERE status IN ('sending', 'uncertain', 'failed');
