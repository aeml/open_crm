package activityfeed_test

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
	moduleactivityfeed "github.com/aeml/open_crm/apps/api/internal/modules/activityfeed"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

func TestActivityCursorIsStableBoundedAndTenantScopedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to timeline test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_activity_cursor_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create timeline schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := timelineDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate timeline schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to timeline schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES ('Timeline',$1) RETURNING id`, "timeline-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create timeline organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES ('Foreign',$1) RETURNING id`, "foreign-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}

	base := time.Date(2026, 7, 22, 2, 0, 0, 0, time.UTC)
	ids := make([]int64, 0, 7)
	for i := 0; i < 7; i++ {
		createdAt := base.Add(-time.Duration(i/2) * time.Minute)
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO activities(organization_id,entity_type,entity_id,action,summary,created_at)
			VALUES ($1,'deal',77,'deal.test',$2,$3) RETURNING id
		`, organizationID, fmt.Sprintf("Activity %d", i), createdAt).Scan(&id); err != nil {
			t.Fatalf("insert activity %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO activities(organization_id,entity_type,entity_id,action,summary,created_at) VALUES ($1,'deal',77,'deal.foreign','Foreign activity',$2)`, foreignOrganizationID, base.Add(time.Hour)); err != nil {
		t.Fatalf("insert foreign activity: %v", err)
	}

	service := moduleactivityfeed.NewService(pool)
	first, err := service.ListByEntity(ctx, organizationID, "deal", 77, platformtimeline.Query{Limit: 3})
	if err != nil {
		t.Fatalf("list first activity page: %v", err)
	}
	if len(first.Activities) != 3 || !first.Meta.HasMore || first.Meta.NextCursor == "" {
		t.Fatalf("unexpected first page: %#v", first)
	}
	if first.Activities[0].ID != ids[1] || first.Activities[1].ID != ids[0] || first.Activities[2].ID != ids[3] {
		t.Fatalf("same-timestamp order is unstable: ids=%v page=%#v", ids, first.Activities)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO activities(organization_id,entity_type,entity_id,action,summary,created_at) VALUES ($1,'deal',77,'deal.new','New after first page',$2)`, organizationID, base.Add(time.Hour)); err != nil {
		t.Fatalf("insert newer activity: %v", err)
	}
	cursor, err := platformtimeline.Decode(first.Meta.NextCursor)
	if err != nil {
		t.Fatalf("decode first cursor: %v", err)
	}
	second, err := service.ListByEntity(ctx, organizationID, "deal", 77, platformtimeline.Query{Limit: 3, Cursor: &cursor})
	if err != nil {
		t.Fatalf("list second activity page: %v", err)
	}
	if len(second.Activities) != 3 || !second.Meta.HasMore || second.Activities[0].ID != ids[2] || second.Activities[1].ID != ids[5] || second.Activities[2].ID != ids[4] {
		t.Fatalf("unexpected stable second page: %#v", second)
	}
	seen := map[int64]bool{}
	for _, entry := range first.Activities {
		seen[entry.ID] = true
	}
	for _, entry := range second.Activities {
		if seen[entry.ID] {
			t.Fatalf("activity %d overlapped cursor pages", entry.ID)
		}
	}

	lastCursor, err := platformtimeline.Decode(second.Meta.NextCursor)
	if err != nil {
		t.Fatalf("decode second cursor: %v", err)
	}
	last, err := service.ListByEntity(ctx, organizationID, "deal", 77, platformtimeline.Query{Limit: 3, Cursor: &lastCursor})
	if err != nil || len(last.Activities) != 1 || last.Activities[0].ID != ids[6] || last.Meta.HasMore || last.Meta.NextCursor != "" {
		t.Fatalf("unexpected last activity page: %#v err=%v", last, err)
	}
	foreignView, err := service.ListByEntity(ctx, foreignOrganizationID, "deal", 77, platformtimeline.Query{})
	if err != nil || len(foreignView.Activities) != 1 || foreignView.Activities[0].Summary != "Foreign activity" {
		t.Fatalf("activity feed crossed tenant boundary: %#v err=%v", foreignView, err)
	}
	if _, err := service.ListByEntity(ctx, organizationID, "deal", 77, platformtimeline.Query{Limit: 101}); !errors.Is(err, platformtimeline.ErrInvalid) {
		t.Fatalf("unsafe service limit error = %v, want ErrInvalid", err)
	}

	var contactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts(organization_id,first_name,last_name,status) VALUES ($1,'Bounded','History','lead') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("create bounded-history contact: %v", err)
	}
	for i := 0; i < platformtimeline.DefaultLimit+1; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO activities(organization_id,entity_type,entity_id,action,summary,created_at) VALUES ($1,'contact',$2,'contact.test',$3,$4)`, organizationID, contactID, fmt.Sprintf("Contact activity %d", i), base.Add(-time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("insert contact activity %d: %v", i, err)
		}
	}
	detail, err := modulecontacts.NewService(pool).GetByID(ctx, organizationID, contactID)
	if err != nil || len(detail.Activities) != platformtimeline.DefaultLimit || !detail.ActivityMeta.HasMore || detail.ActivityMeta.NextCursor == "" {
		t.Fatalf("contact detail did not expose a bounded first page: activities=%d meta=%+v err=%v", len(detail.Activities), detail.ActivityMeta, err)
	}
}

func timelineDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
