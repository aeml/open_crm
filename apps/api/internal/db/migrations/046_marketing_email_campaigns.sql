-- 1.4.5 Marketing email campaigns foundation: campaign definitions tied to
-- saved audiences with schedule metadata and per-campaign analytics counters.
-- Delivery, recipient expansion, and tracking workers are future slices.

ALTER TABLE lead_audiences
  ADD CONSTRAINT lead_audiences_org_id_unique UNIQUE (organization_id, id);

CREATE TABLE marketing_email_campaigns (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  audience_id BIGINT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  subject TEXT NOT NULL,
  preview_text TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft',
  scheduled_at TIMESTAMPTZ,
  sent_at TIMESTAMPTZ,
  recipient_count INTEGER NOT NULL DEFAULT 0,
  sent_count INTEGER NOT NULL DEFAULT 0,
  opened_count INTEGER NOT NULL DEFAULT 0,
  clicked_count INTEGER NOT NULL DEFAULT 0,
  bounced_count INTEGER NOT NULL DEFAULT 0,
  unsubscribed_count INTEGER NOT NULL DEFAULT 0,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT marketing_email_campaigns_audience_org_fk FOREIGN KEY (organization_id, audience_id) REFERENCES lead_audiences(organization_id, id) ON DELETE RESTRICT,
  CONSTRAINT marketing_email_campaigns_name_check CHECK (length(trim(name)) > 0),
  CONSTRAINT marketing_email_campaigns_subject_check CHECK (length(trim(subject)) > 0),
  CONSTRAINT marketing_email_campaigns_body_check CHECK (length(trim(body)) > 0),
  CONSTRAINT marketing_email_campaigns_status_check CHECK (status IN ('draft', 'scheduled', 'paused', 'sent', 'cancelled')),
  CONSTRAINT marketing_email_campaigns_schedule_check CHECK (status <> 'scheduled' OR scheduled_at IS NOT NULL),
  CONSTRAINT marketing_email_campaigns_counts_check CHECK (
    recipient_count >= 0 AND
    sent_count >= 0 AND
    opened_count >= 0 AND
    clicked_count >= 0 AND
    bounced_count >= 0 AND
    unsubscribed_count >= 0
  )
);

CREATE UNIQUE INDEX idx_marketing_email_campaigns_org_name_unique
  ON marketing_email_campaigns(organization_id, lower(name));

CREATE INDEX idx_marketing_email_campaigns_org_status
  ON marketing_email_campaigns(organization_id, status, updated_at DESC, id DESC);

CREATE INDEX idx_marketing_email_campaigns_org_scheduled
  ON marketing_email_campaigns(organization_id, scheduled_at, id)
  WHERE status = 'scheduled';
