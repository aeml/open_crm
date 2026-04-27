ALTER TABLE users
    ADD COLUMN IF NOT EXISTS password_setup_token_hash TEXT,
    ADD COLUMN IF NOT EXISTS password_setup_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS password_setup_consumed_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_setup_token_hash
    ON users (password_setup_token_hash)
    WHERE password_setup_token_hash IS NOT NULL;
