-- open-crm-deploy: expand
-- Replace manual proposal status changes with a first-party ceremony that is
-- bound to one immutable quote and its recipient-specific delivery link.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE deal_signature_requests
  ADD COLUMN quote_id BIGINT,
  ADD COLUMN signed_name TEXT DEFAULT '',
  ADD COLUMN consent_text_snapshot TEXT DEFAULT '',
  ADD COLUMN consented_at TIMESTAMPTZ,
  ADD COLUMN authentication_method TEXT DEFAULT '',
  ADD COLUMN completion_idempotency_key_hash TEXT DEFAULT '',
  ADD COLUMN completion_request_sha256 TEXT DEFAULT '',
  ADD COLUMN declined_reason TEXT DEFAULT '',
  ADD COLUMN certificate_filename TEXT DEFAULT '',
  ADD COLUMN certificate_content BYTEA DEFAULT '\x',
  ADD COLUMN certificate_sha256 TEXT DEFAULT '',
  ADD CONSTRAINT deal_signature_requests_quote_fk
    FOREIGN KEY (organization_id, quote_id)
    REFERENCES deal_quotes(organization_id, id) ON DELETE CASCADE NOT VALID,
  ADD CONSTRAINT deal_signature_requests_native_quote_check CHECK (
    provider <> 'open_crm_native'
    OR (
      quote_id IS NOT NULL
      AND external_id = ''
      AND CHAR_LENGTH(signer_name) BETWEEN 1 AND 200
      AND CHAR_LENGTH(signer_email) BETWEEN 3 AND 320
      AND CHAR_LENGTH(quote_file_name) BETWEEN 5 AND 240
      AND CHAR_LENGTH(COALESCE(consent_text_snapshot,'')) BETWEEN 50 AND 2000
    )
  ) NOT VALID,
  ADD CONSTRAINT deal_signature_requests_completion_hashes_check CHECK (
    (COALESCE(completion_idempotency_key_hash,'') = '' AND COALESCE(completion_request_sha256,'') = '')
    OR (
      COALESCE(completion_idempotency_key_hash,'') ~ '^[0-9a-f]{64}$'
      AND COALESCE(completion_request_sha256,'') ~ '^[0-9a-f]{64}$'
    )
  ) NOT VALID,
  ADD CONSTRAINT deal_signature_requests_native_state_check CHECK (
    provider <> 'open_crm_native'
    OR (
      (status = 'draft' AND sent_at IS NULL AND signed_at IS NULL AND declined_at IS NULL AND voided_at IS NULL)
      OR (status = 'sent' AND sent_at IS NOT NULL AND signed_at IS NULL AND declined_at IS NULL AND voided_at IS NULL)
      OR (
        status = 'signed'
        AND sent_at IS NOT NULL AND signed_at IS NOT NULL AND consented_at IS NOT NULL
        AND COALESCE(signed_name,'') <> '' AND COALESCE(authentication_method,'') = 'recipient_email_link'
        AND COALESCE(completion_idempotency_key_hash,'') <> '' AND COALESCE(completion_request_sha256,'') <> ''
        AND CHAR_LENGTH(COALESCE(certificate_filename,'')) BETWEEN 5 AND 240
        AND OCTET_LENGTH(COALESCE(certificate_content,'\x'::bytea)) BETWEEN 100 AND 262144
        AND COALESCE(certificate_sha256,'') ~ '^[0-9a-f]{64}$'
        AND declined_at IS NULL AND voided_at IS NULL
      )
      OR (
        status = 'declined'
        AND sent_at IS NOT NULL AND declined_at IS NOT NULL
        AND COALESCE(completion_idempotency_key_hash,'') <> '' AND COALESCE(completion_request_sha256,'') <> ''
        AND signed_at IS NULL AND consented_at IS NULL AND voided_at IS NULL
      )
      OR (status = 'voided' AND voided_at IS NOT NULL AND signed_at IS NULL AND declined_at IS NULL)
    )
  ) NOT VALID,
  ADD CONSTRAINT deal_signature_requests_native_terminal_evidence_check CHECK (
    provider <> 'open_crm_native'
    OR status IN ('signed','declined')
    OR (
      COALESCE(completion_idempotency_key_hash,'') = '' AND COALESCE(completion_request_sha256,'') = ''
      AND COALESCE(signed_name,'') = '' AND consented_at IS NULL AND COALESCE(authentication_method,'') = ''
      AND COALESCE(declined_reason,'') = '' AND COALESCE(certificate_filename,'') = ''
      AND OCTET_LENGTH(COALESCE(certificate_content,'\x'::bytea)) = 0 AND COALESCE(certificate_sha256,'') = ''
    )
  ) NOT VALID,
  ADD CONSTRAINT deal_signature_requests_declined_reason_check CHECK (CHAR_LENGTH(COALESCE(declined_reason,'')) <= 1000) NOT VALID;

CREATE UNIQUE INDEX idx_deal_signature_requests_org_id_quote_deal
  ON deal_signature_requests(organization_id, id, quote_id, deal_id);

CREATE UNIQUE INDEX idx_deal_signature_requests_one_active_quote
  ON deal_signature_requests(organization_id, quote_id)
  WHERE provider = 'open_crm_native' AND status IN ('draft', 'sent', 'signed');

CREATE INDEX idx_deal_signature_requests_org_quote_created
  ON deal_signature_requests(organization_id, quote_id, created_at DESC, id DESC)
  WHERE quote_id IS NOT NULL;

ALTER TABLE deal_quote_deliveries
  ADD COLUMN signature_request_id BIGINT,
  ADD CONSTRAINT deal_quote_deliveries_signature_fk
    FOREIGN KEY (organization_id, signature_request_id, quote_id, deal_id)
    REFERENCES deal_signature_requests(organization_id, id, quote_id, deal_id) NOT VALID;

CREATE UNIQUE INDEX idx_deal_quote_deliveries_org_signature_unique
  ON deal_quote_deliveries(organization_id, signature_request_id);

CREATE INDEX idx_deal_quote_deliveries_org_signature
  ON deal_quote_deliveries(organization_id, signature_request_id)
  WHERE signature_request_id IS NOT NULL;
