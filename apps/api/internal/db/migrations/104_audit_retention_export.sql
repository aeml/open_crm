-- open-crm-deploy: expand

CREATE OR REPLACE FUNCTION open_crm_audit_metadata_keys_are_safe(metadata JSONB)
RETURNS BOOLEAN
LANGUAGE SQL
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN jsonb_typeof(metadata) <> 'object' THEN FALSE
        ELSE NOT EXISTS (
            SELECT 1
            FROM jsonb_object_keys(metadata) AS key_name
            WHERE lower(key_name) ~ '(password|token|secret|credential|authorization|cookie)'
        )
    END
$$;

ALTER TABLE audit_events
    ADD CONSTRAINT audit_events_metadata_keys_safe
    CHECK (open_crm_audit_metadata_keys_are_safe(metadata_json)) NOT VALID;

ALTER TABLE audit_events
    VALIDATE CONSTRAINT audit_events_metadata_keys_safe;

CREATE OR REPLACE FUNCTION open_crm_protect_audit_event_history()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'TRUNCATE' THEN
        RAISE EXCEPTION 'audit events are retained for the workspace lifetime'
            USING ERRCODE = '55000';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'audit events are append-only'
            USING ERRCODE = '55000';
    END IF;

    IF TG_OP = 'DELETE' AND EXISTS (
        SELECT 1 FROM organizations WHERE id = OLD.organization_id
    ) THEN
        RAISE EXCEPTION 'audit events are retained for the workspace lifetime'
            USING ERRCODE = '55000';
    END IF;

    RETURN OLD;
END
$$;

CREATE TRIGGER audit_events_protect_history
BEFORE UPDATE OR DELETE ON audit_events
FOR EACH ROW
EXECUTE FUNCTION open_crm_protect_audit_event_history();

CREATE TRIGGER audit_events_protect_truncate
BEFORE TRUNCATE ON audit_events
FOR EACH STATEMENT
EXECUTE FUNCTION open_crm_protect_audit_event_history();
