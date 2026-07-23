package customfields_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	moduleduplicates "github.com/aeml/open_crm/apps/api/internal/modules/duplicateoperations"
	moduleexports "github.com/aeml/open_crm/apps/api/internal/modules/exports"
	moduleimports "github.com/aeml/open_crm/apps/api/internal/modules/imports"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

func TestCustomFieldsEndToEndAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to custom fields test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_custom_fields_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create custom fields schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := customFieldsDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate custom fields schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated custom fields schema: %v", err)
	}
	defer pool.Close()

	organizationID := insertCustomFieldsOrganization(t, ctx, pool, "Custom fields", "custom-fields-"+schema)
	foreignOrganizationID := insertCustomFieldsOrganization(t, ctx, pool, "Foreign fields", "foreign-fields-"+schema)
	ownerID := insertCustomFieldsUser(t, ctx, pool, "owner-"+schema+"@example.test")
	disabledID := insertCustomFieldsUser(t, ctx, pool, "disabled-"+schema+"@example.test")
	foreignOwnerID := insertCustomFieldsUser(t, ctx, pool, "foreign-"+schema+"@example.test")
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		status         string
	}{
		{organizationID, ownerID, "active"},
		{organizationID, disabledID, "disabled"},
		{foreignOrganizationID, foreignOwnerID, "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner',$3)`, membership.organizationID, membership.userID, membership.status); err != nil {
			t.Fatalf("create custom fields membership: %v", err)
		}
	}

	fieldsService := modulecustomfields.NewService(pool)
	contactsService := modulecontacts.NewService(pool)
	companiesService := modulecompanies.NewService(pool)
	exportsService := moduleexports.NewService(pool)
	importsService := moduleimports.NewService(pool)
	duplicatesService := moduleduplicates.NewService(pool)

	region, err := fieldsService.Create(ctx, organizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "contact", Label: "Region", DataType: "select", Options: []string{"West", "East"}, Required: true, ShowInList: true,
	})
	if err != nil || region.FieldKey != "region" {
		t.Fatalf("create region definition: definition=%#v err=%v", region, err)
	}
	annualValue, err := fieldsService.Create(ctx, organizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "contact", Label: "Annual value", DataType: "number", ShowInList: true, Position: 2,
	})
	if err != nil || annualValue.FieldKey != "annual_value" {
		t.Fatalf("create annual value definition: definition=%#v err=%v", annualValue, err)
	}
	serviceTier, err := fieldsService.Create(ctx, organizationID, ownerID, modulecustomfields.CreateInput{
		EntityType: "company", Label: "Service tier", DataType: "select", Options: []string{"Gold", "Silver"}, Required: true, ShowInList: true,
	})
	if err != nil {
		t.Fatalf("create service tier definition: %v", err)
	}
	if _, err := fieldsService.Create(ctx, organizationID, disabledID, modulecustomfields.CreateInput{EntityType: "contact", Label: "Disabled field", DataType: "text"}); !errors.Is(err, modulecustomfields.ErrInactiveActor) {
		t.Fatalf("expected disabled custom field actor rejection, got %v", err)
	}
	if _, err := fieldsService.Create(ctx, organizationID, ownerID, modulecustomfields.CreateInput{EntityType: "contact", Label: "Broken options", DataType: "select", Options: []string{strings.Repeat("x", 101)}}); !errors.Is(err, modulecustomfields.ErrInvalidInput) {
		t.Fatalf("expected oversized select option rejection, got %v", err)
	}

	if _, err := contactsService.Create(ctx, organizationID, ownerID, modulecontacts.CreateInput{FirstName: "Missing", LastName: "Region"}); !errors.Is(err, modulecustomfields.ErrInvalidInput) {
		t.Fatalf("expected required custom field rejection, got %v", err)
	}
	west, err := contactsService.Create(ctx, organizationID, ownerID, modulecontacts.CreateInput{
		FirstName: "Willa", LastName: "West", Email: "willa-" + schema + "@example.test",
		CustomFields: customValues(map[string]string{"region": `"West"`, "annual_value": `12500.50`}),
	})
	if err != nil {
		t.Fatalf("create west contact with custom values: %v", err)
	}
	_, err = contactsService.Create(ctx, organizationID, ownerID, modulecontacts.CreateInput{
		FirstName: "Eli", LastName: "East", Email: "eli-" + schema + "@example.test",
		CustomFields: customValues(map[string]string{"region": `"East"`, "annual_value": `500`}),
	})
	if err != nil {
		t.Fatalf("create east contact with custom values: %v", err)
	}
	if string(west.Summary.CustomFields["region"]) != `"West"` || string(west.Summary.CustomFields["annual_value"]) != `12500.50` {
		t.Fatalf("custom values did not round-trip: %#v", west.Summary.CustomFields)
	}
	if _, err := contactsService.Create(ctx, organizationID, ownerID, modulecontacts.CreateInput{
		FirstName: "Wrong", LastName: "Type", CustomFields: customValues(map[string]string{"region": `"West"`, "annual_value": `"large"`}),
	}); !errors.Is(err, modulecustomfields.ErrInvalidInput) {
		t.Fatalf("expected typed custom value rejection, got %v", err)
	}

	westList, err := contactsService.ListByOrganization(ctx, organizationID, modulecontacts.ListQuery{CustomField: modulecustomfields.Filter{FieldKey: "region", Operator: "eq", Value: "West"}})
	if err != nil || westList.Meta.Total != 1 || westList.Contacts[0].ID != west.Summary.ID {
		t.Fatalf("filter contacts by custom select: list=%#v err=%v", westList, err)
	}
	highValueList, err := contactsService.ListByOrganization(ctx, organizationID, modulecontacts.ListQuery{CustomField: modulecustomfields.Filter{FieldKey: "annual_value", Operator: "gte", Value: "10000"}})
	if err != nil || highValueList.Meta.Total != 1 || highValueList.Contacts[0].ID != west.Summary.ID {
		t.Fatalf("filter contacts by custom number: list=%#v err=%v", highValueList, err)
	}
	if _, err := contactsService.ListByOrganization(ctx, organizationID, modulecontacts.ListQuery{CustomField: modulecustomfields.Filter{FieldKey: "region", Operator: "gte", Value: "West"}}); !errors.Is(err, modulecustomfields.ErrInvalidInput) {
		t.Fatalf("expected unsupported custom filter rejection, got %v", err)
	}

	company, err := companiesService.Create(ctx, organizationID, ownerID, modulecompanies.CreateInput{
		Name: "Gold Client " + schema, ClientType: "organization", Status: "prospect", CustomFields: customValues(map[string]string{"service_tier": `"Gold"`}),
	})
	if err != nil || string(company.Summary.CustomFields["service_tier"]) != `"Gold"` {
		t.Fatalf("create organization client with custom field: company=%#v err=%v", company, err)
	}
	companyList, err := companiesService.ListByOrganization(ctx, organizationID, modulecompanies.ListQuery{CustomField: modulecustomfields.Filter{FieldKey: serviceTier.FieldKey, Operator: "eq", Value: "Gold"}})
	if err != nil || companyList.Meta.Total != 1 || companyList.Companies[0].ID != company.Summary.ID {
		t.Fatalf("filter organization clients by custom field: list=%#v err=%v", companyList, err)
	}

	exported, err := exportsService.ContactsCSV(ctx, organizationID, moduleexports.ContactsQuery{CustomField: modulecustomfields.Filter{FieldKey: "region", Operator: "eq", Value: "West"}})
	if err != nil {
		t.Fatalf("export filtered custom fields: %v", err)
	}
	exportedText := string(exported.Content)
	if !strings.Contains(exportedText, "custom:region") || !strings.Contains(exportedText, "custom:annual_value") || !strings.Contains(exportedText, "Willa") || strings.Contains(exportedText, "Eli") {
		t.Fatalf("unexpected custom field export:\n%s", exportedText)
	}
	companyExport, err := exportsService.CompaniesCSV(ctx, organizationID, moduleexports.CompaniesQuery{})
	if err != nil || !strings.Contains(string(companyExport.Content), "custom:service_tier") || !strings.Contains(string(companyExport.Content), "Gold") {
		t.Fatalf("unexpected company custom field export: err=%v csv=%s", err, companyExport.Content)
	}

	csvData := "First,Last,Region,Annual Value\nIvy,Import,West,7000.25\n"
	mapping := map[string]string{"first_name": "First", "last_name": "Last", "custom:region": "Region", "custom:annual_value": "Annual Value"}
	preview, err := importsService.Preview(ctx, moduleimports.PreviewInput{OrganizationID: organizationID, EntityType: "contacts", Reader: strings.NewReader(csvData), Mapping: mapping})
	if err != nil || preview.Summary.ValidRows != 1 || preview.Rows[0].Values["custom:annual_value"] != "7000.25" {
		t.Fatalf("preview custom field import: preview=%#v err=%v", preview, err)
	}
	batch, err := importsService.Execute(ctx, moduleimports.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contacts", OriginalName: "custom-fields.csv", IdempotencyKey: "custom-fields-import-001", Reader: strings.NewReader(csvData), Mapping: mapping,
	})
	if err != nil || batch.Status != "processing" {
		t.Fatalf("queue custom field import: batch=%#v err=%v", batch, err)
	}
	importQueue := modulejobs.NewService(pool)
	importWorker := modulejobs.NewWorker(importQueue, map[string]modulejobs.Handler{moduleimports.JobType: importsService.HandleJob}, "custom-fields-import-test", nil)
	if summary, runErr := importWorker.RunOnce(ctx); runErr != nil || summary.Succeeded != 1 {
		t.Fatalf("execute custom field import worker: summary=%#v err=%v", summary, runErr)
	}
	var importedRegion, importedAnnual string
	if err := pool.QueryRow(ctx, `SELECT custom_fields->>'region',custom_fields->>'annual_value' FROM contacts WHERE organization_id=$1 AND first_name='Ivy' AND last_name='Import'`, organizationID).Scan(&importedRegion, &importedAnnual); err != nil || importedRegion != "West" || importedAnnual != "7000.25" {
		t.Fatalf("custom fields were not imported: region=%q annual=%q err=%v", importedRegion, importedAnnual, err)
	}
	companyCSV := "Account Name,Service Tier\nImported Company,Silver\n"
	companyMapping := map[string]string{"name": "Account Name", "custom:service_tier": "Service Tier"}
	companyPreview, err := importsService.Preview(ctx, moduleimports.PreviewInput{OrganizationID: organizationID, EntityType: "companies", Reader: strings.NewReader(companyCSV), Mapping: companyMapping})
	if err != nil || companyPreview.Summary.ValidRows != 1 {
		t.Fatalf("preview company custom field import: preview=%#v err=%v", companyPreview, err)
	}
	companyBatch, err := importsService.Execute(ctx, moduleimports.ExecuteInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "companies", OriginalName: "company-custom-fields.csv", IdempotencyKey: "company-custom-fields-import-001", Reader: strings.NewReader(companyCSV), Mapping: companyMapping,
	})
	if err != nil || companyBatch.Status != "processing" {
		t.Fatalf("queue company custom field import: batch=%#v err=%v", companyBatch, err)
	}
	if summary, runErr := importWorker.RunOnce(ctx); runErr != nil || summary.Succeeded != 1 {
		t.Fatalf("execute company custom field import worker: summary=%#v err=%v", summary, runErr)
	}
	var importedServiceTier string
	if err := pool.QueryRow(ctx, `SELECT custom_fields->>'service_tier' FROM companies WHERE organization_id=$1 AND name='Imported Company'`, organizationID).Scan(&importedServiceTier); err != nil || importedServiceTier != "Silver" {
		t.Fatalf("company custom field was not imported: serviceTier=%q err=%v", importedServiceTier, err)
	}

	if _, err := fieldsService.Update(ctx, organizationID, ownerID, region.ID, modulecustomfields.UpdateInput{Label: "Region", Options: []string{"East"}, Revision: region.Revision}); !errors.Is(err, modulecustomfields.ErrConflict) {
		t.Fatalf("expected used select option removal conflict, got %v", err)
	}
	testCustomFieldMerge(t, ctx, pool, duplicatesService, organizationID, ownerID)

	if err := fieldsService.Archive(ctx, organizationID, ownerID, annualValue.ID, annualValue.Revision); err != nil {
		t.Fatalf("archive custom field definition: %v", err)
	}
	definitions, err := fieldsService.List(ctx, organizationID, "contact", false)
	if err != nil || len(definitions) != 1 || definitions[0].FieldKey != "region" {
		t.Fatalf("archived definition remained active: definitions=%#v err=%v", definitions, err)
	}
	retained, err := contactsService.GetByID(ctx, organizationID, west.Summary.ID)
	if err != nil || string(retained.Summary.CustomFields["annual_value"]) != `12500.50` {
		t.Fatalf("archived custom value was not retained: detail=%#v err=%v", retained, err)
	}
	afterArchiveExport, err := exportsService.ContactsCSV(ctx, organizationID, moduleexports.ContactsQuery{})
	if err != nil || strings.Contains(string(afterArchiveExport.Content), "custom:annual_value") || !strings.Contains(string(afterArchiveExport.Content), "custom:region") {
		t.Fatalf("archived field export policy failed: err=%v csv=%s", err, afterArchiveExport.Content)
	}

	foreignRegion, err := fieldsService.Create(ctx, foreignOrganizationID, foreignOwnerID, modulecustomfields.CreateInput{EntityType: "contact", Label: "Region", DataType: "text"})
	if err != nil {
		t.Fatalf("create foreign tenant definition: %v", err)
	}
	foreignDefinitions, err := fieldsService.List(ctx, foreignOrganizationID, "contact", false)
	if err != nil || len(foreignDefinitions) != 1 || foreignDefinitions[0].ID != foreignRegion.ID {
		t.Fatalf("foreign tenant definition isolation failed: definitions=%#v err=%v", foreignDefinitions, err)
	}
	if err := fieldsService.Archive(ctx, organizationID, ownerID, foreignRegion.ID, foreignRegion.Revision); !errors.Is(err, modulecustomfields.ErrNotFound) {
		t.Fatalf("expected cross-tenant definition rejection, got %v", err)
	}
	foreignContactList, err := contactsService.ListByOrganization(ctx, foreignOrganizationID, modulecontacts.ListQuery{})
	if err != nil || foreignContactList.Meta.Total != 0 {
		t.Fatalf("foreign tenant saw contacts: list=%#v err=%v", foreignContactList, err)
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE organization_id=$1 AND entity_type='custom_field'`, organizationID).Scan(&auditCount); err != nil || auditCount != 4 {
		t.Fatalf("unexpected custom field audit count: got=%d want=4 err=%v", auditCount, err)
	}
}

func testCustomFieldMerge(t *testing.T, ctx context.Context, pool *moduledb.Pool, service *moduleduplicates.Service, organizationID, ownerID int64) {
	t.Helper()
	var sourceID, targetID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,phone,status,owner_user_id,custom_fields) VALUES ($1,'Merge','Source','merge-source@example.test','+1 313 555 0199','lead',$2,'{"region":"West","annual_value":1}'::jsonb) RETURNING id`, organizationID, ownerID).Scan(&sourceID); err != nil {
		t.Fatalf("create custom field merge source: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,phone,status,owner_user_id,custom_fields) VALUES ($1,'Merge','Target','merge-target@example.test','+1 313 555 0199','lead',$2,'{"region":"East","annual_value":2}'::jsonb) RETURNING id`, organizationID, ownerID).Scan(&targetID); err != nil {
		t.Fatalf("create custom field merge target: %v", err)
	}
	review, err := service.Review(ctx, organizationID, "contact", 50)
	if err != nil {
		t.Fatalf("review custom field duplicates: %v", err)
	}
	var source, target moduleduplicates.Record
	for _, candidate := range review.Candidates {
		if candidate.First.ID == sourceID && candidate.Second.ID == targetID {
			source, target = candidate.First, candidate.Second
		} else if candidate.First.ID == targetID && candidate.Second.ID == sourceID {
			source, target = candidate.Second, candidate.First
		}
	}
	if source.ID == 0 || !recordHasField(source, "custom:region", "West") || !recordHasField(target, "custom:annual_value", "2") {
		t.Fatalf("custom fields missing from duplicate review: source=%#v target=%#v", source, target)
	}
	if _, err := service.Merge(ctx, moduleduplicates.MergeInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contact", SourceEntityID: sourceID, TargetEntityID: targetID,
		SourceFields: []string{"custom:region"}, SourceUpdatedAt: source.UpdatedAt, TargetUpdatedAt: target.UpdatedAt, IdempotencyKey: "custom-field-merge-001",
	}); err != nil {
		t.Fatalf("merge selected custom field: %v", err)
	}
	var region, annualValue string
	if err := pool.QueryRow(ctx, `SELECT custom_fields->>'region',custom_fields->>'annual_value' FROM contacts WHERE organization_id=$1 AND id=$2`, organizationID, targetID).Scan(&region, &annualValue); err != nil || region != "West" || annualValue != "2" {
		t.Fatalf("custom field merge resolution failed: region=%q annual=%q err=%v", region, annualValue, err)
	}
}

func recordHasField(record moduleduplicates.Record, key, value string) bool {
	for _, field := range record.Fields {
		if field.Key == key && field.Value == value {
			return true
		}
	}
	return false
}

func customValues(values map[string]string) modulecustomfields.Values {
	result := modulecustomfields.Values{}
	for key, value := range values {
		result[key] = json.RawMessage(value)
	}
	return result
}

func insertCustomFieldsOrganization(t *testing.T, ctx context.Context, pool *moduledb.Pool, name, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ($1,$2) RETURNING id`, name, slug).Scan(&id); err != nil {
		t.Fatalf("create custom fields organization: %v", err)
	}
	return id
}

func insertCustomFieldsUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'test-hash','Custom','Owner') RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("create custom fields user: %v", err)
	}
	return id
}

func customFieldsDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse custom fields database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
