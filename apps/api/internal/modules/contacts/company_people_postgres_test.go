package contacts

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
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateLinkedCompanyPersonIsAtomicAndTenantScopedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to company-people postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_company_people_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create company-people schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := companyPeopleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate company-people schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to company-people schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Acme','company-people-acme') RETURNING id`).Scan(&organizationID); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign','company-people-foreign') RETURNING id`).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign tenant: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at)
		VALUES ('company-people-owner@example.test','hash','Company','Owner',NOW()) RETURNING id
	`).Scan(&actorUserID); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'owner')`, organizationID, actorUserID); err != nil {
		t.Fatalf("create actor membership: %v", err)
	}
	var companyID, individualCompanyID, foreignCompanyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,client_type) VALUES ($1,'Atlas','organization') RETURNING id`, organizationID).Scan(&companyID); err != nil {
		t.Fatalf("create company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,client_type) VALUES ($1,'Solo','individual') RETURNING id`, organizationID).Scan(&individualCompanyID); err != nil {
		t.Fatalf("create individual company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,client_type) VALUES ($1,'Foreign Atlas','organization') RETURNING id`, foreignOrganizationID).Scan(&foreignCompanyID); err != nil {
		t.Fatalf("create foreign company: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_field_definitions (organization_id,created_by_user_id,entity_type,field_key,label,data_type,is_required)
		VALUES ($1,$2,'contact','region','Region','text',TRUE)
	`, organizationID, actorUserID); err != nil {
		t.Fatalf("create contact custom field: %v", err)
	}

	service := NewService(pool)
	first, err := service.CreateLinkedCompanyPerson(ctx, organizationID, companyID, actorUserID, CreateInput{
		FirstName: " Riley ", LastName: " Chen ", Email: " RILEY@ATLAS.TEST ", JobTitle: "Procurement Lead",
		Status: "prospect", IsClient: true, CustomFields: modulecustomfields.Values{"region": json.RawMessage(`"West"`)},
	})
	if err != nil {
		t.Fatalf("create first linked person: %v", err)
	}
	if first.Contact.ID <= 0 || first.Contact.Email != "riley@atlas.test" || first.Contact.IsClient || !first.Link.IsPrimary || first.Link.RelationshipTitle != "Procurement Lead" || first.Activity.ID <= 0 || first.Activity.Summary != "Contact linked: Riley Chen" {
		t.Fatalf("unexpected first linked-person result: %#v", first)
	}
	if string(first.Contact.CustomFields["region"]) != `"West"` {
		t.Fatalf("custom field did not round trip: %#v", first.Contact.CustomFields)
	}

	second, err := service.CreateLinkedCompanyPerson(ctx, organizationID, companyID, actorUserID, CreateInput{FirstName: "Alex", LastName: "Kim", Status: "lead", CustomFields: modulecustomfields.Values{"region": json.RawMessage(`"East"`)}})
	if err != nil {
		t.Fatalf("create second linked person: %v", err)
	}
	if second.Link.IsPrimary {
		t.Fatal("second visible linked person unexpectedly became primary")
	}
	assertCompanyPeopleCounts(t, ctx, pool, organizationID, companyID, 2, 2, 2)
	if _, err := pool.Exec(ctx, `UPDATE contacts SET archived_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, first.Contact.ID); err != nil {
		t.Fatalf("archive primary contact: %v", err)
	}
	third, err := service.CreateLinkedCompanyPerson(ctx, organizationID, companyID, actorUserID, CreateInput{FirstName: "Jamie", LastName: "Park", Status: "lead", CustomFields: modulecustomfields.Values{"region": json.RawMessage(`"North"`)}})
	if err != nil {
		t.Fatalf("replace archived primary person: %v", err)
	}
	if !third.Link.IsPrimary {
		t.Fatal("first visible person after primary archive did not become primary")
	}
	if got := countCompanyPeopleRows(t, ctx, pool, `SELECT COUNT(*) FROM contact_company_links WHERE organization_id=$1 AND company_id=$2 AND is_primary`, organizationID, companyID); got != 1 {
		t.Fatalf("primary company-link count=%d, want 1", got)
	}

	beforeContacts := countCompanyPeopleRows(t, ctx, pool, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1`, organizationID)
	if _, err := service.CreateLinkedCompanyPerson(ctx, organizationID, foreignCompanyID, actorUserID, CreateInput{FirstName: "Cross", LastName: "Tenant", CustomFields: modulecustomfields.Values{"region": json.RawMessage(`"West"`)}}); !errors.Is(err, ErrLinkedCompanyNotFound) {
		t.Fatalf("cross-tenant company returned %v", err)
	}
	if _, err := service.CreateLinkedCompanyPerson(ctx, organizationID, individualCompanyID, actorUserID, CreateInput{FirstName: "Extra", LastName: "Person", CustomFields: modulecustomfields.Values{"region": json.RawMessage(`"West"`)}}); !errors.Is(err, ErrIndividualCompany) {
		t.Fatalf("individual company returned %v", err)
	}
	if got := countCompanyPeopleRows(t, ctx, pool, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1`, organizationID); got != beforeContacts {
		t.Fatalf("rejected company writes changed contacts: before=%d after=%d", beforeContacts, got)
	}

	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION reject_company_people_link() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'forced linked-person failure'; END $$;
		CREATE TRIGGER reject_company_people_link BEFORE INSERT ON contact_company_links
		FOR EACH ROW EXECUTE FUNCTION reject_company_people_link();
	`); err != nil {
		t.Fatalf("install rollback trigger: %v", err)
	}
	if _, err := service.CreateLinkedCompanyPerson(ctx, organizationID, companyID, actorUserID, CreateInput{FirstName: "Rollback", LastName: "Proof", Email: "rollback@example.test", CustomFields: modulecustomfields.Values{"region": json.RawMessage(`"West"`)}}); err == nil || !strings.Contains(err.Error(), "forced linked-person failure") {
		t.Fatalf("expected forced link failure, got %v", err)
	}
	if got := countCompanyPeopleRows(t, ctx, pool, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1 AND email='rollback@example.test'`, organizationID); got != 0 {
		t.Fatalf("failed link left %d orphan contacts", got)
	}
	if got := countCompanyPeopleRows(t, ctx, pool, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND summary='Contact linked: Rollback Proof'`, organizationID); got != 0 {
		t.Fatalf("failed link left %d company activities", got)
	}
	assertCompanyPeopleCounts(t, ctx, pool, organizationID, companyID, 2, 3, 3)
}

func assertCompanyPeopleCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, companyID int64, contacts, links, companyActivities int) {
	t.Helper()
	if got := countCompanyPeopleRows(t, ctx, pool, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1 AND archived_at IS NULL`, organizationID); got != contacts {
		t.Fatalf("contact count=%d, want %d", got, contacts)
	}
	if got := countCompanyPeopleRows(t, ctx, pool, `SELECT COUNT(*) FROM contact_company_links WHERE organization_id=$1 AND company_id=$2`, organizationID, companyID); got != links {
		t.Fatalf("link count=%d, want %d", got, links)
	}
	if got := countCompanyPeopleRows(t, ctx, pool, `SELECT COUNT(*) FROM activities WHERE organization_id=$1 AND entity_type='company' AND entity_id=$2 AND action='company.contact_linked'`, organizationID, companyID); got != companyActivities {
		t.Fatalf("company activity count=%d, want %d", got, companyActivities)
	}
}

func countCompanyPeopleRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("count company-people rows: %v", err)
	}
	return count
}

func companyPeopleDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse company-people database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
