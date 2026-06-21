-- 1.4.6 Drip/nurture campaigns foundation: campaign plans connect saved
-- audiences to existing email sequences. Automatic bulk enrollment and sending
-- are future slices so sequence delivery remains explicit and safe.

ALTER TABLE email_sequences
  ADD CONSTRAINT email_sequences_org_id_unique UNIQUE (organization_id, id);

CREATE TABLE lead_nurture_campaigns (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  audience_id BIGINT NOT NULL,
  sequence_id BIGINT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft',
  eligible_count INTEGER NOT NULL DEFAULT 0,
  enrolled_count INTEGER NOT NULL DEFAULT 0,
  last_enrolled_at TIMESTAMPTZ,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT lead_nurture_campaigns_audience_org_fk FOREIGN KEY (organization_id, audience_id) REFERENCES lead_audiences(organization_id, id) ON DELETE RESTRICT,
  CONSTRAINT lead_nurture_campaigns_sequence_org_fk FOREIGN KEY (organization_id, sequence_id) REFERENCES email_sequences(organization_id, id) ON DELETE RESTRICT,
  CONSTRAINT lead_nurture_campaigns_name_check CHECK (length(trim(name)) > 0),
  CONSTRAINT lead_nurture_campaigns_status_check CHECK (status IN ('draft', 'active', 'paused', 'archived')),
  CONSTRAINT lead_nurture_campaigns_counts_check CHECK (eligible_count >= 0 AND enrolled_count >= 0)
);

CREATE UNIQUE INDEX idx_lead_nurture_campaigns_org_name_unique
  ON lead_nurture_campaigns(organization_id, lower(name));

CREATE INDEX idx_lead_nurture_campaigns_org_status
  ON lead_nurture_campaigns(organization_id, status, updated_at DESC, id DESC);

CREATE INDEX idx_lead_nurture_campaigns_org_audience
  ON lead_nurture_campaigns(organization_id, audience_id);

CREATE INDEX idx_lead_nurture_campaigns_org_sequence
  ON lead_nurture_campaigns(organization_id, sequence_id);
