-- open-crm-deploy: expand
-- Bind a deliberate staff conversion of retained native signature evidence to
-- exactly one won-stage transition, activity, actor, and idempotent request.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE UNIQUE INDEX idx_deal_stages_org_id_unique
  ON deal_stages(organization_id, id);

CREATE UNIQUE INDEX idx_activities_org_id_unique
  ON activities(organization_id, id);

ALTER TABLE deal_signature_requests
  ADD COLUMN conversion_stage_id BIGINT,
  ADD COLUMN conversion_stage_name TEXT DEFAULT '',
  ADD COLUMN conversion_close_reason_code TEXT DEFAULT '',
  ADD COLUMN conversion_close_reason_label TEXT DEFAULT '',
  ADD COLUMN conversion_close_notes TEXT DEFAULT '',
  ADD COLUMN conversion_activity_id BIGINT,
  ADD COLUMN converted_by_user_id BIGINT,
  ADD COLUMN converted_at TIMESTAMPTZ,
  ADD COLUMN conversion_idempotency_key_hash TEXT DEFAULT '',
  ADD COLUMN conversion_request_sha256 TEXT DEFAULT '',
  ADD CONSTRAINT deal_signature_requests_conversion_stage_fk
    FOREIGN KEY (organization_id, conversion_stage_id)
    REFERENCES deal_stages(organization_id, id) NOT VALID,
  ADD CONSTRAINT deal_signature_requests_conversion_activity_fk
    FOREIGN KEY (organization_id, conversion_activity_id)
    REFERENCES activities(organization_id, id) NOT VALID,
  ADD CONSTRAINT deal_signature_requests_converted_by_fk
    FOREIGN KEY (organization_id, converted_by_user_id)
    REFERENCES organization_memberships(organization_id, user_id) NOT VALID,
  ADD CONSTRAINT deal_signature_requests_conversion_shape_check CHECK (
    (
      converted_at IS NULL
      AND conversion_stage_id IS NULL
      AND COALESCE(conversion_stage_name,'') = ''
      AND COALESCE(conversion_close_reason_code,'') = ''
      AND COALESCE(conversion_close_reason_label,'') = ''
      AND COALESCE(conversion_close_notes,'') = ''
      AND conversion_activity_id IS NULL
      AND converted_by_user_id IS NULL
      AND COALESCE(conversion_idempotency_key_hash,'') = ''
      AND COALESCE(conversion_request_sha256,'') = ''
    )
    OR (
      converted_at IS NOT NULL
      AND provider = 'open_crm_native'
      AND status = 'signed'
      AND conversion_stage_id IS NOT NULL
      AND CHAR_LENGTH(COALESCE(conversion_stage_name,'')) BETWEEN 1 AND 200
      AND COALESCE(conversion_close_reason_code,'') IN ('relationship','solution_fit','price_value','service_quality','timing','other')
      AND CHAR_LENGTH(COALESCE(conversion_close_reason_label,'')) BETWEEN 1 AND 100
      AND CHAR_LENGTH(COALESCE(conversion_close_notes,'')) <= 2000
      AND conversion_activity_id IS NOT NULL
      AND converted_by_user_id IS NOT NULL
      AND COALESCE(conversion_idempotency_key_hash,'') ~ '^[0-9a-f]{64}$'
      AND COALESCE(conversion_request_sha256,'') ~ '^[0-9a-f]{64}$'
    )
  ) NOT VALID;

CREATE INDEX idx_deal_signature_requests_signed_unconverted
  ON deal_signature_requests(organization_id, signed_at, id)
  WHERE provider='open_crm_native' AND status='signed' AND converted_at IS NULL;

CREATE INDEX idx_deal_signature_requests_org_converted
  ON deal_signature_requests(organization_id, converted_at DESC, id DESC)
  WHERE converted_at IS NOT NULL;
