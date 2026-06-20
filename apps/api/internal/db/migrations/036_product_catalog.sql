-- 1.3.1 Product/service catalog foundation.

CREATE TABLE product_catalog_items (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  sku TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  item_type TEXT NOT NULL DEFAULT 'product',
  unit_price NUMERIC(12,2) NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT 'USD',
  unit_name TEXT NOT NULL DEFAULT 'unit',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by_user_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT product_catalog_items_name_nonempty CHECK (name <> ''),
  CONSTRAINT product_catalog_items_type_check CHECK (item_type IN ('product', 'service')),
  CONSTRAINT product_catalog_items_price_nonnegative CHECK (unit_price >= 0),
  CONSTRAINT product_catalog_items_currency_check CHECK (currency ~ '^[A-Z]{3}$'),
  CONSTRAINT product_catalog_items_unit_name_nonempty CHECK (unit_name <> '')
);

CREATE UNIQUE INDEX idx_product_catalog_items_org_sku
  ON product_catalog_items(organization_id, lower(sku))
  WHERE sku <> '';

CREATE INDEX idx_product_catalog_items_org_active_name
  ON product_catalog_items(organization_id, is_active, lower(name), id);
