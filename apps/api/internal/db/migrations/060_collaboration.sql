-- open-crm-deploy: expand
-- Collaboration state stays organization scoped. Polymorphic record identity is
-- validated by the service before writes; user references remain relational.

CREATE TABLE record_followers (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL CHECK (entity_type IN ('contact', 'company', 'deal')),
  entity_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (organization_id, entity_type, entity_id, user_id),
  FOREIGN KEY (organization_id, user_id)
    REFERENCES organization_memberships(organization_id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_record_followers_record
  ON record_followers(organization_id, entity_type, entity_id, created_at);
CREATE INDEX idx_record_followers_user
  ON record_followers(organization_id, user_id, created_at DESC);

CREATE UNIQUE INDEX idx_notes_org_id_unique
  ON notes(organization_id, id);

CREATE TABLE note_mentions (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  note_id BIGINT NOT NULL,
  mentioned_user_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (organization_id, note_id, mentioned_user_id),
  FOREIGN KEY (organization_id, note_id)
    REFERENCES notes(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, mentioned_user_id)
    REFERENCES organization_memberships(organization_id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_note_mentions_user
  ON note_mentions(organization_id, mentioned_user_id, created_at DESC);

ALTER TABLE notifications
  ADD COLUMN idempotency_key TEXT;

CREATE UNIQUE INDEX idx_notifications_idempotency
  ON notifications(organization_id, user_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
