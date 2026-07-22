package workflowautomations_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

func TestWorkflowActivationAuthorizationCapacityAndRecovery(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow activation postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_workflow_activation_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow activation schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := taskRuleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate workflow activation schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to workflow activation schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Activation',$1) RETURNING id`, "activation-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create workflow activation organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign activation',$1) RETURNING id`, "foreign-activation-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign workflow activation organization: %v", err)
	}
	users := map[string]int64{}
	for _, name := range []string{"owner", "admin", "member", "disabled", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'hash',$2,'Activation') RETURNING id
		`, name+"-"+schema+"@example.test", name).Scan(&userID); err != nil {
			t.Fatalf("create %s workflow activation user: %v", name, err)
		}
		users[name] = userID
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$3,'owner','active'),($1,$4,'admin','active'),($1,$5,'member','active'),
		       ($1,$6,'admin','disabled'),($2,$7,'owner','active')
	`, organizationID, foreignOrganizationID, users["owner"], users["admin"], users["member"], users["disabled"], users["foreign"]); err != nil {
		t.Fatalf("create workflow activation memberships: %v", err)
	}

	service := moduleworkflowautomations.NewService(pool)
	active := true
	inactive := false
	ownerRuleInput := reviewedDealRuleInput("Owner rule", &active)
	ownerRule, err := service.Create(ctx, organizationID, users["owner"], ownerRuleInput)
	if err != nil {
		t.Fatalf("owner could not create reviewed active rule: %v", err)
	}
	adminRuleInput := reviewedDealRuleInput("Admin inactive rule", &inactive)
	adminRule, err := service.Create(ctx, organizationID, users["admin"], adminRuleInput)
	if err != nil {
		t.Fatalf("admin could not create reviewed inactive rule: %v", err)
	}
	adminRuleInput.Name = "Admin renamed rule"
	if _, err := service.Update(ctx, organizationID, adminRule.ID, users["admin"], adminRuleInput); err != nil {
		t.Fatalf("admin could not update workflow rule: %v", err)
	}

	for actor, expected := range map[string]error{
		"member":   moduleworkflowautomations.ErrForbidden,
		"disabled": moduleworkflowautomations.ErrForbidden,
		"foreign":  moduleworkflowautomations.ErrForbidden,
	} {
		input := reviewedDealRuleInput("Forbidden "+actor, &inactive)
		if _, err := service.Create(ctx, organizationID, users[actor], input); !errors.Is(err, expected) {
			t.Fatalf("expected %s writer denial, got %v", actor, err)
		}
	}
	if _, err := service.Update(ctx, organizationID, adminRule.ID, users["foreign"], adminRuleInput); !errors.Is(err, moduleworkflowautomations.ErrForbidden) {
		t.Fatalf("expected cross-tenant writer denial, got %v", err)
	}

	unsupported := moduleworkflowautomations.Input{
		Name: "Stored email foundation", TriggerType: "record_created", TargetEntityType: "contact",
		Actions:  []moduleworkflowautomations.Action{{Type: "send_email", Config: map[string]any{"subject": "Hello", "body": "World"}}},
		IsActive: &active,
	}
	if _, err := service.Create(ctx, organizationID, users["owner"], unsupported); !errors.Is(err, moduleworkflowautomations.ErrNotExecutable) {
		t.Fatalf("expected unsupported active definition rejection, got %v", err)
	}
	unsupported.IsActive = &inactive
	storedFoundation, err := service.Create(ctx, organizationID, users["owner"], unsupported)
	if err != nil {
		t.Fatalf("inactive foundation should remain storable: %v", err)
	}
	unsupported.IsActive = &active
	if _, err := service.Update(ctx, organizationID, storedFoundation.ID, users["owner"], unsupported); !errors.Is(err, moduleworkflowautomations.ErrNotExecutable) {
		t.Fatalf("expected unsupported foundation activation rejection, got %v", err)
	}
	var malformedActiveID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO workflow_automations
			(organization_id,name,trigger_type,target_entity_type,actions_json,is_active,created_by_user_id,updated_by_user_id)
		VALUES ($1,'Unknown future active definition','webhook','webhook',
		        '[{"type":"future_action","config":{"future":true}}]'::jsonb,TRUE,$2,$2)
		RETURNING id
	`, organizationID, users["owner"]).Scan(&malformedActiveID); err != nil {
		t.Fatalf("seed unknown future active definition: %v", err)
	}
	recoveredMalformed, err := service.Update(ctx, organizationID, malformedActiveID, users["admin"], moduleworkflowautomations.Input{DeactivateOnly: true})
	if err != nil || recoveredMalformed.IsActive || len(recoveredMalformed.Actions) != 1 || recoveredMalformed.Actions[0].Type != "future_action" {
		t.Fatalf("explicit deactivation did not preserve unknown stored definition: definition=%#v err=%v", recoveredMalformed, err)
	}

	legacyActions := make([]moduleworkflowautomations.Action, 48)
	legacyJSON, err := json.Marshal(legacyActions)
	if err != nil {
		t.Fatalf("encode legacy workflow actions: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_automations
			(organization_id,name,trigger_type,target_entity_type,actions_json,is_active,created_by_user_id,updated_by_user_id)
		VALUES ($1,'Pre-contract active overflow','record_created','deal',$2::jsonb,TRUE,$3,$3)
	`, organizationID, string(legacyJSON), users["owner"]); err != nil {
		t.Fatalf("seed pre-contract active action allocation: %v", err)
	}

	type createResult struct {
		automation moduleworkflowautomations.Automation
		err        error
	}
	start := make(chan struct{})
	results := make(chan createResult, 2)
	for _, name := range []string{"Concurrent final slot A", "Concurrent final slot B"} {
		name := name
		go func() {
			<-start
			automation, createErr := service.Create(ctx, organizationID, users["owner"], reviewedDealRuleInput(name, &active))
			results <- createResult{automation: automation, err: createErr}
		}()
	}
	close(start)
	var successful moduleworkflowautomations.Automation
	var successes, limited int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			successes++
			successful = result.automation
		case errors.Is(result.err, moduleworkflowautomations.ErrActiveLimit):
			limited++
		default:
			t.Fatalf("unexpected concurrent activation result: %#v", result)
		}
	}
	if successes != 1 || limited != 1 || successful.ID <= 0 {
		t.Fatalf("expected one exact final-slot winner: successes=%d limited=%d winner=%#v", successes, limited, successful)
	}
	assertActiveWorkflowActionCount(t, ctx, pool, organizationID, moduleworkflowautomations.MaxActiveWorkflowActions)

	ownerRuleInput.IsActive = &inactive
	if _, err := service.Update(ctx, organizationID, ownerRule.ID, users["owner"], ownerRuleInput); err != nil {
		t.Fatalf("deactivate active rule for capacity recovery: %v", err)
	}
	if _, err := service.Create(ctx, organizationID, users["admin"], reviewedDealRuleInput("Recovered active slot", &active)); err != nil {
		t.Fatalf("activate after capacity recovery: %v", err)
	}
	assertActiveWorkflowActionCount(t, ctx, pool, organizationID, moduleworkflowautomations.MaxActiveWorkflowActions)

	var forbiddenDefinitions int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_automations WHERE organization_id=$1 AND name LIKE 'Forbidden %'`, organizationID).Scan(&forbiddenDefinitions); err != nil || forbiddenDefinitions != 0 {
		t.Fatalf("forbidden writes left definitions: count=%d err=%v", forbiddenDefinitions, err)
	}
}

func reviewedDealRuleInput(name string, active *bool) moduleworkflowautomations.Input {
	return moduleworkflowautomations.Input{
		Name: name, TriggerType: "record_created", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"taskPlanContract": moduleworkflowautomations.DealTaskPlanContract},
		Actions:       []moduleworkflowautomations.Action{{Type: "create_task", Config: map[string]any{"title": "Qualify deal"}}},
		IsActive:      active,
	}
}

func assertActiveWorkflowActionCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID int64, expected int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(jsonb_array_length(actions_json)),0)::int
		FROM workflow_automations WHERE organization_id=$1 AND is_active=TRUE
	`, organizationID).Scan(&count); err != nil || count != expected {
		t.Fatalf("unexpected active workflow action count: count=%d expected=%d err=%v", count, expected, err)
	}
}
