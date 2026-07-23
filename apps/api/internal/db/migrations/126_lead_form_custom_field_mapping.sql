-- open-crm-deploy: expand
-- Bind public lead submissions to the exact reviewed form definition and retain
-- a value-free destination snapshot for configurable contact custom-field
-- mappings. Nullable columns plus validated presence checks keep this rolling
-- compatible with the previous writer while enforcing the final shape.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

ALTER TABLE lead_capture_forms
  ADD COLUMN revision INTEGER DEFAULT 1;

UPDATE lead_capture_forms
SET revision = 1
WHERE revision IS NULL;

ALTER TABLE lead_capture_forms
  ADD CONSTRAINT lead_capture_forms_revision_present
    CHECK (revision IS NOT NULL) NOT VALID,
  ADD CONSTRAINT lead_capture_forms_revision_positive
    CHECK (revision > 0) NOT VALID;

ALTER TABLE lead_capture_forms
  VALIDATE CONSTRAINT lead_capture_forms_revision_present;
ALTER TABLE lead_capture_forms
  VALIDATE CONSTRAINT lead_capture_forms_revision_positive;

ALTER TABLE lead_capture_submission_challenges
  ADD COLUMN form_revision INTEGER DEFAULT 1;

UPDATE lead_capture_submission_challenges
SET form_revision = 1
WHERE form_revision IS NULL;

ALTER TABLE lead_capture_submission_challenges
  ADD CONSTRAINT lead_capture_submission_challenges_form_revision_present
    CHECK (form_revision IS NOT NULL) NOT VALID,
  ADD CONSTRAINT lead_capture_submission_challenges_form_revision_positive
    CHECK (form_revision > 0) NOT VALID;

ALTER TABLE lead_capture_submission_challenges
  VALIDATE CONSTRAINT lead_capture_submission_challenges_form_revision_present;
ALTER TABLE lead_capture_submission_challenges
  VALIDATE CONSTRAINT lead_capture_submission_challenges_form_revision_positive;

ALTER TABLE lead_capture_submissions
  ADD COLUMN form_revision INTEGER DEFAULT 1,
  ADD COLUMN field_mapping_snapshot_json JSONB DEFAULT '[]'::jsonb;

UPDATE lead_capture_submissions
SET form_revision = COALESCE(form_revision, 1),
    field_mapping_snapshot_json = COALESCE(field_mapping_snapshot_json, '[]'::jsonb)
WHERE form_revision IS NULL OR field_mapping_snapshot_json IS NULL;

ALTER TABLE lead_capture_submissions
  ADD CONSTRAINT lead_capture_submissions_form_revision_present
    CHECK (form_revision IS NOT NULL) NOT VALID,
  ADD CONSTRAINT lead_capture_submissions_form_revision_positive
    CHECK (form_revision > 0) NOT VALID,
  ADD CONSTRAINT lead_capture_submissions_mapping_snapshot_present
    CHECK (field_mapping_snapshot_json IS NOT NULL) NOT VALID,
  ADD CONSTRAINT lead_capture_submissions_mapping_snapshot_array
    CHECK (jsonb_typeof(field_mapping_snapshot_json) = 'array') NOT VALID;

ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_form_revision_present;
ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_form_revision_positive;
ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_mapping_snapshot_present;
ALTER TABLE lead_capture_submissions
  VALIDATE CONSTRAINT lead_capture_submissions_mapping_snapshot_array;
