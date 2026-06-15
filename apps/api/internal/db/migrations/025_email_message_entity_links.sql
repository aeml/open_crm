-- 1.1.3 Automatic email logging to matching records. One email can be relevant
-- to a contact, that contact's company, and active deals, so keep the legacy
-- primary entity fields while adding a normalized multi-link table.

CREATE TABLE email_message_entity_links (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  email_message_id BIGINT NOT NULL REFERENCES email_messages(id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL,
  entity_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT email_message_entity_links_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal')),
  CONSTRAINT email_message_entity_links_entity_id_check CHECK (entity_id > 0),
  UNIQUE (email_message_id, entity_type, entity_id)
);

CREATE INDEX idx_email_message_entity_links_entity
  ON email_message_entity_links(organization_id, entity_type, entity_id, email_message_id DESC);

INSERT INTO email_message_entity_links (organization_id, email_message_id, entity_type, entity_id)
SELECT organization_id, id, entity_type, entity_id
FROM email_messages
WHERE entity_type IN ('contact', 'company', 'deal')
  AND entity_id IS NOT NULL
  AND entity_id > 0
ON CONFLICT (email_message_id, entity_type, entity_id) DO NOTHING;
