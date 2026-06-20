-- 1.3.5 Multiple deal pipelines foundation.

CREATE TABLE deal_pipelines (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  position INTEGER NOT NULL,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  created_by_user_id BIGINT REFERENCES users(id),
  updated_by_user_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT deal_pipelines_name_nonempty CHECK (btrim(name) <> '')
);

INSERT INTO deal_pipelines (organization_id, name, position, is_default)
SELECT organizations.id, 'Sales pipeline', 1, TRUE
FROM organizations
WHERE NOT EXISTS (
  SELECT 1
  FROM deal_pipelines existing
  WHERE existing.organization_id = organizations.id
);

ALTER TABLE deal_stages
  ADD COLUMN pipeline_id BIGINT REFERENCES deal_pipelines(id) ON DELETE CASCADE;

UPDATE deal_stages stages
SET pipeline_id = pipelines.id
FROM deal_pipelines pipelines
WHERE stages.organization_id = pipelines.organization_id
  AND pipelines.is_default
  AND stages.pipeline_id IS NULL;

ALTER TABLE deal_stages
  ALTER COLUMN pipeline_id SET NOT NULL;

DROP INDEX IF EXISTS idx_deal_stages_org_position_unique;
DROP INDEX IF EXISTS idx_deal_stages_org_name_unique;

CREATE UNIQUE INDEX idx_deal_pipelines_org_position_unique
  ON deal_pipelines(organization_id, position);

CREATE UNIQUE INDEX idx_deal_pipelines_org_name_unique
  ON deal_pipelines(organization_id, lower(name));

CREATE UNIQUE INDEX idx_deal_pipelines_org_default_unique
  ON deal_pipelines(organization_id)
  WHERE is_default;

CREATE UNIQUE INDEX idx_deal_stages_org_pipeline_position_unique
  ON deal_stages(organization_id, pipeline_id, position);

CREATE UNIQUE INDEX idx_deal_stages_org_pipeline_name_unique
  ON deal_stages(organization_id, pipeline_id, lower(name));

CREATE INDEX idx_deal_stages_org_pipeline
  ON deal_stages(organization_id, pipeline_id, position, id);
