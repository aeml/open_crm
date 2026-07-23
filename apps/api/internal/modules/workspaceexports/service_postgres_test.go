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
	"strconv"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	moduleusers "github.com/aeml/open_crm/apps/api/internal/modules/users"
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

	var organizationID, ownerID, approverID, foreignOrganizationID, foreignOwnerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug,plan,subscription_status) VALUES ('Portable Pilot','Portable Pilot','pro','active') RETURNING id`).Scan(&organizationID); err != nil {
		t.Fatalf("create workspace export organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at) VALUES ('owner@portable.test','secret-password-hash','Portia','Owner',NOW()) RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatalf("create workspace export owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'owner')`, organizationID, ownerID); err != nil {
		t.Fatalf("create workspace export membership: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name,email_verified_at) VALUES ('approver@portable.test','secret-password-hash','Priya','Approver',NOW()) RETURNING id`).Scan(&approverID); err != nil {
		t.Fatalf("create workspace export quote approver: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'admin')`, organizationID, approverID); err != nil {
		t.Fatalf("create workspace export approver membership: %v", err)
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
	var portableContactID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM contacts WHERE organization_id=$1 AND email='morgan@portable.test'`, organizationID).Scan(&portableContactID); err != nil {
		t.Fatalf("load portable workspace contact: %v", err)
	}
	var portableReportID, portableDashboardID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_definitions(
			organization_id,name,source_type,visualization_type,visualization_contract,
			columns_json,filters_json,group_by,aggregation_json,created_by_user_id,updated_by_user_id
		) VALUES($1,'Portable contact dashboard','contacts','bar','grouped_bar_v1','[]','[]','status','{"function":"count","field":""}',$2,$2)
		RETURNING id
	`, organizationID, ownerID).Scan(&portableReportID); err != nil {
		t.Fatalf("seed portable dashboard report: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO custom_report_dashboards(organization_id,revision,updated_by_user_id)
		VALUES($1,2,$2) RETURNING id
	`, organizationID, ownerID).Scan(&portableDashboardID); err != nil {
		t.Fatalf("seed portable shared dashboard: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO custom_report_dashboard_widgets(organization_id,dashboard_id,report_definition_id,position,width)
		VALUES($1,$2,$3,0,'full')
	`, organizationID, portableDashboardID, portableReportID); err != nil {
		t.Fatalf("seed portable dashboard widget: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		WITH schedule AS (
		  INSERT INTO custom_report_schedules(
		    organization_id,report_definition_id,revision,cadence,hour_utc,is_active,next_run_at,
		    created_by_user_id,updated_by_user_id
		  ) VALUES($1,$2,2,'daily',9,TRUE,NOW()+INTERVAL '1 day',$3,$3)
		  RETURNING id
		), recipient AS (
		  INSERT INTO custom_report_schedule_recipients(organization_id,schedule_id,recipient_user_id)
		  SELECT $1,id,$3 FROM schedule RETURNING schedule_id
		), run AS (
		  INSERT INTO custom_report_delivery_runs(
		    organization_id,schedule_id,report_definition_id,schedule_revision,scheduled_for,status,
		    filename,content_sha256,byte_size,row_count,artifact,artifact_expires_at,last_error,completed_at
		  ) SELECT $1,schedule_id,$2,2,NOW()-INTERVAL '1 day','partial','portable-report.csv',
		    repeat('a',64),octet_length(convert_to('scheduled-report-artifact-secret','UTF8')),1,
		    convert_to('scheduled-report-artifact-secret','UTF8'),NOW()+INTERVAL '7 days',
		    'scheduled-report-internal-run-error',NOW()
		  FROM recipient RETURNING id
		)
		INSERT INTO custom_report_recipient_deliveries(
		  organization_id,delivery_run_id,recipient_user_id,status,attempt_count,provider_message_id,last_error,attempted_at
		) SELECT $1,id,$3,'uncertain',1,'scheduled-report-provider-secret','scheduled-report-internal-delivery-error',NOW()
		FROM run
	`, organizationID, portableReportID, ownerID); err != nil {
		t.Fatalf("seed portable scheduled-report evidence: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		WITH definition AS (
			INSERT INTO custom_report_definitions(
				organization_id,name,source_type,visualization_type,visualization_contract,
				columns_json,filters_json,group_by,aggregation_json,created_by_user_id,updated_by_user_id
			) VALUES($1,'Foreign private dashboard','contacts','bar','grouped_bar_v1','[]','[]','status','{"function":"count","field":""}',$2,$2)
			RETURNING id
		), dashboard AS (
			INSERT INTO custom_report_dashboards(organization_id,revision,updated_by_user_id)
			VALUES($1,1,$2) RETURNING id
		)
		INSERT INTO custom_report_dashboard_widgets(organization_id,dashboard_id,report_definition_id,position,width)
		SELECT $1,dashboard.id,definition.id,0,'half' FROM dashboard,definition
	`, foreignOrganizationID, foreignOwnerID); err != nil {
		t.Fatalf("seed foreign dashboard portability boundary: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		WITH form AS (
			INSERT INTO lead_capture_forms (
				organization_id,public_id,name,slug,title,fields_json,consent_text,revision,created_by_user_id,updated_by_user_id
			) VALUES (
				$1,'lf_portable','Portable lead form','portable-lead','Portable lead form',
				'[{"key":"segment","label":"Relationship segment","fieldType":"select","required":true,"mapTo":"custom:relationship_segment","options":["Customer","Partner"]}]'::jsonb,
				'I agree that Portable Pilot may contact me.',3,$2,$2
			) RETURNING id
		), submission AS (
			INSERT INTO lead_capture_submissions (
				organization_id,form_id,payload_json,source_url,lead_source,consent_text_snapshot,consented_at,
				form_revision,field_mapping_snapshot_json,
				review_status,review_version,review_note,reviewed_at,reviewed_by_user_id
			)
			SELECT $1,id,'{"message":"Portable inquiry"}'::jsonb,'https://portable.test/contact','Website',
				'I agree that Portable Pilot may contact me.',NOW(),
				3,'[{"formFieldKey":"segment","destination":"custom:relationship_segment","dataType":"select"}]'::jsonb,
				'legitimate',1,'Verified pilot inquiry.',NOW(),$2
			FROM form RETURNING id,form_id
		), review_request AS (
			INSERT INTO lead_capture_submission_review_requests (
				organization_id,submission_id,key_digest,request_sha256,result_review_version
			)
			SELECT $1,id,repeat('3',64),repeat('4',64),1 FROM submission
			RETURNING submission_id
		)
		INSERT INTO lead_capture_submission_challenges (
			organization_id,form_id,token_digest,consent_text_snapshot,request_digest,submission_id,
			form_revision,issued_at,not_before,expires_at,consumed_at
		)
		SELECT $1,form_id,repeat('1',64),'I agree that Portable Pilot may contact me.',repeat('2',64),id,
			3,NOW()-INTERVAL '3 seconds',NOW()-INTERVAL '1 second',NOW()+INTERVAL '30 minutes',NOW()
		FROM submission JOIN review_request ON review_request.submission_id=submission.id
	`, organizationID, ownerID); err != nil {
		t.Fatalf("seed portable lead consent evidence: %v", err)
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
			'morgan@portable.test','Portia Owner','USD',125,0,0,125,CURRENT_DATE-1,
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
	var replacementQuoteID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deal_quotes (
			organization_id,deal_id,version,quote_number,organization_name,deal_name,recipient_name,
			recipient_email,prepared_by_name,currency,subtotal,discount_total,tax_total,total,
			quote_base_currency,exchange_rate_to_base,exchange_rate_effective_date,exchange_rate_source,total_in_base_currency,valid_until,
			terms,pdf_filename,pdf_content,pdf_sha256,idempotency_key_hash,request_sha256,created_by_user_id,
			created_at,reissued_from_quote_id
		)
		SELECT organization_id,deal_id,2,'Q-PORTABLE-V2',organization_name,deal_name,recipient_name,
			recipient_email,prepared_by_name,currency,subtotal,discount_total,tax_total,total,
			'USD',1,CURRENT_DATE,'identity',total,CURRENT_DATE+60,
			terms,'quote-portable-v2.pdf',convert_to(repeat('R',100),'UTF8'),repeat('d',64),repeat('e',64),
			repeat('f',64),$2,NOW(),id
		FROM deal_quotes WHERE organization_id=$1 AND id=$3
		RETURNING id
	`, organizationID, ownerID, quoteID).Scan(&replacementQuoteID); err != nil {
		t.Fatalf("seed portable quote reissue lineage: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deal_quote_line_items (
			organization_id,quote_id,source_line_item_id,source_catalog_item_id,name,sku,item_type,
			quantity,unit_name,unit_price,subtotal,discount_amount,tax_rate,tax_amount,total,currency,position
		)
		SELECT organization_id,$3,source_line_item_id,source_catalog_item_id,name,sku,item_type,
			quantity,unit_name,unit_price,subtotal,discount_amount,tax_rate,tax_amount,total,currency,position
		FROM deal_quote_line_items WHERE organization_id=$1 AND quote_id=$2
	`, organizationID, quoteID, replacementQuoteID); err != nil {
		t.Fatalf("seed portable replacement quote line: %v", err)
	}
	var quoteTemplateID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO quote_templates (
			organization_id,name,terms,default_validity_days,delivery_subject_template,
			delivery_message_template,request_signature,requires_approval,created_by_user_id,updated_by_user_id
		) VALUES ($1,'Portable proposal','Portable quote terms',30,'Quote {{quote_number}}','Hi {{recipient_name}}',TRUE,TRUE,$2,$2)
		RETURNING id
	`, organizationID, ownerID).Scan(&quoteTemplateID); err != nil {
		t.Fatalf("seed portable quote template: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_quote_policies (organization_id,approval_required,updated_by_user_id) VALUES ($1,TRUE,$2)`, organizationID, ownerID); err != nil {
		t.Fatalf("seed portable quote approval policy: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE deal_quotes SET source_quote_template_id=$2,quote_template_name='Portable proposal',quote_template_revision=1,
			delivery_subject_template='Quote {{quote_number}}',delivery_message_template='Hi {{recipient_name}}',
			delivery_subject_default='Portable finalized quote',delivery_message_default='Hi Portable Buyer',
			template_request_signature=TRUE,template_requires_approval=TRUE
		WHERE organization_id=$1 AND id=$3
	`, organizationID, quoteTemplateID, replacementQuoteID); err != nil {
		t.Fatalf("seed portable quote template snapshot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deal_quote_approvals (
			organization_id,deal_id,quote_id,quote_pdf_sha256,status,requested_by_user_id,decided_by_user_id,
			decided_at,decision_note,decision_key_hash,decision_request_sha256
		) VALUES ($1,(SELECT deal_id FROM deal_quotes WHERE organization_id=$1 AND id=$3),$3,repeat('d',64),'approved',$2,$4,
			NOW(),'Scope and totals approved.',repeat('3',64),repeat('4',64))
	`, organizationID, ownerID, replacementQuoteID, approverID); err != nil {
		t.Fatalf("seed portable quote approval evidence: %v", err)
	}
	var quoteDealID int64
	if err := pool.QueryRow(ctx, `SELECT deal_id FROM deal_quotes WHERE organization_id=$1 AND id=$2`, organizationID, quoteID).Scan(&quoteDealID); err != nil {
		t.Fatalf("load portable quote deal: %v", err)
	}
	var conversionStageID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deal_stages (organization_id,pipeline_id,name,position,probability_percent,is_closed,is_won)
		SELECT $1,stage.pipeline_id,'Closed Won',2,100,TRUE,TRUE
		FROM deals deal
		JOIN deal_stages stage ON stage.organization_id=deal.organization_id AND stage.id=deal.stage_id
		WHERE deal.organization_id=$1 AND deal.id=$2
		RETURNING id
	`, organizationID, quoteDealID).Scan(&conversionStageID); err != nil {
		t.Fatalf("seed portable won stage: %v", err)
	}
	var signatureRequestID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO deal_signature_requests (
			organization_id,deal_id,quote_id,signer_name,signer_email,status,provider,quote_file_name,
			signed_name,consent_text_snapshot,consented_at,authentication_method,
			completion_idempotency_key_hash,completion_request_sha256,sent_at,signed_at,
			certificate_filename,certificate_content,certificate_sha256,created_by_user_id
		) VALUES (
			$1,$2,$3,'Morgan Pilot','morgan@portable.test','signed','open_crm_native','quote-portable-v2.pdf',
			'Morgan Pilot','I agree to use an electronic signature for immutable quote Q-PORTABLE-V2.',NOW(),'recipient_email_link',
			repeat('7',64),repeat('8',64),NOW()-INTERVAL '1 hour',NOW()-INTERVAL '30 minutes',
			'signature-certificate-q-portable-v2.pdf',convert_to(repeat('C',100),'UTF8'),repeat('9',64),$4
		) RETURNING id
	`, organizationID, quoteDealID, replacementQuoteID, ownerID).Scan(&signatureRequestID); err != nil {
		t.Fatalf("seed portable quote signature evidence: %v", err)
	}
	var conversionActivityID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (organization_id,entity_type,entity_id,actor_user_id,action,summary)
		VALUES ($1,'deal',$2,$3,'deal.stage_changed','Stage changed to Closed Won (Best solution fit)')
		RETURNING id
	`, organizationID, quoteDealID, ownerID).Scan(&conversionActivityID); err != nil {
		t.Fatalf("seed portable quote conversion activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE deals
		SET stage_id=$3,status='won',close_reason_code='solution_fit',close_reason_label='Best solution fit',
		    close_notes='Signed scope accepted.',closed_at=NOW(),closed_by_user_id=$4
		WHERE organization_id=$1 AND id=$2
	`, organizationID, quoteDealID, conversionStageID, ownerID); err != nil {
		t.Fatalf("seed portable won deal evidence: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE deal_signature_requests
		SET conversion_stage_id=$2,conversion_stage_name='Closed Won',
		    conversion_close_reason_code='solution_fit',conversion_close_reason_label='Best solution fit',
		    conversion_close_notes='Signed scope accepted.',conversion_activity_id=$3,
		    converted_by_user_id=$4,converted_at=NOW(),
		    conversion_idempotency_key_hash=repeat('6',64),conversion_request_sha256=repeat('5',64)
		WHERE organization_id=$1 AND id=$5
	`, organizationID, conversionStageID, conversionActivityID, ownerID, signatureRequestID); err != nil {
		t.Fatalf("seed portable quote conversion evidence: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO deal_quote_deliveries (
			organization_id,deal_id,quote_id,signature_request_id,actor_user_id,sender_email,recipient_email,subject,message_body,
			rfc_message_id,access_token_digest,access_expires_at,idempotency_key_hash,request_sha256,status,
			claimed_at,finalized_at,sent_at,first_accessed_at,last_accessed_at,access_count,
			first_downloaded_at,last_downloaded_at,download_count,receipt_confirmed_at
		) VALUES (
			$1,$2,$3,$4,$5,'owner@portable.test','morgan@portable.test','Portable finalized quote','Please review and sign the portable quote.',
			'<portable-quote@crm.example.test>',repeat('d',64),NOW()+INTERVAL '30 days',repeat('e',64),repeat('f',64),'sent',
			NOW()-INTERVAL '1 hour',NOW()-INTERVAL '1 hour',NOW()-INTERVAL '1 hour',NOW()-INTERVAL '30 minutes',NOW()-INTERVAL '10 minutes',2,
			NOW()-INTERVAL '5 minutes',NOW()-INTERVAL '5 minutes',1,NOW()
		)
	`, organizationID, quoteDealID, replacementQuoteID, signatureRequestID, ownerID); err != nil {
		t.Fatalf("seed portable quote delivery evidence: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_messages (organization_id,to_email,subject,body,status,visibility,rfc_message_id,in_reply_to,reference_message_ids) VALUES
			($1,'shared@portable.test','Shared customer thread',E'Shared body\n\nUnsubscribe: https://crm.example.test/u/shared-unsubscribe-secret','sent','shared','<shared@crm.example.test>','<prior@buyer.test>',ARRAY['<older@buyer.test>']),
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
		INSERT INTO record_email_deliveries (
		  organization_id,entity_type,entity_id,recipient_contact_id,actor_user_id,sender_email,recipient_email,
		  subject,text_body,rfc_message_id,idempotency_key_hash,request_sha256,tracking_token,tracked_links_json,
		  html_body,list_unsubscribe_url
		) VALUES ($1,'contact',$2,$2,$3,'owner@portable.test','morgan@portable.test',
		  'Portable record email intent',E'Portable durable body\n\nUnsubscribe: https://crm.example.test/u/unsubscribe-secret',
		  '<portable-record-email@crm.example.test>',$4,$5,'','[]'::jsonb,
		  '<p>Portable durable body</p><img src="https://crm.example.test/open/html-tracking-secret">',
		  'https://crm.example.test/u/unsubscribe-secret')
	`, organizationID, portableContactID, ownerID, strings.Repeat("e", 64), strings.Repeat("f", 64)); err != nil {
		t.Fatalf("seed portable record email intent: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_email_accounts (
			organization_id,user_id,from_email,from_name,smtp_host,smtp_port,smtp_username,smtp_password_enc,
			imap_host,imap_port,imap_username,imap_password_enc,oauth_access_token_enc,oauth_refresh_token_enc
		) VALUES ($1,$2,'owner@portable.test','Portia','smtp.test',587,'smtp-owner','encrypted-smtp','imap.test',993,'imap-owner','encrypted-imap','encrypted-access','encrypted-refresh')
	`, organizationID, ownerID); err != nil {
		t.Fatalf("seed portable workspace data: %v", err)
	}
	const retainedImportSecret = "retained-import-source-must-not-be-portable"
	if _, err := pool.Exec(ctx, `
		INSERT INTO import_batches (
		  organization_id,created_by_user_id,entity_type,original_filename,idempotency_key,
		  source_sha256,mapping_json,status,total_rows,source_csv,source_expires_at
		) VALUES ($1,$2,'contacts','portable-import.csv','portable-import-request',
		  $3,'{}'::jsonb,'processing',1,$4,NOW()+INTERVAL '7 days')
	`, organizationID, ownerID, strings.Repeat("a", 64), []byte(retainedImportSecret)); err != nil {
		t.Fatalf("seed retained import source: %v", err)
	}
	roleChanged, err := moduleusers.NewService(pool).UpdateRole(ctx, organizationID, approverID, ownerID, "member")
	if err != nil || roleChanged.Role != "member" {
		t.Fatalf("seed portable transactional role history: user=%#v err=%v", roleChanged, err)
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
		var lastError string
		_ = pool.QueryRow(ctx, `SELECT COALESCE(last_error,'') FROM background_jobs WHERE organization_id=$1 AND job_type=$2 ORDER BY id DESC LIMIT 1`, organizationID, JobType).Scan(&lastError)
		t.Fatalf("generate workspace export: summary=%#v err=%v job_error=%s", summary, err, lastError)
	}
	history, err := service.List(ctx, organizationID)
	if err != nil || len(history) != 4 || history[0].ID != requested.ID || history[0].Status != "ready" || history[0].ContentSHA256 == "" || history[0].ByteSize <= 0 || history[0].DatasetCounts["contacts"] != 1 || history[0].DatasetCounts["deal_quotes"] != 2 || history[0].DatasetCounts["deal_quote_line_items"] != 2 || history[0].DatasetCounts["deal_quote_deliveries"] != 1 || history[0].DatasetCounts["deal_quote_approvals"] != 1 || history[0].DatasetCounts["quote_templates"] != 1 || history[0].DatasetCounts["organization_quote_policies"] != 1 || history[0].DatasetCounts["deal_signature_requests"] != 1 || history[0].DatasetCounts["email_messages_shared"] != 1 || history[0].DatasetCounts["custom_report_definitions"] != 1 || history[0].DatasetCounts["custom_report_dashboards"] != 1 || history[0].DatasetCounts["custom_report_dashboard_widgets"] != 1 || history[0].DatasetCounts["custom_report_schedules"] != 1 || history[0].DatasetCounts["custom_report_schedule_recipients"] != 1 || history[0].DatasetCounts["custom_report_delivery_runs"] != 1 || history[0].DatasetCounts["custom_report_recipient_deliveries"] != 1 {
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
	portableReportDefinitions := string(files["data/custom_report_definitions.ndjson"])
	portableDashboards := string(files["data/custom_report_dashboards.ndjson"])
	portableDashboardWidgets := string(files["data/custom_report_dashboard_widgets.ndjson"])
	if !strings.Contains(portableReportDefinitions, "Portable contact dashboard") || strings.Contains(portableReportDefinitions, "Foreign private dashboard") || !strings.Contains(portableDashboards, `"revision": 2`) || !strings.Contains(portableDashboardWidgets, `"width": "full"`) || !strings.Contains(portableDashboardWidgets, `"report_definition_id": `+strconv.FormatInt(portableReportID, 10)) {
		t.Fatalf("portable shared dashboard missing or cross-tenant: definitions=%s dashboards=%s widgets=%s", portableReportDefinitions, portableDashboards, portableDashboardWidgets)
	}
	portableSchedules := string(files["data/custom_report_schedules.ndjson"])
	portableScheduleRecipients := string(files["data/custom_report_schedule_recipients.ndjson"])
	portableDeliveryRuns := string(files["data/custom_report_delivery_runs.ndjson"])
	portableRecipientDeliveries := string(files["data/custom_report_recipient_deliveries.ndjson"])
	if !strings.Contains(portableSchedules, `"cadence": "daily"`) || !strings.Contains(portableScheduleRecipients, `"recipient_user_id": `+strconv.FormatInt(ownerID, 10)) || !strings.Contains(portableDeliveryRuns, `"status": "partial"`) || !strings.Contains(portableDeliveryRuns, `"row_count": 1`) || !strings.Contains(portableRecipientDeliveries, `"status": "uncertain"`) {
		t.Fatalf("portable scheduled-report configuration/evidence missing: schedules=%s recipients=%s runs=%s deliveries=%s", portableSchedules, portableScheduleRecipients, portableDeliveryRuns, portableRecipientDeliveries)
	}
	for _, secret := range []string{"scheduled-report-artifact-secret", "scheduled-report-provider-secret", "scheduled-report-internal-run-error", "scheduled-report-internal-delivery-error", "provider_message_id", "last_error", `"artifact"`} {
		if strings.Contains(portableDeliveryRuns, secret) || strings.Contains(portableRecipientDeliveries, secret) {
			t.Fatalf("workspace export leaked scheduled-report internal data %q", secret)
		}
	}
	portableImports := string(files["data/import_batches.ndjson"])
	if !strings.Contains(portableImports, "portable-import.csv") || strings.Contains(portableImports, retainedImportSecret) || strings.Contains(portableImports, "source_csv") || strings.Contains(portableImports, "source_expires_at") {
		t.Fatalf("portable import ledger omitted history or leaked retained source: %s", portableImports)
	}
	portableAuditEvents := string(files["data/audit_events.ndjson"])
	if !strings.Contains(portableAuditEvents, `"event_type": "workspace.export_requested"`) || !strings.Contains(portableAuditEvents, `"event_type": "user.role_changed"`) || !strings.Contains(portableAuditEvents, `"previousRole": "admin"`) || !strings.Contains(portableAuditEvents, `"role": "member"`) || strings.Contains(portableAuditEvents, "idempotency_key") {
		t.Fatalf("portable append-only audit history missing or leaked request correlation: %s", portableAuditEvents)
	}
	portableMembers := string(files["data/members.ndjson"])
	if !strings.Contains(portableMembers, "approver@portable.test") || !strings.Contains(portableMembers, `"role": "member"`) || !strings.Contains(portableMembers, `"membership_status": "active"`) || strings.Contains(portableMembers, "owner@foreign.test") {
		t.Fatalf("portable current membership state is incomplete or cross-tenant: %s", portableMembers)
	}
	portableLeadForms := string(files["data/lead_capture_forms.ndjson"])
	portableLeadSubmissions := string(files["data/lead_capture_submissions.ndjson"])
	if !strings.Contains(portableLeadForms, "I agree that Portable Pilot may contact me.") || !strings.Contains(portableLeadForms, `"revision": 3`) || !strings.Contains(portableLeadForms, "custom:relationship_segment") || !strings.Contains(portableLeadSubmissions, "I agree that Portable Pilot may contact me.") || !strings.Contains(portableLeadSubmissions, "consented_at") || !strings.Contains(portableLeadSubmissions, "Portable inquiry") || !strings.Contains(portableLeadSubmissions, "Verified pilot inquiry.") || !strings.Contains(portableLeadSubmissions, `"review_status": "legitimate"`) || !strings.Contains(portableLeadSubmissions, `"form_revision": 3`) || !strings.Contains(portableLeadSubmissions, `"field_mapping_snapshot_json"`) || !strings.Contains(portableLeadSubmissions, `"formFieldKey": "segment"`) || !strings.Contains(portableLeadSubmissions, `"dataType": "select"`) {
		t.Fatalf("portable lead consent evidence missing: forms=%s submissions=%s", portableLeadForms, portableLeadSubmissions)
	}
	if _, exists := files["data/lead_capture_submission_challenges.ndjson"]; exists {
		t.Fatal("workspace export included internal public challenge ledger")
	}
	if _, exists := files["data/lead_capture_submission_review_requests.ndjson"]; exists {
		t.Fatal("workspace export included internal lead review request ledger")
	}
	for _, secret := range []string{strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64), strings.Repeat("4", 64), "token_digest", "request_digest", "last_review_key_digest", "last_review_request_sha256"} {
		if strings.Contains(portableLeadForms, secret) || strings.Contains(portableLeadSubmissions, secret) {
			t.Fatalf("workspace export leaked public challenge correlation %q", secret)
		}
	}
	portableQuotes := string(files["data/deal_quotes.ndjson"])
	if !strings.Contains(portableQuotes, "Q-PORTABLE-V1") || !strings.Contains(portableQuotes, "Q-PORTABLE-V2") || !strings.Contains(portableQuotes, `"reissued_from_quote_id": `+strconv.FormatInt(quoteID, 10)) || !strings.Contains(portableQuotes, `"quote_base_currency": "USD"`) || !strings.Contains(portableQuotes, `"exchange_rate_source": "identity"`) || !strings.Contains(portableQuotes, `"total_in_base_currency": 125.00`) || !strings.Contains(portableQuotes, "Portable quote terms") || !strings.Contains(portableQuotes, "pdf_content") || strings.Contains(portableQuotes, "idempotency_key_hash") || strings.Contains(portableQuotes, "request_sha256") || strings.Contains(portableQuotes, strings.Repeat("b", 64)) || strings.Contains(portableQuotes, strings.Repeat("e", 64)) {
		t.Fatalf("workspace quote portability/privacy boundary failed: %s", portableQuotes)
	}
	if !strings.Contains(string(files["data/deal_quote_line_items.ndjson"]), "Portable service") {
		t.Fatalf("portable finalized quote line missing: %s", files["data/deal_quote_line_items.ndjson"])
	}
	portableTemplates := string(files["data/quote_templates.ndjson"])
	portablePolicy := string(files["data/organization_quote_policies.ndjson"])
	portableApprovals := string(files["data/deal_quote_approvals.ndjson"])
	if !strings.Contains(portableTemplates, "Portable proposal") || !strings.Contains(portableTemplates, "Portable quote terms") || !strings.Contains(portablePolicy, `"approval_required": true`) || !strings.Contains(portableApprovals, "Scope and totals approved.") || !strings.Contains(portableApprovals, strings.Repeat("d", 64)) {
		t.Fatalf("portable quote preparation/approval evidence missing: templates=%s policy=%s approvals=%s", portableTemplates, portablePolicy, portableApprovals)
	}
	for _, secret := range []string{"decision_key_hash", "decision_request_sha256", strings.Repeat("3", 64), strings.Repeat("4", 64)} {
		if strings.Contains(portableApprovals, secret) {
			t.Fatalf("workspace export leaked quote approval replay correlation %q: %s", secret, portableApprovals)
		}
	}
	portableDeliveries := string(files["data/deal_quote_deliveries.ndjson"])
	if !strings.Contains(portableDeliveries, "Portable finalized quote") || !strings.Contains(portableDeliveries, `"status": "sent"`) || !strings.Contains(portableDeliveries, `"access_count": 2`) || !strings.Contains(portableDeliveries, `"download_count": 1`) || !strings.Contains(portableDeliveries, "receipt_confirmed_at") {
		t.Fatalf("portable quote delivery evidence missing: %s", portableDeliveries)
	}
	for _, secret := range []string{"access_token_digest", "idempotency_key_hash", "request_sha256", "rfc_message_id", "provider_message_id", "provider_thread_id", strings.Repeat("d", 64), strings.Repeat("e", 64), strings.Repeat("f", 64), "portable-quote@crm.example.test"} {
		if strings.Contains(portableDeliveries, secret) {
			t.Fatalf("workspace export leaked quote delivery correlation %q: %s", secret, portableDeliveries)
		}
	}
	portableSignatures := string(files["data/deal_signature_requests.ndjson"])
	if !strings.Contains(portableSignatures, "signature-certificate-q-portable-v2.pdf") || !strings.Contains(portableSignatures, strings.Repeat("9", 64)) || !strings.Contains(portableSignatures, "recipient_email_link") || !strings.Contains(portableSignatures, "consent_text_snapshot") || !strings.Contains(portableSignatures, "certificate_content") || !strings.Contains(portableSignatures, "Closed Won") || !strings.Contains(portableSignatures, "Best solution fit") || !strings.Contains(portableSignatures, "Signed scope accepted.") || !strings.Contains(portableSignatures, "conversion_activity_id") || !strings.Contains(portableSignatures, "converted_at") {
		t.Fatalf("portable quote signature evidence missing: %s", portableSignatures)
	}
	for _, secret := range []string{"completion_idempotency_key_hash", "completion_request_sha256", "conversion_idempotency_key_hash", "conversion_request_sha256", strings.Repeat("7", 64), strings.Repeat("8", 64), strings.Repeat("6", 64), strings.Repeat("5", 64)} {
		if strings.Contains(portableSignatures, secret) {
			t.Fatalf("workspace export leaked signature replay correlation %q: %s", secret, portableSignatures)
		}
	}
	sharedMessages := string(files["data/email_messages_shared.ndjson"])
	if !strings.Contains(sharedMessages, "Shared customer thread") || !strings.Contains(sharedMessages, "Shared body") || strings.Contains(sharedMessages, "shared-unsubscribe-secret") || strings.Contains(sharedMessages, "Private mailbox thread") || strings.Contains(sharedMessages, "tracking_token") || strings.Contains(sharedMessages, "rfc_message_id") || strings.Contains(sharedMessages, "in_reply_to") || strings.Contains(sharedMessages, "reference_message_ids") || strings.Contains(sharedMessages, "delivery_feedback_email_message_id") {
		t.Fatalf("workspace email privacy boundary failed: %s", sharedMessages)
	}
	sharedReplies := string(files["data/email_reply_requests_shared.ndjson"])
	if !strings.Contains(sharedReplies, "Shared pending reply") || strings.Contains(sharedReplies, "Private pending reply") || strings.Contains(sharedReplies, "idempotency_key_hash") || strings.Contains(sharedReplies, "request_sha256") || strings.Contains(sharedReplies, "rfc_message_id") || strings.Contains(sharedReplies, "in_reply_to") || strings.Contains(sharedReplies, strings.Repeat("a", 64)) {
		t.Fatalf("workspace reply privacy/correlation boundary failed: %s", sharedReplies)
	}
	recordDeliveries := string(files["data/record_email_deliveries.ndjson"])
	if !strings.Contains(recordDeliveries, "Portable record email intent") || !strings.Contains(recordDeliveries, "Portable durable body") || strings.Contains(recordDeliveries, "idempotency_key_hash") || strings.Contains(recordDeliveries, "request_sha256") || strings.Contains(recordDeliveries, "tracking_token") || strings.Contains(recordDeliveries, "tracked_links_json") || strings.Contains(recordDeliveries, "rfc_message_id") || strings.Contains(recordDeliveries, "unsubscribe-secret") || strings.Contains(recordDeliveries, "html-tracking-secret") || strings.Contains(recordDeliveries, strings.Repeat("e", 64)) {
		t.Fatalf("workspace record email correlation boundary failed: %s", recordDeliveries)
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
	if err := json.Unmarshal(files["manifest.json"], &manifestValue); err != nil || manifestValue.OmittedPrivateEmailMessages != 1 || manifestValue.OmittedPrivateEmailReplies != 1 || manifestValue.DatasetCounts["members"] != 2 || manifestValue.DatasetCounts["contacts"] != 1 || manifestValue.DatasetCounts["deal_quotes"] != 2 || manifestValue.DatasetCounts["deal_quote_deliveries"] != 1 || manifestValue.DatasetCounts["deal_quote_approvals"] != 1 || manifestValue.DatasetCounts["quote_templates"] != 1 || manifestValue.DatasetCounts["organization_quote_policies"] != 1 || manifestValue.DatasetCounts["deal_signature_requests"] != 1 || manifestValue.DatasetCounts["lead_capture_forms"] != 1 || manifestValue.DatasetCounts["lead_capture_submissions"] != 1 || manifestValue.DatasetCounts["email_reply_requests_shared"] != 1 || manifestValue.DatasetCounts["record_email_deliveries"] != 1 || manifestValue.DatasetCounts["custom_report_definitions"] != 1 || manifestValue.DatasetCounts["custom_report_dashboards"] != 1 || manifestValue.DatasetCounts["custom_report_dashboard_widgets"] != 1 || manifestValue.DatasetCounts["custom_report_schedules"] != 1 || manifestValue.DatasetCounts["custom_report_schedule_recipients"] != 1 || manifestValue.DatasetCounts["custom_report_delivery_runs"] != 1 || manifestValue.DatasetCounts["custom_report_recipient_deliveries"] != 1 {
		t.Fatalf("unexpected workspace export manifest: manifest=%#v err=%v", manifestValue, err)
	}
	if manifestValue.DatasetCounts["audit_events"] < 1 {
		t.Fatalf("workspace export manifest omitted append-only audit history: %#v", manifestValue.DatasetCounts)
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
