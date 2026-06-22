-- 1.6.2 Chart and visualization type foundation.
-- Stores intended visualization type before report query rendering exists.

ALTER TABLE custom_report_definitions
  ADD COLUMN visualization_type TEXT NOT NULL DEFAULT 'table';

ALTER TABLE custom_report_definitions
  ADD CONSTRAINT custom_report_definitions_visualization_type_check
  CHECK (visualization_type IN ('table', 'bar', 'line', 'funnel', 'pie', 'kpi'));

CREATE INDEX idx_custom_report_definitions_org_visualization
  ON custom_report_definitions(organization_id, visualization_type, updated_at DESC, id DESC);
