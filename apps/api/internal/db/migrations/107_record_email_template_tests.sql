-- open-crm-deploy: expand
-- Reuse the durable record-email provider boundary for explicit test-to-self
-- deliveries. Tests retain the source record used for merge rendering, but
-- never create customer timeline evidence or send to the CRM recipient.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE record_email_deliveries
  ADD COLUMN purpose TEXT DEFAULT 'record',
  ADD COLUMN recipient_user_id BIGINT REFERENCES users(id);

ALTER TABLE record_email_deliveries
  ADD CONSTRAINT record_email_deliveries_purpose_check CHECK (
    purpose IS NOT NULL AND (
      (purpose = 'record' AND recipient_user_id IS NULL)
      OR (purpose = 'test' AND recipient_user_id > 0)
    )
  ) NOT VALID;

ALTER TABLE record_email_deliveries
  VALIDATE CONSTRAINT record_email_deliveries_purpose_check;
