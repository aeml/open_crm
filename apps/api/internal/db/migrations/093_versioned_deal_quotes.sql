-- open-crm-deploy: expand
-- Persist the exact commercial snapshot and PDF a teammate finalized so later
-- deal/catalog edits cannot silently change what the customer was quoted.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE UNIQUE INDEX idx_deals_org_id
  ON deals(organization_id, id);

CREATE TABLE deal_quotes (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  deal_id BIGINT NOT NULL,
  version INTEGER NOT NULL,
  quote_number TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'finalized',
  organization_name TEXT NOT NULL,
  deal_name TEXT NOT NULL,
  company_name TEXT NOT NULL DEFAULT '',
  primary_contact_name TEXT NOT NULL DEFAULT '',
  recipient_name TEXT NOT NULL,
  recipient_email TEXT NOT NULL,
  prepared_by_name TEXT NOT NULL,
  currency TEXT NOT NULL,
  subtotal NUMERIC(14,2) NOT NULL,
  discount_total NUMERIC(14,2) NOT NULL,
  tax_total NUMERIC(14,2) NOT NULL,
  total NUMERIC(14,2) NOT NULL,
  valid_until DATE NOT NULL,
  terms TEXT NOT NULL,
  pdf_filename TEXT NOT NULL,
  pdf_content BYTEA NOT NULL,
  pdf_sha256 TEXT NOT NULL,
  idempotency_key_hash TEXT NOT NULL,
  request_sha256 TEXT NOT NULL,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT deal_quotes_deal_fk
    FOREIGN KEY (organization_id, deal_id)
    REFERENCES deals(organization_id, id) ON DELETE CASCADE,
  CONSTRAINT deal_quotes_version_positive CHECK (version > 0),
  CONSTRAINT deal_quotes_number_check CHECK (CHAR_LENGTH(quote_number) BETWEEN 5 AND 80),
  CONSTRAINT deal_quotes_status_check CHECK (status = 'finalized'),
  CONSTRAINT deal_quotes_snapshot_text_check CHECK (
    organization_name <> ''
    AND deal_name <> ''
    AND CHAR_LENGTH(recipient_name) BETWEEN 1 AND 200
    AND CHAR_LENGTH(recipient_email) BETWEEN 3 AND 320
    AND prepared_by_name <> ''
    AND CHAR_LENGTH(terms) BETWEEN 1 AND 10000
  ),
  CONSTRAINT deal_quotes_currency_check CHECK (currency ~ '^[A-Z]{3}$'),
  CONSTRAINT deal_quotes_totals_check CHECK (
    subtotal >= 0 AND discount_total >= 0 AND tax_total >= 0 AND total >= 0
    AND discount_total <= subtotal
  ),
  CONSTRAINT deal_quotes_pdf_check CHECK (
    CHAR_LENGTH(pdf_filename) BETWEEN 5 AND 240
    AND OCTET_LENGTH(pdf_content) BETWEEN 100 AND 2097152
    AND pdf_sha256 ~ '^[0-9a-f]{64}$'
  ),
  CONSTRAINT deal_quotes_key_hash_check CHECK (idempotency_key_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT deal_quotes_request_hash_check CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  UNIQUE (organization_id, id),
  UNIQUE (organization_id, deal_id, version),
  UNIQUE (organization_id, quote_number),
  UNIQUE (organization_id, created_by_user_id, idempotency_key_hash)
);

CREATE INDEX idx_deal_quotes_org_deal_created
  ON deal_quotes(organization_id, deal_id, created_at DESC, id DESC);

CREATE TABLE deal_quote_line_items (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  quote_id BIGINT NOT NULL,
  source_line_item_id BIGINT,
  source_catalog_item_id BIGINT,
  name TEXT NOT NULL,
  sku TEXT NOT NULL DEFAULT '',
  item_type TEXT NOT NULL,
  quantity NUMERIC(12,2) NOT NULL,
  unit_name TEXT NOT NULL,
  unit_price NUMERIC(12,2) NOT NULL,
  subtotal NUMERIC(14,2) NOT NULL,
  discount_amount NUMERIC(12,2) NOT NULL,
  tax_rate NUMERIC(5,2) NOT NULL,
  tax_amount NUMERIC(14,2) NOT NULL,
  total NUMERIC(14,2) NOT NULL,
  currency TEXT NOT NULL,
  position INTEGER NOT NULL,
  CONSTRAINT deal_quote_line_items_quote_fk
    FOREIGN KEY (organization_id, quote_id)
    REFERENCES deal_quotes(organization_id, id) ON DELETE CASCADE,
  CONSTRAINT deal_quote_line_items_name_check CHECK (name <> ''),
  CONSTRAINT deal_quote_line_items_snapshot_check CHECK (
    item_type IN ('product', 'service')
    AND quantity > 0
    AND unit_name <> ''
    AND unit_price >= 0
    AND subtotal >= 0
    AND discount_amount >= 0
    AND discount_amount <= subtotal
    AND tax_rate >= 0 AND tax_rate <= 100
    AND tax_amount >= 0
    AND total >= 0
    AND currency ~ '^[A-Z]{3}$'
    AND position > 0
  ),
  UNIQUE (organization_id, quote_id, position)
);

CREATE INDEX idx_deal_quote_line_items_org_quote
  ON deal_quote_line_items(organization_id, quote_id, position, id);
