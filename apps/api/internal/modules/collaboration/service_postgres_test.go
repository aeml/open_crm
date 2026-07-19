package collaboration_test

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
	modulecollaboration "github.com/aeml/open_crm/apps/api/internal/modules/collaboration"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
)

func TestFollowersMentionsNotificationsAndDigestAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to collaboration test postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_collaboration_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create collaboration schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := collaborationDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate collaboration schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated collaboration schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Collaboration', $1) RETURNING id`, "collaboration-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create collaboration organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Foreign', $1) RETURNING id`, "foreign-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign organization: %v", err)
	}
	ownerID := insertCollaborationUser(t, ctx, pool, "owner-"+schema+"@example.test", "Primary", "Owner")
	followerID := insertCollaborationUser(t, ctx, pool, "follower-"+schema+"@example.test", "Record", "Follower")
	mentionedEmail := "mentioned-" + schema + "@example.test"
	mentionedID := insertCollaborationUser(t, ctx, pool, mentionedEmail, "Mentioned", "Member")
	disabledEmail := "disabled-" + schema + "@example.test"
	disabledID := insertCollaborationUser(t, ctx, pool, disabledEmail, "Disabled", "Member")
	foreignEmail := "foreign-" + schema + "@example.test"
	foreignID := insertCollaborationUser(t, ctx, pool, foreignEmail, "Foreign", "Member")
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		role           string
		status         string
	}{
		{organizationID, ownerID, "owner", "active"},
		{organizationID, followerID, "member", "active"},
		{organizationID, mentionedID, "member", "active"},
		{organizationID, disabledID, "member", "disabled"},
		{foreignOrganizationID, foreignID, "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO organization_memberships (organization_id, user_id, role, membership_status)
			VALUES ($1, $2, $3, $4)
		`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("create collaboration membership: %v", err)
		}
	}

	var contactID, unrelatedContactID, foreignContactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name) VALUES ($1, 'Ada', 'Lovelace') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("create followed contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name) VALUES ($1, 'Grace', 'Hopper') RETURNING id`, organizationID).Scan(&unrelatedContactID); err != nil {
		t.Fatalf("create unrelated contact: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name) VALUES ($1, 'Foreign', 'Contact') RETURNING id`, foreignOrganizationID).Scan(&foreignContactID); err != nil {
		t.Fatalf("create foreign contact: %v", err)
	}

	service := modulecollaboration.NewService(pool)
	if _, err := service.SetFollowing(ctx, organizationID, followerID, "contact", foreignContactID, true); !errors.Is(err, modulecollaboration.ErrNotFound) {
		t.Fatalf("expected cross-tenant record follow denial, got %v", err)
	}
	if _, err := service.SetFollowing(ctx, organizationID, disabledID, "contact", contactID, true); !errors.Is(err, modulecollaboration.ErrNotFound) {
		t.Fatalf("expected disabled follower denial, got %v", err)
	}
	if _, err := service.ActivityDigest(ctx, organizationID, disabledID, modulecollaboration.DigestQuery{Scope: "team", Days: 7}); !errors.Is(err, modulecollaboration.ErrNotFound) {
		t.Fatalf("expected disabled digest reader denial, got %v", err)
	}
	for i := 0; i < 2; i++ {
		followers, err := service.SetFollowing(ctx, organizationID, followerID, "contact", contactID, true)
		if err != nil || !followers.Following || len(followers.Followers) != 1 {
			t.Fatalf("expected idempotent following, result=%#v err=%v", followers, err)
		}
	}

	noteBody := fmt.Sprintf("Please review @%s and again @%s; ignore @%s and @%s.", mentionedEmail, strings.ToUpper(mentionedEmail), foreignEmail, disabledEmail)
	if _, err := modulenotes.NewService(pool).Create(ctx, organizationID, foreignID, modulenotes.CreateInput{EntityType: "contact", EntityID: contactID, Body: noteBody}); err == nil {
		t.Fatal("expected cross-tenant note actor denial")
	}
	noteResult, err := modulenotes.NewService(pool).Create(ctx, organizationID, ownerID, modulenotes.CreateInput{EntityType: "contact", EntityID: contactID, Body: noteBody})
	if err != nil {
		t.Fatalf("create collaborative note: %v", err)
	}
	if noteResult.Note.Body != noteBody {
		t.Fatalf("collaborative note body changed: %q", noteResult.Note.Body)
	}
	var mentions int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM note_mentions WHERE organization_id = $1 AND note_id = $2 AND mentioned_user_id = $3`, organizationID, noteResult.Note.ID, mentionedID).Scan(&mentions); err != nil || mentions != 1 {
		t.Fatalf("expected one deduplicated active mention, count=%d err=%v", mentions, err)
	}
	var foreignOrDisabledMentions int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM note_mentions WHERE organization_id = $1 AND note_id = $2 AND mentioned_user_id IN ($3, $4)`, organizationID, noteResult.Note.ID, foreignID, disabledID).Scan(&foreignOrDisabledMentions); err != nil || foreignOrDisabledMentions != 0 {
		t.Fatalf("expected foreign and disabled mentions ignored, count=%d err=%v", foreignOrDisabledMentions, err)
	}
	for _, expected := range []struct {
		userID    int64
		eventType string
		count     int
	}{
		{followerID, "record.activity", 1},
		{mentionedID, "record.mentioned", 1},
		{disabledID, "record.mentioned", 0},
		{foreignID, "record.mentioned", 0},
	} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE organization_id = $1 AND user_id = $2 AND event_type = $3`, organizationID, expected.userID, expected.eventType).Scan(&count); err != nil || count != expected.count {
			t.Fatalf("notification user=%d event=%s count=%d err=%v", expected.userID, expected.eventType, count, err)
		}
	}
	followers, err := service.Followers(ctx, organizationID, mentionedID, "contact", contactID)
	if err != nil || !followers.Following || len(followers.Followers) != 3 {
		t.Fatalf("expected actor and mentioned user auto-following, result=%#v err=%v", followers, err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1, 'contact', $2, $3, 'contact.updated', 'Unrelated contact updated'),
		       ($4, 'contact', $5, $6, 'contact.updated', 'Foreign contact updated')
	`, organizationID, unrelatedContactID, ownerID, foreignOrganizationID, foreignContactID, foreignID); err != nil {
		t.Fatalf("create digest fixtures: %v", err)
	}
	followingDigest, err := service.ActivityDigest(ctx, organizationID, followerID, modulecollaboration.DigestQuery{Scope: "following", Days: 7})
	if err != nil {
		t.Fatalf("load followed-record digest: %v", err)
	}
	if followingDigest.TotalActivities != 1 || followingDigest.ActiveRecords != 1 || len(followingDigest.Activities) != 1 || followingDigest.Activities[0].EntityLabel != "Ada Lovelace" {
		t.Fatalf("unexpected followed-record digest: %#v", followingDigest)
	}
	teamDigest, err := service.ActivityDigest(ctx, organizationID, followerID, modulecollaboration.DigestQuery{Scope: "team", Days: 7, ActorUserID: ownerID})
	if err != nil {
		t.Fatalf("load team digest: %v", err)
	}
	if teamDigest.TotalActivities != 2 || teamDigest.ActiveRecords != 2 || len(teamDigest.Activities) != 2 {
		t.Fatalf("expected tenant-scoped actor-filtered team digest, got %#v", teamDigest)
	}
	for _, activity := range teamDigest.Activities {
		if activity.EntityID == foreignContactID || activity.ActorUserID != ownerID {
			t.Fatalf("team digest leaked foreign or unfiltered activity: %#v", activity)
		}
	}

	for i := 0; i < 2; i++ {
		followers, err = service.SetFollowing(ctx, organizationID, followerID, "contact", contactID, false)
		if err != nil || followers.Following {
			t.Fatalf("expected idempotent unfollow, result=%#v err=%v", followers, err)
		}
	}
}

func insertCollaborationUser(t *testing.T, ctx context.Context, pool *moduledb.Pool, email, firstName, lastName string) int64 {
	t.Helper()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, 'test-hash', $2, $3) RETURNING id`, email, firstName, lastName).Scan(&userID); err != nil {
		t.Fatalf("create collaboration user %s: %v", email, err)
	}
	return userID
}

func collaborationDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse collaboration database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
