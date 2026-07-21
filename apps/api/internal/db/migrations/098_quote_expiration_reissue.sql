-- open-crm-deploy: expand
-- Preserve an explicit tenant/deal-bound lineage when an expired immutable
-- quote is replaced. The original bytes and evidence remain unchanged.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE UNIQUE INDEX idx_deal_quotes_org_id_deal
  ON deal_quotes(organization_id, id, deal_id);

ALTER TABLE deal_quotes
  ADD COLUMN reissued_from_quote_id BIGINT,
  ADD CONSTRAINT deal_quotes_reissued_from_fk
    FOREIGN KEY (organization_id, reissued_from_quote_id, deal_id)
    REFERENCES deal_quotes(organization_id, id, deal_id) NOT VALID,
  ADD CONSTRAINT deal_quotes_reissued_from_self_check CHECK (
    reissued_from_quote_id IS NULL OR reissued_from_quote_id <> id
  ) NOT VALID;

ALTER TABLE deal_quotes
  VALIDATE CONSTRAINT deal_quotes_reissued_from_fk;
ALTER TABLE deal_quotes
  VALIDATE CONSTRAINT deal_quotes_reissued_from_self_check;

CREATE UNIQUE INDEX idx_deal_quotes_one_reissue
  ON deal_quotes(organization_id, reissued_from_quote_id)
  WHERE reissued_from_quote_id IS NOT NULL;

CREATE INDEX idx_deal_quotes_org_expiration
  ON deal_quotes(organization_id, valid_until, id)
  WHERE status = 'finalized';
