CREATE TABLE IF NOT EXISTS saved_views (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('contacts', 'companies', 'deals', 'tasks')),
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    filters JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_saved_views_org_user_entity_name
    ON saved_views (organization_id, user_id, entity_type, lower(name));

CREATE UNIQUE INDEX IF NOT EXISTS idx_saved_views_default_per_entity
    ON saved_views (organization_id, user_id, entity_type)
    WHERE is_default;

CREATE INDEX IF NOT EXISTS idx_saved_views_org_user_entity
    ON saved_views (organization_id, user_id, entity_type, name);
