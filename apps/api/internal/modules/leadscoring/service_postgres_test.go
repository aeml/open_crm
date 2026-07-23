package leadscoring_test

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
	moduleleadscoring "github.com/aeml/open_crm/apps/api/internal/modules/leadscoring"
)

func TestLeadScoringRulesAndEvaluationAreBoundedAuthorizedAndTenantSafe(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead scoring postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_scoring_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead scoring schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := leadScoringDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead scoring schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated lead scoring schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Lead scoring team',$1) RETURNING id`, "lead-scoring-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create lead scoring organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign lead scoring team',$1) RETURNING id`, "foreign-lead-scoring-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign lead scoring organization: %v", err)
	}

	users := map[string]int64{}
	for _, actor := range []string{"owner", "admin", "member", "viewer", "disabled", "assignee", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'test-hash','Scoring',$2) RETURNING id
		`, actor+"-"+schema+"@example.test", actor).Scan(&userID); err != nil {
			t.Fatalf("create %s lead scoring user: %v", actor, err)
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
		{organizationID, users["assignee"], "member", "active"},
		{foreignOrganizationID, users["foreign"], "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,$3,$4)`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("create lead scoring membership: %v", err)
		}
	}

	var contactID, foreignContactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status) VALUES ($1,'Pilot','Lead','pilot@example.test','lead') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("create lead scoring contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status) VALUES ($1,'Foreign','Lead','foreign@example.test','lead') RETURNING id`, foreignOrganizationID).Scan(&foreignContactID); err != nil {
		t.Fatalf("create foreign lead scoring contact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_scoring_rules (
			organization_id,name,field,operator,value,score_delta,assign_to_user_id,is_active,position,created_by_user_id,updated_by_user_id
		)
		SELECT $1,'Scoring rule ' || lpad(series::text,3,'0'),'status','equals','lead',
		       CASE WHEN series=1 THEN 25 ELSE 1 END,
		       CASE WHEN series=1 THEN $2::bigint ELSE NULL::bigint END,
		       series=1,series,$3,$3
		FROM generate_series(1,99) AS series
	`, organizationID, users["assignee"], users["owner"]); err != nil {
		t.Fatalf("seed lead scoring rules: %v", err)
	}
	var foreignRuleID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_scoring_rules (organization_id,name,field,operator,value,score_delta,is_active,position,created_by_user_id,updated_by_user_id)
		VALUES ($1,'Foreign scoring rule','status','equals','lead',90,TRUE,1,$2,$2) RETURNING id
	`, foreignOrganizationID, users["foreign"]).Scan(&foreignRuleID); err != nil {
		t.Fatalf("seed foreign lead scoring rule: %v", err)
	}

	service := moduleleadscoring.NewService(pool)
	rules, err := service.ListByOrganization(ctx, organizationID)
	if err != nil || len(rules) != 99 {
		t.Fatalf("list tenant lead scoring rules: count=%d err=%v", len(rules), err)
	}
	foreignRules, err := service.ListByOrganization(ctx, foreignOrganizationID)
	if err != nil || len(foreignRules) != 1 || foreignRules[0].ID != foreignRuleID {
		t.Fatalf("lead scoring list crossed tenants: rules=%+v err=%v", foreignRules, err)
	}
	for actor, userID := range map[string]int64{
		"member": users["member"], "viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"],
	} {
		if _, err := service.Create(ctx, organizationID, userID, validRuleInput("Forbidden "+actor, 0)); !errors.Is(err, moduleleadscoring.ErrForbidden) {
			t.Fatalf("%s actor created a lead scoring rule: %v", actor, err)
		}
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validRuleInput("Foreign assignee", users["foreign"])); !errors.Is(err, moduleleadscoring.ErrInvalidAssignee) {
		t.Fatalf("foreign assignee create returned %v", err)
	}

	type createResult struct {
		rule moduleleadscoring.Rule
		err  error
	}
	start := make(chan struct{})
	results := make(chan createResult, 2)
	for index, actorID := range []int64{users["owner"], users["admin"]} {
		go func(index int, actorID int64) {
			<-start
			rule, err := service.Create(ctx, organizationID, actorID, validRuleInput(fmt.Sprintf("Concurrent final rule %d", index+1), 0))
			results <- createResult{rule: rule, err: err}
		}(index, actorID)
	}
	close(start)
	var createdRule moduleleadscoring.Rule
	var succeeded, limited int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			succeeded++
			createdRule = result.rule
		case errors.Is(result.err, moduleleadscoring.ErrRuleLimit):
			limited++
		default:
			t.Fatalf("unexpected concurrent lead scoring create result: %v", result.err)
		}
	}
	if succeeded != 1 || limited != 1 || createdRule.ID <= 0 {
		t.Fatalf("lead scoring capacity was not serialized: succeeded=%d limited=%d rule=%+v", succeeded, limited, createdRule)
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], validRuleInput("Rule 101", 0)); !errors.Is(err, moduleleadscoring.ErrRuleLimit) {
		t.Fatalf("lead scoring capacity returned %v", err)
	}
	if _, err := service.Update(ctx, organizationID, createdRule.ID, users["member"], validRuleInput(createdRule.Name, 0)); !errors.Is(err, moduleleadscoring.ErrForbidden) {
		t.Fatalf("ordinary member updated a scoring rule: %v", err)
	}
	if _, err := service.Update(ctx, organizationID, foreignRuleID, users["owner"], validRuleInput("Foreign scoring rule", 0)); !errors.Is(err, moduleleadscoring.ErrNotFound) {
		t.Fatalf("cross-tenant scoring rule update returned %v", err)
	}

	evaluation, err := service.EvaluateContact(ctx, organizationID, contactID, users["member"])
	if err != nil {
		t.Fatalf("evaluate tenant lead score: %v", err)
	}
	if evaluation.Score != 25 || evaluation.Grade != "D" || evaluation.Contact.OwnerUserID != users["assignee"] || evaluation.AssignedToUserID != users["assignee"] || len(evaluation.MatchedRules) != 1 {
		t.Fatalf("unexpected lead score evaluation: %+v", evaluation)
	}
	for actor, userID := range map[string]int64{"viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"]} {
		if _, err := service.EvaluateContact(ctx, organizationID, contactID, userID); !errors.Is(err, moduleleadscoring.ErrForbidden) {
			t.Fatalf("%s actor evaluated tenant lead score: %v", actor, err)
		}
	}
	if _, err := service.EvaluateContact(ctx, organizationID, foreignContactID, users["member"]); !errors.Is(err, moduleleadscoring.ErrNotFound) {
		t.Fatalf("foreign contact evaluation returned %v", err)
	}
	if _, err := service.EvaluateContact(ctx, foreignOrganizationID, contactID, users["foreign"]); !errors.Is(err, moduleleadscoring.ErrNotFound) {
		t.Fatalf("reverse cross-tenant contact evaluation returned %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, users["assignee"]); err != nil {
		t.Fatalf("disable lead scoring assignee: %v", err)
	}
	evaluation, err = service.EvaluateContact(ctx, organizationID, contactID, users["owner"])
	if err != nil {
		t.Fatalf("reevaluate owned contact with disabled routing target: %v", err)
	}
	if evaluation.AssignedToUserID != 0 || len(evaluation.MatchedRules) != 1 || evaluation.MatchedRules[0].AssignToUserID != 0 || evaluation.MatchedRules[0].AssignToUserName != "" {
		t.Fatalf("disabled assignee was exposed for an already-owned contact: %+v", evaluation)
	}
	var unassignedContactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status) VALUES ($1,'Unassigned','Lead','unassigned@example.test','lead') RETURNING id`, organizationID).Scan(&unassignedContactID); err != nil {
		t.Fatalf("create unassigned lead scoring contact: %v", err)
	}
	evaluation, err = service.EvaluateContact(ctx, organizationID, unassignedContactID, users["owner"])
	if err != nil {
		t.Fatalf("evaluate with disabled routing target: %v", err)
	}
	if evaluation.Score != 25 || evaluation.AssignedToUserID != 0 || evaluation.Contact.OwnerUserID != 0 || len(evaluation.MatchedRules) != 1 || evaluation.MatchedRules[0].AssignToUserID != 0 {
		t.Fatalf("disabled assignee was exposed or assigned: %+v", evaluation)
	}

	var ruleCount, activityCount, foreignScore int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM lead_scoring_rules WHERE organization_id=$1`, organizationID).Scan(&ruleCount); err != nil {
		t.Fatalf("count retained lead scoring rules: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM activities WHERE organization_id=$1 AND action='lead.scored'`, organizationID).Scan(&activityCount); err != nil {
		t.Fatalf("count lead scoring activities: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT lead_score FROM contacts WHERE organization_id=$1 AND id=$2`, foreignOrganizationID, foreignContactID).Scan(&foreignScore); err != nil {
		t.Fatalf("load foreign contact score: %v", err)
	}
	if ruleCount != moduleleadscoring.MaxRulesPerOrganization || activityCount != 3 || foreignScore != 0 {
		t.Fatalf("unexpected retained scoring state: rules=%d activities=%d foreignScore=%d", ruleCount, activityCount, foreignScore)
	}
}

func validRuleInput(name string, assigneeID int64) moduleleadscoring.Input {
	active := false
	return moduleleadscoring.Input{
		Name: name, Description: "Bounded scoring rule", Field: "status", Operator: "equals", Value: "lead",
		ScoreDelta: 10, AssignToUserID: assigneeID, IsActive: &active, Position: 100,
	}
}

func leadScoringDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse lead scoring database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
