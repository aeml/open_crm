-- open-crm-deploy: expand
-- The shared inbox is ordered by its open/closed work bucket, effective
-- message time, and ID. Match that complete keyset order so continuation stays
-- bounded at pilot-scale cardinality, including equal provider timestamps.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_email_messages_shared_inbox_cursor
  ON email_messages(
    organization_id,
    (CASE WHEN shared_inbox_status = 'open' THEN 0 ELSE 1 END),
    (COALESCE(received_at, created_at)) DESC,
    id DESC
  )
  WHERE direction = 'inbound' AND visibility = 'shared';
