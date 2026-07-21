-- open-crm-deploy: expand
-- Reusable quote preparation templates remain mutable definitions, while each
-- finalized quote snapshots the selected revision. Approval is a separate
-- immutable-PDF decision and never rewrites quote evidence.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE quote_templates (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  terms TEXT NOT NULL,
  default_validity_days INTEGER NOT NULL DEFAULT 30,
  delivery_subject_template TEXT NOT NULL,
  delivery_message_template TEXT NOT NULL,
  request_signature BOOLEAN NOT NULL DEFAULT FALSE,
  requires_approval BOOLEAN NOT NULL DEFAULT FALSE,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  revision INTEGER NOT NULL DEFAULT 1,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id),
  updated_by_user_id BIGINT NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT quote_templates_name_check CHECK (CHAR_LENGTH(name) BETWEEN 1 AND 120),
  CONSTRAINT quote_templates_terms_check CHECK (CHAR_LENGTH(terms) BETWEEN 1 AND 10000),
  CONSTRAINT quote_templates_validity_check CHECK (default_validity_days BETWEEN 1 AND 366),
  CONSTRAINT quote_templates_delivery_check CHECK (
    CHAR_LENGTH(delivery_subject_template) BETWEEN 1 AND 500
    AND CHAR_LENGTH(delivery_message_template) BETWEEN 1 AND 10000
  ),
  CONSTRAINT quote_templates_revision_check CHECK (revision > 0),
  UNIQUE (organization_id, id)
);

CREATE UNIQUE INDEX idx_quote_templates_org_name
  ON quote_templates(organization_id, LOWER(name))
  WHERE is_active=TRUE;

CREATE INDEX idx_quote_templates_org_active_name
  ON quote_templates(organization_id, is_active DESC, LOWER(name), id);

CREATE TABLE organization_quote_policies (
  organization_id BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
  approval_required BOOLEAN NOT NULL DEFAULT FALSE,
  updated_by_user_id BIGINT NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE deal_quotes
  ADD COLUMN source_quote_template_id BIGINT,
  ADD COLUMN quote_template_name TEXT,
  ADD COLUMN quote_template_revision INTEGER,
  ADD COLUMN delivery_subject_template TEXT,
  ADD COLUMN delivery_message_template TEXT,
  ADD COLUMN delivery_subject_default TEXT,
  ADD COLUMN delivery_message_default TEXT,
  ADD COLUMN template_request_signature BOOLEAN,
  ADD COLUMN template_requires_approval BOOLEAN;

ALTER TABLE deal_quotes
  ADD CONSTRAINT deal_quotes_template_snapshot_check CHECK (
    (
      source_quote_template_id IS NULL
      AND quote_template_name IS NULL
      AND quote_template_revision IS NULL
      AND delivery_subject_template IS NULL
      AND delivery_message_template IS NULL
      AND delivery_subject_default IS NULL
      AND delivery_message_default IS NULL
      AND template_request_signature IS NULL
      AND template_requires_approval IS NULL
    )
    OR (
      source_quote_template_id IS NOT NULL
      AND CHAR_LENGTH(quote_template_name) BETWEEN 1 AND 120
      AND quote_template_revision > 0
      AND CHAR_LENGTH(delivery_subject_template) BETWEEN 1 AND 500
      AND CHAR_LENGTH(delivery_message_template) BETWEEN 1 AND 10000
      AND CHAR_LENGTH(delivery_subject_default) BETWEEN 1 AND 500
      AND CHAR_LENGTH(delivery_message_default) BETWEEN 1 AND 10000
      AND template_request_signature IS NOT NULL
      AND template_requires_approval IS NOT NULL
    )
  ) NOT VALID,
  ADD CONSTRAINT deal_quotes_source_template_fk
    FOREIGN KEY (organization_id, source_quote_template_id)
    REFERENCES quote_templates(organization_id, id)
    NOT VALID;

ALTER TABLE deal_quotes VALIDATE CONSTRAINT deal_quotes_template_snapshot_check;
ALTER TABLE deal_quotes VALIDATE CONSTRAINT deal_quotes_source_template_fk;

CREATE UNIQUE INDEX idx_deal_quotes_org_deal_id
  ON deal_quotes(organization_id, deal_id, id);

CREATE TABLE deal_quote_approvals (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  deal_id BIGINT NOT NULL,
  quote_id BIGINT NOT NULL,
  quote_pdf_sha256 TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  requested_by_user_id BIGINT NOT NULL REFERENCES users(id),
  requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  decided_by_user_id BIGINT REFERENCES users(id),
  decided_at TIMESTAMPTZ,
  decision_note TEXT NOT NULL DEFAULT '',
  decision_key_hash TEXT,
  decision_request_sha256 TEXT,
  CONSTRAINT deal_quote_approvals_quote_fk
    FOREIGN KEY (organization_id, deal_id, quote_id)
    REFERENCES deal_quotes(organization_id, deal_id, id) ON DELETE CASCADE,
  CONSTRAINT deal_quote_approvals_status_check CHECK (status IN ('pending', 'approved', 'rejected')),
  CONSTRAINT deal_quote_approvals_pdf_check CHECK (quote_pdf_sha256 ~ '^[0-9a-f]{64}$'),
  CONSTRAINT deal_quote_approvals_note_check CHECK (CHAR_LENGTH(decision_note) <= 1000),
  CONSTRAINT deal_quote_approvals_state_check CHECK (
    (
      status = 'pending'
      AND decided_by_user_id IS NULL
      AND decided_at IS NULL
      AND decision_note = ''
      AND decision_key_hash IS NULL
      AND decision_request_sha256 IS NULL
    )
    OR (
      status IN ('approved', 'rejected')
      AND decided_by_user_id IS NOT NULL
      AND decided_by_user_id <> requested_by_user_id
      AND decided_at IS NOT NULL
      AND (status <> 'rejected' OR CHAR_LENGTH(decision_note) BETWEEN 1 AND 1000)
      AND decision_key_hash ~ '^[0-9a-f]{64}$'
      AND decision_request_sha256 ~ '^[0-9a-f]{64}$'
    )
  ),
  UNIQUE (organization_id, id),
  UNIQUE (organization_id, quote_id)
);

CREATE INDEX idx_deal_quote_approvals_org_status_requested
  ON deal_quote_approvals(organization_id, status, requested_at, id);
