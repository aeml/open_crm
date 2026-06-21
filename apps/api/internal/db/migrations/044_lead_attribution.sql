-- 1.4.3 Lead source and UTM/campaign attribution tracking foundation.

ALTER TABLE contacts
  ADD COLUMN lead_source TEXT NOT NULL DEFAULT '',
  ADD COLUMN first_source_url TEXT NOT NULL DEFAULT '',
  ADD COLUMN utm_source TEXT NOT NULL DEFAULT '',
  ADD COLUMN utm_medium TEXT NOT NULL DEFAULT '',
  ADD COLUMN utm_campaign TEXT NOT NULL DEFAULT '',
  ADD COLUMN utm_term TEXT NOT NULL DEFAULT '',
  ADD COLUMN utm_content TEXT NOT NULL DEFAULT '';

ALTER TABLE lead_capture_submissions
  ADD COLUMN lead_source TEXT NOT NULL DEFAULT '',
  ADD COLUMN utm_source TEXT NOT NULL DEFAULT '',
  ADD COLUMN utm_medium TEXT NOT NULL DEFAULT '',
  ADD COLUMN utm_campaign TEXT NOT NULL DEFAULT '',
  ADD COLUMN utm_term TEXT NOT NULL DEFAULT '',
  ADD COLUMN utm_content TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_contacts_org_lead_source
  ON contacts(organization_id, lead_source)
  WHERE lead_source <> '';

CREATE INDEX idx_contacts_org_utm_campaign
  ON contacts(organization_id, utm_campaign)
  WHERE utm_campaign <> '';

CREATE INDEX idx_lead_capture_submissions_org_attribution
  ON lead_capture_submissions(organization_id, lead_source, utm_campaign, created_at DESC)
  WHERE lead_source <> '' OR utm_campaign <> '';
