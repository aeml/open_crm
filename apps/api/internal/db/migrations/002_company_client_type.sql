ALTER TABLE companies
ADD COLUMN IF NOT EXISTS client_type TEXT NOT NULL DEFAULT 'organization';

UPDATE companies
SET client_type = 'organization'
WHERE client_type IS NULL OR client_type = '';

ALTER TABLE companies
DROP CONSTRAINT IF EXISTS companies_client_type_check;

ALTER TABLE companies
ADD CONSTRAINT companies_client_type_check
CHECK (client_type IN ('organization', 'individual'));

CREATE INDEX IF NOT EXISTS idx_companies_org_client_type ON companies (organization_id, client_type);
