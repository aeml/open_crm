-- open-crm-deploy: expand
-- Historical sales reporting starts at this migration for existing tenants and
-- at organization creation for new tenants. Stage-event snapshots remain
-- explainable if pipelines or stages are renamed later.

ALTER TABLE organizations
  ADD COLUMN sales_activity_tracking_started_at TIMESTAMPTZ DEFAULT NOW();

CREATE TABLE deal_stage_events (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  deal_id BIGINT NOT NULL,
  deal_name TEXT NOT NULL,
  event_type TEXT NOT NULL,
  activity_id BIGINT,
  actor_user_id BIGINT REFERENCES users(id),
  owner_user_id BIGINT REFERENCES users(id),
  from_pipeline_id BIGINT,
  from_pipeline_name TEXT,
  from_stage_id BIGINT,
  from_stage_name TEXT,
  from_stage_position INTEGER,
  from_stage_outcome TEXT,
  to_pipeline_id BIGINT NOT NULL,
  to_pipeline_name TEXT NOT NULL,
  to_stage_id BIGINT NOT NULL,
  to_stage_name TEXT NOT NULL,
  to_stage_position INTEGER NOT NULL,
  to_stage_outcome TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT deal_stage_events_type_check CHECK (event_type IN ('created', 'stage_changed')),
  CONSTRAINT deal_stage_events_from_outcome_check CHECK (from_stage_outcome IS NULL OR from_stage_outcome IN ('open', 'won', 'lost')),
  CONSTRAINT deal_stage_events_to_outcome_check CHECK (to_stage_outcome IN ('open', 'won', 'lost')),
  CONSTRAINT deal_stage_events_from_shape_check CHECK (
    event_type = 'created' OR
    (from_pipeline_id IS NOT NULL AND from_pipeline_name IS NOT NULL AND from_stage_id IS NOT NULL AND from_stage_name IS NOT NULL AND from_stage_position IS NOT NULL AND from_stage_outcome IS NOT NULL)
  ),
  UNIQUE (organization_id, activity_id)
);

CREATE INDEX idx_deal_stage_events_org_occurred
  ON deal_stage_events(organization_id, occurred_at DESC, id DESC);

CREATE INDEX idx_deal_stage_events_org_owner_occurred
  ON deal_stage_events(organization_id, owner_user_id, occurred_at DESC, id DESC);

CREATE INDEX idx_deal_stage_events_org_from_stage
  ON deal_stage_events(organization_id, from_stage_id, occurred_at DESC)
  WHERE event_type = 'stage_changed';

CREATE INDEX idx_deal_stage_events_org_to_stage
  ON deal_stage_events(organization_id, to_stage_id, occurred_at DESC);

CREATE INDEX idx_activities_org_action_actor_created
  ON activities(organization_id, action, actor_user_id, created_at DESC);
