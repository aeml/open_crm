DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'companies'
          AND column_name = 'domain'
    ) THEN
        EXECUTE $sql$
            UPDATE companies
            SET website = CASE
                WHEN COALESCE(NULLIF(BTRIM(website), ''), '') = '' AND COALESCE(NULLIF(BTRIM(domain), ''), '') <> ''
                THEN 'https://' || LOWER(BTRIM(domain))
                ELSE website
            END
        $sql$;

        EXECUTE 'DROP INDEX IF EXISTS idx_companies_org_domain';
        EXECUTE 'ALTER TABLE companies DROP COLUMN IF EXISTS domain';
    END IF;
END $$;
