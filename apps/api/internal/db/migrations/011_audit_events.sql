CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL CHECK (length(trim(event_type)) > 0),
    entity_type TEXT NOT NULL CHECK (length(trim(entity_type)) > 0),
    entity_id BIGINT,
    summary TEXT NOT NULL CHECK (length(trim(summary)) > 0),
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_events_org_created
    ON audit_events (organization_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_audit_events_org_type_created
    ON audit_events (organization_id, event_type, created_at DESC);
