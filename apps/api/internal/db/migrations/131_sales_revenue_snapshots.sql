-- open-crm-deploy: expand
-- Retain the exact deal value and workspace-base-currency conversion observed
-- at each future stage event. Historical and rolling-old-writer events remain
-- NULL and are reported as uncaptured rather than inferred from mutable deals.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

ALTER TABLE organizations
  ADD COLUMN sales_revenue_tracking_started_at TIMESTAMPTZ DEFAULT NOW();

ALTER TABLE deal_stage_events
  ADD COLUMN deal_value_amount NUMERIC(12,2),
  ADD COLUMN deal_value_currency TEXT,
  ADD COLUMN revenue_base_currency TEXT,
  ADD COLUMN revenue_exchange_rate_to_base NUMERIC(18,8),
  ADD COLUMN revenue_exchange_rate_effective_date DATE,
  ADD COLUMN revenue_exchange_rate_source TEXT,
  ADD COLUMN deal_value_in_base_currency NUMERIC(24,2),
  ADD CONSTRAINT deal_stage_events_revenue_snapshot_check CHECK (
    (
      deal_value_amount IS NULL
      AND deal_value_currency IS NULL
      AND revenue_base_currency IS NULL
      AND revenue_exchange_rate_to_base IS NULL
      AND revenue_exchange_rate_effective_date IS NULL
      AND revenue_exchange_rate_source IS NULL
      AND deal_value_in_base_currency IS NULL
    )
    OR
    (
      deal_value_amount >= 0
      AND (
        (
          deal_value_currency IS NULL
          AND revenue_base_currency IS NULL
          AND revenue_exchange_rate_to_base IS NULL
          AND revenue_exchange_rate_effective_date IS NULL
          AND revenue_exchange_rate_source IS NULL
          AND deal_value_in_base_currency IS NULL
        )
        OR
        (
          deal_value_currency ~ '^[A-Z]{3}$'
          AND revenue_base_currency ~ '^[A-Z]{3}$'
          AND (
            (
              revenue_exchange_rate_to_base IS NULL
              AND revenue_exchange_rate_effective_date IS NULL
              AND revenue_exchange_rate_source IS NULL
              AND deal_value_in_base_currency IS NULL
              AND deal_value_currency <> revenue_base_currency
            )
            OR
            (
              revenue_exchange_rate_to_base > 0
              AND revenue_exchange_rate_effective_date IS NOT NULL
              AND CHAR_LENGTH(BTRIM(revenue_exchange_rate_source)) BETWEEN 1 AND 200
              AND deal_value_in_base_currency >= 0
              AND (
                (
                  deal_value_currency = revenue_base_currency
                  AND revenue_exchange_rate_to_base = 1
                  AND revenue_exchange_rate_source = 'identity'
                  AND deal_value_in_base_currency = deal_value_amount
                )
                OR
                (
                  deal_value_currency <> revenue_base_currency
                  AND revenue_exchange_rate_source <> 'identity'
                  AND deal_value_in_base_currency = ROUND(deal_value_amount * revenue_exchange_rate_to_base, 2)
                )
              )
            )
          )
        )
      )
    )
  ) NOT VALID;

ALTER TABLE deal_stage_events
  VALIDATE CONSTRAINT deal_stage_events_revenue_snapshot_check;

CREATE INDEX idx_deal_stage_events_org_won_revenue
  ON deal_stage_events(organization_id, occurred_at DESC, owner_user_id, id DESC)
  INCLUDE (deal_value_amount, deal_value_currency, revenue_base_currency, deal_value_in_base_currency)
  WHERE to_stage_outcome = 'won' AND COALESCE(from_stage_outcome, '') <> 'won';
