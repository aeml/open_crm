-- 1.2.3 Call recording metadata, consent, and retention foundation.

ALTER TABLE call_logs
  ADD COLUMN recording_status TEXT NOT NULL DEFAULT 'not_recorded',
  ADD COLUMN recording_url TEXT NOT NULL DEFAULT '',
  ADD COLUMN recording_consent TEXT NOT NULL DEFAULT 'unknown',
  ADD COLUMN recording_retention_until TIMESTAMPTZ,
  ADD COLUMN recording_deleted_at TIMESTAMPTZ;

ALTER TABLE call_logs
  ADD CONSTRAINT call_logs_recording_status_check CHECK (recording_status IN ('not_recorded', 'available', 'deleted')),
  ADD CONSTRAINT call_logs_recording_consent_check CHECK (recording_consent IN ('unknown', 'granted', 'denied', 'not_required')),
  ADD CONSTRAINT call_logs_recording_url_check CHECK (
    (recording_status = 'available' AND recording_url <> '' AND recording_deleted_at IS NULL)
    OR (recording_status <> 'available' AND recording_url = '')
  );

CREATE INDEX idx_call_logs_recording_retention
  ON call_logs(organization_id, recording_retention_until)
  WHERE recording_status = 'available' AND recording_retention_until IS NOT NULL;
