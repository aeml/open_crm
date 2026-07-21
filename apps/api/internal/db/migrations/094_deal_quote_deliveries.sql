-- open-crm-deploy: expand
-- Persist quote-delivery intent before crossing a connected-mailbox provider
-- boundary, and keep customer access/receipt evidence separate from signature
-- or commercial acceptance.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE deal_quote_deliveries (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  deal_id BIGINT NOT NULL,
  quote_id BIGINT NOT NULL,
  actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  sender_email TEXT NOT NULL,
  recipient_email TEXT NOT NULL,
  subject TEXT NOT NULL,
  message_body TEXT NOT NULL,
  rfc_message_id TEXT NOT NULL,
  access_token_digest TEXT NOT NULL,
  access_expires_at TIMESTAMPTZ NOT NULL,
  idempotency_key_hash TEXT NOT NULL,
  request_sha256 TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'prepared',
  provider_message_id TEXT NOT NULL DEFAULT '',
  provider_thread_id TEXT NOT NULL DEFAULT '',
  outbound_email_message_id BIGINT,
  last_error TEXT NOT NULL DEFAULT '',
  claimed_at TIMESTAMPTZ,
  finalized_at TIMESTAMPTZ,
  sent_at TIMESTAMPTZ,
  first_accessed_at TIMESTAMPTZ,
  last_accessed_at TIMESTAMPTZ,
  access_count INTEGER NOT NULL DEFAULT 0,
  first_downloaded_at TIMESTAMPTZ,
  last_downloaded_at TIMESTAMPTZ,
  download_count INTEGER NOT NULL DEFAULT 0,
  receipt_confirmed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT deal_quote_deliveries_quote_fk
    FOREIGN KEY (organization_id, quote_id)
    REFERENCES deal_quotes(organization_id, id) ON DELETE CASCADE,
  CONSTRAINT deal_quote_deliveries_deal_fk
    FOREIGN KEY (organization_id, deal_id)
    REFERENCES deals(organization_id, id) ON DELETE CASCADE,
  CONSTRAINT deal_quote_deliveries_email_fk
    FOREIGN KEY (organization_id, outbound_email_message_id)
    REFERENCES email_messages(organization_id, id)
    ON DELETE SET NULL (outbound_email_message_id),
  CONSTRAINT deal_quote_deliveries_addresses_check CHECK (
    CHAR_LENGTH(sender_email) BETWEEN 3 AND 320
    AND CHAR_LENGTH(recipient_email) BETWEEN 3 AND 320
  ),
  CONSTRAINT deal_quote_deliveries_content_check CHECK (
    CHAR_LENGTH(subject) BETWEEN 1 AND 500
    AND CHAR_LENGTH(message_body) BETWEEN 1 AND 10000
    AND CHAR_LENGTH(rfc_message_id) BETWEEN 3 AND 500
  ),
  CONSTRAINT deal_quote_deliveries_hashes_check CHECK (
    access_token_digest ~ '^[0-9a-f]{64}$'
    AND idempotency_key_hash ~ '^[0-9a-f]{64}$'
    AND request_sha256 ~ '^[0-9a-f]{64}$'
  ),
  CONSTRAINT deal_quote_deliveries_status_check CHECK (
    status IN ('prepared', 'sending', 'sent', 'uncertain', 'failed')
  ),
  CONSTRAINT deal_quote_deliveries_correlation_check CHECK (
    CHAR_LENGTH(provider_message_id) <= 500
    AND CHAR_LENGTH(provider_thread_id) <= 500
  ),
  CONSTRAINT deal_quote_deliveries_counts_check CHECK (
    access_count >= 0 AND download_count >= 0
  ),
  CONSTRAINT deal_quote_deliveries_access_state_check CHECK (
    (access_count = 0 AND first_accessed_at IS NULL AND last_accessed_at IS NULL)
    OR (access_count > 0 AND first_accessed_at IS NOT NULL AND last_accessed_at IS NOT NULL AND first_accessed_at <= last_accessed_at)
  ),
  CONSTRAINT deal_quote_deliveries_download_state_check CHECK (
    (download_count = 0 AND first_downloaded_at IS NULL AND last_downloaded_at IS NULL)
    OR (download_count > 0 AND first_downloaded_at IS NOT NULL AND last_downloaded_at IS NOT NULL AND first_downloaded_at <= last_downloaded_at)
  ),
  CONSTRAINT deal_quote_deliveries_receipt_state_check CHECK (
    receipt_confirmed_at IS NULL OR (status = 'sent' AND sent_at IS NOT NULL AND receipt_confirmed_at >= sent_at)
  ),
  CONSTRAINT deal_quote_deliveries_send_state_check CHECK (
    (status = 'prepared' AND claimed_at IS NULL AND finalized_at IS NULL AND sent_at IS NULL AND last_error = '')
    OR (status = 'sending' AND claimed_at IS NOT NULL AND finalized_at IS NULL AND sent_at IS NULL AND last_error = '')
    OR (status = 'sent' AND claimed_at IS NOT NULL AND finalized_at IS NOT NULL AND sent_at IS NOT NULL AND last_error = '')
    OR (status = 'uncertain' AND claimed_at IS NOT NULL AND finalized_at IS NOT NULL AND sent_at IS NULL AND last_error <> '')
    OR (status = 'failed' AND finalized_at IS NOT NULL AND sent_at IS NULL AND last_error <> '')
  ),
  UNIQUE (organization_id, id),
  UNIQUE (organization_id, actor_user_id, idempotency_key_hash),
  UNIQUE (access_token_digest)
);

CREATE INDEX idx_deal_quote_deliveries_org_quote_created
  ON deal_quote_deliveries(organization_id, quote_id, created_at DESC, id DESC);

CREATE INDEX idx_deal_quote_deliveries_stale_sending
  ON deal_quote_deliveries(claimed_at, id)
  WHERE status = 'sending';

CREATE UNIQUE INDEX idx_deal_quote_deliveries_one_unresolved_quote
  ON deal_quote_deliveries(organization_id, quote_id)
  WHERE status IN ('prepared', 'sending', 'uncertain');
