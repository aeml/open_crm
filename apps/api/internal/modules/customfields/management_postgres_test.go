package customfields_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
)

func TestCustomFieldManagementIsRevisionSafeAuthorizedAndCapacitySerializedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to custom-field management postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_custom_field_management_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create custom-field management schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: customFieldsDatabaseURL(t, databaseURL, schema)})
	if err != nil {
		t.Fatalf("connect to custom-field management schema: %v", err)
	}
	defer pool.Close()
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: customFieldsDatabaseURL(t, databaseURL, schema)}); err != nil {
		t.Fatalf("migrate custom-field management schema: %v", err)
	}

	organizationID := insertCustomFieldsOrganization(t, ctx, pool, "Custom-field management", "custom-field-management-"+schema)
	foreignOrganizationID := insertCustomFieldsOrganization(t, ctx, pool, "Foreign custom fields", "foreign-custom-field-management-"+schema)
	ownerID := insertCustomFieldsUser(t, ctx, pool, "custom-field-owner-"+schema+"@example.test")
	adminID := insertCustomFieldsUser(t, ctx, pool, "custom-field-admin-"+schema+"@example.test")
	memberID := insertCustomFieldsUser(t, ctx, pool, "custom-field-member-"+schema+"@example.test")
	disabledID := insertCustomFieldsUser(t, ctx, pool, "custom-field-disabled-"+schema+"@example.test")
	foreignOwnerID := insertCustomFieldsUser(t, ctx, pool, "custom-field-foreign-"+schema+"@example.test")
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		role           string
		status         string
	}{
		{organizationID, ownerID, "owner", "active"},
		{organizationID, adminID, "admin", "active"},
		{organizationID, memberID, "member", "active"},
		{organizationID, disabledID, "admin", "disabled"},
		{foreignOrganizationID, foreignOwnerID, "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO organization_memberships(organization_id,user_id,role,membership_status)
			VALUES($1,$2,$3,$4)
		`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("seed custom-field membership: %v", err)
		}
	}

	service := modulecustomfields.NewService(pool)
	if _, err := service.Create(ctx, organizationID, memberID, validCustomFieldInput("member_field", "Member field", 0)); !errors.Is(err, modulecustomfields.ErrForbidden) {
		t.Fatalf("active member direct create error=%v, want forbidden", err)
	}
	if _, err := service.Create(ctx, organizationID, disabledID, validCustomFieldInput("disabled_field", "Disabled field", 0)); !errors.Is(err, modulecustomfields.ErrInactiveActor) {
		t.Fatalf("disabled admin direct create error=%v, want inactive actor", err)
	}

	for index := 1; index < modulecustomfields.MaxDefinitionsPerEntity; index++ {
		if _, err := pool.Exec(ctx, `
			INSERT INTO custom_field_definitions(
				organization_id,created_by_user_id,entity_type,field_key,label,data_type,position
			) VALUES($1,$2,'contact',$3,$4,'text',$5)
		`, organizationID, ownerID, fmt.Sprintf("seeded_%02d", index), fmt.Sprintf("Seeded %02d", index), index); err != nil {
			t.Fatalf("seed active custom field %d: %v", index, err)
		}
	}

	type createResult struct {
		definition modulecustomfields.Definition
		err        error
	}
	start := make(chan struct{})
	results := make(chan createResult, 2)
	var group sync.WaitGroup
	for _, candidate := range []struct {
		actorID int64
		key     string
		label   string
	}{
		{ownerID, "final_owner", "Final owner field"},
		{adminID, "final_admin", "Final admin field"},
	} {
		group.Add(1)
		go func(candidate struct {
			actorID int64
			key     string
			label   string
		}) {
			defer group.Done()
			<-start
			definition, createErr := service.Create(ctx, organizationID, candidate.actorID, validCustomFieldInput(candidate.key, candidate.label, 100))
			results <- createResult{definition: definition, err: createErr}
		}(candidate)
	}
	close(start)
	group.Wait()
	close(results)
	var winner modulecustomfields.Definition
	successes, limited := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			successes++
			winner = result.definition
		case errors.Is(result.err, modulecustomfields.ErrConflict):
			limited++
		default:
			t.Fatalf("unexpected final-slot result: definition=%+v err=%v", result.definition, result.err)
		}
	}
	if successes != 1 || limited != 1 || winner.Revision != 1 {
		t.Fatalf("custom-field capacity was not serialized: successes=%d limited=%d winner=%+v", successes, limited, winner)
	}
	definitions, err := service.List(ctx, organizationID, "contact", false)
	if err != nil || len(definitions) != modulecustomfields.MaxDefinitionsPerEntity {
		t.Fatalf("complete capped custom-field catalog: count=%d err=%v", len(definitions), err)
	}
	if _, err := service.Update(ctx, organizationID, memberID, winner.ID, modulecustomfields.UpdateInput{
		Label: "Member overwrite", Position: winner.Position, Revision: winner.Revision,
	}); !errors.Is(err, modulecustomfields.ErrForbidden) {
		t.Fatalf("active member direct update error=%v, want forbidden", err)
	}
	if err := service.Archive(ctx, organizationID, disabledID, winner.ID, winner.Revision); !errors.Is(err, modulecustomfields.ErrInactiveActor) {
		t.Fatalf("disabled admin direct archive error=%v, want inactive actor", err)
	}

	updated, err := service.Update(ctx, organizationID, ownerID, winner.ID, modulecustomfields.UpdateInput{
		Label: "Reviewed final field", Position: winner.Position, Revision: winner.Revision,
	})
	if err != nil || updated.Revision != 2 || updated.Label != "Reviewed final field" {
		t.Fatalf("exact custom-field update: definition=%+v err=%v", updated, err)
	}
	if _, err := service.Update(ctx, organizationID, adminID, winner.ID, modulecustomfields.UpdateInput{
		Label: "Stale overwrite", Position: winner.Position, Revision: winner.Revision,
	}); !errors.Is(err, modulecustomfields.ErrChanged) {
		t.Fatalf("stale custom-field update error=%v, want changed", err)
	}
	if err := service.Archive(ctx, organizationID, adminID, winner.ID, winner.Revision); !errors.Is(err, modulecustomfields.ErrChanged) {
		t.Fatalf("stale custom-field archive error=%v, want changed", err)
	}

	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION reject_custom_field_update_audit() RETURNS TRIGGER LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.event_type='custom_field.updated' THEN
				RAISE EXCEPTION 'forced custom-field audit failure';
			END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER reject_custom_field_update_audit_trigger
		BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_custom_field_update_audit();
	`); err != nil {
		t.Fatalf("install forced custom-field audit failure: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, ownerID, winner.ID, modulecustomfields.UpdateInput{
		Label: "Must roll back", Position: updated.Position, Revision: updated.Revision,
	}); err == nil {
		t.Fatal("custom-field update unexpectedly survived forced audit failure")
	}
	var retainedLabel string
	var retainedRevision int
	if err := pool.QueryRow(ctx, `SELECT label,revision FROM custom_field_definitions WHERE organization_id=$1 AND id=$2`, organizationID, winner.ID).Scan(&retainedLabel, &retainedRevision); err != nil || retainedLabel != updated.Label || retainedRevision != updated.Revision {
		t.Fatalf("failed custom-field audit leaked mutation: label=%q revision=%d err=%v", retainedLabel, retainedRevision, err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_custom_field_update_audit_trigger ON audit_events; DROP FUNCTION reject_custom_field_update_audit()`); err != nil {
		t.Fatalf("remove forced custom-field audit failure: %v", err)
	}

	if err := service.Archive(ctx, organizationID, ownerID, winner.ID, updated.Revision); err != nil {
		t.Fatalf("archive exact custom-field revision: %v", err)
	}
	replacement, err := service.Create(ctx, organizationID, adminID, validCustomFieldInput("replacement_field", "Replacement field", 101))
	if err != nil || replacement.Revision != 1 {
		t.Fatalf("reuse active capacity after archive: definition=%+v err=%v", replacement, err)
	}

	foreign, err := service.Create(ctx, foreignOrganizationID, foreignOwnerID, validCustomFieldInput("foreign_field", "Foreign field", 0))
	if err != nil {
		t.Fatalf("create foreign custom field: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, ownerID, foreign.ID, modulecustomfields.UpdateInput{Label: "Cross tenant", Revision: foreign.Revision}); !errors.Is(err, modulecustomfields.ErrNotFound) {
		t.Fatalf("cross-tenant custom-field update error=%v, want not found", err)
	}
	foreignDefinitions, err := service.List(ctx, foreignOrganizationID, "contact", false)
	if err != nil || len(foreignDefinitions) != 1 || foreignDefinitions[0].ID != foreign.ID {
		t.Fatalf("foreign custom-field catalog isolation: definitions=%+v err=%v", foreignDefinitions, err)
	}

	var auditCount int
	var metadata string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int,COALESCE(string_agg(metadata_json::text,' '),'')
		FROM audit_events
		WHERE organization_id=$1 AND entity_type='custom_field'
	`, organizationID).Scan(&auditCount, &metadata); err != nil || auditCount != 4 {
		t.Fatalf("custom-field audit count=%d, want 4 (err=%v)", auditCount, err)
	}
	if strings.Contains(metadata, "Reviewed final field") || strings.Contains(metadata, "Must roll back") {
		t.Fatalf("custom-field audit retained mutable values: %s", metadata)
	}
}

func validCustomFieldInput(key, label string, position int) modulecustomfields.CreateInput {
	return modulecustomfields.CreateInput{
		EntityType: "contact",
		FieldKey:   key,
		Label:      label,
		DataType:   "text",
		Position:   position,
	}
}
