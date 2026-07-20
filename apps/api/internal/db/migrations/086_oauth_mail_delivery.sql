-- open-crm-deploy: expand
-- Record the delegated scopes actually granted to a connected Google or
-- Microsoft mailbox. Existing OAuth connections remain readable/syncable but
-- must reconnect before provider-backed sending is considered ready.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE user_email_accounts
  ADD COLUMN oauth_scopes TEXT;
