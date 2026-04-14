ALTER TABLE contacts
ADD COLUMN IF NOT EXISTS is_client BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_contacts_org_is_client ON contacts (organization_id, is_client);
