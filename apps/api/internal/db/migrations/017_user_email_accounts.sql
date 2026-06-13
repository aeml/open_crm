-- 1.1.2 Per-user email connections. Customer-facing email is sent through each
-- user's own mailbox (SMTP), not the platform's transactional provider. IMAP
-- fields are reserved for two-way sync in a later slice. Secrets are stored
-- encrypted (AES-GCM) by the application; the database never holds plaintext
-- passwords.

CREATE TABLE user_email_accounts (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  from_email TEXT NOT NULL,
  from_name TEXT NOT NULL DEFAULT '',

  smtp_host TEXT NOT NULL,
  smtp_port INT NOT NULL,
  smtp_username TEXT NOT NULL,
  smtp_password_enc TEXT NOT NULL,
  smtp_use_tls BOOLEAN NOT NULL DEFAULT TRUE,

  imap_host TEXT NOT NULL DEFAULT '',
  imap_port INT NOT NULL DEFAULT 0,
  imap_username TEXT NOT NULL DEFAULT '',
  imap_password_enc TEXT NOT NULL DEFAULT '',

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

  CONSTRAINT user_email_accounts_unique_user UNIQUE (organization_id, user_id),
  CONSTRAINT user_email_accounts_smtp_port_check CHECK (smtp_port > 0 AND smtp_port <= 65535)
);

CREATE INDEX idx_user_email_accounts_user ON user_email_accounts(organization_id, user_id);
