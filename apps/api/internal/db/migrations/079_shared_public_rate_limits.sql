-- open-crm-deploy: expand
-- Public abuse budgets must survive process restarts and coordinate across API
-- replicas. Only a one-way client-key digest is retained; raw addresses and
-- other request-derived identifiers never enter this ledger.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE TABLE IF NOT EXISTS public_rate_limit_buckets (
  scope TEXT NOT NULL,
  client_key_hash BYTEA NOT NULL,
  window_started_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  request_count INTEGER NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (scope, client_key_hash),
  CONSTRAINT public_rate_limit_buckets_scope_check
    CHECK (scope ~ '^[a-z0-9][a-z0-9._-]{0,99}$'),
  CONSTRAINT public_rate_limit_buckets_client_hash_check
    CHECK (octet_length(client_key_hash) = 32),
  CONSTRAINT public_rate_limit_buckets_window_check
    CHECK (expires_at > window_started_at),
  CONSTRAINT public_rate_limit_buckets_count_check
    CHECK (request_count BETWEEN 1 AND 1000001)
);

CREATE INDEX IF NOT EXISTS idx_public_rate_limit_buckets_expiry
  ON public_rate_limit_buckets(expires_at, scope);
