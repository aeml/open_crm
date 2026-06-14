-- 1.1.2 Mailbox sync foundation: store per-user sync/provider metadata and
-- optional IMAP settings. OAuth token exchange and message ingestion are added
-- in later slices; this only records configuration and sync state.

ALTER TABLE user_email_accounts
  ADD COLUMN provider TEXT NOT NULL DEFAULT 'smtp',
  ADD COLUMN auth_method TEXT NOT NULL DEFAULT 'password',
  ADD COLUMN sync_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN sync_status TEXT NOT NULL DEFAULT 'disabled',
  ADD COLUMN sync_cursor TEXT NOT NULL DEFAULT '',
  ADD COLUMN last_sync_at TIMESTAMPTZ,
  ADD COLUMN last_sync_error TEXT NOT NULL DEFAULT '',
  ADD COLUMN imap_use_tls BOOLEAN NOT NULL DEFAULT TRUE,
  ADD COLUMN oauth_subject TEXT NOT NULL DEFAULT '',
  ADD COLUMN oauth_access_token_enc TEXT NOT NULL DEFAULT '',
  ADD COLUMN oauth_refresh_token_enc TEXT NOT NULL DEFAULT '',
  ADD COLUMN oauth_token_expires_at TIMESTAMPTZ;

ALTER TABLE user_email_accounts
  ADD CONSTRAINT user_email_accounts_provider_check CHECK (provider IN ('smtp', 'imap', 'google', 'microsoft')),
  ADD CONSTRAINT user_email_accounts_auth_method_check CHECK (auth_method IN ('password', 'oauth')),
  ADD CONSTRAINT user_email_accounts_sync_status_check CHECK (sync_status IN ('disabled', 'pending', 'ready', 'syncing', 'error')),
  ADD CONSTRAINT user_email_accounts_imap_port_check CHECK (imap_port >= 0 AND imap_port <= 65535);

CREATE INDEX idx_user_email_accounts_sync_due ON user_email_accounts(organization_id, sync_status, last_sync_at) WHERE sync_enabled = TRUE;
