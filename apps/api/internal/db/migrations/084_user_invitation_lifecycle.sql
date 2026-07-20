-- open-crm-deploy: expand
-- Preserve an explicit invitation-revocation state after the one-time token is
-- removed. Existing accepted and outstanding invitations remain derivable from
-- password_setup_consumed_at and the token/expiry pair respectively.

ALTER TABLE users
  ADD COLUMN password_setup_revoked_at TIMESTAMPTZ;

ALTER TABLE users
  ADD CONSTRAINT users_password_setup_terminal_state_check
    CHECK (
      password_setup_revoked_at IS NULL
      OR (
        password_setup_consumed_at IS NULL
        AND password_setup_token_hash IS NULL
        AND password_setup_expires_at IS NULL
      )
    ) NOT VALID;

ALTER TABLE users
  VALIDATE CONSTRAINT users_password_setup_terminal_state_check;
