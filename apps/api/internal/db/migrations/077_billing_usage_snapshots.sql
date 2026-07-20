-- open-crm-deploy: expand
-- Explainable hosted-usage evidence. Source records remain authoritative; this
-- table retains the latest reconciled observation for each billing period so
-- operators can compare what the customer saw with the underlying data.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS subscription_current_period_start TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS billing_usage_snapshots (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  period_start TIMESTAMPTZ NOT NULL,
  period_end TIMESTAMPTZ NOT NULL,
  period_basis TEXT NOT NULL,
  seats_used BIGINT NOT NULL DEFAULT 0,
  contacts_used BIGINT NOT NULL DEFAULT 0,
  deals_used BIGINT NOT NULL DEFAULT 0,
  outbound_messages_used BIGINT NOT NULL DEFAULT 0,
  automation_executions_used BIGINT NOT NULL DEFAULT 0,
  background_job_executions_used BIGINT NOT NULL DEFAULT 0,
  storage_bytes_used BIGINT NOT NULL DEFAULT 0,
  source_table_count INTEGER NOT NULL DEFAULT 0,
  observed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT billing_usage_snapshots_period_check CHECK (period_end > period_start),
  CONSTRAINT billing_usage_snapshots_basis_check CHECK (period_basis IN ('provider_subscription', 'calendar_month')),
  CONSTRAINT billing_usage_snapshots_counts_check CHECK (
    seats_used >= 0 AND contacts_used >= 0 AND deals_used >= 0 AND
    outbound_messages_used >= 0 AND automation_executions_used >= 0 AND
    background_job_executions_used >= 0 AND storage_bytes_used >= 0 AND
    source_table_count >= 0
  ),
  CONSTRAINT billing_usage_snapshots_period_unique UNIQUE (organization_id, period_start, period_end)
);

CREATE INDEX IF NOT EXISTS idx_billing_usage_snapshots_org_observed
  ON billing_usage_snapshots(organization_id, observed_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_email_messages_org_sent_period
  ON email_messages(organization_id, created_at DESC, id DESC)
  WHERE direction = 'outbound' AND status = 'sent';

CREATE INDEX IF NOT EXISTS idx_workflow_automation_runs_org_succeeded_period
  ON workflow_automation_runs(organization_id, completed_at DESC, id DESC)
  WHERE status = 'succeeded';

CREATE INDEX IF NOT EXISTS idx_background_jobs_org_succeeded_period
  ON background_jobs(organization_id, completed_at DESC, id DESC)
  WHERE status = 'succeeded';
