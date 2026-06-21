-- 1.4.7 Rule-based lead scoring and routing foundation.
-- Rules are explicit/admin-managed and are applied by a manual evaluation
-- endpoint before any background routing automation is added.

ALTER TABLE contacts
  ADD COLUMN lead_score INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN lead_grade TEXT NOT NULL DEFAULT '',
  ADD COLUMN lead_scored_at TIMESTAMPTZ,
  ADD COLUMN lead_score_breakdown JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD CONSTRAINT contacts_lead_score_check CHECK (lead_score >= 0 AND lead_score <= 100),
  ADD CONSTRAINT contacts_lead_grade_check CHECK (lead_grade IN ('', 'A', 'B', 'C', 'D')),
  ADD CONSTRAINT contacts_lead_score_breakdown_json_array_check CHECK (jsonb_typeof(lead_score_breakdown) = 'array');

CREATE TABLE lead_scoring_rules (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  field TEXT NOT NULL,
  operator TEXT NOT NULL,
  value TEXT NOT NULL DEFAULT '',
  score_delta INTEGER NOT NULL DEFAULT 0,
  assign_to_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  position INTEGER NOT NULL DEFAULT 0,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT lead_scoring_rules_name_check CHECK (length(trim(name)) > 0),
  CONSTRAINT lead_scoring_rules_field_check CHECK (field IN ('status', 'leadSource', 'utmSource', 'utmMedium', 'utmCampaign', 'jobTitle', 'email', 'phone', 'emailDomain')),
  CONSTRAINT lead_scoring_rules_operator_check CHECK (operator IN ('equals', 'contains', 'exists')),
  CONSTRAINT lead_scoring_rules_value_check CHECK (operator = 'exists' OR length(trim(value)) > 0),
  CONSTRAINT lead_scoring_rules_score_delta_check CHECK (score_delta >= -100 AND score_delta <= 100),
  CONSTRAINT lead_scoring_rules_position_check CHECK (position >= 0)
);

CREATE UNIQUE INDEX idx_lead_scoring_rules_org_name_unique
  ON lead_scoring_rules(organization_id, lower(name));

CREATE INDEX idx_lead_scoring_rules_org_active_position
  ON lead_scoring_rules(organization_id, is_active DESC, position ASC, id ASC);

CREATE INDEX idx_contacts_org_lead_score
  ON contacts(organization_id, lead_score DESC, lead_grade)
  WHERE archived_at IS NULL;
