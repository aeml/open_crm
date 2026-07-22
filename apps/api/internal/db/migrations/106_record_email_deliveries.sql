-- open-crm-deploy: expand
-- Persist one-to-one record-email intent before crossing a connected mailbox
-- provider boundary. Provider calls are claimed once and ambiguous outcomes
-- require an explicit operator decision instead of an automatic retry.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE record_email_deliveries (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL,
  entity_id BIGINT NOT NULL,
  recipient_contact_id BIGINT NOT NULL,
  actor_user_id BIGINT NOT NULL REFERENCES users(id),
  sender_email TEXT NOT NULL,
  recipient_email TEXT NOT NULL,
  subject TEXT NOT NULL,
  text_body TEXT NOT NULL,
  html_body TEXT NOT NULL DEFAULT '',
  list_unsubscribe_url TEXT NOT NULL DEFAULT '',
  rfc_message_id TEXT NOT NULL,
  track_engagement BOOLEAN NOT NULL DEFAULT FALSE,
  tracking_token TEXT NOT NULL DEFAULT '',
  tracked_links_json JSONB NOT NULL DEFAULT '[]'::JSONB,
  idempotency_key_hash TEXT NOT NULL,
  request_sha256 TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'prepared',
  provider_message_id TEXT NOT NULL DEFAULT '',
  provider_thread_id TEXT NOT NULL DEFAULT '',
  outbound_email_message_id BIGINT REFERENCES email_messages(id) ON DELETE SET NULL,
  last_error TEXT NOT NULL DEFAULT '',
  claimed_at TIMESTAMPTZ,
  finalized_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT record_email_deliveries_entity_check CHECK (
    entity_type IN ('contact', 'company', 'deal') AND entity_id > 0 AND recipient_contact_id > 0
  ),
  CONSTRAINT record_email_deliveries_addresses_check CHECK (
    CHAR_LENGTH(sender_email) BETWEEN 3 AND 320 AND CHAR_LENGTH(recipient_email) BETWEEN 3 AND 320
  ),
  CONSTRAINT record_email_deliveries_content_check CHECK (
    CHAR_LENGTH(subject) BETWEEN 1 AND 998
    AND CHAR_LENGTH(text_body) BETWEEN 1 AND 110000
    AND CHAR_LENGTH(html_body) <= 500000
    AND CHAR_LENGTH(list_unsubscribe_url) <= 2000
  ),
  CONSTRAINT record_email_deliveries_message_id_check CHECK (
    CHAR_LENGTH(rfc_message_id) BETWEEN 3 AND 500
    AND CHAR_LENGTH(provider_message_id) <= 500
    AND CHAR_LENGTH(provider_thread_id) <= 500
  ),
  CONSTRAINT record_email_deliveries_tracking_check CHECK (
    JSONB_TYPEOF(tracked_links_json) = 'array'
    AND JSONB_ARRAY_LENGTH(tracked_links_json) <= 100
    AND OCTET_LENGTH(tracked_links_json::TEXT) <= 100000
    AND (
      (track_engagement AND (
        (status IN ('prepared','sending','uncertain') AND CHAR_LENGTH(tracking_token) BETWEEN 40 AND 100)
        OR (status IN ('accepted','failed') AND tracking_token = '' AND tracked_links_json = '[]'::JSONB)
      ))
      OR (NOT track_engagement AND tracking_token = '' AND tracked_links_json = '[]'::JSONB)
    )
  ),
  CONSTRAINT record_email_deliveries_key_hash_check CHECK (CHAR_LENGTH(idempotency_key_hash) = 64),
  CONSTRAINT record_email_deliveries_request_hash_check CHECK (CHAR_LENGTH(request_sha256) = 64),
  CONSTRAINT record_email_deliveries_status_check CHECK (
    status IN ('prepared', 'sending', 'accepted', 'failed', 'uncertain')
  ),
  CONSTRAINT record_email_deliveries_state_check CHECK (
    (status = 'prepared' AND claimed_at IS NULL AND finalized_at IS NULL AND outbound_email_message_id IS NULL)
    OR (status = 'sending' AND claimed_at IS NOT NULL AND finalized_at IS NULL AND outbound_email_message_id IS NULL)
    OR (status = 'accepted' AND claimed_at IS NOT NULL AND finalized_at IS NOT NULL AND outbound_email_message_id IS NOT NULL)
    OR (status = 'failed' AND claimed_at IS NOT NULL AND finalized_at IS NOT NULL)
    OR (status = 'uncertain' AND claimed_at IS NOT NULL AND finalized_at IS NOT NULL AND outbound_email_message_id IS NULL)
  ),
  UNIQUE (organization_id, actor_user_id, idempotency_key_hash)
);

CREATE INDEX idx_record_email_deliveries_org_entity
  ON record_email_deliveries(organization_id, entity_type, entity_id, created_at DESC, id DESC);

CREATE INDEX idx_record_email_deliveries_stale_sending
  ON record_email_deliveries(claimed_at, id)
  WHERE status = 'sending';

CREATE UNIQUE INDEX idx_record_email_deliveries_one_unresolved_actor_entity
  ON record_email_deliveries(organization_id, actor_user_id, entity_type, entity_id)
  WHERE status IN ('prepared', 'sending', 'uncertain');
