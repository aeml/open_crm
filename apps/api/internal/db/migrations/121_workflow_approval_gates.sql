-- open-crm-deploy: expand
-- Add one reviewed human gate for deal task playbooks. Pending work keeps the
-- exact action configuration that was captured by the deal transaction, so an
-- approval decision never consults a later mutable definition.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE workflow_automation_runs
  ADD COLUMN waiting_for_approval BOOLEAN DEFAULT FALSE;

ALTER TABLE workflow_automation_action_outcomes
  ADD COLUMN action_snapshot_json JSONB DEFAULT '{}'::jsonb,
  ADD CONSTRAINT workflow_action_outcomes_snapshot_object_check
    CHECK (jsonb_typeof(action_snapshot_json) = 'object') NOT VALID;

ALTER TABLE workflow_automation_action_outcomes
  VALIDATE CONSTRAINT workflow_action_outcomes_snapshot_object_check;

CREATE TABLE workflow_automation_approvals (
  id BIGSERIAL PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  run_id BIGINT NOT NULL,
  deal_id BIGINT NOT NULL,
  action_position INTEGER NOT NULL,
  approval_name TEXT NOT NULL,
  approver_role TEXT NOT NULL,
  message TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  requested_by_user_id BIGINT NOT NULL REFERENCES users(id),
  requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  decided_by_user_id BIGINT REFERENCES users(id),
  decided_at TIMESTAMPTZ,
  decision_note TEXT NOT NULL DEFAULT '',
  decision_key_hash TEXT NOT NULL DEFAULT '',
  decision_request_sha256 TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT workflow_approvals_run_fk
    FOREIGN KEY (organization_id, run_id)
    REFERENCES workflow_automation_runs(organization_id, id)
    ON DELETE CASCADE,
  CONSTRAINT workflow_approvals_deal_fk
    FOREIGN KEY (organization_id, deal_id)
    REFERENCES deals(organization_id, id)
    ON DELETE CASCADE,
  CONSTRAINT workflow_approvals_action_fk
    FOREIGN KEY (organization_id, run_id, action_position)
    REFERENCES workflow_automation_action_outcomes(organization_id, run_id, action_position)
    ON DELETE CASCADE,
  CONSTRAINT workflow_approvals_name_check
    CHECK (length(trim(approval_name)) BETWEEN 1 AND 200),
  CONSTRAINT workflow_approvals_role_check
    CHECK (approver_role IN ('owner', 'admin', 'record_owner')),
  CONSTRAINT workflow_approvals_message_check
    CHECK (length(trim(message)) BETWEEN 1 AND 2000),
  CONSTRAINT workflow_approvals_status_check
    CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
  CONSTRAINT workflow_approvals_action_position_check
    CHECK (action_position BETWEEN 1 AND 25),
  CONSTRAINT workflow_approvals_decision_note_check
    CHECK (length(decision_note) <= 1000),
  CONSTRAINT workflow_approvals_key_hash_check
    CHECK (decision_key_hash = '' OR length(decision_key_hash) = 64),
  CONSTRAINT workflow_approvals_request_hash_check
    CHECK (decision_request_sha256 = '' OR length(decision_request_sha256) = 64),
  CONSTRAINT workflow_approvals_decision_state_check
    CHECK (
      (status = 'pending' AND decided_by_user_id IS NULL AND decided_at IS NULL
       AND decision_key_hash = '' AND decision_request_sha256 = '')
      OR (status IN ('approved', 'rejected') AND decided_by_user_id IS NOT NULL
          AND decided_at IS NOT NULL AND length(decision_key_hash) = 64
          AND length(decision_request_sha256) = 64)
      OR (status = 'cancelled' AND decided_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX idx_workflow_approvals_org_run_unique
  ON workflow_automation_approvals(organization_id, run_id);

CREATE INDEX idx_workflow_approvals_org_pending_requested
  ON workflow_automation_approvals(organization_id, requested_at, id)
  WHERE status = 'pending';
