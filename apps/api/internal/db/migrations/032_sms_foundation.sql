-- 1.2.4 SMS send/receive and opt-out foundation.

CREATE TABLE sms_messages (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL,
  entity_id BIGINT NOT NULL,
  direction TEXT NOT NULL,
  phone_number TEXT NOT NULL,
  phone_key TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL,
  template_name TEXT NOT NULL DEFAULT '',
  provider_name TEXT NOT NULL DEFAULT '',
  provider_message_id TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_by_user_id BIGINT REFERENCES users(id),
  sent_at TIMESTAMPTZ,
  received_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT sms_messages_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal')),
  CONSTRAINT sms_messages_direction_check CHECK (direction IN ('outbound', 'inbound')),
  CONSTRAINT sms_messages_status_check CHECK (status IN ('sent', 'failed', 'received', 'suppressed')),
  CONSTRAINT sms_messages_phone_key_nonempty CHECK (phone_key <> ''),
  CONSTRAINT sms_messages_body_nonempty CHECK (body <> '')
);

CREATE INDEX idx_sms_messages_org_entity_created
  ON sms_messages(organization_id, entity_type, entity_id, created_at DESC, id DESC);

CREATE INDEX idx_sms_messages_org_phone_created
  ON sms_messages(organization_id, phone_key, created_at DESC, id DESC);

CREATE UNIQUE INDEX idx_sms_messages_org_provider_message
  ON sms_messages(organization_id, provider_name, provider_message_id)
  WHERE provider_message_id <> '';

CREATE TABLE sms_suppressions (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  phone_number TEXT NOT NULL,
  phone_key TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT 'opted_out',
  source TEXT NOT NULL DEFAULT '',
  entity_type TEXT NOT NULL DEFAULT '',
  entity_id BIGINT,
  created_by_user_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT sms_suppressions_reason_check CHECK (reason IN ('opted_out', 'manual', 'complaint')),
  CONSTRAINT sms_suppressions_entity_type_check CHECK (entity_type IN ('', 'contact', 'company', 'deal')),
  CONSTRAINT sms_suppressions_phone_key_nonempty CHECK (phone_key <> '')
);

CREATE UNIQUE INDEX idx_sms_suppressions_org_phone
  ON sms_suppressions(organization_id, phone_key);

CREATE INDEX idx_sms_suppressions_org_created
  ON sms_suppressions(organization_id, created_at DESC);
