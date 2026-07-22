package companies

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
)

func TestLinkedContactsAreBoundedSearchableAndTenantScopedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to linked-contact postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_company_link_page_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create linked-contact schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := companyLinkedContactDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate linked-contact schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to linked-contact schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, actorUserID, companyID, foreignCompanyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES ('Linked Page',$1) RETURNING id`, "linked-page-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES ('Foreign Page',$1) RETURNING id`, "foreign-page-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(email,password_hash,first_name,last_name,email_verified_at)
		VALUES ($1,'hash','Linked','Owner',NOW()) RETURNING id
	`, "linked-owner-"+schema+"@example.test").Scan(&actorUserID); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role) VALUES ($1,$2,'owner')`, organizationID, actorUserID); err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies(organization_id,name,client_type,owner_user_id,status) VALUES ($1,'Atlas','organization',$2,'prospect') RETURNING id`, organizationID, actorUserID).Scan(&companyID); err != nil {
		t.Fatalf("create company: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies(organization_id,name,client_type,status) VALUES ($1,'Foreign Atlas','organization','prospect') RETURNING id`, foreignOrganizationID).Scan(&foreignCompanyID); err != nil {
		t.Fatalf("create foreign company: %v", err)
	}

	insertedIDs := make([]int64, 0, 121)
	for index := 0; index < 121; index++ {
		var contactID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO contacts(organization_id,first_name,last_name,email,status)
			VALUES ($1,$2,$3,$4,'lead') RETURNING id
		`, organizationID, fmt.Sprintf("Person %03d", index), fmt.Sprintf("Surname %03d", index), fmt.Sprintf("person-%03d@example.test", index)).Scan(&contactID); err != nil {
			t.Fatalf("create contact %d: %v", index, err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO contact_company_links(organization_id,contact_id,company_id,relationship_title,is_primary)
			VALUES ($1,$2,$3,$4,$5)
		`, organizationID, contactID, companyID, fmt.Sprintf("Role %03d", index), index == 120); err != nil {
			t.Fatalf("link contact %d: %v", index, err)
		}
		insertedIDs = append(insertedIDs, contactID)
	}
	var foreignContactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts(organization_id,first_name,last_name,email,status) VALUES ($1,'Foreign','Person','foreign@example.test','lead') RETURNING id`, foreignOrganizationID).Scan(&foreignContactID); err != nil {
		t.Fatalf("create foreign contact: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contact_company_links(organization_id,contact_id,company_id,is_primary) VALUES ($1,$2,$3,TRUE)`, foreignOrganizationID, foreignContactID, foreignCompanyID); err != nil {
		t.Fatalf("link foreign contact: %v", err)
	}

	service := NewService(pool)
	first, err := service.ListLinkedContacts(ctx, organizationID, companyID, LinkedContactListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list first linked-contact page: %v", err)
	}
	if len(first.LinkedContacts) != 50 || first.Meta.Total != 121 || first.Meta.Page != 1 || first.LinkedContacts[0].ID != insertedIDs[120] || !first.LinkedContacts[0].IsPrimary {
		t.Fatalf("unexpected first linked-contact page: meta=%+v first=%#v count=%d", first.Meta, first.LinkedContacts[0], len(first.LinkedContacts))
	}
	second, err := service.ListLinkedContacts(ctx, organizationID, companyID, LinkedContactListQuery{Page: 2, PageSize: 50})
	if err != nil || len(second.LinkedContacts) != 50 || second.Meta.Total != 121 {
		t.Fatalf("unexpected second linked-contact page: page=%#v err=%v", second, err)
	}
	seen := make(map[int64]bool, len(first.LinkedContacts))
	for _, contact := range first.LinkedContacts {
		seen[contact.ID] = true
	}
	for _, contact := range second.LinkedContacts {
		if seen[contact.ID] {
			t.Fatalf("linked contact %d overlapped adjacent pages", contact.ID)
		}
	}
	last, err := service.ListLinkedContacts(ctx, organizationID, companyID, LinkedContactListQuery{Page: 3, PageSize: 50})
	if err != nil || len(last.LinkedContacts) != 21 || last.Meta.Total != 121 {
		t.Fatalf("unexpected final linked-contact page: page=%#v err=%v", last, err)
	}

	for _, search := range []string{"Person 077", "person-077@example.test", "Role 077"} {
		result, err := service.ListLinkedContacts(ctx, organizationID, companyID, LinkedContactListQuery{Search: search, PageSize: 10})
		if err != nil || result.Meta.Total != 1 || len(result.LinkedContacts) != 1 || result.LinkedContacts[0].ID != insertedIDs[77] {
			t.Fatalf("search %q returned %#v, err=%v", search, result, err)
		}
	}
	if _, err := service.ListLinkedContacts(ctx, organizationID, foreignCompanyID, LinkedContactListQuery{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign company list error=%v, want ErrNotFound", err)
	}
	if _, err := service.ListLinkedContacts(ctx, organizationID, companyID, LinkedContactListQuery{PageSize: 101}); !errors.Is(err, platformpagination.ErrInvalid) {
		t.Fatalf("unsafe linked-contact page error=%v, want pagination.ErrInvalid", err)
	}

	detail, err := service.GetByID(ctx, organizationID, companyID)
	if err != nil || len(detail.LinkedContacts) != 50 || detail.LinkedContactMeta.Total != 121 || detail.LinkedContactMeta.PageSize != 50 {
		t.Fatalf("company detail was not bounded: count=%d meta=%+v err=%v", len(detail.LinkedContacts), detail.LinkedContactMeta, err)
	}
	if _, err := service.Update(ctx, organizationID, companyID, actorUserID, UpdateInput{
		Name: "Atlas Updated", ClientType: "organization", Status: "prospect", LinkedContactIDs: nil,
	}); err != nil {
		t.Fatalf("update without relationship replacement: %v", err)
	}
	var retained int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contact_company_links WHERE organization_id=$1 AND company_id=$2`, organizationID, companyID).Scan(&retained); err != nil || retained != 121 {
		t.Fatalf("ordinary company edit replaced unseen links: retained=%d err=%v", retained, err)
	}

	var candidateID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts(organization_id,first_name,last_name,email,status) VALUES ($1,'Newest','Buyer','newest@example.test','lead') RETURNING id`, organizationID).Scan(&candidateID); err != nil {
		t.Fatalf("create relationship candidate: %v", err)
	}
	linked, err := service.LinkContact(ctx, organizationID, companyID, candidateID, actorUserID, LinkedContactInput{RelationshipTitle: "Decision maker", IsPrimary: true})
	if err != nil || linked.ID != candidateID || linked.RelationshipTitle != "Decision maker" || !linked.IsPrimary {
		t.Fatalf("link existing contact result=%#v err=%v", linked, err)
	}
	var primaryCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contact_company_links WHERE organization_id=$1 AND company_id=$2 AND is_primary`, organizationID, companyID).Scan(&primaryCount); err != nil || primaryCount != 1 {
		t.Fatalf("primary count after relink=%d err=%v", primaryCount, err)
	}
	if _, err := service.LinkContact(ctx, organizationID, companyID, foreignContactID, actorUserID, LinkedContactInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign contact link error=%v, want ErrNotFound", err)
	}
	var primaryChangeActivities int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM activities
		WHERE organization_id=$1 AND entity_type='company' AND entity_id=$2
		  AND action='company.contact_primary_changed'
	`, organizationID, companyID).Scan(&primaryChangeActivities); err != nil || primaryChangeActivities != 1 {
		t.Fatalf("primary change activity count=%d err=%v", primaryChangeActivities, err)
	}
	repeated, err := service.LinkContact(ctx, organizationID, companyID, candidateID, actorUserID, LinkedContactInput{RelationshipTitle: "Decision maker"})
	if err != nil || !repeated.IsPrimary {
		t.Fatalf("idempotent primary relink result=%#v err=%v", repeated, err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM activities
		WHERE organization_id=$1 AND entity_type='company' AND entity_id=$2
		  AND action='company.contact_primary_changed'
	`, organizationID, companyID).Scan(&primaryChangeActivities); err != nil || primaryChangeActivities != 1 {
		t.Fatalf("repeated link added activity: count=%d err=%v", primaryChangeActivities, err)
	}
	if _, err := service.LinkContact(ctx, organizationID, companyID, candidateID, actorUserID, LinkedContactInput{RelationshipTitle: strings.Repeat("x", 201)}); !errors.Is(err, ErrRelationshipTitleLong) {
		t.Fatalf("long relationship title error=%v, want ErrRelationshipTitleLong", err)
	}
	if err := service.UnlinkContact(ctx, organizationID, companyID, candidateID, actorUserID); err != nil {
		t.Fatalf("unlink primary contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM contact_company_links WHERE organization_id=$1 AND company_id=$2 AND is_primary`, organizationID, companyID).Scan(&primaryCount); err != nil || primaryCount != 1 {
		t.Fatalf("replacement primary count=%d err=%v", primaryCount, err)
	}
	if err := service.UnlinkContact(ctx, organizationID, companyID, foreignContactID, actorUserID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign unlink error=%v, want ErrNotFound", err)
	}
	var individualCompanyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies(organization_id,name,client_type,status) VALUES ($1,'Individual','individual','customer') RETURNING id`, organizationID).Scan(&individualCompanyID); err != nil {
		t.Fatalf("create individual company: %v", err)
	}
	if _, err := service.LinkContact(ctx, organizationID, individualCompanyID, candidateID, actorUserID, LinkedContactInput{}); err != nil {
		t.Fatalf("link individual person: %v", err)
	}
	replacement, err := service.LinkContact(ctx, organizationID, individualCompanyID, insertedIDs[0], actorUserID, LinkedContactInput{})
	if err != nil || !replacement.IsPrimary {
		t.Fatalf("replace active individual person=%#v err=%v", replacement, err)
	}
	individualPage, err := service.ListLinkedContacts(ctx, organizationID, individualCompanyID, LinkedContactListQuery{})
	if err != nil || individualPage.Meta.Total != 1 || len(individualPage.LinkedContacts) != 1 || individualPage.LinkedContacts[0].ID != insertedIDs[0] {
		t.Fatalf("active individual replacement page=%#v err=%v", individualPage, err)
	}
	if err := service.UnlinkContact(ctx, organizationID, individualCompanyID, insertedIDs[0], actorUserID); !errors.Is(err, ErrIndividualCompanyLink) {
		t.Fatalf("individual unlink error=%v, want invariant conflict", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE contacts SET archived_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, insertedIDs[0]); err != nil {
		t.Fatalf("archive individual person: %v", err)
	}
	replacement, err = service.LinkContact(ctx, organizationID, individualCompanyID, insertedIDs[1], actorUserID, LinkedContactInput{})
	if err != nil || !replacement.IsPrimary {
		t.Fatalf("replace archived individual person=%#v err=%v", replacement, err)
	}
	replacement, err = service.LinkContact(ctx, organizationID, individualCompanyID, candidateID, actorUserID, LinkedContactInput{})
	if err != nil || !replacement.IsPrimary {
		t.Fatalf("replace second active individual person=%#v err=%v", replacement, err)
	}
	if _, err := service.Update(ctx, organizationID, individualCompanyID, actorUserID, UpdateInput{
		Name: "Individual Updated", ClientType: "individual", Status: "customer", LinkedContactIDs: nil,
	}); err != nil {
		t.Fatalf("update individual with one active person: %v", err)
	}

	var scaleCompanyID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(organization_id,name,client_type,status)
		VALUES ($1,'Scale Atlas','organization','prospect') RETURNING id
	`, organizationID).Scan(&scaleCompanyID); err != nil {
		t.Fatalf("create scale company: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		WITH inserted AS (
			INSERT INTO contacts(organization_id,first_name,last_name,email,status)
			SELECT $1,'Scale ' || value::text,'Person ' || LPAD(value::text,4,'0'),
			       'scale-' || value::text || '@example.test','lead'
			FROM generate_series(1,1000) value
			RETURNING id
		)
		INSERT INTO contact_company_links(organization_id,contact_id,company_id,is_primary)
		SELECT $1,id,$2,ROW_NUMBER() OVER (ORDER BY id)=1 FROM inserted
	`, organizationID, scaleCompanyID); err != nil {
		t.Fatalf("seed scale company links: %v", err)
	}
	started := time.Now()
	scalePage, err := service.ListLinkedContacts(ctx, organizationID, scaleCompanyID, LinkedContactListQuery{Page: 5, PageSize: 100})
	if err != nil || len(scalePage.LinkedContacts) != 100 || scalePage.Meta.Total != 1000 {
		t.Fatalf("scale linked-contact page=%#v err=%v", scalePage.Meta, err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("1000-link page exceeded two-second budget: %s", elapsed)
	}
}

func companyLinkedContactDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
