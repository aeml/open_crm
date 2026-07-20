-- open-crm-deploy: expand
-- Successful durable jobs keep short-lived diagnostic detail and a longer
-- idempotency window. This partial index makes bounded global retention passes
-- independent of tenant count without affecting active or dead-letter work.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX IF NOT EXISTS idx_background_jobs_retention_succeeded
  ON background_jobs(completed_at ASC, id ASC)
  WHERE status = 'succeeded';
