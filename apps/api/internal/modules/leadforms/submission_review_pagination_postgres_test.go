package leadforms

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

func TestLeadSubmissionReviewStableBoundedContinuationAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to lead review pagination postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_lead_review_page_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create lead review pagination schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := databaseURLWithLeadFormSearchPath(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate lead review pagination schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to lead review pagination schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Paged Lead Review',$1) RETURNING id`, "paged-lead-review-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create paged lead review organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name,slug) VALUES ('Foreign Lead Review',$1) RETURNING id`, "foreign-lead-review-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign lead review organization: %v", err)
	}
	var formAID, formBID, foreignFormID int64
	slugSuffix := strings.ReplaceAll(schema, "_", "-")
	for _, target := range []struct {
		organizationID int64
		publicID       string
		slug           string
		name           string
		id             *int64
	}{
		{organizationID, "lf_page_a_" + schema, "page-a-" + slugSuffix, "Paged form A", &formAID},
		{organizationID, "lf_page_b_" + schema, "page-b-" + slugSuffix, "Paged form B", &formBID},
		{foreignOrganizationID, "lf_page_foreign_" + schema, "page-foreign-" + slugSuffix, "Foreign form", &foreignFormID},
	} {
		if err := pool.QueryRow(ctx, `
			INSERT INTO lead_capture_forms (organization_id,public_id,name,slug,title,fields_json,success_message,source_label)
			VALUES ($1,$2,$3,$4,$3,'[]'::jsonb,'Thanks','Browser') RETURNING id
		`, target.organizationID, target.publicID, target.name, target.slug).Scan(target.id); err != nil {
			t.Fatalf("create lead review form %q: %v", target.name, err)
		}
	}

	createdAt := time.Date(2026, 7, 1, 12, 0, 0, 123456000, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_capture_submissions (
		  organization_id,form_id,payload_json,review_status,review_version,reviewed_at,created_at
		)
		SELECT $1,CASE WHEN series % 2=0 THEN $2::bigint ELSE $3::bigint END,
		       jsonb_build_object('message','Review ' || series),
		       CASE WHEN series <= 701 THEN 'unreviewed' WHEN series <= 901 THEN 'spam' ELSE 'legitimate' END,
		       CASE WHEN series <= 701 THEN 0 ELSE 1 END,
		       CASE WHEN series <= 701 THEN NULL ELSE $4::timestamptz END,
		       $4
		FROM generate_series(1,1001) AS series
	`, organizationID, formAID, formBID, createdAt); err != nil {
		t.Fatalf("seed paged lead reviews: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lead_capture_submissions (organization_id,form_id,payload_json,created_at)
		VALUES ($1,$2,jsonb_build_object('message','Foreign sentinel'),$3)
	`, foreignOrganizationID, foreignFormID, createdAt); err != nil {
		t.Fatalf("seed foreign lead review: %v", err)
	}

	expectedRows, err := pool.Query(ctx, `
		SELECT id FROM lead_capture_submissions
		WHERE organization_id=$1 AND form_id=$2 AND review_status='unreviewed'
		ORDER BY created_at DESC,id DESC
	`, organizationID, formAID)
	if err != nil {
		t.Fatalf("load expected lead review order: %v", err)
	}
	expectedIDs := make([]int64, 0, 400)
	for expectedRows.Next() {
		var id int64
		if err := expectedRows.Scan(&id); err != nil {
			expectedRows.Close()
			t.Fatalf("scan expected lead review id: %v", err)
		}
		expectedIDs = append(expectedIDs, id)
	}
	if err := expectedRows.Err(); err != nil {
		expectedRows.Close()
		t.Fatalf("iterate expected lead review ids: %v", err)
	}
	expectedRows.Close()
	if len(expectedIDs) != 350 {
		t.Fatalf("expected 350 unreviewed form-A submissions, got %d", len(expectedIDs))
	}

	var expectedCounts SubmissionReviewCounts
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE review_status='unreviewed')::int,
		       COUNT(*) FILTER (WHERE review_status='legitimate')::int,
		       COUNT(*) FILTER (WHERE review_status='spam')::int
		FROM lead_capture_submissions WHERE organization_id=$1 AND form_id=$2
	`, organizationID, formAID).Scan(&expectedCounts.Unreviewed, &expectedCounts.Legitimate, &expectedCounts.Spam); err != nil {
		t.Fatalf("count expected lead review statuses: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE lead_capture_submissions`); err != nil {
		t.Fatalf("analyze lead review fixtures: %v", err)
	}
	planRows, err := pool.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id FROM lead_capture_submissions
		WHERE organization_id=$1 AND form_id=$2 AND review_status='unreviewed'
		ORDER BY created_at DESC,id DESC LIMIT 101
	`, organizationID, formAID)
	if err != nil {
		t.Fatalf("explain lead review cursor query: %v", err)
	}
	plan := make([]string, 0)
	for planRows.Next() {
		var line string
		if err := planRows.Scan(&line); err != nil {
			planRows.Close()
			t.Fatalf("scan lead review cursor plan: %v", err)
		}
		plan = append(plan, line)
	}
	planRows.Close()
	if joined := strings.Join(plan, "\n"); !strings.Contains(joined, "idx_lead_capture_submissions_org_form_review_created") {
		t.Fatalf("lead review query did not use combined cursor index:\n%s", joined)
	}

	service := NewService(pool)
	if _, err := service.ListSubmissionReviews(ctx, organizationID, SubmissionReviewQuery{Limit: platformtimeline.MaxLimit + 1}); !errors.Is(err, ErrInvalidReview) {
		t.Fatalf("direct service accepted oversized review page: %v", err)
	}
	query := SubmissionReviewQuery{Status: ReviewStatusUnreviewed, FormID: formAID, Limit: 100}
	started := time.Now()
	page, err := service.ListSubmissionReviews(ctx, organizationID, query)
	if err != nil {
		t.Fatalf("list first lead review page: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("first 100-row lead review page took %s, budget is 2s", elapsed)
	}
	if len(page.Submissions) != 100 || !page.Meta.HasMore || page.Meta.NextCursor == "" || page.Meta.Limit != 100 || page.Limit != 100 || page.Counts != expectedCounts {
		t.Fatalf("unexpected first lead review page: submissions=%d page=%+v counts=%+v want=%+v", len(page.Submissions), page.Meta, page.Counts, expectedCounts)
	}
	firstCursor, err := platformtimeline.Decode(page.Meta.NextCursor)
	if err != nil || firstCursor.ID != page.Submissions[99].ID || !firstCursor.CreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected first lead review cursor: cursor=%+v err=%v", firstCursor, err)
	}

	seen := make([]int64, 0, len(expectedIDs))
	for _, submission := range page.Submissions {
		seen = append(seen, submission.ID)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE lead_capture_submissions
		SET review_status='spam',review_version=1,reviewed_at=clock_timestamp()
		WHERE organization_id=$1 AND id=$2
	`, organizationID, page.Submissions[0].ID); err != nil {
		t.Fatalf("mutate first-page lead review: %v", err)
	}
	var insertedAfterFirstPageID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO lead_capture_submissions (organization_id,form_id,payload_json,created_at)
		VALUES ($1,$2,jsonb_build_object('message','Arrived after page one'),$3) RETURNING id
	`, organizationID, formAID, createdAt.Add(24*time.Hour)).Scan(&insertedAfterFirstPageID); err != nil {
		t.Fatalf("insert lead review after first page: %v", err)
	}

	pageCount := 1
	for page.Meta.HasMore {
		pagination, err := platformtimeline.Parse(page.Meta.NextCursor, "100")
		if err != nil {
			t.Fatalf("parse lead review page %d cursor: %v", pageCount, err)
		}
		started = time.Now()
		page, err = service.ListSubmissionReviews(ctx, organizationID, SubmissionReviewQuery{
			Status: ReviewStatusUnreviewed, FormID: formAID, Cursor: pagination.Cursor, Limit: pagination.Limit,
		})
		if err != nil {
			t.Fatalf("list lead review page %d: %v", pageCount+1, err)
		}
		if pageCount == 1 && time.Since(started) > 2*time.Second {
			t.Fatal("adjacent 100-row lead review page exceeded 2s")
		}
		for _, submission := range page.Submissions {
			seen = append(seen, submission.ID)
		}
		pageCount++
		if pageCount > 10 {
			t.Fatal("lead review continuation did not terminate")
		}
	}
	if page.Meta.NextCursor != "" || len(page.Submissions) != 50 || len(seen) != len(expectedIDs) {
		t.Fatalf("unexpected final lead review traversal: final=%d seen=%d want=%d meta=%+v", len(page.Submissions), len(seen), len(expectedIDs), page.Meta)
	}
	for index := range expectedIDs {
		if seen[index] != expectedIDs[index] {
			t.Fatalf("lead review order at %d = %d, want %d", index, seen[index], expectedIDs[index])
		}
		if seen[index] == insertedAfterFirstPageID {
			t.Fatalf("new lead review %d leaked into older continuation", insertedAfterFirstPageID)
		}
	}

	refreshed, err := service.ListSubmissionReviews(ctx, organizationID, SubmissionReviewQuery{Status: ReviewStatusUnreviewed, FormID: formAID, Limit: 1})
	if err != nil || len(refreshed.Submissions) != 1 || refreshed.Submissions[0].ID != insertedAfterFirstPageID {
		t.Fatalf("refresh did not expose new lead review: page=%#v err=%v", refreshed, err)
	}
	foreign, err := service.ListSubmissionReviews(ctx, foreignOrganizationID, SubmissionReviewQuery{Limit: 100})
	if err != nil || len(foreign.Submissions) != 1 || foreign.Submissions[0].Values["message"] != "Foreign sentinel" {
		t.Fatalf("unexpected foreign lead review page: page=%#v err=%v", foreign, err)
	}
}
