package billing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	moduledb "github.com/aeml/open_crm/apps/api/internal/db"
)

func TestCapacityReservationsSerializeHostedLimitsAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminPool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to capacity postgres: %v", err)
	}
	defer adminPool.Close()
	schema := fmt.Sprintf("open_crm_billing_capacity_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create capacity schema: %v", err)
	}
	defer func() { _, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()
	schemaURL := billingDatabaseURL(t, databaseURL, schema)
	if _, err := moduledb.RunMigrations(ctx, moduledb.Config{DatabaseURL: schemaURL}); err != nil {
		t.Fatalf("migrate capacity schema: %v", err)
	}
	pool, err := moduledb.NewPool(ctx, moduledb.Config{DatabaseURL: schemaURL})
	if err != nil {
		t.Fatalf("connect to capacity schema: %v", err)
	}
	defer pool.Close()

	var organizationID, foreignOrganizationID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO organizations (name,slug,plan,subscription_status,billing_provider)
		VALUES ('Capacity Pilot','capacity-pilot','free','active','stripe') RETURNING id
	`).Scan(&organizationID); err != nil {
		t.Fatalf("create capacity tenant: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO organizations (name,slug,plan,subscription_status,billing_provider)
		VALUES ('Foreign Capacity','foreign-capacity','free','active','stripe') RETURNING id
	`).Scan(&foreignOrganizationID); err != nil {
		t.Fatalf("create foreign capacity tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contacts (organization_id,first_name,last_name)
		SELECT $1,'Capacity',index::text FROM generate_series(1,499) index
	`, organizationID); err != nil {
		t.Fatalf("seed contacts at one below limit: %v", err)
	}

	service := NewService(pool, newStripeProvider(ProviderConfig{}))
	type reservationResult struct {
		reservation CapacityReservation
		err         error
	}
	start := make(chan struct{})
	results := make(chan reservationResult, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			reservation, err := service.ReserveCapacity(ctx, organizationID, ResourceContacts, 1)
			results <- reservationResult{reservation: reservation, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	var winner CapacityReservation
	allowed, rejected := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			allowed++
			winner = result.reservation
		case errors.Is(result.err, ErrLimitReached):
			rejected++
		default:
			t.Fatalf("concurrent capacity result: %v", result.err)
		}
	}
	if allowed != 1 || rejected != 1 || !winner.Enforced() {
		t.Fatalf("concurrent claims allowed=%d rejected=%d winner=%+v", allowed, rejected, winner)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin capacity effect: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO contacts (organization_id,first_name,last_name) VALUES ($1,'Final','Contact')`, organizationID); err != nil {
		t.Fatalf("insert reserved contact: %v", err)
	}
	if err := service.ConsumeCapacity(ctx, tx, winner); err != nil {
		t.Fatalf("consume winning reservation: %v", err)
	}
	blockedReserve := make(chan error, 1)
	go func() {
		_, err := service.ReserveCapacity(ctx, organizationID, ResourceContacts, 1)
		blockedReserve <- err
	}()
	select {
	case err := <-blockedReserve:
		t.Fatalf("reservation crossed an uncommitted consume: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit reserved contact: %v", err)
	}
	if err := <-blockedReserve; !errors.Is(err, ErrLimitReached) {
		t.Fatalf("post-commit full tenant reservation returned %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE contacts SET archived_at=NOW() WHERE organization_id=$1 AND first_name='Final'`, organizationID); err != nil {
		t.Fatalf("archive contact to free capacity: %v", err)
	}
	first, err := service.ReserveCapacity(ctx, organizationID, ResourceContacts, 1)
	if err != nil {
		t.Fatalf("reserve freed capacity: %v", err)
	}
	if err := service.CancelCapacity(ctx, first); err != nil {
		t.Fatalf("cancel reservation: %v", err)
	}
	second, err := service.ReserveCapacity(ctx, organizationID, ResourceContacts, 1)
	if err != nil {
		t.Fatalf("reserve canceled capacity: %v", err)
	}
	defer CancelReservation(service, second)

	foreign, err := service.ReserveCapacity(ctx, foreignOrganizationID, ResourceContacts, 1)
	if err != nil {
		t.Fatalf("reserve foreign capacity: %v", err)
	}
	defer CancelReservation(service, foreign)
	foreign.OrganizationID = organizationID
	foreignTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin foreign consume: %v", err)
	}
	if err := service.ConsumeCapacity(ctx, foreignTx, foreign); !errors.Is(err, ErrCapacityReservationExpired) {
		_ = foreignTx.Rollback(ctx)
		t.Fatalf("cross-tenant consume returned %v", err)
	}
	_ = foreignTx.Rollback(ctx)

	if err := service.CancelCapacity(ctx, second); err != nil {
		t.Fatalf("release second reservation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO billing_capacity_reservations (organization_id,resource,amount,expires_at,created_at)
		VALUES ($1,'contacts',1,NOW()-INTERVAL '1 minute',NOW()-INTERVAL '2 minutes')
	`, organizationID); err != nil {
		t.Fatalf("seed expired reservation: %v", err)
	}
	afterExpiry, err := service.ReserveCapacity(ctx, organizationID, ResourceContacts, 1)
	if err != nil {
		t.Fatalf("expired reservation did not release capacity: %v", err)
	}
	if err := service.CancelCapacity(ctx, afterExpiry); err != nil {
		t.Fatalf("cancel post-expiry reservation: %v", err)
	}

	unmanaged := NewService(pool, FakeProvider{})
	noOp, err := unmanaged.ReserveCapacity(ctx, organizationID, ResourceContacts, 1)
	if err != nil || noOp.Enforced() {
		t.Fatalf("self-hosted reservation=%+v err=%v", noOp, err)
	}
}
