package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	UsageScopeCurrent = "current"
	UsageScopePeriod  = "period"
	// Concurrent billing-page loads and the daily worker may update the same
	// retained period row. PostgreSQL repeatable-read correctly asks losers to
	// retry; eight attempts cover a bounded small-team burst without looping
	// indefinitely when the database remains contended.
	usageAttempts = 8
)

// UsageMetric is one explainable usage observation. Limits remain in the plan
// contract; this type reports source-backed consumption without inventing a
// quota for a metric whose commercial policy has not been approved.
type UsageMetric struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Used   int64  `json:"used"`
	Unit   string `json:"unit"`
	Scope  string `json:"scope"`
	Source string `json:"source"`
}

// UsageSnapshot is the retained, tenant-scoped observation for one provider
// subscription period or (when no exact provider period exists) UTC month.
type UsageSnapshot struct {
	SnapshotID       int64         `json:"snapshotId"`
	PeriodStart      time.Time     `json:"periodStart"`
	PeriodEnd        time.Time     `json:"periodEnd"`
	PeriodBasis      string        `json:"periodBasis"`
	ObservedAt       time.Time     `json:"observedAt"`
	SourceTableCount int           `json:"sourceTableCount"`
	Metrics          []UsageMetric `json:"metrics"`
}

// Usage reconciles the latest billing view from durable source records inside
// one repeatable-read snapshot, then upserts retained evidence for the period.
func (s *Service) Usage(ctx context.Context, organizationID int64) (UsageSnapshot, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return UsageSnapshot{}, fmt.Errorf("billing service not configured")
	}
	var lastErr error
	for attempt := 0; attempt < usageAttempts; attempt++ {
		usage, err := s.reconcileUsage(ctx, organizationID)
		if err == nil {
			return usage, nil
		}
		lastErr = err
		if !retryableUsageReconciliation(err) || ctx.Err() != nil {
			return UsageSnapshot{}, err
		}
	}
	return UsageSnapshot{}, fmt.Errorf("reconcile billing usage after %d attempts: %w", usageAttempts, lastErr)
}

func (s *Service) reconcileUsage(ctx context.Context, organizationID int64) (UsageSnapshot, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return UsageSnapshot{}, fmt.Errorf("begin billing usage reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var providerName string
	var providerPeriodStart, providerPeriodEnd *time.Time
	var observedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(billing_provider,''), subscription_current_period_start,
		       subscription_current_period_end, NOW()
		FROM organizations WHERE id=$1
	`, organizationID).Scan(&providerName, &providerPeriodStart, &providerPeriodEnd, &observedAt); err != nil {
		return UsageSnapshot{}, fmt.Errorf("load billing usage period: %w", err)
	}
	managedProviderPeriod := s.Hosted() && strings.EqualFold(strings.TrimSpace(providerName), "stripe")
	periodStart, periodEnd, periodBasis := resolveUsagePeriod(observedAt, managedProviderPeriod, providerPeriodStart, providerPeriodEnd)

	var seats, contacts, deals int64
	var outboundMessages, automationExecutions, backgroundJobExecutions int64
	if err := tx.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM organization_memberships WHERE organization_id=$1 AND COALESCE(membership_status,'active')='active'),
		  (SELECT COUNT(*) FROM contacts WHERE organization_id=$1 AND archived_at IS NULL),
		  (SELECT COUNT(*) FROM deals WHERE organization_id=$1 AND archived_at IS NULL),
		  (SELECT COUNT(*) FROM email_messages
		   WHERE organization_id=$1 AND direction='outbound' AND status='sent'
		     AND created_at >= $2 AND created_at < $3),
		  (SELECT COUNT(*) FROM workflow_automation_runs
		   WHERE organization_id=$1 AND status='succeeded'
		     AND completed_at >= $2 AND completed_at < $3),
		  (SELECT COUNT(*) FROM background_jobs
		   WHERE organization_id=$1 AND status='succeeded'
		     AND completed_at >= $2 AND completed_at < $3)
	`, organizationID, periodStart, periodEnd).Scan(
		&seats, &contacts, &deals, &outboundMessages, &automationExecutions, &backgroundJobExecutions,
	); err != nil {
		return UsageSnapshot{}, fmt.Errorf("reconcile billing usage counters: %w", err)
	}

	storageTables, err := tenantStorageTables(ctx, tx)
	if err != nil {
		return UsageSnapshot{}, err
	}
	storageBytes, err := tenantStorageBytes(ctx, tx, organizationID, storageTables)
	if err != nil {
		return UsageSnapshot{}, err
	}

	var snapshotID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO billing_usage_snapshots (
		  organization_id,period_start,period_end,period_basis,
		  seats_used,contacts_used,deals_used,outbound_messages_used,
		  automation_executions_used,background_job_executions_used,
		  storage_bytes_used,source_table_count,observed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (organization_id,period_start,period_end) DO UPDATE SET
		  period_basis=EXCLUDED.period_basis,
		  seats_used=EXCLUDED.seats_used,
		  contacts_used=EXCLUDED.contacts_used,
		  deals_used=EXCLUDED.deals_used,
		  outbound_messages_used=EXCLUDED.outbound_messages_used,
		  automation_executions_used=EXCLUDED.automation_executions_used,
		  background_job_executions_used=EXCLUDED.background_job_executions_used,
		  storage_bytes_used=EXCLUDED.storage_bytes_used,
		  source_table_count=EXCLUDED.source_table_count,
		  observed_at=EXCLUDED.observed_at,
		  updated_at=NOW()
		RETURNING id
	`, organizationID, periodStart, periodEnd, periodBasis, seats, contacts, deals,
		outboundMessages, automationExecutions, backgroundJobExecutions,
		storageBytes, len(storageTables), observedAt).Scan(&snapshotID); err != nil {
		return UsageSnapshot{}, fmt.Errorf("retain billing usage snapshot: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return UsageSnapshot{}, fmt.Errorf("commit billing usage reconciliation: %w", err)
	}

	return UsageSnapshot{
		SnapshotID:       snapshotID,
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		PeriodBasis:      periodBasis,
		ObservedAt:       observedAt.UTC(),
		SourceTableCount: len(storageTables),
		Metrics: []UsageMetric{
			{Key: "seats", Label: "Active team seats", Used: seats, Unit: "seats", Scope: UsageScopeCurrent, Source: "active organization memberships"},
			{Key: "contacts", Label: "Active contacts", Used: contacts, Unit: "records", Scope: UsageScopeCurrent, Source: "non-archived contacts"},
			{Key: "deals", Label: "Active deals", Used: deals, Unit: "records", Scope: UsageScopeCurrent, Source: "non-archived deals"},
			{Key: "outbound_messages", Label: "Sent outbound email", Used: outboundMessages, Unit: "messages", Scope: UsageScopePeriod, Source: "outbound email messages recorded as sent"},
			{Key: "automation_executions", Label: "Successful automation runs", Used: automationExecutions, Unit: "runs", Scope: UsageScopePeriod, Source: "workflow automation runs recorded as succeeded"},
			{Key: "background_job_executions", Label: "Successful background jobs", Used: backgroundJobExecutions, Unit: "jobs", Scope: UsageScopePeriod, Source: "durable background jobs recorded as succeeded"},
			{Key: "storage_bytes", Label: "Tenant database row storage", Used: storageBytes, Unit: "bytes", Scope: UsageScopeCurrent, Source: "PostgreSQL row bytes across tenant-scoped base tables"},
		},
	}, nil
}

func retryableUsageReconciliation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && (postgresError.Code == "40001" || postgresError.Code == "40P01")
}

func resolveUsagePeriod(now time.Time, managedProviderPeriod bool, providerStart, providerEnd *time.Time) (time.Time, time.Time, string) {
	if managedProviderPeriod && providerStart != nil && providerEnd != nil && providerEnd.After(*providerStart) {
		return providerStart.UTC(), providerEnd.UTC(), "provider_subscription"
	}
	now = now.UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0), "calendar_month"
}

type tenantStorageTable struct {
	schema string
	name   string
}

func tenantStorageTables(ctx context.Context, tx pgx.Tx) ([]tenantStorageTable, error) {
	rows, err := tx.Query(ctx, `
		SELECT columns.table_schema, columns.table_name
		FROM information_schema.columns columns
		JOIN information_schema.tables tables
		  ON tables.table_schema=columns.table_schema AND tables.table_name=columns.table_name
		WHERE columns.table_schema=current_schema()
		  AND columns.column_name='organization_id'
		  AND tables.table_type='BASE TABLE'
		  AND columns.table_name NOT IN ('billing_usage_snapshots','billing_capacity_reservations')
		ORDER BY columns.table_name
	`)
	if err != nil {
		return nil, fmt.Errorf("list tenant storage sources: %w", err)
	}
	defer rows.Close()
	tables := make([]tenantStorageTable, 0)
	for rows.Next() {
		var table tenantStorageTable
		if err := rows.Scan(&table.schema, &table.name); err != nil {
			return nil, fmt.Errorf("scan tenant storage source: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenant storage sources: %w", err)
	}
	if len(tables) > 256 {
		return nil, fmt.Errorf("tenant storage source count %d exceeds safety bound", len(tables))
	}
	return tables, nil
}

func tenantStorageBytes(ctx context.Context, tx pgx.Tx, organizationID int64, tables []tenantStorageTable) (int64, error) {
	if len(tables) == 0 {
		return 0, nil
	}
	queries := make([]string, 0, len(tables))
	for _, table := range tables {
		identifier := pgx.Identifier{table.schema, table.name}.Sanitize()
		queries = append(queries, fmt.Sprintf("SELECT COALESCE(SUM(pg_column_size(stored_row)),0)::bigint AS bytes FROM %s stored_row WHERE organization_id=$1", identifier))
	}
	query := "SELECT COALESCE(SUM(bytes),0)::bigint FROM (" + strings.Join(queries, " UNION ALL ") + ") tenant_storage"
	var bytes int64
	if err := tx.QueryRow(ctx, query, organizationID).Scan(&bytes); err != nil {
		return 0, fmt.Errorf("measure tenant storage: %w", err)
	}
	return bytes, nil
}
