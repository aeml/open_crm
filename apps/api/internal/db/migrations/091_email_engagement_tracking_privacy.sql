-- open-crm-deploy: expand
-- Make one-to-one engagement tracking explicit, time-bounded, and purgeable.
-- Legacy tracked messages have no recorded sender acknowledgement, so their
-- collection window ends at migration time and the bounded scheduler scrubs
-- their observations after deployment. Retained click mappings keep old links
-- usable without collecting another event.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE email_messages
  ADD COLUMN engagement_tracking_enabled BOOLEAN DEFAULT FALSE,
  ADD COLUMN engagement_tracking_authorized_by_user_id BIGINT,
  ADD COLUMN engagement_tracking_authorized_at TIMESTAMPTZ,
  ADD COLUMN engagement_tracking_expires_at TIMESTAMPTZ,
  ADD COLUMN engagement_tracking_purged_at TIMESTAMPTZ;

UPDATE email_messages message
SET engagement_tracking_enabled = TRUE,
    engagement_tracking_expires_at = NOW()
WHERE message.direction = 'outbound'
  AND (
    message.tracking_token IS NOT NULL OR EXISTS (
      SELECT 1 FROM email_message_links link WHERE link.email_message_id = message.id
    )
  );

ALTER TABLE email_messages
  ADD CONSTRAINT email_messages_engagement_tracking_actor_fk
    FOREIGN KEY (engagement_tracking_authorized_by_user_id) REFERENCES users(id) ON DELETE SET NULL NOT VALID,
  ADD CONSTRAINT email_messages_engagement_tracking_state_check CHECK (
    engagement_tracking_enabled IS NOT NULL
    AND (
      engagement_tracking_enabled = FALSE OR (
        direction = 'outbound'
        AND engagement_tracking_expires_at IS NOT NULL
        AND (
          (engagement_tracking_authorized_at IS NULL AND engagement_tracking_authorized_by_user_id IS NULL)
          OR (
            engagement_tracking_authorized_at IS NOT NULL
            AND engagement_tracking_expires_at >= engagement_tracking_authorized_at
          )
        )
      )
    )
  ) NOT VALID,
  ADD CONSTRAINT email_messages_tracking_token_authorized_check CHECK (
    tracking_token IS NULL OR engagement_tracking_enabled = TRUE
  ) NOT VALID,
  ADD CONSTRAINT email_messages_engagement_tracking_purge_check CHECK (
    engagement_tracking_purged_at IS NULL OR (
      engagement_tracking_enabled = TRUE
      AND engagement_tracking_expires_at IS NOT NULL
      AND engagement_tracking_purged_at >= engagement_tracking_expires_at
    )
  ) NOT VALID;

ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_engagement_tracking_actor_fk;
ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_engagement_tracking_state_check;
ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_tracking_token_authorized_check;
ALTER TABLE email_messages VALIDATE CONSTRAINT email_messages_engagement_tracking_purge_check;

CREATE INDEX idx_email_messages_engagement_tracking_retention
  ON email_messages(engagement_tracking_expires_at, id)
  WHERE engagement_tracking_enabled = TRUE AND engagement_tracking_purged_at IS NULL;
