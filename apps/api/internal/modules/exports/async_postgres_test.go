package exports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

func TestDurableCRMExportLifecycleAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to CRM export postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_async_exports_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create CRM export schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := asyncExportDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate CRM export schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to CRM export schema: %v", err)
	}
	defer pool.Close()

	var organizationID, ownerID, foreignOrganizationID, foreignOwnerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Export Pilot','export-pilot') RETURNING id`).Scan(&organizationID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at) VALUES ('owner@exports.test','hash','Export','Owner',NOW()) RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner','active')`, organizationID, ownerID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign Export','foreign-export') RETURNING id`).Scan(&foreignOrganizationID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at) VALUES ('foreign@exports.test','hash','Foreign','Owner',NOW()) RETURNING id`).Scan(&foreignOwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role,membership_status) VALUES ($1,$2,'owner','active')`, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO custom_field_definitions (organization_id,created_by_user_id,entity_type,field_key,label,data_type) VALUES ($1,$2,'contact','region','Region','text')`, organizationID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name,email,custom_fields) VALUES
		($1,'Morgan','Pilot','morgan@exports.test','{"region":"West"}'::jsonb),
		($1,'Riley','Pilot','riley@exports.test','{"region":"West"}'::jsonb),
		($2,'Foreign','Pilot','foreign-contact@exports.test','{"region":"West"}'::jsonb)
	`, organizationID, foreignOrganizationID); err != nil {
		t.Fatal(err)
	}

	service := NewService(pool)
	request := AsyncRequest{Resource: "contacts", Search: "Pilot", UnassignedOnly: true, CustomField: modulecustomfields.Filter{FieldKey: "region", Operator: "eq", Value: "West"}}
	queued, err := service.RequestAsync(ctx, organizationID, ownerID, "durable-export-key-1", request)
	if err != nil || queued.Status != "pending" || queued.Resource != "contacts" || !queued.Criteria.UnassignedOnly {
		t.Fatalf("queue durable CRM export: export=%#v err=%v", queued, err)
	}
	replayed, err := service.RequestAsync(ctx, organizationID, ownerID, "durable-export-key-1", request)
	if err != nil || replayed.ID != queued.ID {
		t.Fatalf("idempotent CRM export request: export=%#v err=%v", replayed, err)
	}
	if _, err := service.RequestAsync(ctx, organizationID, ownerID, "durable-export-key-1", AsyncRequest{Resource: "contacts", Search: "different"}); !errors.Is(err, ErrAsyncIdempotencyConflict) {
		t.Fatalf("different criteria reused request key: %v", err)
	}
	foreignHistory, err := service.ListAsync(ctx, foreignOrganizationID)
	if err != nil || len(foreignHistory) != 0 {
		t.Fatalf("foreign tenant saw CRM export history: history=%#v err=%v", foreignHistory, err)
	}

	var job modulejobs.Job
	if err := pool.QueryRow(ctx, `SELECT id,organization_id,job_type,idempotency_key,payload_json,status,attempts,max_attempts,run_at,created_at,updated_at FROM background_jobs WHERE organization_id=$1 AND job_type=$2`, organizationID, AsyncJobType).Scan(&job.ID, &job.OrganizationID, &job.Type, &job.IdempotencyKey, &job.Payload, &job.Status, &job.Attempts, &job.MaxAttempts, &job.RunAt, &job.CreatedAt, &job.UpdatedAt); err != nil {
		t.Fatalf("load durable CRM export job: %v", err)
	}
	result, err := service.HandleAsyncJob(ctx, job)
	if err != nil || result["rowCount"] != 2 {
		t.Fatalf("generate durable CRM export: result=%#v err=%v", result, err)
	}
	duplicate, err := service.HandleAsyncJob(ctx, job)
	if err != nil || duplicate["alreadyReady"] != true {
		t.Fatalf("replay ready CRM export: result=%#v err=%v", duplicate, err)
	}
	history, err := service.ListAsync(ctx, organizationID)
	if err != nil || len(history) != 1 || history[0].Status != "ready" || history[0].ProgressRows != 2 || history[0].RowCount != 2 || history[0].ByteSize <= 0 {
		t.Fatalf("unexpected CRM export history: %#v err=%v", history, err)
	}
	download, err := service.DownloadAsync(ctx, organizationID, ownerID, queued.ID)
	if err != nil || !strings.Contains(string(download.Content), "morgan@exports.test") || !strings.Contains(string(download.Content), "riley@exports.test") || strings.Contains(string(download.Content), "foreign-contact@exports.test") {
		t.Fatalf("unexpected CRM export artifact: err=%v body=%q", err, string(download.Content))
	}
	digest := sha256.Sum256(download.Content)
	if download.ContentSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("CRM export digest mismatch: %s", download.ContentSHA256)
	}
	if _, err := service.DownloadAsync(ctx, foreignOrganizationID, foreignOwnerID, queued.ID); !errors.Is(err, ErrAsyncNotFound) {
		t.Fatalf("foreign tenant downloaded CRM export: %v", err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND entity_id=$2 AND event_type IN ('crm.export_queued','crm.export_ready','crm.export_downloaded')`, organizationID, queued.ID).Scan(&auditCount); err != nil || auditCount != 3 {
		t.Fatalf("CRM export audit coverage: count=%d err=%v", auditCount, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE crm_exports SET expires_at=NOW()-INTERVAL '1 second' WHERE organization_id=$1 AND id=$2`, organizationID, queued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExpireAsyncArtifacts(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DownloadAsync(ctx, organizationID, ownerID, queued.ID); !errors.Is(err, ErrAsyncExpired) {
		t.Fatalf("expired CRM export remained downloadable: %v", err)
	}

	type requestResult struct {
		export AsyncExport
		err    error
	}
	results := make(chan requestResult, 2)
	var requests sync.WaitGroup
	for range 2 {
		requests.Add(1)
		go func() {
			defer requests.Done()
			export, requestErr := service.RequestAsync(ctx, organizationID, ownerID, "durable-export-key-2", AsyncRequest{Resource: "tasks"})
			results <- requestResult{export: export, err: requestErr}
		}()
	}
	requests.Wait()
	close(results)
	var second AsyncExport
	for result := range results {
		if result.err != nil {
			t.Fatalf("queue concurrent idempotent CRM export: %v", result.err)
		}
		if second.ID == 0 {
			second = result.export
		} else if result.export.ID != second.ID {
			t.Fatalf("concurrent same-key requests created different exports: %d and %d", second.ID, result.export.ID)
		}
	}
	var concurrentExportCount, concurrentJobCount int
	concurrentKeyDigest := sha256.Sum256([]byte("durable-export-key-2"))
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_exports WHERE organization_id=$1 AND idempotency_key_hash=$2`, organizationID, hex.EncodeToString(concurrentKeyDigest[:])).Scan(&concurrentExportCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id=$1 AND job_type=$2 AND payload_json->>'exportId'=$3`, organizationID, AsyncJobType, fmt.Sprint(second.ID)).Scan(&concurrentJobCount); err != nil {
		t.Fatal(err)
	}
	if concurrentExportCount != 1 || concurrentJobCount != 1 {
		t.Fatalf("concurrent same-key request was not singular: exports=%d jobs=%d", concurrentExportCount, concurrentJobCount)
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(ctx, `LOCK TABLE tasks IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	workerResult := make(chan error, 1)
	go func() {
		_, handleErr := service.HandleAsyncJob(ctx, modulejobs.Job{OrganizationID: organizationID, Payload: map[string]any{"exportId": fmt.Sprint(second.ID)}})
		workerResult <- handleErr
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM crm_exports WHERE organization_id=$1 AND id=$2`, organizationID, second.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "processing" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("CRM export worker did not reach processing state: %s", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_memberships SET membership_status='disabled' WHERE organization_id=$1 AND user_id=$2`, organizationID, ownerID); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-workerResult; !errors.Is(err, ErrAsyncInactiveActor) {
		t.Fatalf("worker finalized after its actor was disabled: %v", err)
	}
	var quiescedStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM crm_exports WHERE organization_id=$1 AND id=$2`, organizationID, second.ID).Scan(&quiescedStatus); err != nil {
		t.Fatal(err)
	}
	if quiescedStatus != "failed" {
		t.Fatalf("disabled-actor export finalized instead of remaining failed: %s", quiescedStatus)
	}
	if _, err := service.RequestAsync(ctx, organizationID, ownerID, "durable-export-key-3", AsyncRequest{Resource: "tasks"}); !errors.Is(err, ErrAsyncInactiveActor) {
		t.Fatalf("disabled actor requested export: %v", err)
	}
}

func asyncExportDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse CRM export database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
