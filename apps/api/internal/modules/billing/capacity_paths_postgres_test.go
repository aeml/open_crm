package billing_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulearchiveoperations "github.com/aeml/open_crm/apps/api/internal/modules/archiveoperations"
	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	modulebulkoperations "github.com/aeml/open_crm/apps/api/internal/modules/bulkoperations"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleimports "github.com/aeml/open_crm/apps/api/internal/modules/imports"
	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
)

func TestEveryCapacityIncreasingPathUsesHostedReservationsAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to capacity-path postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_capacity_paths_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create capacity-path schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := capacityPathsDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate capacity-path schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to capacity-path schema: %v", err)
	}
	defer pool.Close()

	var organizationID, ownerID, memberID, disabledID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO organizations (name,slug,plan,subscription_status,billing_provider)
		VALUES ('Capacity Paths','capacity-paths','free','active','stripe') RETURNING id
	`).Scan(&organizationID); err != nil {
		t.Fatalf("create capacity-path tenant: %v", err)
	}
	for index, target := range []*int64{&ownerID, &memberID, &disabledID} {
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at)
			VALUES ($1,'hash','Capacity',$2,NOW()) RETURNING id
		`, fmt.Sprintf("capacity-path-%d@example.test", index), fmt.Sprintf("User%d", index)).Scan(target); err != nil {
			t.Fatalf("create capacity-path user %d: %v", index, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES
		  ($1,$2,'owner','active'),($1,$3,'member','active'),($1,$4,'member','disabled')
	`, organizationID, ownerID, memberID, disabledID); err != nil {
		t.Fatalf("create capacity-path memberships: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name)
		SELECT $1,'Seed',index::text FROM generate_series(1,500) index
	`, organizationID); err != nil {
		t.Fatalf("seed full contact capacity: %v", err)
	}
	var archivedContactID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name,archived_at)
		VALUES ($1,'Archived','Contact',NOW()) RETURNING id
	`, organizationID).Scan(&archivedContactID); err != nil {
		t.Fatalf("seed archived contact: %v", err)
	}
	var pipelineID, stageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default) VALUES ($1,'Sales',1,TRUE) RETURNING id`, organizationID).Scan(&pipelineID); err != nil {
		t.Fatalf("create capacity pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position) VALUES ($1,$2,'Open',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create capacity stage: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,owner_user_id)
		SELECT $1,$2,'Seed deal ' || index::text,$3 FROM generate_series(1,250) index
	`, organizationID, stageID, ownerID); err != nil {
		t.Fatalf("seed full deal capacity: %v", err)
	}
	var archivedDealID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,owner_user_id,archived_at)
		VALUES ($1,$2,'Archived deal',$3,NOW()) RETURNING id
	`, organizationID, stageID, ownerID).Scan(&archivedDealID); err != nil {
		t.Fatalf("seed archived deal: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_capture_forms (organization_id,public_id,name,slug,title,fields_json,success_message,is_active)
		VALUES ($1,'lf_capacity_paths','Capacity form','capacity-form','Talk to us',
		'[{
		  "key":"first","label":"First name","fieldType":"text","required":true,"mapTo":"firstName"
		},{
		  "key":"last","label":"Last name","fieldType":"text","required":true,"mapTo":"lastName"
		}]'::jsonb,'Thanks',TRUE)
	`, organizationID); err != nil {
		t.Fatalf("create capacity lead form: %v", err)
	}

	capacity := modulebilling.NewService(pool, modulebilling.NewProvider("stripe"))
	contacts := modulecontacts.NewServiceWithCapacity(pool, capacity)
	deals := moduledeals.NewServiceWithCapacity(pool, capacity)
	users := moduleusers.NewServiceWithCapacity(pool, capacity)
	archive := modulearchiveoperations.NewServiceWithCapacity(pool, capacity)
	imports := moduleimports.NewServiceWithCapacity(pool, capacity)
	forms := moduleleadforms.NewServiceWithCapacity(pool, capacity, true)
	bulk := modulebulkoperations.NewServiceWithCapacity(pool, capacity)
	var companyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,client_type) VALUES ($1,'Capacity Company','organization') RETURNING id`, organizationID).Scan(&companyID); err != nil {
		t.Fatalf("create capacity company: %v", err)
	}

	assertLimitReached(t, "direct contact create", func() error {
		_, err := contacts.Create(ctx, organizationID, ownerID, modulecontacts.CreateInput{FirstName: "New", LastName: "Contact"})
		return err
	})
	assertLimitReached(t, "linked company person create", func() error {
		_, err := contacts.CreateLinkedCompanyPerson(ctx, organizationID, companyID, ownerID, modulecontacts.CreateInput{FirstName: "Linked", LastName: "Person"})
		return err
	})
	assertLimitReached(t, "direct deal create", func() error {
		_, err := deals.Create(ctx, organizationID, ownerID, moduledeals.CreateInput{Name: "New deal", StageID: stageID, OwnerUserID: ownerID})
		return err
	})
	assertLimitReached(t, "user invite", func() error {
		_, err := users.CreateForOrganization(ctx, organizationID, moduleusers.CreateUserInput{Email: "new-seat@example.test", FirstName: "New", LastName: "Seat", Role: "member"})
		return err
	})
	assertLimitReached(t, "user reactivation", func() error {
		_, err := users.SetStatus(ctx, organizationID, disabledID, ownerID, moduleusers.SetStatusInput{Status: moduleusers.MembershipStatusActive})
		return err
	})
	unchangedMember, err := users.SetStatus(ctx, organizationID, memberID, ownerID, moduleusers.SetStatusInput{Status: moduleusers.MembershipStatusActive})
	if err != nil || unchangedMember.Changed || unchangedMember.User.Status != moduleusers.MembershipStatusActive {
		t.Fatalf("already-active seat was treated as new capacity: result=%+v err=%v", unchangedMember, err)
	}
	assertLimitReached(t, "single contact restore", func() error {
		_, err := archive.Restore(ctx, organizationID, ownerID, "contact", archivedContactID)
		return err
	})
	var activeContactID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM contacts WHERE organization_id=$1 AND archived_at IS NULL ORDER BY id LIMIT 1`, organizationID).Scan(&activeContactID); err != nil {
		t.Fatalf("load active contact: %v", err)
	}
	if _, err := archive.Restore(ctx, organizationID, ownerID, "contact", activeContactID); !errors.Is(err, modulearchiveoperations.ErrNotFound) {
		t.Fatalf("already-active contact restore was treated as new capacity: %v", err)
	}
	assertLimitReached(t, "single deal restore", func() error {
		_, err := archive.Restore(ctx, organizationID, ownerID, "deal", archivedDealID)
		return err
	})
	assertLimitReached(t, "public lead capture", func() error {
		_, err := forms.SubmitByPublicID(ctx, "lf_capacity_paths", moduleleadforms.SubmissionInput{Values: map[string]string{"first": "Public", "last": "Lead"}})
		return err
	})
	assertLimitReached(t, "contact import", func() error {
		_, err := imports.Execute(ctx, moduleimports.ExecuteInput{
			OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contacts",
			OriginalName: "capacity.csv", IdempotencyKey: "capacity-import-001",
			Reader: bytes.NewBufferString("first_name,last_name\nImported,Contact\n"),
		})
		return err
	})

	var bulkOperationID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO bulk_operations (
		  organization_id,created_by_user_id,entity_type,action,idempotency_key,
		  request_sha256,target_count,changed_count
		) VALUES ($1,$2,'contact','archive','capacity-bulk-001',repeat('a',64),1,1)
		RETURNING id
	`, organizationID, ownerID).Scan(&bulkOperationID); err != nil {
		t.Fatalf("create capacity bulk operation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bulk_operation_rows (
		  organization_id,bulk_operation_id,entity_id,before_archived_at,applied_entity_updated_at
		) SELECT $1,$2,id,NULL,updated_at FROM contacts WHERE organization_id=$1 AND id=$3
	`, organizationID, bulkOperationID, archivedContactID); err != nil {
		t.Fatalf("create capacity bulk row: %v", err)
	}
	assertLimitReached(t, "bulk contact restore", func() error {
		_, err := bulk.Rollback(ctx, organizationID, ownerID, bulkOperationID)
		return err
	})
	if _, err := archive.Restore(ctx, organizationID, disabledID, "contact", archivedContactID); !errors.Is(err, modulearchiveoperations.ErrInactiveActor) {
		t.Fatalf("inactive archive actor saw capacity policy before authorization: %v", err)
	}
	if _, err := bulk.Rollback(ctx, organizationID, disabledID, bulkOperationID); !errors.Is(err, modulebulkoperations.ErrInactiveActor) {
		t.Fatalf("inactive bulk actor saw capacity policy before authorization: %v", err)
	}
	if _, err := deals.Create(ctx, organizationID, disabledID, moduledeals.CreateInput{Name: "Forbidden deal", StageID: stageID, OwnerUserID: ownerID}); !errors.Is(err, moduledeals.ErrInvalidAssignee) {
		t.Fatalf("inactive deal actor saw capacity policy before authorization: %v", err)
	}

	selfHosted := modulecontacts.NewServiceWithCapacity(pool, modulebilling.NewService(pool, modulebilling.FakeProvider{}))
	if _, err := selfHosted.Create(ctx, organizationID, ownerID, modulecontacts.CreateInput{FirstName: "Self", LastName: "Hosted"}); err != nil {
		t.Fatalf("self-hosted capacity was restricted: %v", err)
	}
}

func assertLimitReached(t *testing.T, path string, run func() error) {
	t.Helper()
	if err := run(); !errors.Is(err, modulebilling.ErrLimitReached) {
		t.Fatalf("%s returned %v", path, err)
	}
}

func capacityPathsDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse capacity-path database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
