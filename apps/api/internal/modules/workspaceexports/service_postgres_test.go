package workspaceexports

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	"github.com/jackc/pgx/v5"
)

func TestWorkspaceExportLifecycleAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to workspace export postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_workspace_exports_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create workspace export schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := workspaceExportDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate workspace export schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to workspace export schema: %v", err)
	}
	defer pool.Close()

	var organizationID, ownerID, foreignOrganizationID, foreignOwnerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,plan,subscription_status) VALUES ('Portable Pilot','Portable Pilot','pro','active') RETURNING id`).Scan(&organizationID); err != nil {
		t.Fatalf("create workspace export organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at) VALUES ('owner@portable.test','secret-password-hash','Portia','Owner',NOW()) RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatalf("create workspace export owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'owner')`, organizationID, ownerID); err != nil {
		t.Fatalf("create workspace export membership: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign Workspace','foreign-workspace') RETURNING id`).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ('owner@foreign.test','foreign-hash','Foreign','Owner') RETURNING id`).Scan(&foreignOwnerID); err != nil {
		t.Fatalf("create foreign owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'owner')`, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatalf("create foreign membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,custom_fields) VALUES ($1,'Morgan','Pilot','morgan@portable.test','{"region":"West"}'::jsonb)`, organizationID); err != nil {
		t.Fatalf("seed portable workspace contact: %v", err)
	}
	var quoteID int64
	if err := pool.QueryRow(ctx, `
		WITH pipeline AS (
			INSERT INTO deal_pipelines (organization_id,name,position,is_default,created_by_user_id)
			VALUES ($1,'Portable sales',1,TRUE,$2) RETURNING id
		), stage AS (
			INSERT INTO deal_stages (organization_id,pipeline_id,name,position,probability_percent)
			SELECT $1,id,'Proposal',1,60 FROM pipeline RETURNING id
		), deal AS (
			INSERT INTO deals (organization_id,stage_id,name,status,value_amount,value_currency,owner_user_id)
			SELECT $1,id,'Portable quote','open',125,'USD',$2 FROM stage RETURNING id
		)
		INSERT INTO deal_quotes (
			organization_id,deal_id,version,quote_number,organization_name,deal_name,recipient_name,
			recipient_email,prepared_by_name,currency,subtotal,discount_total,tax_total,total,valid_until,
			terms,pdf_filename,pdf_content,pdf_sha256,idempotency_key_hash,request_sha256,created_by_user_id,created_at
		)
		SELECT $1,id,1,'Q-PORTABLE-V1','Portable Pilot','Portable quote','Morgan Pilot',
			'morgan@portable.test','Portia Owner','USD',125,0,0,125,CURRENT_DATE+30,
			'Portable quote terms','quote-portable-v1.pdf',convert_to(repeat('P',100),'UTF8'),repeat('a',64),repeat('b',64),repeat('c',64),$2,NOW()
		FROM deal RETURNING id
	`, organizationID, ownerID).Scan(&quoteID); err != nil {
		t.Fatalf("seed portable finalized quote: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deal_quote_line_items (
			organization_id,quote_id,name,item_type,quantity,unit_name,unit_price,subtotal,
			discount_amount,tax_rate,tax_amount,total,currency,position
		) VALUES ($1,$2,'Portable service','service',1,'project',125,125,0,0,0,125,'USD',1)
	`, organizationID, quoteID); err != nil {
		t.Fatalf("seed portable finalized quote line: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_messages (organization_id,to_email,subject,body,status,visibility,rfc_message_id,in_reply_to,reference_message_ids) VALUES
			($1,'shared@portable.test','Shared customer thread','Shared body','sent','shared','<shared@crm.example.test>','<prior@buyer.test>',ARRAY['<older@buyer.test>']),
			($1,'private@portable.test','Private mailbox thread','Private body','sent','private','','','{}'::TEXT[])
	`, organizationID); err != nil {
		t.Fatalf("seed portable workspace email: %v", err)
	}
	var sharedMessageID, privateMessageID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM email_messages WHERE organization_id=$1 AND subject='Shared customer thread'`, organizationID).Scan(&sharedMessageID); err != nil {
		t.Fatalf("load shared portable email: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM email_messages WHERE organization_id=$1 AND subject='Private mailbox thread'`, organizationID).Scan(&privateMessageID); err != nil {
		t.Fatalf("load private portable email: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_reply_requests (
		  organization_id,source_message_id,thread_root_message_id,actor_user_id,sender_email,recipient_email,
		  subject,body,visibility,rfc_message_id,in_reply_to,idempotency_key_hash,request_sha256
		) VALUES
		($1,$2,$2,$3,'owner@portable.test','shared@portable.test','Re: Shared customer thread','Shared pending reply','shared','<reply-shared@crm.example.test>','<shared@crm.example.test>',$4,$5),
		($1,$6,$6,$3,'owner@portable.test','private@portable.test','Re: Private mailbox thread','Private pending reply','private','<reply-private@crm.example.test>','<private@buyer.test>',$7,$8)
	`, organizationID, sharedMessageID, ownerID, strings.Repeat("a", 64), strings.Repeat("b", 64), privateMessageID, strings.Repeat("c", 64), strings.Repeat("d", 64)); err != nil {
		t.Fatalf("seed portable email reply intents: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_email_accounts (
			organization_id,user_id,from_email,from_name,smtp_host,smtp_port,smtp_username,smtp_password_enc,
			imap_host,imap_port,imap_username,imap_password_enc,oauth_access_token_enc,oauth_refresh_token_enc
		) VALUES ($1,$2,'owner@portable.test','Portia','smtp.test',587,'smtp-owner','encrypted-smtp','imap.test',993,'imap-owner','encrypted-imap','encrypted-access','encrypted-refresh')
	`, organizationID, ownerID); err != nil {
		t.Fatalf("seed portable workspace data: %v", err)
	}

	service := NewService(pool)
	requested, err := service.Request(ctx, organizationID, ownerID, "portable-request-1")
	if err != nil || requested.Status != "pending" {
		t.Fatalf("request workspace export: export=%#v err=%v", requested, err)
	}
	duplicate, err := service.Request(ctx, organizationID, ownerID, "portable-request-1")
	if err != nil || duplicate.ID != requested.ID {
		t.Fatalf("workspace export idempotency failed: duplicate=%#v err=%v", duplicate, err)
	}
	if _, err := service.Request(ctx, organizationID, ownerID, "parallel-portable-request"); !errors.Is(err, ErrExportInProgress) {
		t.Fatalf("parallel workspace export error=%v, want in progress", err)
	}
	var jobCount, requestAuditCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM background_jobs WHERE organization_id=$1 AND job_type=$2`, organizationID, JobType).Scan(&jobCount); err != nil || jobCount != 1 {
		t.Fatalf("workspace export job was not unique: count=%d err=%v", jobCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='workspace.export_requested'`, organizationID).Scan(&requestAuditCount); err != nil || requestAuditCount != 1 {
		t.Fatalf("workspace export request audit was not unique: count=%d err=%v", requestAuditCount, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspace_exports (
			organization_id,requested_by_user_id,idempotency_key_hash,status,filename,content_sha256,
			byte_size,dataset_counts,artifact,completed_at,expires_at,created_at,updated_at
		)
		SELECT $1,$2,repeat(series::text,64),'ready','older-'||series||'.zip',repeat(series::text,64),
			1,'{}'::jsonb,decode('50','hex'),NOW()-(series||' hours')::interval,NOW()+INTERVAL '7 days',
			NOW()-(series||' hours')::interval,NOW()-(series||' hours')::interval
		FROM generate_series(2,4) AS series
	`, organizationID, ownerID); err != nil {
		t.Fatalf("seed older retained workspace exports: %v", err)
	}

	queue := modulejobs.NewService(pool)
	worker := modulejobs.NewWorker(queue, map[string]modulejobs.Handler{JobType: service.HandleJob}, "workspace-export-test", nil)
	summary, err := worker.RunOnce(ctx)
	if err != nil || summary.Succeeded != 1 {
		t.Fatalf("generate workspace export: summary=%#v err=%v", summary, err)
	}
	history, err := service.List(ctx, organizationID)
	if err != nil || len(history) != 4 || history[0].ID != requested.ID || history[0].Status != "ready" || history[0].ContentSHA256 == "" || history[0].ByteSize <= 0 || history[0].DatasetCounts["contacts"] != 1 || history[0].DatasetCounts["deal_quotes"] != 1 || history[0].DatasetCounts["deal_quote_line_items"] != 1 || history[0].DatasetCounts["email_messages_shared"] != 1 {
		t.Fatalf("unexpected workspace export history: history=%#v err=%v", history, err)
	}
	var retainedReady, cappedExpired int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE status='ready' AND artifact IS NOT NULL),COUNT(*) FILTER (WHERE status='expired' AND artifact IS NULL) FROM workspace_exports WHERE organization_id=$1`, organizationID).Scan(&retainedReady, &cappedExpired); err != nil || retainedReady != MaxReadyFiles || cappedExpired != 1 {
		t.Fatalf("workspace export artifact cap failed: ready=%d expired=%d err=%v", retainedReady, cappedExpired, err)
	}
	if _, err := service.Download(ctx, foreignOrganizationID, foreignOwnerID, requested.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant workspace download error=%v, want not found", err)
	}
	download, err := service.Download(ctx, organizationID, ownerID, requested.ID)
	if err != nil {
		t.Fatalf("download workspace export: %v", err)
	}
	if !strings.HasSuffix(download.Filename, ".zip") || download.ContentSHA256 != history[0].ContentSHA256 {
		t.Fatalf("unexpected workspace export download metadata: %#v", download)
	}
	files := readWorkspaceExportZip(t, download.Content)
	if !bytes.Contains(files["data/contacts.ndjson"], []byte(`"morgan@portable.test"`)) {
		t.Fatalf("portable contact missing: %s", files["data/contacts.ndjson"])
	}
	portableQuotes := string(files["data/deal_quotes.ndjson"])
	if !strings.Contains(portableQuotes, "Q-PORTABLE-V1") || !strings.Contains(portableQuotes, "Portable quote terms") || !strings.Contains(portableQuotes, "pdf_content") || strings.Contains(portableQuotes, "idempotency_key_hash") || strings.Contains(portableQuotes, "request_sha256") || strings.Contains(portableQuotes, strings.Repeat("b", 64)) {
		t.Fatalf("workspace quote portability/privacy boundary failed: %s", portableQuotes)
	}
	if !strings.Contains(string(files["data/deal_quote_line_items.ndjson"]), "Portable service") {
		t.Fatalf("portable finalized quote line missing: %s", files["data/deal_quote_line_items.ndjson"])
	}
	sharedMessages := string(files["data/email_messages_shared.ndjson"])
	if !strings.Contains(sharedMessages, "Shared customer thread") || strings.Contains(sharedMessages, "Private mailbox thread") || strings.Contains(sharedMessages, "tracking_token") || strings.Contains(sharedMessages, "rfc_message_id") || strings.Contains(sharedMessages, "in_reply_to") || strings.Contains(sharedMessages, "reference_message_ids") || strings.Contains(sharedMessages, "delivery_feedback_email_message_id") {
		t.Fatalf("workspace email privacy boundary failed: %s", sharedMessages)
	}
	sharedReplies := string(files["data/email_reply_requests_shared.ndjson"])
	if !strings.Contains(sharedReplies, "Shared pending reply") || strings.Contains(sharedReplies, "Private pending reply") || strings.Contains(sharedReplies, "idempotency_key_hash") || strings.Contains(sharedReplies, "request_sha256") || strings.Contains(sharedReplies, "rfc_message_id") || strings.Contains(sharedReplies, "in_reply_to") || strings.Contains(sharedReplies, strings.Repeat("a", 64)) {
		t.Fatalf("workspace reply privacy/correlation boundary failed: %s", sharedReplies)
	}
	if _, exists := files["data/customer_email_feedback_events.ndjson"]; exists {
		t.Fatal("workspace export included internal customer feedback correlation ledger")
	}
	emailAccount := string(files["data/email_account_configuration.ndjson"])
	for _, secret := range []string{"encrypted-smtp", "encrypted-imap", "encrypted-access", "encrypted-refresh", "smtp_password_enc", "oauth_access_token_enc"} {
		if strings.Contains(emailAccount, secret) {
			t.Fatalf("workspace export leaked email credential %q: %s", secret, emailAccount)
		}
	}
	var manifestValue manifest
	if err := json.Unmarshal(files["manifest.json"], &manifestValue); err != nil || manifestValue.OmittedPrivateEmailMessages != 1 || manifestValue.OmittedPrivateEmailReplies != 1 || manifestValue.DatasetCounts["contacts"] != 1 || manifestValue.DatasetCounts["deal_quotes"] != 1 || manifestValue.DatasetCounts["email_reply_requests_shared"] != 1 {
		t.Fatalf("unexpected workspace export manifest: manifest=%#v err=%v", manifestValue, err)
	}
	var downloadAudits int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE organization_id=$1 AND event_type='workspace.export_downloaded'`, organizationID).Scan(&downloadAudits); err != nil || downloadAudits != 1 {
		t.Fatalf("workspace export download was not audited: count=%d err=%v", downloadAudits, err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE unclassified_portable_data (id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL)`); err != nil {
		t.Fatalf("create unclassified tenant table: %v", err)
	}
	coverageTx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatalf("begin schema coverage check: %v", err)
	}
	coverageErr := checkSchemaCoverage(ctx, coverageTx)
	_ = coverageTx.Rollback(ctx)
	if !errors.Is(coverageErr, ErrUnclassifiedDataset) || !strings.Contains(coverageErr.Error(), "unclassified_portable_data") {
		t.Fatalf("unclassified tenant table did not fail closed: %v", coverageErr)
	}
	if _, err := pool.Exec(ctx, `DROP TABLE unclassified_portable_data`); err != nil {
		t.Fatalf("remove unclassified tenant table: %v", err)
	}

	service.now = func() time.Time { return history[0].ExpiresAt.Add(time.Second) }
	if _, err := pool.Exec(ctx, `UPDATE workspace_exports SET expires_at=NOW()-INTERVAL '1 second' WHERE id=$1`, requested.ID); err != nil {
		t.Fatalf("age workspace export artifact: %v", err)
	}
	if expiredCount, err := service.ExpireReadyArtifacts(ctx); err != nil || expiredCount != 1 {
		t.Fatalf("expire workspace export artifact: count=%d err=%v", expiredCount, err)
	}
	if _, err := service.Download(ctx, organizationID, ownerID, requested.ID); err != ErrExpired {
		t.Fatalf("expired workspace export download error=%v, want expired", err)
	}
	var artifactBytes int
	if err := pool.QueryRow(ctx, `SELECT COALESCE(octet_length(artifact),0) FROM workspace_exports WHERE id=$1`, requested.ID).Scan(&artifactBytes); err != nil || artifactBytes != 0 {
		t.Fatalf("expired workspace export retained bytes: size=%d err=%v", artifactBytes, err)
	}
}

func readWorkspaceExportZip(t *testing.T, content []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("open workspace export zip: %v", err)
	}
	files := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatalf("open workspace export file %s: %v", file.Name, err)
		}
		value, err := io.ReadAll(opened)
		_ = opened.Close()
		if err != nil {
			t.Fatalf("read workspace export file %s: %v", file.Name, err)
		}
		files[file.Name] = value
	}
	return files
}

func workspaceExportDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse workspace export database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
