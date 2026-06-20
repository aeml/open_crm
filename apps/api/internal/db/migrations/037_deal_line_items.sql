-- 1.3.2 Deal line items, discounts, taxes, and totals foundation.

CREATE TABLE deal_line_items (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  deal_id BIGINT NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
  product_catalog_item_id BIGINT REFERENCES product_catalog_items(id) ON DELETE SET NULL,
  name TEXT NOT NULL,
  sku TEXT NOT NULL DEFAULT '',
  item_type TEXT NOT NULL DEFAULT 'product',
  quantity NUMERIC(12,2) NOT NULL DEFAULT 1,
  unit_name TEXT NOT NULL DEFAULT 'unit',
  unit_price NUMERIC(12,2) NOT NULL DEFAULT 0,
  discount_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
  tax_rate NUMERIC(5,2) NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT 'USD',
  position INTEGER NOT NULL DEFAULT 0,
  created_by_user_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT deal_line_items_name_nonempty CHECK (name <> ''),
  CONSTRAINT deal_line_items_type_check CHECK (item_type IN ('product', 'service')),
  CONSTRAINT deal_line_items_quantity_positive CHECK (quantity > 0),
  CONSTRAINT deal_line_items_unit_name_nonempty CHECK (unit_name <> ''),
  CONSTRAINT deal_line_items_unit_price_nonnegative CHECK (unit_price >= 0),
  CONSTRAINT deal_line_items_discount_nonnegative CHECK (discount_amount >= 0),
  CONSTRAINT deal_line_items_discount_lte_subtotal CHECK (discount_amount <= quantity * unit_price),
  CONSTRAINT deal_line_items_tax_rate_range CHECK (tax_rate >= 0 AND tax_rate <= 100),
  CONSTRAINT deal_line_items_currency_check CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE INDEX idx_deal_line_items_org_deal_position
  ON deal_line_items(organization_id, deal_id, position, id);

CREATE INDEX idx_deal_line_items_catalog_item
  ON deal_line_items(product_catalog_item_id);
