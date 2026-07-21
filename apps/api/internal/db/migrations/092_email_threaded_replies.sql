-- open-crm-deploy: expand
-- Give mailbox conversations a stable tenant-scoped root and persist each
-- reply intent before crossing the external mailbox-provider boundary.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE email_messages
  ADD COLUMN thread_root_message_id BIGINT;

CREATE FUNCTION email_messages_default_thread_root()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  IF NEW.thread_root_message_id IS NULL THEN
    NEW.thread_root_message_id := NEW.id;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER trg_email_messages_default_thread_root
BEFORE INSERT ON email_messages
FOR EACH ROW EXECUTE FUNCTION email_messages_default_thread_root();

UPDATE email_messages
SET thread_root_message_id = id
WHERE thread_root_message_id IS NULL;

CREATE INDEX idx_email_messages_org_rfc_message
  ON email_messages(organization_id, mailbox_user_id, rfc_message_id)
  WHERE rfc_message_id <> '';

CREATE INDEX idx_email_messages_org_provider_thread
  ON email_messages(organization_id, mailbox_user_id, provider_thread_id)
  WHERE provider_thread_id <> '';

WITH provider_roots AS (
  SELECT organization_id, mailbox_user_id, provider_thread_id, MIN(id) AS root_id
  FROM email_messages
  WHERE provider_thread_id <> '' AND mailbox_user_id IS NOT NULL
  GROUP BY organization_id, mailbox_user_id, provider_thread_id
)
UPDATE email_messages message
SET thread_root_message_id = provider_roots.root_id
FROM provider_roots
WHERE message.organization_id = provider_roots.organization_id
  AND message.mailbox_user_id = provider_roots.mailbox_user_id
  AND message.provider_thread_id = provider_roots.provider_thread_id;

-- Propagate header-only chains to their oldest known ancestor. Roots move only
-- toward a smaller message ID, so malformed cycles converge instead of
-- oscillating, and a three-or-more-message historical chain is not split.
DO $$
DECLARE
  updated_count BIGINT;
BEGIN
  LOOP
    WITH resolved_roots AS MATERIALIZED (
      SELECT child.organization_id, child.id, MIN(parent.thread_root_message_id) AS root_id
      FROM email_messages child
      JOIN email_messages parent
        ON parent.organization_id = child.organization_id
       AND parent.mailbox_user_id = child.mailbox_user_id
       AND parent.rfc_message_id = child.in_reply_to
      WHERE child.in_reply_to <> ''
        AND parent.thread_root_message_id < child.thread_root_message_id
      GROUP BY child.organization_id, child.id
    ), updated AS (
      UPDATE email_messages child
      SET thread_root_message_id = resolved_roots.root_id
      FROM resolved_roots
      WHERE child.organization_id = resolved_roots.organization_id
        AND child.id = resolved_roots.id
        AND child.thread_root_message_id > resolved_roots.root_id
      RETURNING child.id
    )
    SELECT COUNT(*) INTO updated_count FROM updated;

    EXIT WHEN updated_count = 0;
  END LOOP;
END;
$$;

CREATE UNIQUE INDEX idx_email_messages_org_id
  ON email_messages(organization_id, id);

ALTER TABLE email_messages
  ADD CONSTRAINT email_messages_thread_root_fk
    FOREIGN KEY (organization_id, thread_root_message_id)
    REFERENCES email_messages(organization_id, id) NOT VALID,
  ADD CONSTRAINT email_messages_thread_root_present_check
    CHECK (thread_root_message_id IS NOT NULL) NOT VALID;

ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_thread_root_fk;
ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_thread_root_present_check;

CREATE INDEX idx_email_messages_org_thread
  ON email_messages(organization_id, thread_root_message_id, received_at, created_at, id);

CREATE TABLE email_reply_requests (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  source_message_id BIGINT NOT NULL,
  thread_root_message_id BIGINT NOT NULL,
  actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  sender_email TEXT NOT NULL,
  recipient_email TEXT NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  visibility TEXT NOT NULL,
  rfc_message_id TEXT NOT NULL,
  in_reply_to TEXT NOT NULL,
  reference_message_ids TEXT[] NOT NULL DEFAULT '{}'::TEXT[],
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
  CONSTRAINT email_reply_requests_key_hash_check CHECK (CHAR_LENGTH(idempotency_key_hash) = 64),
  CONSTRAINT email_reply_requests_request_hash_check CHECK (CHAR_LENGTH(request_sha256) = 64),
  CONSTRAINT email_reply_requests_addresses_check CHECK (
    CHAR_LENGTH(sender_email) BETWEEN 3 AND 320 AND CHAR_LENGTH(recipient_email) BETWEEN 3 AND 320
  ),
  CONSTRAINT email_reply_requests_content_check CHECK (
    CHAR_LENGTH(subject) BETWEEN 1 AND 998 AND CHAR_LENGTH(body) BETWEEN 1 AND 100000
  ),
  CONSTRAINT email_reply_requests_visibility_check CHECK (visibility IN ('private', 'shared')),
  CONSTRAINT email_reply_requests_correlation_check CHECK (
    CHAR_LENGTH(rfc_message_id) BETWEEN 3 AND 500
    AND CHAR_LENGTH(in_reply_to) BETWEEN 3 AND 500
    AND CARDINALITY(reference_message_ids) <= 50
    AND ARRAY_POSITION(reference_message_ids, NULL) IS NULL
    AND CHAR_LENGTH(ARRAY_TO_STRING(reference_message_ids, '')) <= 25000
    AND CHAR_LENGTH(provider_message_id) <= 500
    AND CHAR_LENGTH(provider_thread_id) <= 500
  ),
  CONSTRAINT email_reply_requests_status_check CHECK (
    status IN ('prepared', 'sending', 'accepted', 'failed', 'uncertain')
  ),
  CONSTRAINT email_reply_requests_state_check CHECK (
    (status = 'prepared' AND claimed_at IS NULL AND finalized_at IS NULL AND outbound_email_message_id IS NULL)
    OR (status = 'sending' AND claimed_at IS NOT NULL AND finalized_at IS NULL AND outbound_email_message_id IS NULL)
    OR (status = 'accepted' AND claimed_at IS NOT NULL AND finalized_at IS NOT NULL AND outbound_email_message_id IS NOT NULL)
    OR (status IN ('failed', 'uncertain') AND claimed_at IS NOT NULL AND finalized_at IS NOT NULL AND outbound_email_message_id IS NULL)
  ),
  CONSTRAINT email_reply_requests_source_fk
    FOREIGN KEY (organization_id, source_message_id)
    REFERENCES email_messages(organization_id, id) ON DELETE CASCADE,
  CONSTRAINT email_reply_requests_thread_root_fk
    FOREIGN KEY (organization_id, thread_root_message_id)
    REFERENCES email_messages(organization_id, id),
  UNIQUE (organization_id, actor_user_id, idempotency_key_hash)
);

CREATE INDEX idx_email_reply_requests_org_thread
  ON email_reply_requests(organization_id, thread_root_message_id, created_at, id);

CREATE INDEX idx_email_reply_requests_stale_sending
  ON email_reply_requests(claimed_at, id)
  WHERE status = 'sending';

CREATE UNIQUE INDEX idx_email_reply_requests_one_unresolved_actor_thread
  ON email_reply_requests(organization_id, actor_user_id, thread_root_message_id)
  WHERE status IN ('prepared', 'sending', 'uncertain');
