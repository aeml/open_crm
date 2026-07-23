-- open-crm-deploy: expand
-- Add one revisioned, workspace-shared analytics dashboard backed only by the
-- already executable grouped-bar report contract. Existing report definitions
-- and rolling old application versions remain unchanged.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE UNIQUE INDEX custom_report_definitions_org_id_unique
  ON custom_report_definitions(organization_id, id);

CREATE TABLE custom_report_dashboards (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
  revision BIGINT NOT NULL DEFAULT 1,
  updated_by_user_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT custom_report_dashboards_revision_positive CHECK (revision > 0),
  CONSTRAINT custom_report_dashboards_org_id_unique UNIQUE (organization_id, id),
  CONSTRAINT custom_report_dashboards_updater_membership_fk
    FOREIGN KEY (organization_id, updated_by_user_id)
    REFERENCES organization_memberships(organization_id, user_id)
);

CREATE TABLE custom_report_dashboard_widgets (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL,
  dashboard_id BIGINT NOT NULL,
  report_definition_id BIGINT NOT NULL,
  position SMALLINT NOT NULL,
  width TEXT NOT NULL DEFAULT 'half',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT custom_report_dashboard_widgets_position_check CHECK (position BETWEEN 0 AND 5),
  CONSTRAINT custom_report_dashboard_widgets_width_check CHECK (width IN ('half', 'full')),
  CONSTRAINT custom_report_dashboard_widgets_dashboard_fk
    FOREIGN KEY (organization_id, dashboard_id)
    REFERENCES custom_report_dashboards(organization_id, id)
    ON DELETE CASCADE,
  CONSTRAINT custom_report_dashboard_widgets_definition_fk
    FOREIGN KEY (organization_id, report_definition_id)
    REFERENCES custom_report_definitions(organization_id, id),
  CONSTRAINT custom_report_dashboard_widgets_position_unique
    UNIQUE (organization_id, dashboard_id, position),
  CONSTRAINT custom_report_dashboard_widgets_definition_unique
    UNIQUE (organization_id, dashboard_id, report_definition_id)
);

CREATE INDEX idx_custom_report_dashboard_widgets_definition
  ON custom_report_dashboard_widgets(organization_id, report_definition_id, dashboard_id);
