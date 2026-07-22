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
	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

func TestSequenceEnrollmentHistoryStableBoundedTenantContinuationAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to sequence enrollment history postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_sequence_history_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create sequence enrollment history schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithSequenceOutcomeSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate sequence enrollment history schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to sequence enrollment history schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, userID, foreignUserID, contactID, foreignContactID, sequenceID, foreignSequenceID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Sequence History',$1) RETURNING id`, "sequence-history-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create sequence history organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign Sequence History',$1) RETURNING id`, "foreign-sequence-history-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign sequence history organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Sequence','Operator') RETURNING id`, "sequence-operator-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("create sequence history user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'owner')`, organizationID, userID); err != nil {
		t.Fatalf("create sequence history membership: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,first_name,last_name) VALUES ($1,'hash','Foreign','Operator') RETURNING id`, "foreign-sequence-operator-"+schema+"@example.test").Scan(&foreignUserID); err != nil {
		t.Fatalf("create foreign sequence history user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,user_id,role) VALUES ($1,$2,'owner')`, foreignOrganizationID, foreignUserID); err != nil {
		t.Fatalf("create foreign sequence history membership: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status) VALUES ($1,'Pilot','Buyer',$2,'lead') RETURNING id`, organizationID, "pilot-history-"+schema+"@example.test").Scan(&contactID); err != nil {
		t.Fatalf("create sequence history contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id,first_name,last_name,email,status) VALUES ($1,'Foreign','Buyer',$2,'lead') RETURNING id`, foreignOrganizationID, "foreign-history-"+schema+"@example.test").Scan(&foreignContactID); err != nil {
		t.Fatalf("create foreign sequence history contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO email_sequences (organization_id,name,status,created_by_user_id) VALUES ($1,'Paged cadence','paused',$2) RETURNING id`, organizationID, userID).Scan(&sequenceID); err != nil {
		t.Fatalf("create paged sequence: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO email_sequences (organization_id,name,status) VALUES ($1,'Foreign cadence','paused') RETURNING id`, foreignOrganizationID).Scan(&foreignSequenceID); err != nil {
		t.Fatalf("create foreign paged sequence: %v", err)
	}

	createdAt := time.Date(2026, 7, 22, 12, 0, 0, 123456000, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_sequence_enrollments (
			organization_id,sequence_id,contact_id,enrolled_by_user_id,status,current_step_order,
			completed_at,completion_reason,created_at,updated_at
		)
		SELECT $1,$2,$3,$4,'completed',1,$5,'finished',$5,$5
		FROM generate_series(1,1001)
	`, organizationID, sequenceID, contactID, userID, createdAt); err != nil {
		t.Fatalf("seed sequence enrollment history: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE email_sequence_enrollments SET enrolled_by_user_id=$3
		WHERE organization_id=$1 AND sequence_id=$2
		  AND id=(SELECT MIN(id) FROM email_sequence_enrollments WHERE organization_id=$1 AND sequence_id=$2)
	`, organizationID, sequenceID, foreignUserID); err != nil {
		t.Fatalf("seed corrupt foreign enroller reference: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_sequence_enrollments (
			organization_id,sequence_id,contact_id,status,current_step_order,completed_at,completion_reason,created_at,updated_at
		) VALUES ($1,$2,$3,'completed',1,$4,'finished',$4,$4)
	`, foreignOrganizationID, foreignSequenceID, foreignContactID, createdAt); err != nil {
		t.Fatalf("seed foreign sequence enrollment history: %v", err)
	}

	var newestID, feedbackMessageID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM email_sequence_enrollments
		WHERE organization_id=$1 AND sequence_id=$2
		ORDER BY created_at DESC,id DESC LIMIT 1
	`, organizationID, sequenceID).Scan(&newestID); err != nil {
		t.Fatalf("load newest sequence enrollment: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO email_messages (
			organization_id,to_email,subject,body,status,direction,from_email,mailbox_user_id,
			provider_message_id,received_at,visibility
		) VALUES ($1,$2,'Delivery feedback','Feedback','received','inbound','mailer-daemon@example.test',$3,$4,$5,'private')
		RETURNING id
	`, organizationID, "pilot-history-"+schema+"@example.test", userID, "sequence-feedback-"+schema, createdAt.Add(time.Minute)).Scan(&feedbackMessageID); err != nil {
		t.Fatalf("create sequence delivery feedback evidence: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_sequence_deliveries (
			organization_id,enrollment_id,step_order,recipient_email,subject,text_body,status,
			attempt_started_at,finalized_at,delivery_outcome,delivery_outcome_at,
			delivery_feedback_email_message_id,delivery_feedback_status_code
		) VALUES
			($1,$2,1,$3,'One','Body','sent',$4,$4,'bounced',$5,$6,'5.1.1'),
			($1,$2,2,$3,'Two','Body','sent',$4,$4,'complaint',$5,$6,'abuse'),
			($1,$2,3,$3,'Three','Body','uncertain',$4,$4,'',NULL,NULL,''),
			($1,$2,4,$3,'Four','Body','suppressed',$4,$4,'',NULL,NULL,''),
			($1,$2,5,$3,'Five','Body','queued',NULL,NULL,'',NULL,NULL,'')
	`, organizationID, newestID, "pilot-history-"+schema+"@example.test", createdAt, createdAt.Add(time.Minute), feedbackMessageID); err != nil {
		t.Fatalf("seed per-enrollment delivery outcomes: %v", err)
	}

	if _, err := pool.Exec(ctx, `ANALYZE email_sequence_enrollments`); err != nil {
		t.Fatalf("analyze sequence enrollment history: %v", err)
	}
	planTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin sequence history plan transaction: %v", err)
	}
	defer func() { _ = planTx.Rollback(context.Background()) }()
	if _, err := planTx.Exec(ctx, `SET LOCAL enable_seqscan=off`); err != nil {
		t.Fatalf("disable sequence scan for sequence history plan: %v", err)
	}
	planRows, err := planTx.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM email_sequence_enrollments
		WHERE organization_id=$1 AND sequence_id=$2
		ORDER BY created_at DESC,id DESC LIMIT 101
	`, organizationID, sequenceID)
	if err != nil {
		t.Fatalf("explain sequence enrollment history query: %v", err)
	}
	plan := make([]string, 0)
	for planRows.Next() {
		var line string
		if err := planRows.Scan(&line); err != nil {
			planRows.Close()
			t.Fatalf("scan sequence enrollment history plan: %v", err)
		}
		plan = append(plan, line)
	}
	planRows.Close()
	if joined := strings.Join(plan, "\n"); !strings.Contains(joined, "idx_email_sequence_enrollments_org_sequence_created") {
		t.Fatalf("sequence enrollment history query did not use cursor index:\n%s", joined)
	}
	if err := planTx.Rollback(ctx); err != nil {
		t.Fatalf("rollback sequence history plan transaction: %v", err)
	}

	service := NewService(pool)
	if _, err := service.ListEnrollmentsBySequence(ctx, organizationID, sequenceID, platformtimeline.Query{Limit: platformtimeline.MaxLimit + 1}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("direct service accepted oversized sequence enrollment page: %v", err)
	}
	first, err := service.ListEnrollmentsBySequence(ctx, organizationID, sequenceID, platformtimeline.Query{Limit: 50})
	if err != nil {
		t.Fatalf("list first sequence enrollment history page: %v", err)
	}
	if len(first.Enrollments) != 50 || !first.Meta.HasMore || first.Meta.NextCursor == "" || first.Meta.Limit != 50 {
		t.Fatalf("unexpected first sequence enrollment history page: rows=%d meta=%+v", len(first.Enrollments), first.Meta)
	}
	if first.Enrollments[0].ID != newestID || first.Enrollments[0].ContactName != "Pilot Buyer" || first.Enrollments[0].ContactEmail == "" || first.Enrollments[0].EnrolledByName != "Sequence Operator" {
		t.Fatalf("unexpected newest sequence enrollment identity: %+v", first.Enrollments[0])
	}
	if newest := first.Enrollments[0]; newest.ProviderAccepted != 2 || newest.BouncedMessages != 1 || newest.Complaints != 1 || newest.SuppressedMessages != 1 || newest.QueuedMessages != 1 || newest.NeedsReview != 1 {
		t.Fatalf("unexpected per-enrollment delivery outcomes: %+v", newest)
	}
	cursor, err := platformtimeline.Decode(first.Meta.NextCursor)
	if err != nil || cursor.ID != first.Enrollments[49].ID || !cursor.CreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected first sequence enrollment cursor: cursor=%+v err=%v", cursor, err)
	}
	repeated, err := service.ListEnrollmentsBySequence(ctx, organizationID, sequenceID, platformtimeline.Query{Limit: 50})
	if err != nil || repeated.Enrollments[0].ID != first.Enrollments[0].ID || repeated.Enrollments[49].ID != first.Enrollments[49].ID {
		t.Fatalf("sequence enrollment first page was unstable: err=%v repeated=%+v", err, repeated.Meta)
	}
	second, err := service.ListEnrollmentsBySequence(ctx, organizationID, sequenceID, platformtimeline.Query{Cursor: &cursor, Limit: 50})
	if err != nil {
		t.Fatalf("list adjacent sequence enrollment history page: %v", err)
	}
	if len(second.Enrollments) != 50 || second.Enrollments[0].ID >= first.Enrollments[49].ID {
		t.Fatalf("unexpected adjacent sequence enrollment history page: first=%d second=%d rows=%d", first.Enrollments[49].ID, second.Enrollments[0].ID, len(second.Enrollments))
	}
	foreign, err := service.ListEnrollmentsBySequence(ctx, organizationID, foreignSequenceID, platformtimeline.Query{})
	if err != nil || len(foreign.Enrollments) != 0 || foreign.Meta.Limit != platformtimeline.DefaultLimit {
		t.Fatalf("foreign sequence enrollment history crossed tenant boundary: page=%+v err=%v", foreign, err)
	}

	seen := make(map[int64]struct{}, 1001)
	page := first
	for {
		for _, enrollment := range page.Enrollments {
			if _, duplicate := seen[enrollment.ID]; duplicate {
				t.Fatalf("sequence enrollment %d appeared on multiple pages", enrollment.ID)
			}
			seen[enrollment.ID] = struct{}{}
		}
		if !page.Meta.HasMore {
			break
		}
		nextQuery, parseErr := platformtimeline.Parse(page.Meta.NextCursor, "50")
		if parseErr != nil {
			t.Fatalf("parse sequence enrollment continuation: %v", parseErr)
		}
		page, err = service.ListEnrollmentsBySequence(ctx, organizationID, sequenceID, nextQuery)
		if err != nil {
			t.Fatalf("continue sequence enrollment history: %v", err)
		}
	}
	if len(seen) != 1001 || len(page.Enrollments) != 1 || page.Meta.HasMore || page.Meta.NextCursor != "" {
		t.Fatalf("unexpected complete sequence enrollment traversal: seen=%d final=%d meta=%+v", len(seen), len(page.Enrollments), page.Meta)
	}
	if oldest := page.Enrollments[0]; oldest.EnrolledByUserID != 0 || oldest.EnrolledByName != "" {
		t.Fatalf("foreign enroller identity leaked through local history: %+v", oldest)
	}
}
