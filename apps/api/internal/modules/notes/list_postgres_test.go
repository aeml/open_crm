package notes_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

func TestNoteCursorIsStableBoundedAndTenantScopedAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to note cursor postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_note_cursor_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create note cursor schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := noteCursorDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate note cursor schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to note cursor schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID, userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES ('Notes',$1) RETURNING id`, "notes-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create notes organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES ('Foreign',$1) RETURNING id`, "foreign-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,first_name,last_name) VALUES ($1,'hash','Note','Author') RETURNING id`, "notes-"+schema+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("create note author: %v", err)
	}

	base := time.Date(2026, 7, 22, 2, 0, 0, 0, time.UTC)
	ids := make([]int64, 0, 5)
	for i := 0; i < 5; i++ {
		createdAt := base.Add(-time.Duration(i/2) * time.Minute)
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO notes(organization_id,entity_type,entity_id,body,created_by_user_id,created_at,updated_at)
			VALUES ($1,'contact',71,$2,$3,$4,$4) RETURNING id
		`, organizationID, fmt.Sprintf("Note %d", i), userID, createdAt).Scan(&id); err != nil {
			t.Fatalf("insert note %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notes(organization_id,entity_type,entity_id,body,created_by_user_id) VALUES ($1,'contact',71,'Foreign note',$2)`, foreignOrganizationID, userID); err != nil {
		t.Fatalf("insert foreign note: %v", err)
	}

	service := modulenotes.NewService(pool)
	first, err := service.ListByEntity(ctx, organizationID, "contact", 71, platformtimeline.Query{Limit: 2})
	if err != nil || len(first.Notes) != 2 || first.Notes[0].ID != ids[1] || first.Notes[1].ID != ids[0] || !first.Meta.HasMore {
		t.Fatalf("unexpected first note page: %#v err=%v", first, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notes(organization_id,entity_type,entity_id,body,created_by_user_id,created_at,updated_at) VALUES ($1,'contact',71,'New note',$2,$3,$3)`, organizationID, userID, base.Add(time.Hour)); err != nil {
		t.Fatalf("insert newer note: %v", err)
	}
	cursor, err := platformtimeline.Decode(first.Meta.NextCursor)
	if err != nil {
		t.Fatalf("decode note cursor: %v", err)
	}
	second, err := service.ListByEntity(ctx, organizationID, "contact", 71, platformtimeline.Query{Limit: 2, Cursor: &cursor})
	if err != nil || len(second.Notes) != 2 || second.Notes[0].ID != ids[3] || second.Notes[1].ID != ids[2] || !second.Meta.HasMore {
		t.Fatalf("unexpected second note page: %#v err=%v", second, err)
	}
	lastCursor, err := platformtimeline.Decode(second.Meta.NextCursor)
	if err != nil {
		t.Fatalf("decode last note cursor: %v", err)
	}
	last, err := service.ListByEntity(ctx, organizationID, "contact", 71, platformtimeline.Query{Limit: 2, Cursor: &lastCursor})
	if err != nil || len(last.Notes) != 1 || last.Notes[0].ID != ids[4] || last.Meta.HasMore {
		t.Fatalf("unexpected last note page: %#v err=%v", last, err)
	}
	foreign, err := service.ListByEntity(ctx, foreignOrganizationID, "contact", 71, platformtimeline.Query{})
	if err != nil || len(foreign.Notes) != 1 || foreign.Notes[0].Body != "Foreign note" {
		t.Fatalf("notes crossed tenant boundary: %#v err=%v", foreign, err)
	}
}

func noteCursorDatabaseURL(t *testing.T, rawURL, schema string) string {
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
