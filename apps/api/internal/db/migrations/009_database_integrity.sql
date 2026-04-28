UPDATE organizations
SET business_type = 'general'
WHERE business_type IS NULL
   OR business_type NOT IN ('general', 'services', 'product-sales', 'construction-services');

UPDATE organization_memberships
SET role = 'member'
WHERE role IS NULL
   OR role NOT IN ('owner', 'admin', 'member', 'viewer');

UPDATE companies
SET client_type = 'organization'
WHERE client_type IS NULL
   OR client_type NOT IN ('organization', 'individual');

UPDATE companies
SET status = NULL
WHERE status IS NOT NULL
  AND status NOT IN ('lead', 'prospect', 'customer');

UPDATE contacts
SET status = NULL
WHERE status IS NOT NULL
  AND status NOT IN ('lead', 'prospect', 'customer');

UPDATE deals
SET status = NULL
WHERE status IS NOT NULL
  AND status NOT IN ('open', 'won', 'lost');

UPDATE deals
SET value_amount = NULL
WHERE value_amount IS NOT NULL
  AND value_amount < 0;

UPDATE deals
SET value_currency = UPPER(value_currency)
WHERE value_currency IS NOT NULL;

UPDATE deals
SET value_currency = NULL
WHERE value_currency IS NOT NULL
  AND value_currency !~ '^[A-Z]{3}$';

UPDATE notes
SET entity_type = LOWER(entity_type)
WHERE entity_type IS NOT NULL;

UPDATE tasks
SET entity_type = LOWER(entity_type)
WHERE entity_type IS NOT NULL;

UPDATE tasks
SET status = 'open'
WHERE status IS NULL
   OR status NOT IN ('open', 'completed');

UPDATE activities
SET entity_type = LOWER(entity_type)
WHERE entity_type IS NOT NULL;

WITH ranked_links AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY organization_id, contact_id, company_id ORDER BY is_primary DESC, id ASC) AS duplicate_rank
    FROM contact_company_links
)
DELETE FROM contact_company_links links
USING ranked_links ranked
WHERE links.id = ranked.id
  AND ranked.duplicate_rank > 1;

WITH ranked_primary_links AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY organization_id, company_id ORDER BY id ASC) AS primary_rank
    FROM contact_company_links
    WHERE is_primary
)
UPDATE contact_company_links links
SET is_primary = FALSE
FROM ranked_primary_links ranked
WHERE links.id = ranked.id
  AND ranked.primary_rank > 1;

WITH ranked_stage_positions AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY organization_id ORDER BY position ASC, id ASC) AS next_position
    FROM deal_stages
)
UPDATE deal_stages stages
SET position = ranked.next_position
FROM ranked_stage_positions ranked
WHERE stages.id = ranked.id;

WITH duplicate_stage_names AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY organization_id, lower(name) ORDER BY id ASC) AS duplicate_rank
    FROM deal_stages
)
UPDATE deal_stages stages
SET name = stages.name || ' #' || stages.id::text
FROM duplicate_stage_names ranked
WHERE stages.id = ranked.id
  AND ranked.duplicate_rank > 1;

ALTER TABLE organizations
DROP CONSTRAINT IF EXISTS organizations_business_type_check;

ALTER TABLE organizations
ADD CONSTRAINT organizations_business_type_check
CHECK (business_type IN ('general', 'services', 'product-sales', 'construction-services'));

ALTER TABLE organization_memberships
DROP CONSTRAINT IF EXISTS organization_memberships_role_check;

ALTER TABLE organization_memberships
ADD CONSTRAINT organization_memberships_role_check
CHECK (role IN ('owner', 'admin', 'member', 'viewer'));

ALTER TABLE companies
DROP CONSTRAINT IF EXISTS companies_status_check;

ALTER TABLE companies
ADD CONSTRAINT companies_status_check
CHECK (status IS NULL OR status IN ('lead', 'prospect', 'customer'));

ALTER TABLE contacts
DROP CONSTRAINT IF EXISTS contacts_status_check;

ALTER TABLE contacts
ADD CONSTRAINT contacts_status_check
CHECK (status IS NULL OR status IN ('lead', 'prospect', 'customer'));

ALTER TABLE deals
DROP CONSTRAINT IF EXISTS deals_status_check;

ALTER TABLE deals
ADD CONSTRAINT deals_status_check
CHECK (status IS NULL OR status IN ('open', 'won', 'lost'));

ALTER TABLE deals
DROP CONSTRAINT IF EXISTS deals_value_amount_nonnegative_check;

ALTER TABLE deals
ADD CONSTRAINT deals_value_amount_nonnegative_check
CHECK (value_amount IS NULL OR value_amount >= 0);

ALTER TABLE deals
DROP CONSTRAINT IF EXISTS deals_value_currency_code_check;

ALTER TABLE deals
ADD CONSTRAINT deals_value_currency_code_check
CHECK (value_currency IS NULL OR value_currency ~ '^[A-Z]{3}$');

ALTER TABLE notes
DROP CONSTRAINT IF EXISTS notes_entity_type_check;

ALTER TABLE notes
ADD CONSTRAINT notes_entity_type_check
CHECK (entity_type IN ('contact', 'company', 'deal'));

ALTER TABLE tasks
DROP CONSTRAINT IF EXISTS tasks_entity_type_check;

ALTER TABLE tasks
ADD CONSTRAINT tasks_entity_type_check
CHECK (entity_type IN ('contact', 'company', 'deal'));

ALTER TABLE tasks
DROP CONSTRAINT IF EXISTS tasks_status_check;

ALTER TABLE tasks
ADD CONSTRAINT tasks_status_check
CHECK (status IN ('open', 'completed'));

ALTER TABLE activities
DROP CONSTRAINT IF EXISTS activities_entity_type_check;

ALTER TABLE activities
ADD CONSTRAINT activities_entity_type_check
CHECK (entity_type IN ('contact', 'company', 'deal', 'task'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_deal_stages_org_position_unique
    ON deal_stages (organization_id, position);

CREATE UNIQUE INDEX IF NOT EXISTS idx_deal_stages_org_name_unique
    ON deal_stages (organization_id, lower(name));

CREATE UNIQUE INDEX IF NOT EXISTS idx_contact_company_links_unique
    ON contact_company_links (organization_id, contact_id, company_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_contact_company_links_primary_company
    ON contact_company_links (organization_id, company_id)
    WHERE is_primary;

CREATE INDEX IF NOT EXISTS idx_sessions_token_hash
    ON sessions (token_hash);

CREATE INDEX IF NOT EXISTS idx_sessions_expires_at
    ON sessions (expires_at);

CREATE INDEX IF NOT EXISTS idx_companies_org_archived_name
    ON companies (organization_id, archived_at, name);

CREATE INDEX IF NOT EXISTS idx_contacts_org_archived_name
    ON contacts (organization_id, archived_at, last_name, first_name);

CREATE INDEX IF NOT EXISTS idx_deals_org_archived_stage
    ON deals (organization_id, archived_at, stage_id);

CREATE INDEX IF NOT EXISTS idx_deals_org_archived_owner
    ON deals (organization_id, archived_at, owner_user_id);

CREATE INDEX IF NOT EXISTS idx_tasks_org_archived_entity
    ON tasks (organization_id, archived_at, entity_type, entity_id);

CREATE INDEX IF NOT EXISTS idx_notes_org_entity_created
    ON notes (organization_id, entity_type, entity_id, created_at DESC);
