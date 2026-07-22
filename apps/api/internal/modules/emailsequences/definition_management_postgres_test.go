package emailsequences

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestSequenceDefinitionCatalogIsBoundedTenantSafeRevisionedAndCapacitySerialized(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to email sequence postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_sequence_definitions_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create email sequence schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSequenceApprovalSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate email sequence schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated email sequence schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Sequence team',$1) RETURNING id`, "sequence-definitions-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create email sequence organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign sequence team',$1) RETURNING id`, "foreign-sequence-definitions-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign email sequence organization: %v", err)
	}

	users := map[string]int64{}
	for _, actor := range []string{"owner", "admin", "member", "viewer", "disabled", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'test-hash','Sequence',$2) RETURNING id
		`, actor+"-"+schema+"@example.test", actor).Scan(&userID); err != nil {
			t.Fatalf("create %s email sequence user: %v", actor, err)
		}
		users[actor] = userID
	}
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		role           string
		status         string
	}{
		{organizationID, users["owner"], "owner", "active"},
		{organizationID, users["admin"], "admin", "active"},
		{organizationID, users["member"], "member", "active"},
		{organizationID, users["viewer"], "viewer", "active"},
		{organizationID, users["disabled"], "admin", "disabled"},
		{foreignOrganizationID, users["foreign"], "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,$3,$4)`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("create email sequence membership: %v", err)
		}
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO email_sequences (
		  organization_id,name,description,status,created_by_user_id,
		  approved_revision,approved_by_user_id,approved_at
		)
		SELECT $1,'Sequence active ' || lpad(series::text,3,'0'),'Approved cadence','active',$2,1,$2,NOW()
		FROM generate_series(1,99) AS series
	`, organizationID, users["owner"]); err != nil {
		t.Fatalf("seed active email sequences: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_sequences (organization_id,name,description,status,created_by_user_id)
		SELECT $1,
		       CASE WHEN series=1 THEN 'Literal %_ sequence' ELSE 'Sequence retained ' || lpad(series::text,3,'0') END,
		       'Retained definition',CASE WHEN series % 2 = 0 THEN 'draft' ELSE 'paused' END,$2
		FROM generate_series(1,902) AS series
	`, organizationID, users["owner"]); err != nil {
		t.Fatalf("seed retained email sequences: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_sequence_steps (sequence_id,step_order,delay_days,subject,body)
		SELECT id,1,0,'Pilot follow-up','Hello {{first_name}}'
		FROM email_sequences WHERE organization_id=$1
	`, organizationID); err != nil {
		t.Fatalf("seed email sequence steps: %v", err)
	}
	var foreignSequenceID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_sequences (organization_id,name,description,status,created_by_user_id)
		VALUES ($1,'Foreign sequence sentinel','Foreign definition','draft',$2) RETURNING id
	`, foreignOrganizationID, users["foreign"]).Scan(&foreignSequenceID); err != nil {
		t.Fatalf("seed foreign email sequence: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_sequence_steps (sequence_id,step_order,delay_days,subject,body) VALUES ($1,1,0,'Foreign','Foreign body')`, foreignSequenceID); err != nil {
		t.Fatalf("seed foreign email sequence step: %v", err)
	}

	service := NewService(pool)
	for _, query := range []ListQuery{
		{Page: 502, PageSize: 100},
		{PageSize: 101},
		{Status: "unknown"},
		{Search: strings.Repeat("x", MaxListSearchLength+1)},
	} {
		if _, err := service.ListByOrganization(ctx, organizationID, query); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("direct service accepted invalid email sequence query %+v: %v", query, err)
		}
	}
	firstPage, err := service.ListByOrganization(ctx, organizationID, ListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list first email sequence page: %v", err)
	}
	started := time.Now()
	secondPage, err := service.ListByOrganization(ctx, organizationID, ListQuery{Page: 2, PageSize: 50})
	if err != nil {
		t.Fatalf("list second email sequence page: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("adjacent email sequence page took %s, budget is 2s", elapsed)
	}
	if firstPage.Total != 1001 || secondPage.Total != 1001 || len(firstPage.Sequences) != 50 || len(secondPage.Sequences) != 50 {
		t.Fatalf("unexpected email sequence pagination: first=%+v second=%+v", firstPage, secondPage)
	}
	seen := make(map[int64]struct{}, len(firstPage.Sequences))
	for _, sequence := range firstPage.Sequences {
		seen[sequence.ID] = struct{}{}
		if len(sequence.Steps) != 1 {
			t.Fatalf("sequence %d did not retain its selected-page steps: %+v", sequence.ID, sequence.Steps)
		}
	}
	for _, sequence := range secondPage.Sequences {
		if _, duplicate := seen[sequence.ID]; duplicate {
			t.Fatalf("email sequence %d appeared on adjacent pages", sequence.ID)
		}
	}
	literal, err := service.ListByOrganization(ctx, organizationID, ListQuery{Search: "%_", Status: "paused", Page: 1, PageSize: 10})
	if err != nil || literal.Total != 1 || len(literal.Sequences) != 1 || literal.Sequences[0].Name != "Literal %_ sequence" {
		t.Fatalf("literal email sequence wildcard search failed: page=%+v err=%v", literal, err)
	}
	foreign, err := service.ListByOrganization(ctx, foreignOrganizationID, ListQuery{Page: 1, PageSize: 50})
	if err != nil || foreign.Total != 1 || len(foreign.Sequences) != 1 || foreign.Sequences[0].ID != foreignSequenceID {
		t.Fatalf("email sequence list crossed tenant boundaries: page=%+v err=%v", foreign, err)
	}

	if _, err := pool.Exec(ctx, `ANALYZE email_sequences`); err != nil {
		t.Fatalf("analyze email sequence fixtures: %v", err)
	}
	planRows, err := pool.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM email_sequences
		WHERE organization_id=$1 AND status='active'
		ORDER BY LOWER(name),id LIMIT 100
	`, organizationID)
	if err != nil {
		t.Fatalf("explain active email sequence query: %v", err)
	}
	var plan []string
	for planRows.Next() {
		var line string
		if err := planRows.Scan(&line); err != nil {
			planRows.Close()
			t.Fatalf("scan active email sequence plan: %v", err)
		}
		plan = append(plan, line)
	}
	if err := planRows.Err(); err != nil {
		planRows.Close()
		t.Fatalf("iterate active email sequence plan: %v", err)
	}
	planRows.Close()
	if joined := strings.Join(plan, "\n"); !strings.Contains(joined, "idx_email_sequences_org_status") && !strings.Contains(joined, "idx_email_sequences_org_name") {
		t.Fatalf("active email sequence query did not use the tenant/status/name index:\n%s", joined)
	}
	started = time.Now()
	activePage, err := service.ListByOrganization(ctx, organizationID, ListQuery{Status: "active", Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list active email sequences: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("100-row active email sequence page took %s, budget is 2s", elapsed)
	}
	if activePage.Total != 99 || len(activePage.Sequences) != 99 {
		t.Fatalf("unexpected active email sequence page: %+v", activePage)
	}

	for actor, userID := range map[string]int64{"viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"]} {
		if _, err := service.Create(ctx, organizationID, userID, validDefinitionInput("Forbidden "+actor, 0)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s actor created an email sequence: %v", actor, err)
		}
	}
	created, err := service.Create(ctx, organizationID, users["member"], validDefinitionInput("Member draft", 0))
	if err != nil {
		t.Fatalf("member create email sequence: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, created.ID, users["member"], validDefinitionInput("Member draft edited", created.Revision+1)); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale email sequence update returned %v", err)
	}
	updated, err := service.Update(ctx, organizationID, created.ID, users["member"], validDefinitionInput("Member draft edited", created.Revision))
	if err != nil || updated.Revision != created.Revision+1 {
		t.Fatalf("update exact email sequence revision: sequence=%+v err=%v", updated, err)
	}
	if err := service.Delete(ctx, organizationID, updated.ID, users["member"], created.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale email sequence delete returned %v", err)
	}
	if err := service.Delete(ctx, organizationID, updated.ID, users["member"], updated.Revision); err != nil {
		t.Fatalf("delete exact email sequence revision: %v", err)
	}
	for action, want := range map[string]int{"created": 1, "updated": 1, "deleted": 1} {
		if got := sequenceAuditCount(t, ctx, pool, organizationID, updated.ID, action); got != want {
			t.Fatalf("email sequence %s audit count=%d want=%d", action, got, want)
		}
	}

	firstCandidateID, firstRevision := definitionIdentity(t, ctx, pool, organizationID, "Sequence retained 002")
	secondCandidateID, secondRevision := definitionIdentity(t, ctx, pool, organizationID, "Sequence retained 004")
	if _, err := service.Approve(ctx, organizationID, firstCandidateID, users["owner"], firstRevision+1); !errors.Is(err, ErrConflict) {
		t.Fatalf("approval accepted an unreviewed revision: %v", err)
	}
	if _, err := service.Approve(ctx, organizationID, firstCandidateID, users["member"], firstRevision); !errors.Is(err, ErrNotFound) {
		t.Fatalf("member approved an email sequence: %v", err)
	}
	if _, err := service.Approve(ctx, organizationID, foreignSequenceID, users["owner"], 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant email sequence approval returned %v", err)
	}

	type approvalResult struct {
		sequence Sequence
		err      error
	}
	results := make(chan approvalResult, 2)
	for _, candidate := range []struct {
		id       int64
		revision int
		actorID  int64
	}{{firstCandidateID, firstRevision, users["owner"]}, {secondCandidateID, secondRevision, users["admin"]}} {
		go func(candidate struct {
			id       int64
			revision int
			actorID  int64
		}) {
			sequence, err := service.Approve(ctx, organizationID, candidate.id, candidate.actorID, candidate.revision)
			results <- approvalResult{sequence: sequence, err: err}
		}(candidate)
	}
	var activated, limited Sequence
	var successes, capacityErrors int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			successes++
			activated = result.sequence
		case errors.Is(result.err, ErrActiveLimit):
			capacityErrors++
			limited = result.sequence
		default:
			t.Fatalf("unexpected concurrent email sequence approval error: %v", result.err)
		}
	}
	if successes != 1 || capacityErrors != 1 || activated.ID == 0 || limited.ID != 0 {
		t.Fatalf("active email sequence ceiling was not serialized: successes=%d limited=%d activated=%+v rejected=%+v", successes, capacityErrors, activated, limited)
	}
	if repeated, err := service.Approve(ctx, organizationID, activated.ID, users["admin"], activated.Revision); err != nil || repeated.Status != "active" {
		t.Fatalf("repeat exact email sequence approval: sequence=%+v err=%v", repeated, err)
	}
	if got := sequenceAuditCount(t, ctx, pool, organizationID, activated.ID, "approved"); got != 1 {
		t.Fatalf("repeated approval wrote %d audit events, want 1", got)
	}
	paused, err := service.Pause(ctx, organizationID, activated.ID, users["member"])
	if err != nil || paused.Status != "paused" {
		t.Fatalf("member safety pause: sequence=%+v err=%v", paused, err)
	}
	if repeated, err := service.Pause(ctx, organizationID, activated.ID, users["member"]); err != nil || repeated.Status != "paused" {
		t.Fatalf("repeat email sequence pause: sequence=%+v err=%v", repeated, err)
	}
	if got := sequenceAuditCount(t, ctx, pool, organizationID, activated.ID, "paused"); got != 1 {
		t.Fatalf("repeated pause wrote %d audit events, want 1", got)
	}
	loserID, loserRevision := firstCandidateID, firstRevision
	if activated.ID == firstCandidateID {
		loserID, loserRevision = secondCandidateID, secondRevision
	}
	if resumed, err := service.Approve(ctx, organizationID, loserID, users["admin"], loserRevision); err != nil || resumed.Status != "active" {
		t.Fatalf("approve after freeing email sequence capacity: sequence=%+v err=%v", resumed, err)
	}
	activePage, err = service.ListByOrganization(ctx, organizationID, ListQuery{Status: "active", Page: 1, PageSize: 100})
	if err != nil || activePage.Total != MaxActiveSequences || len(activePage.Sequences) != MaxActiveSequences {
		t.Fatalf("email sequence active ceiling page mismatch: page=%+v err=%v", activePage, err)
	}
	if _, err := service.Pause(ctx, organizationID, loserID, users["viewer"]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("viewer email sequence pause returned %v", err)
	}
}

func validDefinitionInput(name string, revision int) Input {
	return Input{
		Name: name, Status: "draft", ExpectedRevision: revision,
		Steps: []StepInput{{DelayDays: 1, Subject: "Pilot follow-up", Body: "Hello {{first_name}}"}},
	}
}

func definitionIdentity(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID int64, name string) (int64, int) {
	t.Helper()
	var sequenceID int64
	var revision int
	if err := pool.QueryRow(ctx, `SELECT id,revision FROM email_sequences WHERE organization_id=$1 AND name=$2`, organizationID, name).Scan(&sequenceID, &revision); err != nil {
		t.Fatalf("load email sequence %s: %v", name, err)
	}
	return sequenceID, revision
}

func sequenceAuditCount(t *testing.T, ctx context.Context, pool *moduledb.Pool, organizationID, sequenceID int64, action string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM audit_events
		WHERE organization_id=$1 AND entity_type='email_sequence' AND entity_id=$2 AND event_type=$3
	`, organizationID, sequenceID, "email_sequence."+action).Scan(&count); err != nil {
		t.Fatalf("count email sequence %s audit: %v", action, err)
	}
	return count
}
