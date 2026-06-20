-- 1.2.7 Meeting reminder and automatic activity foundation.

CREATE TABLE calendar_event_reminders (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  calendar_event_id BIGINT NOT NULL REFERENCES calendar_events(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  reminder_minutes INT NOT NULL DEFAULT 15,
  remind_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  delivered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT calendar_event_reminders_minutes_check CHECK (reminder_minutes BETWEEN 0 AND 10080),
  CONSTRAINT calendar_event_reminders_status_check CHECK (status IN ('pending', 'sent', 'skipped')),
  UNIQUE (calendar_event_id, user_id, reminder_minutes)
);

CREATE INDEX idx_calendar_event_reminders_due
  ON calendar_event_reminders(status, remind_at, id)
  WHERE status = 'pending';

CREATE INDEX idx_calendar_event_reminders_org_event
  ON calendar_event_reminders(organization_id, calendar_event_id);

CREATE INDEX idx_calendar_event_reminders_user_status
  ON calendar_event_reminders(organization_id, user_id, status, remind_at DESC);
