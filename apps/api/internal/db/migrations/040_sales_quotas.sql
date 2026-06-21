-- 1.3.6 Quotas, goals, and forecasting foundation.

CREATE TABLE sales_quotas (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  period_start DATE NOT NULL,
  period_end DATE NOT NULL,
  quota_amount NUMERIC(12,2) NOT NULL,
  currency TEXT NOT NULL DEFAULT 'USD',
  created_by_user_id BIGINT REFERENCES users(id),
  updated_by_user_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT sales_quotas_period_order_check CHECK (period_end >= period_start),
  CONSTRAINT sales_quotas_amount_nonnegative_check CHECK (quota_amount >= 0),
  CONSTRAINT sales_quotas_currency_code_check CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE UNIQUE INDEX idx_sales_quotas_org_user_period_unique
  ON sales_quotas(organization_id, user_id, period_start, period_end);

CREATE INDEX idx_sales_quotas_org_period
  ON sales_quotas(organization_id, period_start, period_end);
