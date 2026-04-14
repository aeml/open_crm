ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS address_line1 TEXT,
    ADD COLUMN IF NOT EXISTS address_line2 TEXT,
    ADD COLUMN IF NOT EXISTS city TEXT,
    ADD COLUMN IF NOT EXISTS state TEXT,
    ADD COLUMN IF NOT EXISTS postal_code TEXT,
    ADD COLUMN IF NOT EXISTS country TEXT;

UPDATE companies
SET address_line1 = COALESCE(NULLIF(address_line1, ''), NULLIF(address, ''))
WHERE COALESCE(address, '') <> '';

ALTER TABLE contacts
    ADD COLUMN IF NOT EXISTS address_line1 TEXT,
    ADD COLUMN IF NOT EXISTS address_line2 TEXT,
    ADD COLUMN IF NOT EXISTS city TEXT,
    ADD COLUMN IF NOT EXISTS state TEXT,
    ADD COLUMN IF NOT EXISTS postal_code TEXT,
    ADD COLUMN IF NOT EXISTS country TEXT;

UPDATE contacts
SET address_line1 = COALESCE(NULLIF(address_line1, ''), NULLIF(address, ''))
WHERE COALESCE(address, '') <> '';

ALTER TABLE companies
    DROP COLUMN IF EXISTS address;

ALTER TABLE contacts
    DROP COLUMN IF EXISTS address;
