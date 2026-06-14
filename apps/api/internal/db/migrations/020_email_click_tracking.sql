-- 1.1.4 Email engagement tracking: add aggregate click counts and a
-- per-link token table so click redirects never accept arbitrary target URLs.

ALTER TABLE email_messages
  ADD COLUMN click_count INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN first_clicked_at TIMESTAMPTZ,
  ADD COLUMN last_clicked_at TIMESTAMPTZ;

CREATE TABLE email_message_links (
  id BIGSERIAL PRIMARY KEY,
  email_message_id BIGINT NOT NULL REFERENCES email_messages(id) ON DELETE CASCADE,
  click_token TEXT NOT NULL UNIQUE,
  target_url TEXT NOT NULL,
  click_count INTEGER NOT NULL DEFAULT 0,
  first_clicked_at TIMESTAMPTZ,
  last_clicked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_message_links_message ON email_message_links(email_message_id);
