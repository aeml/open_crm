-- open-crm-deploy: expand
-- Retain the exact workspace-base-currency conversion disclosed when a quote
-- is finalized. Existing immutable PDFs remain untouched and therefore keep
-- these columns NULL; every quote created by the converged application writes
-- the complete snapshot.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER TABLE deal_quotes
  ADD COLUMN quote_base_currency TEXT,
  ADD COLUMN exchange_rate_to_base NUMERIC(18,8),
  ADD COLUMN exchange_rate_effective_date DATE,
  ADD COLUMN exchange_rate_source TEXT,
  ADD COLUMN total_in_base_currency NUMERIC(24,2),
  ADD CONSTRAINT deal_quotes_fx_snapshot_check CHECK (
    (
      quote_base_currency IS NULL
      AND exchange_rate_to_base IS NULL
      AND exchange_rate_effective_date IS NULL
      AND exchange_rate_source IS NULL
      AND total_in_base_currency IS NULL
    )
    OR
    (
      quote_base_currency ~ '^[A-Z]{3}$'
      AND exchange_rate_to_base > 0
      AND exchange_rate_to_base <= 9999999999.99999999
      AND exchange_rate_effective_date IS NOT NULL
      AND CHAR_LENGTH(BTRIM(exchange_rate_source)) BETWEEN 1 AND 200
      AND total_in_base_currency >= 0
      AND (
        (
          currency = quote_base_currency
          AND exchange_rate_to_base = 1
          AND exchange_rate_source = 'identity'
          AND total_in_base_currency = total
        )
        OR
        (
          currency <> quote_base_currency
          AND exchange_rate_source <> 'identity'
        )
      )
    )
  ) NOT VALID;

ALTER TABLE deal_quotes
  VALIDATE CONSTRAINT deal_quotes_fx_snapshot_check;
