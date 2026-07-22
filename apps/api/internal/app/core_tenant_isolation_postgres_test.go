package app

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
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	modulesavedviews "github.com/aeml/open_crm/apps/api/internal/modules/savedviews"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCoreRecordTenantBoundariesAgainstPostgres proves the database-backed
// boundary beneath the handler-level cross-org tests. It deliberately attempts
// reads, mutations, relationship injection, actor/assignee injection, notes,
// and saved-view operations across two real tenants, then verifies that every
// rejected transaction left the owning tenant unchanged.
func TestCoreRecordTenantBoundariesAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to core tenant-isolation postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_core_tenant_isolation_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create core tenant-isolation schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := coreTenantIsolationDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate core tenant-isolation schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to core tenant-isolation schema: %v", err)
	}
	defer pool.Close()

	alpha := seedCoreTenant(t, ctx, pool, schema, "alpha")
	beta := seedCoreTenant(t, ctx, pool, schema, "beta")

	contactsService := modulecontacts.NewService(pool)
	companiesService := modulecompanies.NewService(pool)
	dealsService := moduledeals.NewService(pool)
	tasksService := moduletasks.NewService(pool)
	savedViewsService := modulesavedviews.NewService(pool)
	notesService := modulenotes.NewService(pool)

	contactList, err := contactsService.ListByOrganization(ctx, beta.organizationID, modulecontacts.ListQuery{Page: 1, PageSize: 20})
	if err != nil || contactList.Meta.Total != 1 || len(contactList.Contacts) != 1 || contactList.Contacts[0].ID != beta.contactID {
		t.Fatalf("contact list crossed tenant boundary: result=%#v err=%v", contactList, err)
	}
	companyList, err := companiesService.ListByOrganization(ctx, beta.organizationID, modulecompanies.ListQuery{Page: 1, PageSize: 20})
	if err != nil || companyList.Meta.Total != 1 || len(companyList.Companies) != 1 || companyList.Companies[0].ID != beta.companyID {
		t.Fatalf("company list crossed tenant boundary: result=%#v err=%v", companyList, err)
	}
	dealList, err := dealsService.ListByOrganization(ctx, beta.organizationID, moduledeals.ListQuery{Page: 1, PageSize: 20})
	if err != nil || dealList.Meta.Total != 1 || len(dealList.Deals) != 1 || dealList.Deals[0].ID != beta.dealID {
		t.Fatalf("deal list crossed tenant boundary: result=%#v err=%v", dealList, err)
	}
	taskList, err := tasksService.ListByOrganization(ctx, beta.organizationID, moduletasks.ListQuery{Page: 1, PageSize: 20})
	if err != nil || taskList.Meta.Total != 1 || len(taskList.Tasks) != 1 || taskList.Tasks[0].ID != beta.taskID {
		t.Fatalf("task list crossed tenant boundary: result=%#v err=%v", taskList, err)
	}
	viewList, err := savedViewsService.ListByEntity(ctx, beta.organizationID, beta.userID, "contacts", modulesavedviews.ListQuery{})
	if err != nil || len(viewList.Views) != 1 || viewList.Views[0].ID != beta.savedViewID {
		t.Fatalf("saved-view list crossed tenant boundary: result=%#v err=%v", viewList, err)
	}
	noteList, err := notesService.ListByEntity(ctx, beta.organizationID, "contact", beta.contactID, platformtimeline.Query{})
	if err != nil || len(noteList.Notes) != 1 || noteList.Notes[0].ID != beta.noteID {
		t.Fatalf("note list crossed tenant boundary: result=%#v err=%v", noteList, err)
	}

	assertCoreTenantNotFound(t, "get contact", modulecontacts.ErrNotFound, func() error {
		_, err := contactsService.GetByID(ctx, beta.organizationID, alpha.contactID)
		return err
	})
	assertCoreTenantNotFound(t, "update contact", modulecontacts.ErrNotFound, func() error {
		_, err := contactsService.Update(ctx, beta.organizationID, alpha.contactID, beta.userID, modulecontacts.UpdateInput{FirstName: "Denied", LastName: "Contact", Status: "lead"})
		return err
	})
	assertCoreTenantNotFound(t, "archive contact", modulecontacts.ErrNotFound, func() error {
		return contactsService.Archive(ctx, beta.organizationID, alpha.contactID, beta.userID)
	})

	assertCoreTenantNotFound(t, "get company", modulecompanies.ErrNotFound, func() error {
		_, err := companiesService.GetByID(ctx, beta.organizationID, alpha.companyID)
		return err
	})
	assertCoreTenantNotFound(t, "update company", modulecompanies.ErrNotFound, func() error {
		_, err := companiesService.Update(ctx, beta.organizationID, alpha.companyID, beta.userID, modulecompanies.UpdateInput{Name: "Denied Company", ClientType: "organization", Status: "prospect"})
		return err
	})
	assertCoreTenantNotFound(t, "archive company", modulecompanies.ErrNotFound, func() error {
		return companiesService.Archive(ctx, beta.organizationID, alpha.companyID, beta.userID)
	})

	assertCoreTenantNotFound(t, "get deal", moduledeals.ErrNotFound, func() error {
		_, err := dealsService.GetByID(ctx, beta.organizationID, alpha.dealID)
		return err
	})
	assertCoreTenantNotFound(t, "update deal", moduledeals.ErrNotFound, func() error {
		_, err := dealsService.Update(ctx, beta.organizationID, alpha.dealID, beta.userID, moduledeals.UpdateInput{Name: "Denied Deal", OwnerUserID: beta.userID})
		return err
	})
	assertCoreTenantNotFound(t, "move deal stage", moduledeals.ErrNotFound, func() error {
		_, err := dealsService.UpdateStage(ctx, beta.organizationID, alpha.dealID, beta.userID, moduledeals.UpdateStageInput{StageID: beta.stageID})
		return err
	})
	assertCoreTenantNotFound(t, "archive deal", moduledeals.ErrNotFound, func() error {
		return dealsService.Archive(ctx, beta.organizationID, alpha.dealID, beta.userID)
	})

	assertCoreTenantNotFound(t, "get task", moduletasks.ErrNotFound, func() error {
		_, err := tasksService.GetByID(ctx, beta.organizationID, alpha.taskID)
		return err
	})
	assertCoreTenantNotFound(t, "update task", moduletasks.ErrNotFound, func() error {
		_, err := tasksService.Update(ctx, beta.organizationID, alpha.taskID, beta.userID, moduletasks.UpdateInput{Title: "Denied Task", Status: "open", AssignedToUserID: beta.userID})
		return err
	})
	assertCoreTenantNotFound(t, "archive task", moduletasks.ErrNotFound, func() error {
		return tasksService.Archive(ctx, beta.organizationID, alpha.taskID, beta.userID)
	})

	assertCoreTenantNotFound(t, "update saved view", modulesavedviews.ErrNotFound, func() error {
		_, err := savedViewsService.Update(ctx, beta.organizationID, beta.userID, alpha.savedViewID, modulesavedviews.Input{EntityType: "contacts", Name: "Denied View", Filters: map[string]string{}, IsDefault: true, ExpectedRevision: 1})
		return err
	})
	assertCoreTenantNotFound(t, "delete saved view", modulesavedviews.ErrNotFound, func() error {
		return savedViewsService.Delete(ctx, beta.organizationID, beta.userID, alpha.savedViewID, 1)
	})

	foreignNotes, err := notesService.ListByEntity(ctx, beta.organizationID, "contact", alpha.contactID, platformtimeline.Query{})
	if err != nil || len(foreignNotes.Notes) != 0 {
		t.Fatalf("foreign contact notes leaked: notes=%#v err=%v", foreignNotes, err)
	}
	if _, err := notesService.Create(ctx, beta.organizationID, beta.userID, modulenotes.CreateInput{EntityType: "contact", EntityID: alpha.contactID, Body: "cross-tenant note"}); err == nil {
		t.Fatal("created a note against a foreign contact")
	}
	if _, err := notesService.Create(ctx, beta.organizationID, alpha.userID, modulenotes.CreateInput{EntityType: "contact", EntityID: beta.contactID, Body: "foreign actor note"}); err == nil {
		t.Fatal("created a note with a foreign actor")
	}

	assertCoreTenantNotFound(t, "link foreign contact", modulecompanies.ErrNotFound, func() error {
		_, err := companiesService.Update(ctx, beta.organizationID, beta.companyID, beta.userID, modulecompanies.UpdateInput{
			Name: beta.companyName, ClientType: "organization", Status: "prospect", LinkedContactIDs: []int64{beta.contactID, alpha.contactID},
		})
		return err
	})
	assertCoreTenantNotFound(t, "create individual for foreign contact", modulecompanies.ErrNotFound, func() error {
		_, err := companiesService.Create(ctx, beta.organizationID, beta.userID, modulecompanies.CreateInput{
			Name: "Denied Individual", ClientType: "individual", Status: "prospect", LinkedContactIDs: []int64{alpha.contactID},
		})
		return err
	})
	assertCoreTenantNotFound(t, "create person for foreign company", modulecontacts.ErrLinkedCompanyNotFound, func() error {
		_, err := contactsService.CreateLinkedCompanyPerson(ctx, beta.organizationID, alpha.companyID, beta.userID, modulecontacts.CreateInput{FirstName: "Denied", LastName: "Person", Status: "lead"})
		return err
	})
	assertCoreTenantNotFound(t, "attach foreign company to deal", moduledeals.ErrNotFound, func() error {
		_, err := dealsService.Update(ctx, beta.organizationID, beta.dealID, beta.userID, moduledeals.UpdateInput{Name: beta.dealName, CompanyID: alpha.companyID, OwnerUserID: beta.userID})
		return err
	})
	assertCoreTenantNotFound(t, "use foreign stage", moduledeals.ErrNotFound, func() error {
		_, err := dealsService.UpdateStage(ctx, beta.organizationID, beta.dealID, beta.userID, moduledeals.UpdateStageInput{StageID: alpha.stageID})
		return err
	})
	assertCoreTenantNotFound(t, "create task for foreign contact", moduletasks.ErrNotFound, func() error {
		_, err := tasksService.Create(ctx, beta.organizationID, beta.userID, moduletasks.CreateInput{EntityType: "contact", EntityID: alpha.contactID, Title: "Denied foreign task", Status: "open", AssignedToUserID: beta.userID})
		return err
	})
	if _, err := tasksService.Create(ctx, beta.organizationID, beta.userID, moduletasks.CreateInput{EntityType: "contact", EntityID: beta.contactID, Title: "Denied assignee", Status: "open", AssignedToUserID: alpha.userID}); !errors.Is(err, moduleusers.ErrInvalidAssignee) {
		t.Fatalf("foreign task assignee returned %v, want invalid assignee", err)
	}
	if _, err := dealsService.Update(ctx, beta.organizationID, beta.dealID, beta.userID, moduledeals.UpdateInput{Name: beta.dealName, OwnerUserID: alpha.userID}); !errors.Is(err, moduleusers.ErrInvalidAssignee) {
		t.Fatalf("foreign deal owner returned %v, want invalid assignee", err)
	}

	assertCoreTenantUnchanged(t, ctx, pool, alpha)
	assertCoreTenantUnchanged(t, ctx, pool, beta)
	if got := coreTenantCount(t, ctx, pool, `SELECT COUNT(*) FROM contact_company_links WHERE organization_id=$1 AND company_id=$2`, beta.organizationID, beta.companyID); got != 0 {
		t.Fatalf("rejected relationship injection left %d company links", got)
	}
	if got := coreTenantCount(t, ctx, pool, `SELECT COUNT(*) FROM companies WHERE organization_id=$1`, beta.organizationID); got != 1 {
		t.Fatalf("rejected individual-client create changed tenant company count: %d", got)
	}
	if got := coreTenantCount(t, ctx, pool, `SELECT COUNT(*) FROM notes WHERE organization_id=$1`, beta.organizationID); got != 1 {
		t.Fatalf("rejected note writes changed tenant note count: %d", got)
	}
	if got := coreTenantCount(t, ctx, pool, `SELECT COUNT(*) FROM tasks WHERE organization_id=$1`, beta.organizationID); got != 1 {
		t.Fatalf("rejected task writes changed tenant task count: %d", got)
	}
}

type coreTenantFixture struct {
	organizationID int64
	userID         int64
	contactID      int64
	companyID      int64
	dealID         int64
	taskID         int64
	savedViewID    int64
	noteID         int64
	stageID        int64
	contactFirst   string
	companyName    string
	dealName       string
	taskTitle      string
	viewName       string
	noteBody       string
}

func seedCoreTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema, key string) coreTenantFixture {
	t.Helper()
	fixture := coreTenantFixture{
		contactFirst: strings.ToUpper(key[:1]) + key[1:] + " Contact",
		companyName:  strings.ToUpper(key[:1]) + key[1:] + " Client",
		dealName:     strings.ToUpper(key[:1]) + key[1:] + " Opportunity",
		taskTitle:    strings.ToUpper(key[:1]) + key[1:] + " Follow-up",
		viewName:     strings.ToUpper(key[:1]) + key[1:] + " Contacts",
		noteBody:     strings.ToUpper(key[:1]) + key[1:] + " retained note",
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,base_currency) VALUES ($1,$2,'USD') RETURNING id`, fixture.companyName, key+"-"+schema).Scan(&fixture.organizationID); err != nil {
		t.Fatalf("create %s organization: %v", key, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at) VALUES ($1,'hash',$2,'Owner',NOW()) RETURNING id`, key+"-"+schema+"@example.test", strings.ToUpper(key[:1])+key[1:]).Scan(&fixture.userID); err != nil {
		t.Fatalf("create %s user: %v", key, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner','active')`, fixture.organizationID, fixture.userID); err != nil {
		t.Fatalf("create %s membership: %v", key, err)
	}
	var pipelineID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Sales',1,TRUE,$2) RETURNING id`, fixture.organizationID, fixture.userID).Scan(&pipelineID); err != nil {
		t.Fatalf("create %s pipeline: %v", key, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position,is_closed,is_won,probability_percent) VALUES ($1,$2,'Open',1,FALSE,FALSE,25) RETURNING id`, fixture.organizationID, pipelineID).Scan(&fixture.stageID); err != nil {
		t.Fatalf("create %s stage: %v", key, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status,owner_user_id) VALUES ($1,$2,'Person',$3,'lead',$4) RETURNING id`, fixture.organizationID, fixture.contactFirst, key+"-contact-"+schema+"@example.test", fixture.userID).Scan(&fixture.contactID); err != nil {
		t.Fatalf("create %s contact: %v", key, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,client_type,status,owner_user_id) VALUES ($1,$2,'organization','prospect',$3) RETURNING id`, fixture.organizationID, fixture.companyName, fixture.userID).Scan(&fixture.companyID); err != nil {
		t.Fatalf("create %s company: %v", key, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deals (organization_id,stage_id,name,status,value_amount,value_currency,owner_user_id) VALUES ($1,$2,$3,'open',1000,'USD',$4) RETURNING id`, fixture.organizationID, fixture.stageID, fixture.dealName, fixture.userID).Scan(&fixture.dealID); err != nil {
		t.Fatalf("create %s deal: %v", key, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,assigned_to_user_id,created_by_user_id) VALUES ($1,'contact',$2,$3,'open',$4,$4) RETURNING id`, fixture.organizationID, fixture.contactID, fixture.taskTitle, fixture.userID).Scan(&fixture.taskID); err != nil {
		t.Fatalf("create %s task: %v", key, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO saved_views (organization_id,user_id,entity_type,name,filters,is_default) VALUES ($1,$2,'contacts',$3,'{}',TRUE) RETURNING id`, fixture.organizationID, fixture.userID, fixture.viewName).Scan(&fixture.savedViewID); err != nil {
		t.Fatalf("create %s saved view: %v", key, err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO notes (organization_id,entity_type,entity_id,body,created_by_user_id) VALUES ($1,'contact',$2,$3,$4) RETURNING id`, fixture.organizationID, fixture.contactID, fixture.noteBody, fixture.userID).Scan(&fixture.noteID); err != nil {
		t.Fatalf("create %s note: %v", key, err)
	}
	return fixture
}

func assertCoreTenantNotFound(t *testing.T, operation string, want error, run func() error) {
	t.Helper()
	if err := run(); !errors.Is(err, want) {
		t.Fatalf("%s returned %v, want %v", operation, err, want)
	}
}

func assertCoreTenantUnchanged(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fixture coreTenantFixture) {
	t.Helper()
	var contactFirst, companyName, dealName, taskTitle, viewName, noteBody string
	var contactArchived, companyArchived, dealArchived, taskArchived bool
	if err := pool.QueryRow(ctx, `SELECT first_name,archived_at IS NOT NULL FROM contacts WHERE organization_id=$1 AND id=$2`, fixture.organizationID, fixture.contactID).Scan(&contactFirst, &contactArchived); err != nil || contactFirst != fixture.contactFirst || contactArchived {
		t.Fatalf("contact changed after rejected tenant operations: first=%q archived=%v err=%v", contactFirst, contactArchived, err)
	}
	if err := pool.QueryRow(ctx, `SELECT name,archived_at IS NOT NULL FROM companies WHERE organization_id=$1 AND id=$2`, fixture.organizationID, fixture.companyID).Scan(&companyName, &companyArchived); err != nil || companyName != fixture.companyName || companyArchived {
		t.Fatalf("company changed after rejected tenant operations: name=%q archived=%v err=%v", companyName, companyArchived, err)
	}
	if err := pool.QueryRow(ctx, `SELECT name,archived_at IS NOT NULL FROM deals WHERE organization_id=$1 AND id=$2`, fixture.organizationID, fixture.dealID).Scan(&dealName, &dealArchived); err != nil || dealName != fixture.dealName || dealArchived {
		t.Fatalf("deal changed after rejected tenant operations: name=%q archived=%v err=%v", dealName, dealArchived, err)
	}
	if err := pool.QueryRow(ctx, `SELECT title,archived_at IS NOT NULL FROM tasks WHERE organization_id=$1 AND id=$2`, fixture.organizationID, fixture.taskID).Scan(&taskTitle, &taskArchived); err != nil || taskTitle != fixture.taskTitle || taskArchived {
		t.Fatalf("task changed after rejected tenant operations: title=%q archived=%v err=%v", taskTitle, taskArchived, err)
	}
	if err := pool.QueryRow(ctx, `SELECT name FROM saved_views WHERE organization_id=$1 AND id=$2`, fixture.organizationID, fixture.savedViewID).Scan(&viewName); err != nil || viewName != fixture.viewName {
		t.Fatalf("saved view changed after rejected tenant operations: name=%q err=%v", viewName, err)
	}
	if err := pool.QueryRow(ctx, `SELECT body FROM notes WHERE organization_id=$1 AND id=$2`, fixture.organizationID, fixture.noteID).Scan(&noteBody); err != nil || noteBody != fixture.noteBody {
		t.Fatalf("note changed after rejected tenant operations: body=%q err=%v", noteBody, err)
	}
}

func coreTenantCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("count core tenant records: %v", err)
	}
	return count
}

func coreTenantIsolationDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse core tenant-isolation database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
