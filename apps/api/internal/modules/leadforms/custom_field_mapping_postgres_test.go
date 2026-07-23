package leadforms

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
)

func TestLeadFormCustomFieldMappingIsTypedRevisionedAuditedAndTenantSafeAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead mapping postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_mapping_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead mapping schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := databaseURLWithLeadFormSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead mapping schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated lead mapping schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, ownerID, memberID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Mapped Leads',$1) RETURNING id`, "mapped-leads-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create lead mapping workspace: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign Mapping',$1) RETURNING id`, "foreign-mapping-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign workspace: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Mapping','Owner') RETURNING id`, "mapping-owner-"+schema+"@example.test").Scan(&ownerID); err != nil {
		t.Fatalf("create mapping owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Mapping','Member') RETURNING id`, "mapping-member-"+schema+"@example.test").Scan(&memberID); err != nil {
		t.Fatalf("create mapping member: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$2,'owner','active'),($1,$3,'member','active'),($4,$2,'owner','active')
	`, organizationID, ownerID, memberID, foreignOrganizationID); err != nil {
		t.Fatalf("create mapping memberships: %v", err)
	}

	customFields := modulecustomfields.NewService(pool)
	region := createLeadMappingDefinition(t, ctx, customFields, organizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "contact", FieldKey: "region", Label: "Region", DataType: "select", Options: []string{"North", "South"}, Required: true,
	})
	budget := createLeadMappingDefinition(t, ctx, customFields, organizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "contact", FieldKey: "budget", Label: "Budget", DataType: "number",
	})
	startDate := createLeadMappingDefinition(t, ctx, customFields, organizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "contact", FieldKey: "start_date", Label: "Start date", DataType: "date",
	})
	qualified := createLeadMappingDefinition(t, ctx, customFields, organizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "contact", FieldKey: "qualified", Label: "Qualified", DataType: "boolean",
	})
	notes := createLeadMappingDefinition(t, ctx, customFields, organizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "contact", FieldKey: "intake_notes", Label: "Intake notes", DataType: "text",
	})
	createLeadMappingDefinition(t, ctx, customFields, foreignOrganizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "contact", FieldKey: "foreign_only", Label: "Foreign only", DataType: "text",
	})

	service := NewService(pool)
	baseInput := Input{
		Name: "Qualified inquiry", Slug: "qualified-inquiry", Title: "Tell us about your project",
		SuccessMessage: "Thanks", SourceLabel: "Pilot website",
		Fields: []Field{
			{Key: "first", Label: "First name", FieldType: "text", Required: true, MapTo: "firstName"},
			{Key: "last", Label: "Last name", FieldType: "text", Required: true, MapTo: "lastName"},
			{Key: "region", Label: "Region", FieldType: "text", MapTo: "custom:region"},
			{Key: "budget", Label: "Budget", FieldType: "text", MapTo: "custom:budget"},
			{Key: "start", Label: "Start date", FieldType: "text", MapTo: "custom:start_date"},
			{Key: "qualified", Label: "Already qualified", FieldType: "text", MapTo: "custom:qualified"},
			{Key: "notes", Label: "Project notes", FieldType: "textarea", MapTo: "custom:intake_notes"},
		},
	}
	if _, err := service.Create(ctx, organizationID, memberID, baseInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-admin direct form create error=%v, want not found", err)
	}
	missingRequired := baseInput
	missingRequired.Name, missingRequired.Slug = "Missing required", "missing-required"
	missingRequired.Fields = append([]Field(nil), baseInput.Fields[:2]...)
	if _, err := service.Create(ctx, organizationID, ownerID, missingRequired); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("active form without required mapping error=%v, want invalid mapping", err)
	}
	foreignMapping := baseInput
	foreignMapping.Name, foreignMapping.Slug = "Foreign mapping", "foreign-mapping"
	foreignMapping.Fields = append(cloneFields(baseInput.Fields), Field{Key: "foreign", Label: "Foreign", FieldType: "text", MapTo: "custom:foreign_only"})
	if _, err := service.Create(ctx, organizationID, ownerID, foreignMapping); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("foreign custom-field mapping error=%v, want invalid mapping", err)
	}

	form, err := service.Create(ctx, organizationID, ownerID, baseInput)
	if err != nil {
		t.Fatalf("create custom-mapped lead form: %v", err)
	}
	if form.Revision != 1 || !form.Fields[2].Required || form.Fields[2].FieldType != "select" || len(form.Fields[2].Options) != 2 || form.Fields[3].FieldType != "number" || form.Fields[4].FieldType != "date" || form.Fields[5].FieldType != "boolean" || form.Fields[6].FieldType != "textarea" {
		t.Fatalf("custom mapping was not normalized from definitions: %#v", form)
	}
	challenge, err := service.IssueSubmissionChallenge(ctx, form.PublicID)
	if err != nil || challenge.FormRevision != 1 {
		t.Fatalf("issue revision-one challenge: challenge=%#v err=%v", challenge, err)
	}

	region, err = customFields.Update(ctx, organizationID, ownerID, region.ID, modulecustomfields.UpdateInput{
		Label: region.Label, Options: []string{"North", "South", "West"}, Required: true, Position: region.Position, Revision: region.Revision,
	})
	if err != nil {
		t.Fatalf("update mapped select definition: %v", err)
	}
	service.now = func() time.Time { return challenge.NotBefore.Add(time.Millisecond) }
	submissionInput := SubmissionInput{Values: map[string]string{
		"first": "Ada", "last": "Lovelace", "region": "West", "budget": "1250.50",
		"start": "2026-08-01", "qualified": "false", "notes": "Needs a migration plan",
	}, ChallengeToken: challenge.Token, ConsentGranted: true}
	if _, err := service.SubmitByPublicID(ctx, form.PublicID, submissionInput); !errors.Is(err, ErrChallengeInvalid) {
		t.Fatalf("stale challenge after mapped-definition update error=%v, want invalid challenge", err)
	}

	formPage, err := service.ListByOrganization(ctx, organizationID, FormListQuery{})
	if err != nil || len(formPage.Forms) != 1 {
		t.Fatalf("reload mapped form: page=%#v err=%v", formPage, err)
	}
	forms := formPage.Forms
	form = forms[0]
	if form.Revision != 2 || len(form.Fields[2].Options) != 3 || form.Fields[2].Options[2] != "West" {
		t.Fatalf("mapped definition did not advance/hydrate form: %#v", form)
	}
	companyRegion := createLeadMappingDefinition(t, ctx, customFields, organizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "company", FieldKey: "region", Label: "Company region", DataType: "text", Required: true,
	})
	if err := customFields.Archive(ctx, organizationID, ownerID, companyRegion.ID, companyRegion.Revision); err != nil {
		t.Fatalf("same-key company field should not be coupled to contact lead forms: %v", err)
	}
	var revisionAfterCompanyLifecycle int
	if err := pool.QueryRow(ctx, `SELECT revision FROM lead_capture_forms WHERE organization_id=$1 AND id=$2`, organizationID, form.ID).Scan(&revisionAfterCompanyLifecycle); err != nil || revisionAfterCompanyLifecycle != 2 {
		t.Fatalf("company custom-field lifecycle revised contact form: revision=%d err=%v", revisionAfterCompanyLifecycle, err)
	}
	challenge, err = service.IssueSubmissionChallenge(ctx, form.PublicID)
	if err != nil || challenge.FormRevision != 2 {
		t.Fatalf("issue current mapped challenge: challenge=%#v err=%v", challenge, err)
	}
	service.now = func() time.Time { return challenge.NotBefore.Add(time.Millisecond) }
	submissionInput.ChallengeToken = challenge.Token
	created, err := service.SubmitByPublicID(ctx, form.PublicID, submissionInput)
	if err != nil {
		t.Fatalf("submit typed mapped lead: %v", err)
	}
	replayed, err := service.SubmitByPublicID(ctx, form.PublicID, submissionInput)
	if err != nil || !replayed.Replayed || replayed.Submission.ID != created.Submission.ID {
		t.Fatalf("mapped submission replay mismatch: result=%#v err=%v", replayed, err)
	}
	revisionInput := inputFromLeadForm(form)
	revisionInput.Description = "Definition revised after acceptance"
	form, err = service.Update(ctx, organizationID, form.ID, ownerID, revisionInput)
	if err != nil || form.Revision != 3 {
		t.Fatalf("revise form after accepted submission: form=%#v err=%v", form, err)
	}
	replayed, err = service.SubmitByPublicID(ctx, form.PublicID, submissionInput)
	if err != nil || !replayed.Replayed || replayed.Submission.ID != created.Submission.ID {
		t.Fatalf("exact replay after form revision mismatch: result=%#v err=%v", replayed, err)
	}
	legacyDigest, err := legacySubmissionRequestDigest(form.ID, submissionInput.Values, "", Attribution{LeadSource: form.SourceLabel}, challenge.ConsentText)
	if err != nil {
		t.Fatalf("create rolling legacy replay digest: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE lead_capture_submission_challenges SET request_digest=$1 WHERE token_digest=$2`, legacyDigest, submissionChallengeDigest(challenge.Token)); err != nil {
		t.Fatalf("seed rolling legacy replay digest: %v", err)
	}
	replayed, err = service.SubmitByPublicID(ctx, form.PublicID, submissionInput)
	if err != nil || !replayed.Replayed || replayed.Submission.ID != created.Submission.ID {
		t.Fatalf("rolling legacy digest replay mismatch: result=%#v err=%v", replayed, err)
	}
	changedReplay := submissionInput
	changedReplay.Values = cloneSubmissionValues(submissionInput.Values)
	changedReplay.Values["notes"] = "Changed after acceptance"
	if _, err := service.SubmitByPublicID(ctx, form.PublicID, changedReplay); !errors.Is(err, ErrChallengeInvalid) {
		t.Fatalf("changed replay after form revision error=%v, want invalid challenge", err)
	}

	var regionValue, budgetValue, startValue, qualifiedValue, notesValue string
	if err := pool.QueryRow(ctx, `
		SELECT custom_fields->>'region', custom_fields->>'budget', custom_fields->>'start_date',
		       custom_fields->>'qualified', custom_fields->>'intake_notes'
		FROM contacts WHERE organization_id=$1 AND id=$2
	`, organizationID, created.Submission.ContactID).Scan(&regionValue, &budgetValue, &startValue, &qualifiedValue, &notesValue); err != nil {
		t.Fatalf("load mapped contact values: %v", err)
	}
	if regionValue != "West" || budgetValue != "1250.50" || startValue != "2026-08-01" || qualifiedValue != "false" || notesValue != "Needs a migration plan" {
		t.Fatalf("unexpected typed contact custom fields: region=%q budget=%q start=%q qualified=%q notes=%q", regionValue, budgetValue, startValue, qualifiedValue, notesValue)
	}
	var storedRevision int
	var mappingSnapshot string
	if err := pool.QueryRow(ctx, `SELECT form_revision, field_mapping_snapshot_json::text FROM lead_capture_submissions WHERE organization_id=$1 AND id=$2`, organizationID, created.Submission.ID).Scan(&storedRevision, &mappingSnapshot); err != nil {
		t.Fatalf("load mapped submission evidence: %v", err)
	}
	for _, expected := range []string{`"destination": "custom:region"`, `"dataType": "select"`, `"destination": "custom:qualified"`, `"dataType": "boolean"`} {
		if !strings.Contains(mappingSnapshot, expected) {
			t.Fatalf("mapping snapshot missing %q: %s", expected, mappingSnapshot)
		}
	}
	for _, privateValue := range []string{"Ada", "West", "1250.50", "Needs a migration plan"} {
		if strings.Contains(mappingSnapshot, privateValue) {
			t.Fatalf("mapping snapshot leaked submitted value %q: %s", privateValue, mappingSnapshot)
		}
	}
	if storedRevision != 2 {
		t.Fatalf("submission form revision=%d, want 2", storedRevision)
	}

	staleInput := inputFromLeadForm(form)
	staleInput.Revision = 1
	if _, err := service.Update(ctx, organizationID, form.ID, ownerID, staleInput); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("stale form update error=%v, want stale revision", err)
	}
	if err := customFields.Archive(ctx, organizationID, ownerID, budget.ID, budget.Revision); !errors.Is(err, modulecustomfields.ErrConflict) {
		t.Fatalf("archive active mapped field error=%v, want conflict", err)
	}
	newRequired := createLeadMappingDefinition(t, ctx, customFields, organizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "contact", FieldKey: "new_required", Label: "New required", DataType: "text",
	})
	if _, err := customFields.Update(ctx, organizationID, ownerID, newRequired.ID, modulecustomfields.UpdateInput{Label: newRequired.Label, Required: true, Position: newRequired.Position, Revision: newRequired.Revision}); !errors.Is(err, modulecustomfields.ErrConflict) {
		t.Fatalf("required field without active-form coverage error=%v, want conflict", err)
	}

	deactivate := inputFromLeadForm(form)
	active := false
	deactivate.IsActive = &active
	deactivated, err := service.Update(ctx, organizationID, form.ID, ownerID, deactivate)
	if err != nil || deactivated.IsActive || deactivated.Revision != 4 {
		t.Fatalf("deactivate mapped form: form=%#v err=%v", deactivated, err)
	}
	if err := customFields.Archive(ctx, organizationID, ownerID, region.ID, region.Revision); err != nil {
		t.Fatalf("archive field mapped only by inactive form: %v", err)
	}
	formPage, err = service.ListByOrganization(ctx, organizationID, FormListQuery{})
	if err != nil || len(formPage.Forms) != 1 || formPage.Forms[0].Revision != 5 {
		t.Fatalf("archived mapping did not advance inactive form: page=%#v err=%v", formPage, err)
	}
	forms = formPage.Forms
	reactivate := inputFromLeadForm(forms[0])
	active = true
	reactivate.IsActive = &active
	if _, err := service.Update(ctx, organizationID, form.ID, ownerID, reactivate); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("reactivate form with archived mapping error=%v, want invalid mapping", err)
	}

	var createAudits, updateAudits, mappingAudits int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE event_type='lead_form.created')::int,
		       COUNT(*) FILTER (WHERE event_type='lead_form.updated')::int,
		       COUNT(*) FILTER (WHERE event_type='lead_form.mapping_revised')::int
		FROM audit_events WHERE organization_id=$1 AND entity_type='lead_capture_form' AND entity_id=$2
	`, organizationID, form.ID).Scan(&createAudits, &updateAudits, &mappingAudits); err != nil {
		t.Fatalf("count lead form definition audits: %v", err)
	}
	if createAudits != 1 || updateAudits != 2 || mappingAudits != 2 {
		t.Fatalf("unexpected lead form audit counts: create=%d update=%d mapping=%d", createAudits, updateAudits, mappingAudits)
	}
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION reject_lead_form_create_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.event_type='lead_form.created' THEN
				RAISE EXCEPTION 'forced lead form audit failure';
			END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER reject_lead_form_create_audit
		BEFORE INSERT ON audit_events
		FOR EACH ROW EXECUTE FUNCTION reject_lead_form_create_audit()
	`); err != nil {
		t.Fatalf("install forced audit failure: %v", err)
	}
	if _, err := service.Create(ctx, organizationID, ownerID, Input{Name: "Audit rollback"}); err == nil {
		t.Fatal("lead form create unexpectedly survived forced audit failure")
	}
	var rolledBackForms int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM lead_capture_forms WHERE organization_id=$1 AND slug='audit-rollback'`, organizationID).Scan(&rolledBackForms); err != nil || rolledBackForms != 0 {
		t.Fatalf("failed lead form audit left a definition: count=%d err=%v", rolledBackForms, err)
	}
	_ = startDate
	_ = qualified
	_ = notes
}

func createLeadMappingDefinition(t *testing.T, ctx context.Context, service *modulecustomfields.Service, organizationID, actorUserID int64, input modulecustomfields.CreateInput) modulecustomfields.Definition {
	t.Helper()
	definition, err := service.Create(ctx, organizationID, actorUserID, input)
	if err != nil {
		t.Fatalf("create contact custom field %q: %v", input.FieldKey, err)
	}
	return definition
}

func inputFromLeadForm(form Form) Input {
	return Input{
		Name: form.Name, Slug: form.Slug, Title: form.Title, Description: form.Description,
		Fields: cloneFields(form.Fields), SuccessMessage: form.SuccessMessage, SourceLabel: form.SourceLabel,
		ConsentText: form.ConsentText, Revision: form.Revision,
	}
}

func cloneSubmissionValues(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
