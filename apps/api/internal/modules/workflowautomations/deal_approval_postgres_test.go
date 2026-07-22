package workflowautomations_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

func TestDealApprovalTaskPlanPausesDecidesCancelsAndIsolates(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workflow approval postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_workflow_approval_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workflow approval schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := taskRuleDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate workflow approval schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to workflow approval schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Approval workspace',$1) RETURNING id`, "approval-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create approval organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign approvals',$1) RETURNING id`, "foreign-approval-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign approval organization: %v", err)
	}
	userIDs := make([]int64, 4)
	for index, identity := range []string{"requester", "reviewer", "deal-owner", "foreign-reviewer"} {
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email,password_hash,first_name,last_name)
			VALUES ($1,'hash',$2,'User') RETURNING id
		`, identity+"-"+schema+"@example.test", identity).Scan(&userIDs[index]); err != nil {
			t.Fatalf("create %s: %v", identity, err)
		}
	}
	requesterID, reviewerID, dealOwnerID, foreignReviewerID := userIDs[0], userIDs[1], userIDs[2], userIDs[3]
	if _, err := pool.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id,user_id,role,membership_status)
		VALUES ($1,$3,'admin','active'),($1,$4,'owner','active'),($1,$5,'member','active'),
		       ($2,$6,'owner','active')
	`, organizationID, foreignOrganizationID, requesterID, reviewerID, dealOwnerID, foreignReviewerID); err != nil {
		t.Fatalf("create workflow approval memberships: %v", err)
	}
	var pipelineID, stageID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id)
		VALUES ($1,'Sales',1,TRUE,$2) RETURNING id
	`, organizationID, requesterID).Scan(&pipelineID); err != nil {
		t.Fatalf("create approval pipeline: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO deal_stages (organization_id,pipeline_id,name,position,probability_percent)
		VALUES ($1,$2,'New',1,20) RETURNING id
	`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("create approval stage: %v", err)
	}

	automations := moduleworkflowautomations.NewService(pool)
	deals := moduledeals.NewService(pool)
	active := true
	approvalActions := []moduleworkflowautomations.Action{
		{Type: "request_approval", Config: map[string]any{"approvalName": "Proposal readiness", "approverRole": "owner", "message": "Confirm scope before follow-up tasks are created."}},
		{Type: "create_task", Config: map[string]any{"title": "Prepare proposal", "description": "Confirm scope."}, DelayMinutes: 1440},
		{Type: "create_task", Config: map[string]any{"title": "Schedule decision review"}, DelayMinutes: 4320},
	}
	rule, err := automations.Create(ctx, organizationID, requesterID, moduleworkflowautomations.Input{
		Name: "Approved proposal playbook", TriggerType: "record_created", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"taskPlanContract": moduleworkflowautomations.DealApprovalTaskPlanContract},
		Actions:       approvalActions, IsActive: &active,
	})
	if err != nil {
		t.Fatalf("create approval-gated task plan: %v", err)
	}

	firstDeal := createApprovalDeal(t, ctx, deals, organizationID, requesterID, stageID, dealOwnerID, "Approved opportunity")
	assertApprovalTaskCount(t, ctx, pool, organizationID, firstDeal, 0)
	runs, err := automations.ListRuns(ctx, organizationID, moduleworkflowautomations.RunListQuery{AutomationID: rule.ID, Limit: 10})
	if err != nil || len(runs) != 1 || runs[0].Status != "waiting_approval" || len(runs[0].Actions) != 3 || runs[0].Actions[0].Approval == nil || runs[0].Actions[0].Status != "queued" {
		t.Fatalf("pending run evidence mismatch: runs=%#v err=%v", runs, err)
	}
	approvals, err := automations.ListApprovals(ctx, organizationID, reviewerID)
	if err != nil || len(approvals) != 1 || approvals[0].PendingTaskCount != 2 || approvals[0].DealID != firstDeal {
		t.Fatalf("owner approval queue mismatch: approvals=%#v err=%v", approvals, err)
	}
	approvalID := approvals[0].ID
	stats, err := automations.OperationalStats(ctx)
	if err != nil || stats.ApprovalsPending != 1 || stats.Queued != 0 || stats.Running != 0 || stats.OldestApprovalAge < 0 {
		t.Fatalf("pending approval operational stats mismatch: stats=%#v err=%v", stats, err)
	}
	if hidden, err := automations.ListApprovals(ctx, organizationID, requesterID); err != nil || len(hidden) != 0 {
		t.Fatalf("admin saw owner-only approval: approvals=%#v err=%v", hidden, err)
	}
	if _, err := automations.DecideApproval(ctx, organizationID, approvalID, requesterID, moduleworkflowautomations.ApprovalDecisionInput{Decision: "approved", IdempotencyKey: "requester-forbidden-approval"}); !errors.Is(err, moduleworkflowautomations.ErrForbidden) {
		t.Fatalf("non-owner approval decision = %v", err)
	}
	if _, err := automations.DecideApproval(ctx, foreignOrganizationID, approvalID, foreignReviewerID, moduleworkflowautomations.ApprovalDecisionInput{Decision: "approved", IdempotencyKey: "foreign-tenant-approval-key"}); !errors.Is(err, moduleworkflowautomations.ErrNotFound) {
		t.Fatalf("cross-tenant approval decision = %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE workflow_automations
		SET actions_json=jsonb_set(actions_json,'{1,config,title}',to_jsonb($3::text),FALSE)
		WHERE organization_id=$1 AND id=$2
	`, organizationID, rule.ID, "Mutated after capture"); err != nil {
		t.Fatalf("mutate live definition after capture: %v", err)
	}
	decision := moduleworkflowautomations.ApprovalDecisionInput{Decision: "approved", IdempotencyKey: "approve-proposal-readiness-0001"}
	approved, err := automations.DecideApproval(ctx, organizationID, approvalID, reviewerID, decision)
	if err != nil || approved.Status != "approved" || approved.DecidedByUserID != reviewerID {
		t.Fatalf("approve captured plan: approval=%#v err=%v", approved, err)
	}
	if replay, err := automations.DecideApproval(ctx, organizationID, approvalID, reviewerID, decision); err != nil || replay.Status != "approved" {
		t.Fatalf("replay exact approval: approval=%#v err=%v", replay, err)
	}
	conflicting := decision
	conflicting.IdempotencyKey = "approve-proposal-readiness-0002"
	if _, err := automations.DecideApproval(ctx, organizationID, approvalID, reviewerID, conflicting); !errors.Is(err, moduleworkflowautomations.ErrApprovalConflict) {
		t.Fatalf("mismatched approval replay = %v", err)
	}
	assertApprovalTaskCount(t, ctx, pool, organizationID, firstDeal, 2)
	assertApprovalTaskTitle(t, ctx, pool, organizationID, firstDeal, "Prepare proposal", 1)
	assertApprovalTaskTitle(t, ctx, pool, organizationID, firstDeal, "Mutated after capture", 0)
	runs, err = automations.ListRuns(ctx, organizationID, moduleworkflowautomations.RunListQuery{AutomationID: rule.ID, Limit: 10})
	if err != nil || runs[0].Status != "succeeded" || runs[0].ActionsCompleted != 3 || runs[0].Actions[0].Approval.Status != "approved" {
		t.Fatalf("approved run evidence mismatch: runs=%#v err=%v", runs, err)
	}
	var evidenceCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_events
		WHERE organization_id=$1 AND entity_id=$2
		  AND event_type IN ('workflow_approval.requested','workflow_approval.decided')
	`, organizationID, approvalID).Scan(&evidenceCount); err != nil || evidenceCount != 2 {
		t.Fatalf("approval audit evidence count=%d err=%v", evidenceCount, err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notifications
		WHERE organization_id=$1 AND idempotency_key LIKE $2
	`, organizationID, fmt.Sprintf("workflow-approval:%d:%%", approvalID)).Scan(&evidenceCount); err != nil || evidenceCount != 2 {
		t.Fatalf("approval notification evidence count=%d err=%v", evidenceCount, err)
	}

	secondDeal := createApprovalDeal(t, ctx, deals, organizationID, requesterID, stageID, dealOwnerID, "Rejected opportunity")
	secondApproval := pendingApprovalForDeal(t, ctx, automations, organizationID, reviewerID, secondDeal)
	rejected, err := automations.DecideApproval(ctx, organizationID, secondApproval.ID, reviewerID, moduleworkflowautomations.ApprovalDecisionInput{
		Decision: "rejected", Note: "Scope is incomplete.", IdempotencyKey: "reject-proposal-readiness-0001",
	})
	if err != nil || rejected.Status != "rejected" || rejected.DecisionNote != "Scope is incomplete." {
		t.Fatalf("reject captured plan: approval=%#v err=%v", rejected, err)
	}
	assertApprovalTaskCount(t, ctx, pool, organizationID, secondDeal, 0)
	assertApprovalRunState(t, ctx, pool, organizationID, secondApproval.RunID, "cancelled", false)

	thirdDeal := createApprovalDeal(t, ctx, deals, organizationID, requesterID, stageID, dealOwnerID, "Definition change opportunity")
	thirdApproval := pendingApprovalForDeal(t, ctx, automations, organizationID, reviewerID, thirdDeal)
	updatedActions := append([]moduleworkflowautomations.Action(nil), approvalActions...)
	updatedActions[1].Config = map[string]any{"title": "Updated future proposal"}
	if _, err := automations.Update(ctx, organizationID, rule.ID, requesterID, moduleworkflowautomations.Input{
		Name: rule.Name, TriggerType: rule.TriggerType, TargetEntityType: rule.TargetEntityType,
		TriggerConfig: map[string]any{"taskPlanContract": moduleworkflowautomations.DealApprovalTaskPlanContract},
		Actions:       updatedActions, IsActive: &active,
	}); err != nil {
		t.Fatalf("update definition with pending approval: %v", err)
	}
	assertApprovalStatus(t, ctx, pool, organizationID, thirdApproval.ID, "cancelled")
	assertApprovalRunState(t, ctx, pool, organizationID, thirdApproval.RunID, "cancelled", false)
	assertApprovalTaskCount(t, ctx, pool, organizationID, thirdDeal, 0)

	fourthDeal := createApprovalDeal(t, ctx, deals, organizationID, requesterID, stageID, dealOwnerID, "Requester disabled opportunity")
	fourthApproval := pendingApprovalForDeal(t, ctx, automations, organizationID, reviewerID, fourthDeal)
	users := moduleusers.NewService(pool)
	if _, err := users.SetStatus(ctx, organizationID, requesterID, reviewerID, moduleusers.SetStatusInput{Status: moduleusers.MembershipStatusDisabled, ReassignToUserID: reviewerID}); err != nil {
		t.Fatalf("disable approval requester: %v", err)
	}
	assertApprovalStatus(t, ctx, pool, organizationID, fourthApproval.ID, "cancelled")
	assertApprovalRunState(t, ctx, pool, organizationID, fourthApproval.RunID, "cancelled", false)
	assertApprovalTaskCount(t, ctx, pool, organizationID, fourthDeal, 0)

	if _, err := automations.Update(ctx, organizationID, rule.ID, reviewerID, moduleworkflowautomations.Input{DeactivateOnly: true}); err != nil {
		t.Fatalf("deactivate first approval rule: %v", err)
	}
	recordOwnerRule, err := automations.Create(ctx, organizationID, reviewerID, moduleworkflowautomations.Input{
		Name: "Ownerless record approval", TriggerType: "record_created", TargetEntityType: "deal",
		TriggerConfig: map[string]any{"taskPlanContract": moduleworkflowautomations.DealApprovalTaskPlanContract}, IsActive: &active,
		Actions: []moduleworkflowautomations.Action{
			{Type: "request_approval", Config: map[string]any{"approvalName": "Record owner review", "approverRole": "record_owner", "message": "Only the current deal owner can approve."}},
			{Type: "create_task", Config: map[string]any{"title": "Must not be created"}},
		},
	})
	if err != nil {
		t.Fatalf("create record-owner approval rule: %v", err)
	}
	var ownerlessDeal int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deals (organization_id,stage_id,name,status,owner_user_id,value_amount,value_currency)
		VALUES ($1,$2,'Unavailable owner opportunity','open',$3,5000,'USD') RETURNING id
	`, organizationID, stageID, requesterID).Scan(&ownerlessDeal); err != nil {
		t.Fatalf("create deal retained by disabled owner: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin unavailable-reviewer deal event: %v", err)
	}
	if err := moduleworkflowautomations.ExecuteDealTaskRules(ctx, tx, moduleworkflowautomations.DealTaskEvent{
		OrganizationID: organizationID, ActorUserID: reviewerID, DealID: ownerlessDeal,
		DealName: "Unavailable owner opportunity", StageID: stageID, StageName: "New",
		OwnerUserID: requesterID, EventType: moduleworkflowautomations.DealEventCreated,
		EventKey: fmt.Sprintf("deal:%d:manual-unavailable-owner", ownerlessDeal),
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("execute unavailable-reviewer deal event: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit unavailable-reviewer deal event: %v", err)
	}
	assertApprovalTaskCount(t, ctx, pool, organizationID, ownerlessDeal, 0)
	failedRuns, err := automations.ListRuns(ctx, organizationID, moduleworkflowautomations.RunListQuery{AutomationID: recordOwnerRule.ID, Limit: 10})
	if err != nil || len(failedRuns) != 1 || failedRuns[0].Status != "failed" || len(failedRuns[0].Actions) != 2 || failedRuns[0].Actions[0].Status != "failed" || failedRuns[0].Actions[1].Status != "cancelled" {
		t.Fatalf("unavailable reviewer did not fail closed: runs=%#v err=%v", failedRuns, err)
	}
}

func createApprovalDeal(t *testing.T, ctx context.Context, deals *moduledeals.Service, organizationID, actorUserID, stageID, ownerUserID int64, name string) int64 {
	t.Helper()
	created, err := deals.Create(ctx, organizationID, actorUserID, moduledeals.CreateInput{Name: name, StageID: stageID, OwnerUserID: ownerUserID, ValueAmount: "5000", ValueCurrency: "USD"})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return created.Summary.ID
}

func pendingApprovalForDeal(t *testing.T, ctx context.Context, service *moduleworkflowautomations.Service, organizationID, reviewerID, dealID int64) moduleworkflowautomations.Approval {
	t.Helper()
	approvals, err := service.ListApprovals(ctx, organizationID, reviewerID)
	if err != nil {
		t.Fatalf("list pending approval for deal %d: %v", dealID, err)
	}
	for _, approval := range approvals {
		if approval.DealID == dealID {
			return approval
		}
	}
	t.Fatalf("pending approval for deal %d not found in %#v", dealID, approvals)
	return moduleworkflowautomations.Approval{}
}

func assertApprovalTaskCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, dealID int64, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2`, organizationID, dealID).Scan(&count); err != nil || count != want {
		t.Fatalf("deal %d task count=%d want=%d err=%v", dealID, count, want, err)
	}
}

func assertApprovalTaskTitle(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, dealID int64, title string, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE organization_id=$1 AND entity_type='deal' AND entity_id=$2 AND title=$3`, organizationID, dealID, title).Scan(&count); err != nil || count != want {
		t.Fatalf("deal %d task %q count=%d want=%d err=%v", dealID, title, count, want, err)
	}
}

func assertApprovalStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, approvalID int64, want string) {
	t.Helper()
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM workflow_automation_approvals WHERE organization_id=$1 AND id=$2`, organizationID, approvalID).Scan(&status); err != nil || status != want {
		t.Fatalf("approval %d status=%q want=%q err=%v", approvalID, status, want, err)
	}
}

func assertApprovalRunState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, organizationID, runID int64, wantStatus string, wantWaiting bool) {
	t.Helper()
	var status string
	var waiting bool
	if err := pool.QueryRow(ctx, `SELECT status,COALESCE(waiting_for_approval,FALSE) FROM workflow_automation_runs WHERE organization_id=$1 AND id=$2`, organizationID, runID).Scan(&status, &waiting); err != nil || status != wantStatus || waiting != wantWaiting {
		t.Fatalf("run %d state=(%q,%t) want=(%q,%t) err=%v", runID, status, waiting, wantStatus, wantWaiting, err)
	}
}
