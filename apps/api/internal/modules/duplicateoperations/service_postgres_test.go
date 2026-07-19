package duplicateoperations_test

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
	moduleclientreviews "github.com/aeml/open_crm/apps/api/internal/modules/clientreviews"
	moduleduplicates "github.com/aeml/open_crm/apps/api/internal/modules/duplicateoperations"
)

func TestDuplicateReviewAndMergePreserveRelationshipsAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to duplicate operations test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_duplicate_operations_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create duplicate operations schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := duplicateDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate duplicate operations schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated duplicate operations schema: %v", err)
	}
	defer pool.Close()

	organizationID := duplicateInsertOrganization(t, ctx, pool, "Duplicate operations", "duplicates-"+schema)
	foreignOrganizationID := duplicateInsertOrganization(t, ctx, pool, "Foreign duplicates", "foreign-duplicates-"+schema)
	ownerID := duplicateInsertUser(t, ctx, pool, "owner-"+schema+"@example.test", "Merge", "Owner")
	disabledID := duplicateInsertUser(t, ctx, pool, "disabled-"+schema+"@example.test", "Disabled", "Admin")
	foreignID := duplicateInsertUser(t, ctx, pool, "foreign-"+schema+"@example.test", "Foreign", "Owner")
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		role           string
		status         string
	}{
		{organizationID, ownerID, "owner", "active"},
		{organizationID, disabledID, "admin", "disabled"},
		{foreignOrganizationID, foreignID, "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,$3,$4)`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("create duplicate operations membership: %v", err)
		}
	}

	var sourceContactID, targetContactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,phone,job_title,status,owner_user_id,is_client) VALUES ($1,'Ava','Duplicate','ava@example.test','+1 202 555 0199','Decision maker','lead',$2,TRUE) RETURNING id`, organizationID, ownerID).Scan(&sourceContactID); err != nil {
		t.Fatalf("create source contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status,owner_user_id) VALUES ($1,'Ava','Primary','AVA@example.test','prospect',$2) RETURNING id`, organizationID, ownerID).Scan(&targetContactID); err != nil {
		t.Fatalf("create target contact: %v", err)
	}
	foreignContactID := duplicateInsertContact(t, ctx, pool, foreignOrganizationID, foreignID, "Ava", "Foreign", "ava@example.test")
	companyID := duplicateInsertCompany(t, ctx, pool, organizationID, ownerID, "Linked Client", "linked.example.test", "organization")
	if _, err := pool.Exec(ctx, `INSERT INTO contact_company_links (organization_id,contact_id,company_id,relationship_title,is_primary) VALUES ($1,$2,$3,'Buyer',TRUE),($1,$4,$3,'Existing',FALSE)`, organizationID, sourceContactID, companyID, targetContactID); err != nil {
		t.Fatalf("create overlapping contact/client links: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notes (organization_id,entity_type,entity_id,body,created_by_user_id) VALUES ($1,'contact',$2,'Preserve this note',$3)`, organizationID, sourceContactID, ownerID); err != nil {
		t.Fatalf("create source note: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tasks (organization_id,entity_type,entity_id,title,status,created_by_user_id) VALUES ($1,'contact',$2,'Preserve this task','open',$3)`, organizationID, sourceContactID, ownerID); err != nil {
		t.Fatalf("create source task: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary) VALUES ($1,'contact',$2,$3,'contact.test','Historical activity')`, organizationID, sourceContactID, ownerID); err != nil {
		t.Fatalf("create source activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO record_followers (organization_id,entity_type,entity_id,user_id,created_by_user_id) VALUES ($1,'contact',$2,$3,$3),($1,'contact',$4,$3,$3)`, organizationID, sourceContactID, ownerID, targetContactID); err != nil {
		t.Fatalf("create overlapping followers: %v", err)
	}
	var messageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO email_messages (organization_id,to_email,subject,body,status,entity_type,entity_id) VALUES ($1,'ava@example.test','History','Body','sent','contact',$2) RETURNING id`, organizationID, sourceContactID).Scan(&messageID); err != nil {
		t.Fatalf("create source email: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_message_entity_links (organization_id,email_message_id,entity_type,entity_id) VALUES ($1,$2,'contact',$3),($1,$2,'contact',$4)`, organizationID, messageID, sourceContactID, targetContactID); err != nil {
		t.Fatalf("create overlapping email links: %v", err)
	}
	var sequenceID int64
	if err := pool.QueryRow(ctx, `INSERT INTO email_sequences (organization_id,name,status,created_by_user_id) VALUES ($1,'Merge sequence','active',$2) RETURNING id`, organizationID, ownerID).Scan(&sequenceID); err != nil {
		t.Fatalf("create merge sequence: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_sequence_enrollments (organization_id,sequence_id,contact_id,status,enrolled_by_user_id) VALUES ($1,$2,$3,'active',$5),($1,$2,$4,'active',$5)`, organizationID, sequenceID, sourceContactID, targetContactID, ownerID); err != nil {
		t.Fatalf("create overlapping sequence enrollments: %v", err)
	}
	stageID := duplicateInsertStage(t, ctx, pool, organizationID, ownerID)
	if _, err := pool.Exec(ctx, `INSERT INTO deals (organization_id,primary_contact_id,stage_id,name,status,owner_user_id) VALUES ($1,$2,$3,'Source contact deal','open',$4)`, organizationID, sourceContactID, stageID, ownerID); err != nil {
		t.Fatalf("create source contact deal: %v", err)
	}

	service := moduleduplicates.NewService(pool)
	review, err := service.Review(ctx, organizationID, "contact", 20)
	if err != nil {
		t.Fatalf("review contact duplicates: %v", err)
	}
	candidate := findCandidate(t, review, sourceContactID, targetContactID)
	if !contains(candidate.Reasons, "matching email") || candidate.First.Related["notes"]+candidate.Second.Related["notes"] != 1 {
		t.Fatalf("unexpected duplicate candidate evidence: %#v", candidate)
	}
	sourceRecord := candidate.First
	targetRecord := candidate.Second
	if sourceRecord.ID != sourceContactID {
		sourceRecord, targetRecord = targetRecord, sourceRecord
	}
	mergeInput := moduleduplicates.MergeInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contact", SourceEntityID: sourceContactID, TargetEntityID: targetContactID,
		SourceFields: []string{"phone", "jobTitle"}, SourceUpdatedAt: sourceRecord.UpdatedAt, TargetUpdatedAt: targetRecord.UpdatedAt, IdempotencyKey: "duplicate-contact-merge-001",
	}
	operation, err := service.Merge(ctx, mergeInput)
	if err != nil || operation.SourceEntityID != sourceContactID || operation.TargetEntityID != targetContactID || operation.RelationshipCounts["notes"] != 1 {
		t.Fatalf("merge contact duplicate: operation=%#v err=%v", operation, err)
	}
	var archived, isClient, primary bool
	var phone, jobTitle string
	if err := pool.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM contacts WHERE organization_id=$1 AND id=$2`, organizationID, sourceContactID).Scan(&archived); err != nil || !archived {
		t.Fatalf("source contact was not archived: archived=%v err=%v", archived, err)
	}
	if err := pool.QueryRow(ctx, `SELECT phone,job_title,is_client FROM contacts WHERE organization_id=$1 AND id=$2`, organizationID, targetContactID).Scan(&phone, &jobTitle, &isClient); err != nil || phone != "+1 202 555 0199" || jobTitle != "Decision maker" || !isClient {
		t.Fatalf("surviving contact fields were not resolved: phone=%q job=%q client=%v err=%v", phone, jobTitle, isClient, err)
	}
	if err := pool.QueryRow(ctx, `SELECT is_primary FROM contact_company_links WHERE organization_id=$1 AND contact_id=$2 AND company_id=$3`, organizationID, targetContactID, companyID).Scan(&primary); err != nil || !primary {
		t.Fatalf("contact/client link was not consolidated as primary: primary=%v err=%v", primary, err)
	}
	assertEntityCount(t, ctx, pool, `SELECT count(*) FROM notes WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2`, organizationID, targetContactID, 1, "notes")
	assertEntityCount(t, ctx, pool, `SELECT count(*) FROM tasks WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2`, organizationID, targetContactID, 1, "tasks")
	assertEntityCount(t, ctx, pool, `SELECT count(*) FROM record_followers WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2`, organizationID, targetContactID, 1, "followers")
	assertEntityCount(t, ctx, pool, `SELECT count(*) FROM email_message_entity_links WHERE organization_id=$1 AND entity_type='contact' AND entity_id=$2`, organizationID, targetContactID, 1, "email links")
	assertEntityCount(t, ctx, pool, `SELECT count(*) FROM email_sequence_enrollments WHERE organization_id=$1 AND contact_id=$2`, organizationID, targetContactID, 2, "sequence enrollments")
	assertEntityCount(t, ctx, pool, `SELECT count(*) FROM email_sequence_enrollments WHERE organization_id=$1 AND contact_id=$2 AND status='active'`, organizationID, targetContactID, 1, "active sequence enrollments")
	assertEntityCount(t, ctx, pool, `SELECT count(*) FROM deals WHERE organization_id=$1 AND primary_contact_id=$2`, organizationID, targetContactID, 1, "deals")

	replay, err := service.Merge(ctx, mergeInput)
	if err != nil || !replay.Replayed || replay.ID != operation.ID {
		t.Fatalf("expected idempotent merge replay: replay=%#v err=%v", replay, err)
	}
	conflicting := mergeInput
	conflicting.SourceFields = []string{"email"}
	if _, err := service.Merge(ctx, conflicting); !errors.Is(err, moduleduplicates.ErrIdempotencyConflict) {
		t.Fatalf("expected merge idempotency conflict, got %v", err)
	}
	foreignReview, err := service.Review(ctx, foreignOrganizationID, "contact", 20)
	if err != nil || len(foreignReview.Candidates) != 0 || len(foreignReview.RecentMerges) != 0 {
		t.Fatalf("foreign tenant saw duplicate data: review=%#v err=%v", foreignReview, err)
	}
	foreignInput := mergeInput
	foreignInput.IdempotencyKey = "duplicate-cross-tenant-001"
	foreignInput.SourceEntityID = foreignContactID
	if _, err := service.Merge(ctx, foreignInput); !errors.Is(err, moduleduplicates.ErrNotFound) {
		t.Fatalf("expected cross-tenant merge rejection, got %v", err)
	}
	disabledInput := mergeInput
	disabledInput.IdempotencyKey = "duplicate-disabled-actor-001"
	disabledInput.ActorUserID = disabledID
	if _, err := service.Merge(ctx, disabledInput); !errors.Is(err, moduleduplicates.ErrInactiveActor) {
		t.Fatalf("expected disabled actor rejection, got %v", err)
	}
	staleSourceID := duplicateInsertContact(t, ctx, pool, organizationID, ownerID, "Stale", "Source", "stale-source@example.test")
	staleTargetID := duplicateInsertContact(t, ctx, pool, organizationID, ownerID, "Stale", "Target", "stale-target@example.test")
	if _, err := pool.Exec(ctx, `UPDATE contacts SET phone='+1 303 555 0142' WHERE organization_id=$1 AND id=ANY($2::bigint[])`, organizationID, []int64{staleSourceID, staleTargetID}); err != nil {
		t.Fatalf("prepare stale duplicate pair: %v", err)
	}
	staleReview, err := service.Review(ctx, organizationID, "contact", 20)
	if err != nil {
		t.Fatalf("review stale duplicate pair: %v", err)
	}
	staleCandidate := findCandidate(t, staleReview, staleSourceID, staleTargetID)
	staleSource, staleTarget := staleCandidate.First, staleCandidate.Second
	if staleSource.ID != staleSourceID {
		staleSource, staleTarget = staleTarget, staleSource
	}
	if _, err := pool.Exec(ctx, `UPDATE contacts SET status='customer',updated_at=NOW()+INTERVAL '1 second' WHERE organization_id=$1 AND id=$2`, organizationID, staleTargetID); err != nil {
		t.Fatalf("change duplicate after review: %v", err)
	}
	if _, err := service.Merge(ctx, moduleduplicates.MergeInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contact", SourceEntityID: staleSourceID, TargetEntityID: staleTargetID,
		SourceUpdatedAt: staleSource.UpdatedAt, TargetUpdatedAt: staleTarget.UpdatedAt, IdempotencyKey: "duplicate-stale-merge-001",
	}); !errors.Is(err, moduleduplicates.ErrConflict) {
		t.Fatalf("expected stale duplicate merge conflict, got %v", err)
	}

	scheduledSourceID := duplicateInsertContact(t, ctx, pool, organizationID, ownerID, "Scheduled", "Source", "scheduled-pair@example.test")
	scheduledTargetID := duplicateInsertContact(t, ctx, pool, organizationID, ownerID, "Scheduled", "Target", "scheduled-pair@example.test")
	if _, err := pool.Exec(ctx, `UPDATE contacts SET status='customer',is_client=TRUE WHERE organization_id=$1 AND id=$2`, organizationID, scheduledSourceID); err != nil {
		t.Fatalf("promote scheduled duplicate source: %v", err)
	}
	scheduledReview, err := service.Review(ctx, organizationID, "contact", 20)
	if err != nil {
		t.Fatalf("review scheduled duplicate pair: %v", err)
	}
	scheduledCandidate := findCandidate(t, scheduledReview, scheduledSourceID, scheduledTargetID)
	scheduledSource, scheduledTarget := scheduledCandidate.First, scheduledCandidate.Second
	if scheduledSource.ID != scheduledSourceID {
		scheduledSource, scheduledTarget = scheduledTarget, scheduledSource
	}
	reviews := moduleclientreviews.NewService(pool)
	if _, err := reviews.Upsert(ctx, organizationID, ownerID, "contact", scheduledSourceID, moduleclientreviews.Input{
		ReviewType: "review", NextReviewAt: time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339), CadenceMonths: 1, AssignedToUserID: ownerID,
	}); err != nil {
		t.Fatalf("schedule duplicate source review: %v", err)
	}
	if _, err := service.Merge(ctx, moduleduplicates.MergeInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "contact", SourceEntityID: scheduledSourceID, TargetEntityID: scheduledTargetID,
		SourceUpdatedAt: scheduledSource.UpdatedAt, TargetUpdatedAt: scheduledTarget.UpdatedAt, IdempotencyKey: "duplicate-scheduled-merge-001",
	}); !errors.Is(err, moduleduplicates.ErrConflict) {
		t.Fatalf("expected scheduled duplicate merge conflict, got %v", err)
	}

	testCompanyMerge(t, ctx, pool, service, organizationID, ownerID, stageID)
	var mergeAuditEvents int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE organization_id=$1 AND event_type='duplicate.merged'`, organizationID).Scan(&mergeAuditEvents); err != nil || mergeAuditEvents != 2 {
		t.Fatalf("unexpected merge audit event count: got=%d want=2 err=%v", mergeAuditEvents, err)
	}
}

func testCompanyMerge(t *testing.T, ctx context.Context, pool *moduledb.Pool, service *moduleduplicates.Service, organizationID, ownerID, stageID int64) {
	t.Helper()
	sourceCompanyID := duplicateInsertCompany(t, ctx, pool, organizationID, ownerID, "Northstar", "https://northstar.example/source", "individual")
	targetCompanyID := duplicateInsertCompany(t, ctx, pool, organizationID, ownerID, "NORTHSTAR", "", "individual")
	firstContact := duplicateInsertContact(t, ctx, pool, organizationID, ownerID, "First", "Client", "first@example.test")
	secondContact := duplicateInsertContact(t, ctx, pool, organizationID, ownerID, "Second", "Client", "second@example.test")
	if _, err := pool.Exec(ctx, `INSERT INTO contact_company_links (organization_id,contact_id,company_id,is_primary) VALUES ($1,$2,$3,TRUE),($1,$4,$5,TRUE)`, organizationID, firstContact, sourceCompanyID, secondContact, targetCompanyID); err != nil {
		t.Fatalf("create company contact links: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deals (organization_id,company_id,stage_id,name,status,owner_user_id) VALUES ($1,$2,$3,'Source company deal','open',$4)`, organizationID, sourceCompanyID, stageID, ownerID); err != nil {
		t.Fatalf("create source company deal: %v", err)
	}
	review, err := service.Review(ctx, organizationID, "company", 20)
	if err != nil {
		t.Fatalf("review company duplicates: %v", err)
	}
	candidate := findCandidate(t, review, sourceCompanyID, targetCompanyID)
	sourceRecord, targetRecord := candidate.First, candidate.Second
	if sourceRecord.ID != sourceCompanyID {
		sourceRecord, targetRecord = targetRecord, sourceRecord
	}
	operation, err := service.Merge(ctx, moduleduplicates.MergeInput{
		OrganizationID: organizationID, ActorUserID: ownerID, EntityType: "company", SourceEntityID: sourceCompanyID, TargetEntityID: targetCompanyID,
		SourceFields: []string{"website"}, SourceUpdatedAt: sourceRecord.UpdatedAt, TargetUpdatedAt: targetRecord.UpdatedAt, IdempotencyKey: "duplicate-company-merge-001",
	})
	if err != nil || operation.RelationshipCounts["contactLinks"] != 1 || operation.RelationshipCounts["deals"] != 1 {
		t.Fatalf("merge company duplicate: operation=%#v err=%v", operation, err)
	}
	var website, clientType string
	if err := pool.QueryRow(ctx, `SELECT website,client_type FROM companies WHERE organization_id=$1 AND id=$2`, organizationID, targetCompanyID).Scan(&website, &clientType); err != nil || website != "https://northstar.example/source" || clientType != "organization" {
		t.Fatalf("unexpected surviving company: website=%q type=%q err=%v", website, clientType, err)
	}
	assertEntityCount(t, ctx, pool, `SELECT count(*) FROM contact_company_links WHERE organization_id=$1 AND company_id=$2`, organizationID, targetCompanyID, 2, "company contact links")
	assertEntityCount(t, ctx, pool, `SELECT count(*) FROM deals WHERE organization_id=$1 AND company_id=$2`, organizationID, targetCompanyID, 1, "company deals")
}

func findCandidate(t *testing.T, review moduleduplicates.Review, firstID, secondID int64) moduleduplicates.Candidate {
	t.Helper()
	for _, candidate := range review.Candidates {
		if (candidate.First.ID == firstID && candidate.Second.ID == secondID) || (candidate.First.ID == secondID && candidate.Second.ID == firstID) {
			return candidate
		}
	}
	t.Fatalf("candidate pair %d/%d not found in %#v", firstID, secondID, review.Candidates)
	return moduleduplicates.Candidate{}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func assertEntityCount(t *testing.T, ctx context.Context, pool *moduledb.Pool, query string, organizationID, entityID int64, expected int, label string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, query, organizationID, entityID).Scan(&count); err != nil || count != expected {
		t.Fatalf("unexpected %s count: got=%d want=%d err=%v", label, count, expected, err)
	}
}

func duplicateInsertOrganization(t *testing.T, ctx context.Context, pool *moduledb.Pool, name, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ($1,$2) RETURNING id`, name, slug).Scan(&id); err != nil {
		t.Fatalf("create duplicate organization: %v", err)
	}
	return id
}

func duplicateInsertUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email, firstName, lastName string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'test-hash',$2,$3) RETURNING id`, email, firstName, lastName).Scan(&id); err != nil {
		t.Fatalf("create duplicate user: %v", err)
	}
	return id
}

func duplicateInsertContact(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID int64, firstName, lastName, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status,owner_user_id) VALUES ($1,$2,$3,$4,'lead',$5) RETURNING id`, organizationID, firstName, lastName, email, ownerID).Scan(&id); err != nil {
		t.Fatalf("create duplicate contact: %v", err)
	}
	return id
}

func duplicateInsertCompany(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID int64, name, website, clientType string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id,name,website,status,owner_user_id,client_type) VALUES ($1,$2,NULLIF($3,''),'prospect',$4,$5) RETURNING id`, organizationID, name, website, ownerID, clientType).Scan(&id); err != nil {
		t.Fatalf("create duplicate company: %v", err)
	}
	return id
}

func duplicateInsertStage(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, ownerID int64) int64 {
	t.Helper()
	var pipelineID, stageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id) VALUES ($1,'Merge pipeline',1,TRUE,$2) RETURNING id`, organizationID, ownerID).Scan(&pipelineID); err != nil {
		t.Fatalf("create duplicate pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id,pipeline_id,name,position) VALUES ($1,$2,'Qualified',1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create duplicate stage: %v", err)
	}
	return stageID
}

func duplicateDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse duplicate operations database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
