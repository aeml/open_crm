-- 1.2.6 Meeting scheduler booking-link foundation.

CREATE TABLE calendar_booking_links (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  duration_minutes INT NOT NULL DEFAULT 30,
  buffer_minutes INT NOT NULL DEFAULT 0,
  timezone TEXT NOT NULL DEFAULT 'UTC',
  assignment_mode TEXT NOT NULL DEFAULT 'owner',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT calendar_booking_links_slug_nonempty CHECK (slug <> ''),
  CONSTRAINT calendar_booking_links_name_nonempty CHECK (name <> ''),
  CONSTRAINT calendar_booking_links_duration_check CHECK (duration_minutes BETWEEN 5 AND 480),
  CONSTRAINT calendar_booking_links_buffer_check CHECK (buffer_minutes BETWEEN 0 AND 240),
  CONSTRAINT calendar_booking_links_assignment_mode_check CHECK (assignment_mode IN ('owner', 'round_robin'))
);

CREATE UNIQUE INDEX idx_calendar_booking_links_org_slug
  ON calendar_booking_links(organization_id, lower(slug));

CREATE INDEX idx_calendar_booking_links_org_active
  ON calendar_booking_links(organization_id, is_active, lower(name), id);

CREATE TABLE calendar_booking_link_members (
  id BIGSERIAL PRIMARY KEY,
  booking_link_id BIGINT NOT NULL REFERENCES calendar_booking_links(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  position INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (booking_link_id, user_id)
);

CREATE INDEX idx_calendar_booking_link_members_link
  ON calendar_booking_link_members(booking_link_id, position, id);

CREATE INDEX idx_calendar_booking_link_members_user
  ON calendar_booking_link_members(user_id, booking_link_id);
