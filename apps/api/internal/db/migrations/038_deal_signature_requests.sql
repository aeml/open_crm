-- 1.3.4 E-signature status tracking foundation.

CREATE TABLE deal_signature_requests (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  deal_id BIGINT NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
  signer_name TEXT NOT NULL,
  signer_email TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft',
  provider TEXT NOT NULL DEFAULT 'native_tracking',
  external_id TEXT NOT NULL DEFAULT '',
  quote_file_name TEXT NOT NULL DEFAULT '',
  sent_at TIMESTAMPTZ,
  signed_at TIMESTAMPTZ,
  declined_at TIMESTAMPTZ,
  voided_at TIMESTAMPTZ,
  created_by_user_id BIGINT REFERENCES users(id),
  updated_by_user_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT deal_signature_requests_signer_name_nonempty CHECK (signer_name <> ''),
  CONSTRAINT deal_signature_requests_signer_email_nonempty CHECK (signer_email <> ''),
  CONSTRAINT deal_signature_requests_status_check CHECK (status IN ('draft', 'sent', 'signed', 'declined', 'voided')),
  CONSTRAINT deal_signature_requests_provider_nonempty CHECK (provider <> '')
);

CREATE INDEX idx_deal_signature_requests_org_deal_created
  ON deal_signature_requests(organization_id, deal_id, created_at DESC, id DESC);

CREATE INDEX idx_deal_signature_requests_org_status
  ON deal_signature_requests(organization_id, status, updated_at DESC);
