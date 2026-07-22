-- open-crm-deploy: expand
-- Keep a bounded, short-lived source copy so validated CRM imports can execute
-- through the durable PostgreSQL worker instead of an HTTP request. Existing
-- synchronous writers remain compatible: their processing rows may have no
-- retained source.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

ALTER TABLE import_batches
  ADD COLUMN IF NOT EXISTS source_csv BYTEA,
  ADD COLUMN IF NOT EXISTS source_expires_at TIMESTAMPTZ;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'import_batches_source_size_check'
      AND conrelid = 'import_batches'::regclass
  ) THEN
    ALTER TABLE import_batches
      ADD CONSTRAINT import_batches_source_size_check
      CHECK (source_csv IS NULL OR (octet_length(source_csv) BETWEEN 1 AND 2097152))
      NOT VALID;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'import_batches_source_expiry_check'
      AND conrelid = 'import_batches'::regclass
  ) THEN
    ALTER TABLE import_batches
      ADD CONSTRAINT import_batches_source_expiry_check
      CHECK ((source_csv IS NULL AND source_expires_at IS NULL) OR (source_csv IS NOT NULL AND source_expires_at IS NOT NULL))
      NOT VALID;
  END IF;
END $$;

ALTER TABLE import_batches VALIDATE CONSTRAINT import_batches_source_size_check;
ALTER TABLE import_batches VALIDATE CONSTRAINT import_batches_source_expiry_check;

CREATE INDEX IF NOT EXISTS idx_import_batches_source_expiry
  ON import_batches(source_expires_at, id)
  WHERE source_csv IS NOT NULL;
