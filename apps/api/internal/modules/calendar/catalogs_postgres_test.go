package calendar

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
	"github.com/jackc/pgx/v5"
)

func TestCalendarCatalogsAreBoundedAuthorizedAtomicAndTenantSafe(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to calendar catalog postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_calendar_catalogs_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create calendar catalog schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	schemaURL := calendarCatalogDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate calendar catalog schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to migrated calendar catalog schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Calendar team',$1) RETURNING id`, "calendar-"+schema).Scan(&organizationID); err != nil {
		t.Fatalf("create calendar organization: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES('Foreign calendar team',$1) RETURNING id`, "foreign-calendar-"+schema).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign calendar organization: %v", err)
	}
	users := map[string]int64{}
	for _, actor := range []string{"owner", "admin", "member", "viewer", "disabled", "foreign"} {
		var userID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO users(email,password_hash,first_name,last_name)
			VALUES($1,'test-hash','Calendar',$2) RETURNING id
		`, actor+"-"+schema+"@example.test", actor).Scan(&userID); err != nil {
			t.Fatalf("create %s calendar user: %v", actor, err)
		}
		users[actor] = userID
	}
	for _, membership := range []struct {
		organizationID int64
		userID         int64
		role           string
		status         string
	}{
		{organizationID, users["owner"], "owner", "active"},
		{organizationID, users["admin"], "admin", "active"},
		{organizationID, users["member"], "member", "active"},
		{organizationID, users["viewer"], "viewer", "active"},
		{organizationID, users["disabled"], "admin", "disabled"},
		{foreignOrganizationID, users["foreign"], "owner", "active"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,membership_status) VALUES($1,$2,$3,$4)`, membership.organizationID, membership.userID, membership.role, membership.status); err != nil {
			t.Fatalf("create calendar membership: %v", err)
		}
	}

	service := NewService(pool, NewFakeProvider(nil))
	memberLink, err := service.CreateBookingLink(ctx, organizationID, users["member"], validCalendarBookingLinkInput("Member link", users["member"]))
	if err != nil || memberLink.ID <= 0 {
		t.Fatalf("active ordinary member could not create booking link: link=%+v err=%v", memberLink, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM calendar_booking_links WHERE id=$1`, memberLink.ID); err != nil {
		t.Fatalf("remove member authorization fixture: %v", err)
	}
	for actor, userID := range map[string]int64{"viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"]} {
		if _, err := service.CreateBookingLink(ctx, organizationID, userID, validCalendarBookingLinkInput("Forbidden "+actor, users["owner"])); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s created a calendar booking link: %v", actor, err)
		}
	}
	for actor, hostID := range map[string]int64{"disabled": users["disabled"], "foreign": users["foreign"]} {
		if _, err := service.CreateBookingLink(ctx, organizationID, users["owner"], validCalendarBookingLinkInput("Invalid host "+actor, hostID)); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s booking host returned %v", actor, err)
		}
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO calendar_booking_links(organization_id,slug,name,created_by_user_id)
		SELECT $1,'link-' || lpad(series::text,3,'0'),'Link ' || lpad(series::text,3,'0'),$2
		FROM generate_series(1,99) AS series
	`, organizationID, users["owner"]); err != nil {
		t.Fatalf("seed calendar booking links: %v", err)
	}
	var foreignLinkID int64
	if err := pool.QueryRow(ctx, `INSERT INTO calendar_booking_links(organization_id,slug,name,created_by_user_id) VALUES($1,'foreign','Foreign',$2) RETURNING id`, foreignOrganizationID, users["foreign"]).Scan(&foreignLinkID); err != nil {
		t.Fatalf("seed foreign calendar booking link: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO calendar_booking_link_members(booking_link_id,user_id,position) VALUES($1,$2,1)`, foreignLinkID, users["foreign"]); err != nil {
		t.Fatalf("seed foreign booking member: %v", err)
	}
	var corruptLinkID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM calendar_booking_links WHERE organization_id=$1 ORDER BY id LIMIT 1`, organizationID).Scan(&corruptLinkID); err != nil {
		t.Fatalf("select corrupt-member fixture link: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO calendar_booking_link_members(booking_link_id,user_id,position) VALUES($1,$2,1)`, corruptLinkID, users["foreign"]); err != nil {
		t.Fatalf("seed corrupt foreign booking member: %v", err)
	}

	type createResult struct {
		link BookingLink
		err  error
	}
	start := make(chan struct{})
	results := make(chan createResult, 2)
	for index, actorID := range []int64{users["owner"], users["admin"]} {
		go func(index int, actorID int64) {
			<-start
			link, err := service.CreateBookingLink(ctx, organizationID, actorID, validCalendarBookingLinkInput(fmt.Sprintf("Final link %d", index+1), actorID))
			results <- createResult{link: link, err: err}
		}(index, actorID)
	}
	close(start)
	var finalLink BookingLink
	var succeeded, limited int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			succeeded++
			finalLink = result.link
		case errors.Is(result.err, ErrBookingLinkLimit):
			limited++
		default:
			t.Fatalf("unexpected final booking-link create result: %v", result.err)
		}
	}
	if succeeded != 1 || limited != 1 || finalLink.ID <= 0 {
		t.Fatalf("booking-link capacity was not serialized: succeeded=%d limited=%d link=%+v", succeeded, limited, finalLink)
	}
	if _, err := service.CreateBookingLink(ctx, organizationID, users["owner"], validCalendarBookingLinkInput("Link 101", users["owner"])); !errors.Is(err, ErrBookingLinkLimit) {
		t.Fatalf("booking-link overflow returned %v", err)
	}
	updatedInput := validCalendarBookingLinkInput("Updated at capacity", users["member"])
	updated, err := service.UpdateBookingLink(ctx, organizationID, users["admin"], finalLink.ID, updatedInput)
	if err != nil || updated.Name != updatedInput.Name || len(updated.Members) != 1 || updated.Members[0].UserID != users["member"] {
		t.Fatalf("booking-link update at capacity failed: link=%+v err=%v", updated, err)
	}
	if _, err := service.UpdateBookingLink(ctx, organizationID, users["owner"], foreignLinkID, validCalendarBookingLinkInput("Cross tenant", users["owner"])); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant booking-link update returned %v", err)
	}
	links, err := service.ListBookingLinks(ctx, organizationID)
	if err != nil || len(links) != MaxBookingLinksPerOrganization {
		t.Fatalf("list complete booking-link catalog: count=%d err=%v", len(links), err)
	}
	for _, link := range links {
		for _, member := range link.Members {
			if member.UserID == users["foreign"] || strings.Contains(member.Email, "foreign-") {
				t.Fatalf("local booking-link list exposed foreign member: %+v", member)
			}
		}
	}
	foreignLinks, err := service.ListBookingLinks(ctx, foreignOrganizationID)
	if err != nil || len(foreignLinks) != 1 || foreignLinks[0].ID != foreignLinkID || len(foreignLinks[0].Members) != 1 {
		t.Fatalf("foreign booking-link catalog crossed tenants: links=%+v err=%v", foreignLinks, err)
	}
	expiredCtx, expiredCancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer expiredCancel()
	if _, err := service.ListBookingLinks(expiredCtx, organizationID); !errors.Is(err, ErrQueryTimeout) {
		t.Fatalf("expired booking-link list returned %v", err)
	}

	blocks := make([]AvailabilityBlockInput, MaxAvailabilityBlocksPerUser)
	for index := range blocks {
		blocks[index] = AvailabilityBlockInput{DayOfWeek: index / 4, StartMinute: (index % 4) * 120, EndMinute: (index%4)*120 + 60, Timezone: "UTC"}
	}
	savedBlocks, err := service.SetAvailability(ctx, organizationID, users["member"], AvailabilityInput{Blocks: blocks})
	if err != nil || len(savedBlocks) != MaxAvailabilityBlocksPerUser {
		t.Fatalf("save exact availability capacity: count=%d err=%v", len(savedBlocks), err)
	}
	if _, err := service.SetAvailability(ctx, organizationID, users["member"], AvailabilityInput{Blocks: append(blocks, AvailabilityBlockInput{DayOfWeek: 0, StartMinute: 1000, EndMinute: 1060, Timezone: "UTC"})}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("availability overflow returned %v", err)
	}
	if _, err := service.SetAvailability(ctx, organizationID, users["member"], AvailabilityInput{Blocks: []AvailabilityBlockInput{blocks[0], blocks[0]}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate availability block returned %v", err)
	}
	for actor, userID := range map[string]int64{"viewer": users["viewer"], "disabled": users["disabled"], "foreign": users["foreign"]} {
		if _, err := service.SetAvailability(ctx, organizationID, userID, AvailabilityInput{Blocks: blocks[:1]}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s replaced calendar availability: %v", actor, err)
		}
	}
	blocker, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin calendar availability blocker: %v", err)
	}
	if _, err := blocker.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, organizationID); err != nil {
		t.Fatalf("lock calendar organization for timeout: %v", err)
	}
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 150*time.Millisecond)
	_, replaceErr := service.SetAvailability(timeoutCtx, organizationID, users["member"], AvailabilityInput{Blocks: blocks[:1]})
	timeoutCancel()
	if !errors.Is(replaceErr, ErrQueryTimeout) {
		t.Fatalf("blocked availability replacement returned %v", replaceErr)
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatalf("release calendar availability blocker: %v", err)
	}
	retained, err := service.ListAvailability(ctx, organizationID, users["member"])
	if err != nil || len(retained) != MaxAvailabilityBlocksPerUser {
		t.Fatalf("timed-out availability replacement changed state: count=%d err=%v", len(retained), err)
	}
}

func validCalendarBookingLinkInput(name string, memberUserID int64) BookingLinkInput {
	return BookingLinkInput{Name: name, Description: "Bounded booking link", DurationMinutes: 30, BufferMinutes: 5, Timezone: "UTC", AssignmentMode: "owner", IsActive: true, MemberUserIDs: []int64{memberUserID}}
}

func calendarCatalogDatabaseURL(t *testing.T, rawURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse calendar catalog database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
