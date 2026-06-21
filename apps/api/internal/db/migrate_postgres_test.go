package db

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunMigrationsAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OPEN_CRM_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OPEN_CRM_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminPool, err := NewPool(ctx, Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("connect to test postgres: %v", err)
	}
	defer adminPool.Close()

	schema := fmt.Sprintf("open_crm_test_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)

	config := Config{DatabaseURL: databaseURLWithSearchPath(t, databaseURL, schema)}
	result, err := RunMigrations(ctx, config)
	if err != nil {
		t.Fatalf("run migrations against postgres: %v", err)
	}
	if len(result.Applied) != len(MigrationFiles()) || len(result.Skipped) != 0 {
		t.Fatalf("unexpected initial migration result: %#v", result)
	}

	repeated, err := RunMigrations(ctx, config)
	if err != nil {
		t.Fatalf("rerun migrations against postgres: %v", err)
	}
	if len(repeated.Applied) != 0 || len(repeated.Skipped) != len(MigrationFiles()) {
		t.Fatalf("expected repeat migration run to skip all files, got %#v", repeated)
	}

	pool, err := NewPool(ctx, config)
	if err != nil {
		t.Fatalf("connect to migrated schema: %v", err)
	}
	defer pool.Close()

	for _, constraint := range []string{
		"organizations_business_type_check",
		"organizations_base_currency_code_check",
		"organization_memberships_role_check",
		"companies_status_check",
		"contacts_status_check",
		"deals_status_check",
		"deals_value_amount_nonnegative_check",
		"deals_value_currency_code_check",
		"sales_quotas_period_order_check",
		"sales_quotas_amount_nonnegative_check",
		"sales_quotas_currency_code_check",
		"organization_exchange_rates_currency_code_check",
		"organization_exchange_rates_currency_pair_check",
		"organization_exchange_rates_rate_positive_check",
		"organization_exchange_rates_source_check",
		"lead_capture_forms_slug_check",
		"lead_capture_forms_fields_json_array_check",
		"lead_capture_submissions_payload_json_object_check",
		"lead_capture_forms_org_id_unique",
		"lead_landing_pages_form_org_fk",
		"lead_landing_pages_slug_check",
		"lead_landing_pages_theme_check",
		"lead_audiences_name_check",
		"lead_audiences_filters_json_object_check",
		"notes_entity_type_check",
		"tasks_entity_type_check",
		"tasks_status_check",
		"activities_entity_type_check",
	} {
		assertPostgresConstraint(t, ctx, pool, schema, constraint)
	}

	for _, index := range []string{
		"idx_deal_pipelines_org_position_unique",
		"idx_deal_pipelines_org_name_unique",
		"idx_deal_pipelines_org_default_unique",
		"idx_deal_stages_org_pipeline_position_unique",
		"idx_deal_stages_org_pipeline_name_unique",
		"idx_sales_quotas_org_user_period_unique",
		"idx_sales_quotas_org_period",
		"idx_org_exchange_rates_unique_effective",
		"idx_org_exchange_rates_latest",
		"idx_lead_capture_forms_org_slug_unique",
		"idx_lead_capture_forms_org_active",
		"idx_lead_capture_submissions_org_form_created",
		"idx_lead_capture_submissions_org_attribution",
		"idx_lead_landing_pages_slug_unique",
		"idx_lead_landing_pages_org_active",
		"idx_lead_landing_pages_org_form",
		"idx_contacts_org_lead_source",
		"idx_contacts_org_utm_campaign",
		"idx_lead_audiences_org_name_unique",
		"idx_lead_audiences_org_active",
		"idx_contact_company_links_unique",
		"idx_contact_company_links_primary_company",
		"idx_sessions_token_hash",
		"idx_sessions_expires_at",
		"idx_notes_org_entity_created",
	} {
		assertPostgresIndex(t, ctx, pool, schema, index)
	}

	assertPostgresIntegrityRules(t, ctx, pool)
}

func databaseURLWithSearchPath(t *testing.T, rawURL, schema string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse postgres test database url: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func assertPostgresConstraint(t *testing.T, ctx context.Context, pool *Pool, schema, constraint string) {
	t.Helper()

	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			JOIN pg_namespace n ON n.oid = c.connamespace
			WHERE n.nspname = $1 AND c.conname = $2
		)
	`, schema, constraint).Scan(&exists); err != nil {
		t.Fatalf("check constraint %s: %v", constraint, err)
	}
	if !exists {
		t.Fatalf("expected constraint %s to exist", constraint)
	}
}

func assertPostgresIndex(t *testing.T, ctx context.Context, pool *Pool, schema, index string) {
	t.Helper()

	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = $1 AND indexname = $2
		)
	`, schema, index).Scan(&exists); err != nil {
		t.Fatalf("check index %s: %v", index, err)
	}
	if !exists {
		t.Fatalf("expected index %s to exist", index)
	}
}

func assertPostgresIntegrityRules(t *testing.T, ctx context.Context, pool *Pool) {
	t.Helper()

	var organizationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Test Org', 'test-org') RETURNING id`).Scan(&organizationID); err != nil {
		t.Fatalf("insert organization: %v", err)
	}

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, first_name, last_name) VALUES ('owner@example.com', 'hash', 'Owner', 'User') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	var pipelineID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_pipelines (organization_id, name, position, is_default) VALUES ($1, 'Sales pipeline', 1, TRUE) RETURNING id`, organizationID).Scan(&pipelineID); err != nil {
		t.Fatalf("insert deal pipeline: %v", err)
	}

	var stageID int64
	if err := pool.QueryRow(ctx, `INSERT INTO deal_stages (organization_id, pipeline_id, name, position) VALUES ($1, $2, 'Open', 1) RETURNING id`, organizationID, pipelineID).Scan(&stageID); err != nil {
		t.Fatalf("insert deal stage: %v", err)
	}

	expectExecError(t, ctx, pool, `INSERT INTO organizations (name, slug, business_type) VALUES ('Bad Org', 'bad-org', 'invalid')`)
	expectExecError(t, ctx, pool, `INSERT INTO organizations (name, slug, base_currency) VALUES ('Bad Currency Org', 'bad-currency-org', 'US')`)
	expectExecError(t, ctx, pool, `INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, 'superuser')`, organizationID, userID)
	expectExecError(t, ctx, pool, `INSERT INTO deal_pipelines (organization_id, name, position) VALUES ($1, 'Duplicate Position', 1)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO deal_pipelines (organization_id, name, position) VALUES ($1, 'sales pipeline', 2)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO deal_stages (organization_id, pipeline_id, name, position) VALUES ($1, $2, 'Duplicate Position', 1)`, organizationID, pipelineID)
	expectExecError(t, ctx, pool, `INSERT INTO deal_stages (organization_id, pipeline_id, name, position) VALUES ($1, $2, 'open', 2)`, organizationID, pipelineID)
	expectExecError(t, ctx, pool, `INSERT INTO deals (organization_id, stage_id, name, value_amount) VALUES ($1, $2, 'Bad Deal', -1)`, organizationID, stageID)
	expectExecError(t, ctx, pool, `INSERT INTO deals (organization_id, stage_id, name, value_currency) VALUES ($1, $2, 'Bad Currency', 'US')`, organizationID, stageID)
	expectExecError(t, ctx, pool, `INSERT INTO sales_quotas (organization_id, user_id, period_start, period_end, quota_amount, currency) VALUES ($1, $2, '2026-04-01', '2026-03-31', 1000, 'USD')`, organizationID, userID)
	expectExecError(t, ctx, pool, `INSERT INTO sales_quotas (organization_id, user_id, period_start, period_end, quota_amount, currency) VALUES ($1, $2, '2026-04-01', '2026-06-30', -1, 'USD')`, organizationID, userID)
	expectExecError(t, ctx, pool, `INSERT INTO sales_quotas (organization_id, user_id, period_start, period_end, quota_amount, currency) VALUES ($1, $2, '2026-04-01', '2026-06-30', 1000, 'US')`, organizationID, userID)
	expectExecError(t, ctx, pool, `INSERT INTO organization_exchange_rates (organization_id, base_currency, quote_currency, rate_to_base, source) VALUES ($1, 'USD', 'EUR', 0, 'manual')`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO organization_exchange_rates (organization_id, base_currency, quote_currency, rate_to_base, source) VALUES ($1, 'USD', 'USD', 1, 'manual')`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO organization_exchange_rates (organization_id, base_currency, quote_currency, rate_to_base, source) VALUES ($1, 'US', 'EUR', 1.1, 'manual')`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_capture_forms (organization_id, public_id, name, slug, title, fields_json) VALUES ($1, 'lf_bad', 'Bad Form', 'Bad Slug', 'Bad Form', '[]'::jsonb)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_capture_forms (organization_id, public_id, name, slug, title, fields_json) VALUES ($1, 'lf_bad_json', 'Bad Form', 'bad-form', 'Bad Form', '{}'::jsonb)`, organizationID)
	var leadFormID int64
	if err := pool.QueryRow(ctx, `INSERT INTO lead_capture_forms (organization_id, public_id, name, slug, title, fields_json) VALUES ($1, 'lf_test', 'Lead Form', 'lead-form', 'Lead Form', '[]'::jsonb) RETURNING id`, organizationID).Scan(&leadFormID); err != nil {
		t.Fatalf("insert lead capture form: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO lead_capture_forms (organization_id, public_id, name, slug, title, fields_json) VALUES ($1, 'lf_test_2', 'Lead Form 2', 'lead-form', 'Lead Form 2', '[]'::jsonb)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_capture_submissions (organization_id, form_id, payload_json) VALUES ($1, $2, '[]'::jsonb)`, organizationID, leadFormID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_landing_pages (organization_id, public_id, lead_capture_form_id, name, slug, title, theme) VALUES ($1, 'lp_bad', $2, 'Bad Page', 'Bad Slug', 'Bad Page', 'light')`, organizationID, leadFormID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_landing_pages (organization_id, public_id, lead_capture_form_id, name, slug, title, theme) VALUES ($1, 'lp_bad_theme', $2, 'Bad Page', 'bad-page', 'Bad Page', 'neon')`, organizationID, leadFormID)
	if _, err := pool.Exec(ctx, `INSERT INTO lead_landing_pages (organization_id, public_id, lead_capture_form_id, name, slug, title, theme) VALUES ($1, 'lp_test', $2, 'Landing Page', 'landing-page', 'Landing Page', 'light')`, organizationID, leadFormID); err != nil {
		t.Fatalf("insert lead landing page: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO lead_landing_pages (organization_id, public_id, lead_capture_form_id, name, slug, title, theme) VALUES ($1, 'lp_test_2', $2, 'Landing Page 2', 'landing-page', 'Landing Page 2', 'light')`, organizationID, leadFormID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_audiences (organization_id, name, filters_json) VALUES ($1, '', '{}'::jsonb)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_audiences (organization_id, name, filters_json) VALUES ($1, 'Bad Audience', '[]'::jsonb)`, organizationID)
	if _, err := pool.Exec(ctx, `INSERT INTO lead_audiences (organization_id, name, filters_json) VALUES ($1, 'Lead Audience', '{"status":"lead"}'::jsonb)`, organizationID); err != nil {
		t.Fatalf("insert lead audience: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO lead_audiences (organization_id, name, filters_json) VALUES ($1, 'lead audience', '{}'::jsonb)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO notes (organization_id, entity_type, entity_id, body, created_by_user_id) VALUES ($1, 'task', 1, 'Bad note', $2)`, organizationID, userID)
	expectExecError(t, ctx, pool, `INSERT INTO tasks (organization_id, entity_type, entity_id, title, status, created_by_user_id) VALUES ($1, 'invoice', 1, 'Bad task', 'open', $2)`, organizationID, userID)
	expectExecError(t, ctx, pool, `INSERT INTO tasks (organization_id, entity_type, entity_id, title, status, created_by_user_id) VALUES ($1, 'contact', 1, 'Bad task', 'done', $2)`, organizationID, userID)
	expectExecError(t, ctx, pool, `INSERT INTO activities (organization_id, entity_type, entity_id, action, summary) VALUES ($1, 'invoice', 1, 'created', 'Bad activity')`, organizationID)

	var contactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name) VALUES ($1, 'Jane', 'Contact') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("insert contact: %v", err)
	}
	var otherContactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name) VALUES ($1, 'Other', 'Contact') RETURNING id`, organizationID).Scan(&otherContactID); err != nil {
		t.Fatalf("insert second contact: %v", err)
	}
	var companyID int64
	if err := pool.QueryRow(ctx, `INSERT INTO companies (organization_id, name) VALUES ($1, 'Test Company') RETURNING id`, organizationID).Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contact_company_links (organization_id, contact_id, company_id, is_primary) VALUES ($1, $2, $3, TRUE)`, organizationID, contactID, companyID); err != nil {
		t.Fatalf("insert contact company link: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO contact_company_links (organization_id, contact_id, company_id) VALUES ($1, $2, $3)`, organizationID, contactID, companyID)
	expectExecError(t, ctx, pool, `INSERT INTO contact_company_links (organization_id, contact_id, company_id, is_primary) VALUES ($1, $2, $3, TRUE)`, organizationID, otherContactID, companyID)
}

func expectExecError(t *testing.T, ctx context.Context, pool *Pool, sql string, args ...any) {
	t.Helper()

	if _, err := pool.Exec(ctx, sql, args...); err == nil {
		t.Fatalf("expected SQL to fail: %s", sql)
	}
}
