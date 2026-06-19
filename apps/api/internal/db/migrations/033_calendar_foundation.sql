-- 1.2.5 Calendar meeting and availability foundation.

CREATE TABLE calendar_events (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL,
  entity_id BIGINT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  location TEXT NOT NULL DEFAULT '',
  start_at TIMESTAMPTZ NOT NULL,
  end_at TIMESTAMPTZ NOT NULL,
  timezone TEXT NOT NULL DEFAULT 'UTC',
  status TEXT NOT NULL DEFAULT 'scheduled',
  visibility TEXT NOT NULL DEFAULT 'shared',
  provider_name TEXT NOT NULL DEFAULT '',
  provider_event_id TEXT NOT NULL DEFAULT '',
  calendar_user_id BIGINT REFERENCES users(id),
  created_by_user_id BIGINT NOT NULL REFERENCES users(id),
  last_synced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT calendar_events_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal')),
  CONSTRAINT calendar_events_status_check CHECK (status IN ('scheduled', 'cancelled')),
  CONSTRAINT calendar_events_visibility_check CHECK (visibility IN ('shared', 'private')),
  CONSTRAINT calendar_events_title_nonempty CHECK (title <> ''),
  CONSTRAINT calendar_events_time_order_check CHECK (end_at > start_at)
);

CREATE INDEX idx_calendar_events_org_entity_start
  ON calendar_events(organization_id, entity_type, entity_id, start_at DESC, id DESC);

CREATE INDEX idx_calendar_events_org_user_start
  ON calendar_events(organization_id, calendar_user_id, start_at DESC, id DESC)
  WHERE calendar_user_id IS NOT NULL;

CREATE UNIQUE INDEX idx_calendar_events_org_provider_event
  ON calendar_events(organization_id, provider_name, provider_event_id)
  WHERE provider_event_id <> '';

CREATE TABLE calendar_availability_blocks (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  day_of_week INT NOT NULL,
  start_minute INT NOT NULL,
  end_minute INT NOT NULL,
  timezone TEXT NOT NULL DEFAULT 'UTC',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT calendar_availability_day_check CHECK (day_of_week BETWEEN 0 AND 6),
  CONSTRAINT calendar_availability_minutes_check CHECK (start_minute >= 0 AND start_minute < end_minute AND end_minute <= 1440)
);

CREATE UNIQUE INDEX idx_calendar_availability_org_user_block
  ON calendar_availability_blocks(organization_id, user_id, day_of_week, start_minute, end_minute);

CREATE INDEX idx_calendar_availability_org_user
  ON calendar_availability_blocks(organization_id, user_id, day_of_week, start_minute);
