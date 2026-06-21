-- 1.3.7 Multi-currency exchange-rate foundation.

ALTER TABLE organizations
  ADD COLUMN base_currency TEXT NOT NULL DEFAULT 'USD';

ALTER TABLE organizations
  DROP CONSTRAINT IF EXISTS organizations_base_currency_code_check;

ALTER TABLE organizations
  ADD CONSTRAINT organizations_base_currency_code_check
  CHECK (base_currency ~ '^[A-Z]{3}$');

CREATE TABLE organization_exchange_rates (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  base_currency TEXT NOT NULL,
  quote_currency TEXT NOT NULL,
  rate_to_base NUMERIC(18,8) NOT NULL,
  effective_date DATE NOT NULL DEFAULT CURRENT_DATE,
  source TEXT NOT NULL DEFAULT 'manual',
  created_by_user_id BIGINT REFERENCES users(id),
  updated_by_user_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT organization_exchange_rates_currency_code_check CHECK (base_currency ~ '^[A-Z]{3}$' AND quote_currency ~ '^[A-Z]{3}$'),
  CONSTRAINT organization_exchange_rates_currency_pair_check CHECK (base_currency <> quote_currency),
  CONSTRAINT organization_exchange_rates_rate_positive_check CHECK (rate_to_base > 0),
  CONSTRAINT organization_exchange_rates_source_check CHECK (length(trim(source)) > 0)
);

CREATE UNIQUE INDEX idx_org_exchange_rates_unique_effective
  ON organization_exchange_rates(organization_id, base_currency, quote_currency, effective_date);

CREATE INDEX idx_org_exchange_rates_latest
  ON organization_exchange_rates(organization_id, base_currency, quote_currency, effective_date DESC, id DESC);
