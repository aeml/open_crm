-- open-crm-deploy: expand
-- Cover the bounded customer-health view without widening ordinary write paths.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE INDEX idx_tasks_org_open_entity_due
  ON tasks(organization_id, entity_type, entity_id, due_at, id)
  WHERE archived_at IS NULL AND status='open';

CREATE INDEX idx_companies_org_customer_owner_name
  ON companies(organization_id, owner_user_id, name, id)
  WHERE archived_at IS NULL AND status='customer';

CREATE INDEX idx_contacts_org_client_owner_name
  ON contacts(organization_id, owner_user_id, last_name, first_name, id)
  WHERE archived_at IS NULL AND (is_client=TRUE OR status='customer');
