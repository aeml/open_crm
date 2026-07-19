-- open-crm-deploy: expand
-- Closing context is captured on the live deal and snapshotted on the durable
-- stage event. Existing closed deals remain explicitly "not captured" rather
-- than being assigned a reason that an operator never chose.

ALTER TABLE organizations
  ADD COLUMN deal_close_reason_tracking_started_at TIMESTAMPTZ DEFAULT NOW();

ALTER TABLE deals
  ADD COLUMN close_reason_code TEXT DEFAULT '',
  ADD COLUMN close_reason_label TEXT DEFAULT '',
  ADD COLUMN close_notes TEXT DEFAULT '',
  ADD COLUMN closed_at TIMESTAMPTZ,
  ADD COLUMN closed_by_user_id BIGINT;

ALTER TABLE deal_stage_events
  ADD COLUMN close_reason_code TEXT DEFAULT '',
  ADD COLUMN close_reason_label TEXT DEFAULT '',
  ADD COLUMN close_notes TEXT DEFAULT '';

ALTER TABLE deals
  ADD CONSTRAINT deals_close_reason_code_check CHECK (
    COALESCE(close_reason_code, '') IN ('', 'relationship', 'solution_fit', 'price_value', 'service_quality', 'timing', 'budget', 'competitor', 'no_decision', 'scope_fit', 'unresponsive', 'other')
  ) NOT VALID,
  ADD CONSTRAINT deals_close_reason_label_length_check CHECK (char_length(COALESCE(close_reason_label, '')) <= 100) NOT VALID,
  ADD CONSTRAINT deals_close_notes_length_check CHECK (char_length(COALESCE(close_notes, '')) <= 2000) NOT VALID,
  ADD CONSTRAINT deals_closed_by_user_fk FOREIGN KEY (closed_by_user_id) REFERENCES users(id) NOT VALID;

ALTER TABLE deal_stage_events
  ADD CONSTRAINT deal_stage_events_close_reason_code_check CHECK (
    COALESCE(close_reason_code, '') IN ('', 'relationship', 'solution_fit', 'price_value', 'service_quality', 'timing', 'budget', 'competitor', 'no_decision', 'scope_fit', 'unresponsive', 'other')
  ) NOT VALID,
  ADD CONSTRAINT deal_stage_events_close_reason_label_length_check CHECK (char_length(COALESCE(close_reason_label, '')) <= 100) NOT VALID,
  ADD CONSTRAINT deal_stage_events_close_notes_length_check CHECK (char_length(COALESCE(close_notes, '')) <= 2000) NOT VALID;

UPDATE deals d
SET status = CASE WHEN s.is_closed AND s.is_won THEN 'won' WHEN s.is_closed THEN 'lost' ELSE 'open' END,
    close_reason_code = '',
    close_reason_label = '',
    close_notes = '',
    closed_at = CASE WHEN s.is_closed THEN d.updated_at ELSE NULL END,
    closed_by_user_id = NULL
FROM deal_stages s
WHERE s.organization_id = d.organization_id
  AND s.id = d.stage_id;

CREATE INDEX idx_deal_stage_events_org_outcome_reason
  ON deal_stage_events(organization_id, to_stage_outcome, close_reason_code, occurred_at DESC)
  WHERE to_stage_outcome IN ('won', 'lost');

CREATE INDEX idx_deals_org_closed_at
  ON deals(organization_id, closed_at DESC, id DESC)
  WHERE closed_at IS NOT NULL AND archived_at IS NULL;
