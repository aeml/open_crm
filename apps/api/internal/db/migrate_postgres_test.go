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
		"organization_memberships_status_check",
		"companies_status_check",
		"contacts_status_check",
		"deals_status_check",
		"deals_value_amount_nonnegative_check",
		"deals_value_currency_code_check",
		"sales_quotas_period_order_check",
		"sales_quotas_amount_nonnegative_check",
		"sales_quotas_currency_code_check",
		"deal_stages_probability_percent_check",
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
		"lead_audiences_org_id_unique",
		"email_sequences_org_id_unique",
		"marketing_email_campaigns_audience_org_fk",
		"marketing_email_campaigns_name_check",
		"marketing_email_campaigns_subject_check",
		"marketing_email_campaigns_body_check",
		"marketing_email_campaigns_status_check",
		"marketing_email_campaigns_schedule_check",
		"marketing_email_campaigns_counts_check",
		"lead_nurture_campaigns_audience_org_fk",
		"lead_nurture_campaigns_sequence_org_fk",
		"lead_nurture_campaigns_name_check",
		"lead_nurture_campaigns_status_check",
		"lead_nurture_campaigns_counts_check",
		"contacts_lead_score_check",
		"contacts_lead_grade_check",
		"contacts_lead_score_breakdown_json_array_check",
		"lead_scoring_rules_name_check",
		"lead_scoring_rules_field_check",
		"lead_scoring_rules_operator_check",
		"lead_scoring_rules_value_check",
		"lead_scoring_rules_score_delta_check",
		"lead_scoring_rules_position_check",
		"lead_chat_widgets_form_org_fk",
		"lead_chat_widgets_name_check",
		"lead_chat_widgets_title_check",
		"lead_chat_widgets_prompt_label_check",
		"lead_chat_widgets_cta_label_check",
		"lead_chat_widgets_theme_check",
		"lead_chat_widgets_position_check",
		"workflow_automations_name_check",
		"workflow_automations_trigger_type_check",
		"workflow_automations_target_entity_type_check",
		"workflow_automations_trigger_config_json_object_check",
		"workflow_automations_condition_logic_check",
		"workflow_automations_conditions_json_array_check",
		"workflow_automations_actions_json_array_check",
		"workflow_automations_position_check",
		"workflow_automation_runs_automation_name_check",
		"workflow_automation_runs_trigger_type_check",
		"workflow_automation_runs_target_entity_type_check",
		"workflow_automation_runs_target_entity_id_check",
		"workflow_automation_runs_event_key_check",
		"workflow_automation_runs_status_check",
		"workflow_automation_runs_trigger_payload_json_object_check",
		"workflow_automation_runs_actions_total_check",
		"workflow_automation_runs_actions_completed_check",
		"workflow_automation_runs_retry_count_check",
		"workflow_automation_runs_action_progress_check",
		"workflow_automation_runs_terminal_completed_check",
		"custom_report_definitions_name_check",
		"custom_report_definitions_source_type_check",
		"custom_report_definitions_columns_json_array_check",
		"custom_report_definitions_filters_json_array_check",
		"custom_report_definitions_aggregation_json_object_check",
		"custom_report_definitions_visualization_type_check",
		"background_jobs_type_check",
		"background_jobs_idempotency_key_check",
		"background_jobs_payload_json_object_check",
		"background_jobs_status_check",
		"background_jobs_attempts_check",
		"background_jobs_lock_state_check",
		"background_jobs_completion_state_check",
		"email_sequence_deliveries_step_check",
		"email_sequence_deliveries_recipient_check",
		"email_sequence_deliveries_status_check",
		"email_sequence_deliveries_attempt_state_check",
		"notes_entity_type_check",
		"tasks_entity_type_check",
		"tasks_status_check",
		"activities_entity_type_check",
		"record_followers_entity_type_check",
		"import_batches_entity_type_check",
		"import_batches_status_check",
		"import_batches_mapping_json_object_check",
		"import_batch_rows_status_check",
		"import_batch_rows_errors_json_array_check",
		"bulk_operations_entity_type_check",
		"bulk_operations_action_check",
		"bulk_operations_status_check",
		"bulk_operation_rows_status_check",
		"duplicate_merge_operations_entity_type_check",
		"duplicate_merge_operations_source_fields_check",
		"duplicate_merge_operations_relationship_counts_check",
		"duplicate_merge_operations_distinct_records_check",
		"custom_field_definitions_entity_type_check",
		"custom_field_definitions_key_check",
		"custom_field_definitions_label_check",
		"custom_field_definitions_data_type_check",
		"custom_field_definitions_options_array_check",
		"custom_field_definitions_position_check",
		"contacts_custom_fields_object_check",
		"companies_custom_fields_object_check",
		"billing_usage_snapshots_period_check",
		"billing_usage_snapshots_basis_check",
		"billing_usage_snapshots_counts_check",
		"billing_usage_snapshots_period_unique",
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
		"idx_marketing_email_campaigns_org_name_unique",
		"idx_marketing_email_campaigns_org_status",
		"idx_marketing_email_campaigns_org_scheduled",
		"idx_lead_nurture_campaigns_org_name_unique",
		"idx_lead_nurture_campaigns_org_status",
		"idx_lead_nurture_campaigns_org_audience",
		"idx_lead_nurture_campaigns_org_sequence",
		"idx_lead_scoring_rules_org_name_unique",
		"idx_lead_scoring_rules_org_active_position",
		"idx_contacts_org_lead_score",
		"idx_lead_chat_widgets_org_active",
		"idx_lead_chat_widgets_org_form",
		"idx_workflow_automations_org_name_unique",
		"idx_workflow_automations_org_active_position",
		"idx_workflow_automations_org_trigger",
		"idx_workflow_automation_runs_org_automation_event_unique",
		"idx_workflow_automation_runs_org_created",
		"idx_workflow_automation_runs_org_status",
		"idx_workflow_automation_runs_org_automation_created",
		"idx_custom_report_definitions_org_name_unique",
		"idx_custom_report_definitions_org_active",
		"idx_custom_report_definitions_org_source",
		"idx_custom_report_definitions_org_visualization",
		"idx_background_jobs_claim",
		"idx_background_jobs_expired_leases",
		"idx_background_jobs_org_status_created",
		"idx_user_email_accounts_next_sync",
		"idx_email_sequence_deliveries_org_status",
		"idx_email_sequence_deliveries_enrollment",
		"idx_organization_memberships_org_status",
		"idx_record_followers_record",
		"idx_record_followers_user",
		"idx_note_mentions_user",
		"idx_notes_org_id_unique",
		"idx_notifications_idempotency",
		"idx_import_batches_org_created",
		"idx_import_batches_org_status",
		"idx_import_batch_rows_errors",
		"idx_import_batch_rows_entity",
		"idx_contacts_org_active_email_dedupe",
		"idx_contacts_org_active_phone_dedupe",
		"idx_contacts_org_active_identity_dedupe",
		"idx_companies_org_active_name_dedupe",
		"idx_companies_org_active_phone_dedupe",
		"idx_companies_org_active_website_dedupe",
		"idx_bulk_operations_org_created",
		"idx_bulk_operations_org_entity_created",
		"idx_bulk_operation_rows_entity",
		"idx_duplicate_merge_operations_source",
		"idx_duplicate_merge_operations_history",
		"idx_custom_field_definitions_org_active_label",
		"idx_custom_field_definitions_org_entity_position",
		"idx_contacts_custom_fields_gin",
		"idx_companies_custom_fields_gin",
		"idx_bulk_operation_rows_rollback",
		"idx_contact_company_links_unique",
		"idx_contact_company_links_primary_company",
		"idx_sessions_token_hash",
		"idx_sessions_expires_at",
		"idx_notes_org_entity_created",
		"idx_billing_usage_snapshots_org_observed",
		"idx_email_messages_org_sent_period",
		"idx_workflow_automation_runs_org_succeeded_period",
		"idx_background_jobs_org_succeeded_period",
		"idx_background_jobs_retention_succeeded",
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
	expectExecError(t, ctx, pool, `UPDATE deal_stages SET probability_percent = 101 WHERE id = $1`, stageID)
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
	var leadAudienceID int64
	if err := pool.QueryRow(ctx, `INSERT INTO lead_audiences (organization_id, name, filters_json) VALUES ($1, 'Lead Audience', '{"status":"lead"}'::jsonb) RETURNING id`, organizationID).Scan(&leadAudienceID); err != nil {
		t.Fatalf("insert lead audience: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO lead_audiences (organization_id, name, filters_json) VALUES ($1, 'lead audience', '{}'::jsonb)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO marketing_email_campaigns (organization_id, audience_id, name, subject, body) VALUES ($1, $2, '', 'Subject', 'Body')`, organizationID, leadAudienceID)
	expectExecError(t, ctx, pool, `INSERT INTO marketing_email_campaigns (organization_id, audience_id, name, subject, body, status) VALUES ($1, $2, 'Bad Status', 'Subject', 'Body', 'queued')`, organizationID, leadAudienceID)
	expectExecError(t, ctx, pool, `INSERT INTO marketing_email_campaigns (organization_id, audience_id, name, subject, body, status) VALUES ($1, $2, 'Bad Schedule', 'Subject', 'Body', 'scheduled')`, organizationID, leadAudienceID)
	if _, err := pool.Exec(ctx, `INSERT INTO marketing_email_campaigns (organization_id, audience_id, name, subject, body, recipient_count) VALUES ($1, $2, 'Spring Demo Blast', 'Subject', 'Body', 3)`, organizationID, leadAudienceID); err != nil {
		t.Fatalf("insert marketing email campaign: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO marketing_email_campaigns (organization_id, audience_id, name, subject, body) VALUES ($1, $2, 'spring demo blast', 'Subject', 'Body')`, organizationID, leadAudienceID)
	var sequenceID int64
	if err := pool.QueryRow(ctx, `INSERT INTO email_sequences (organization_id, name, status) VALUES ($1, 'Welcome nurture', 'active') RETURNING id`, organizationID).Scan(&sequenceID); err != nil {
		t.Fatalf("insert email sequence: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_sequence_steps (sequence_id, step_order, delay_days, subject, body) VALUES ($1, 1, 0, 'Welcome', 'Body')`, sequenceID); err != nil {
		t.Fatalf("insert email sequence step: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO lead_nurture_campaigns (organization_id, audience_id, sequence_id, name) VALUES ($1, $2, $3, '')`, organizationID, leadAudienceID, sequenceID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_nurture_campaigns (organization_id, audience_id, sequence_id, name, status) VALUES ($1, $2, $3, 'Bad Status', 'queued')`, organizationID, leadAudienceID, sequenceID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_nurture_campaigns (organization_id, audience_id, sequence_id, name, eligible_count) VALUES ($1, $2, $3, 'Bad Count', -1)`, organizationID, leadAudienceID, sequenceID)
	if _, err := pool.Exec(ctx, `INSERT INTO lead_nurture_campaigns (organization_id, audience_id, sequence_id, name, status, eligible_count) VALUES ($1, $2, $3, 'Lead nurture', 'draft', 3)`, organizationID, leadAudienceID, sequenceID); err != nil {
		t.Fatalf("insert lead nurture campaign: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO lead_nurture_campaigns (organization_id, audience_id, sequence_id, name) VALUES ($1, $2, $3, 'lead nurture')`, organizationID, leadAudienceID, sequenceID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_scoring_rules (organization_id, name, field, operator, value, score_delta) VALUES ($1, '', 'status', 'equals', 'lead', 10)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_scoring_rules (organization_id, name, field, operator, value, score_delta) VALUES ($1, 'Bad Field', 'source', 'equals', 'lead', 10)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_scoring_rules (organization_id, name, field, operator, value, score_delta) VALUES ($1, 'Bad Operator', 'status', 'matches', 'lead', 10)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_scoring_rules (organization_id, name, field, operator, value, score_delta) VALUES ($1, 'Bad Value', 'status', 'equals', '', 10)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_scoring_rules (organization_id, name, field, operator, value, score_delta) VALUES ($1, 'Bad Score', 'status', 'equals', 'lead', 101)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_scoring_rules (organization_id, name, field, operator, value, score_delta, position) VALUES ($1, 'Bad Position', 'status', 'equals', 'lead', 10, -1)`, organizationID)
	if _, err := pool.Exec(ctx, `INSERT INTO lead_scoring_rules (organization_id, name, field, operator, value, score_delta, assign_to_user_id) VALUES ($1, 'Lead status fit', 'status', 'equals', 'lead', 10, $2)`, organizationID, userID); err != nil {
		t.Fatalf("insert lead scoring rule: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO lead_scoring_rules (organization_id, name, field, operator, value, score_delta) VALUES ($1, 'lead status fit', 'status', 'equals', 'lead', 10)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_chat_widgets (organization_id, public_id, lead_capture_form_id, name, title) VALUES ($1, 'cw_bad_name', $2, '', 'Widget')`, organizationID, leadFormID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_chat_widgets (organization_id, public_id, lead_capture_form_id, name, title) VALUES ($1, 'cw_bad_title', $2, 'Widget', '')`, organizationID, leadFormID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_chat_widgets (organization_id, public_id, lead_capture_form_id, name, title, prompt_label) VALUES ($1, 'cw_bad_prompt', $2, 'Widget', 'Widget', '')`, organizationID, leadFormID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_chat_widgets (organization_id, public_id, lead_capture_form_id, name, title, cta_label) VALUES ($1, 'cw_bad_cta', $2, 'Widget', 'Widget', '')`, organizationID, leadFormID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_chat_widgets (organization_id, public_id, lead_capture_form_id, name, title, theme) VALUES ($1, 'cw_bad_theme', $2, 'Widget', 'Widget', 'neon')`, organizationID, leadFormID)
	expectExecError(t, ctx, pool, `INSERT INTO lead_chat_widgets (organization_id, public_id, lead_capture_form_id, name, title, position) VALUES ($1, 'cw_bad_position', $2, 'Widget', 'Widget', 'center')`, organizationID, leadFormID)
	if _, err := pool.Exec(ctx, `INSERT INTO lead_chat_widgets (organization_id, public_id, lead_capture_form_id, name, title, prompt_label, cta_label, theme, position) VALUES ($1, 'cw_test', $2, 'Website chat', 'Need help?', 'Chat with us', 'Send', 'blue', 'bottom-left')`, organizationID, leadFormID); err != nil {
		t.Fatalf("insert lead chat widget: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automations (organization_id, name, trigger_type, target_entity_type) VALUES ($1, '', 'record_created', 'contact')`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automations (organization_id, name, trigger_type, target_entity_type) VALUES ($1, 'Bad Trigger', 'message_received', 'contact')`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automations (organization_id, name, trigger_type, target_entity_type) VALUES ($1, 'Bad Target', 'record_created', 'invoice')`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automations (organization_id, name, trigger_type, target_entity_type, trigger_config_json) VALUES ($1, 'Bad Config', 'record_created', 'contact', '[]'::jsonb)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automations (organization_id, name, trigger_type, target_entity_type, condition_logic) VALUES ($1, 'Bad Logic', 'record_created', 'contact', 'xor')`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automations (organization_id, name, trigger_type, target_entity_type, conditions_json) VALUES ($1, 'Bad Conditions', 'record_created', 'contact', '{}'::jsonb)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automations (organization_id, name, trigger_type, target_entity_type, actions_json) VALUES ($1, 'Bad Actions', 'record_created', 'contact', '{}'::jsonb)`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automations (organization_id, name, trigger_type, target_entity_type, position) VALUES ($1, 'Bad Position', 'record_created', 'contact', -1)`, organizationID)
	var workflowAutomationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO workflow_automations (organization_id, name, trigger_type, target_entity_type, trigger_config_json, condition_logic, conditions_json, actions_json, is_active) VALUES ($1, 'New contact automation', 'record_created', 'contact', '{}'::jsonb, 'all', '[{"field":"status","operator":"equals","value":"lead"}]'::jsonb, '[{"type":"create_task","config":{"title":"Call new lead"}}]'::jsonb, TRUE) RETURNING id`, organizationID).Scan(&workflowAutomationID); err != nil {
		t.Fatalf("insert workflow automation: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automations (organization_id, name, trigger_type, target_entity_type) VALUES ($1, 'new contact automation', 'record_created', 'contact')`, organizationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, trigger_event_key, trigger_payload_json) VALUES ($1, $2, '', 'record_created', 'contact', 'evt_bad_name', '{}'::jsonb)`, organizationID, workflowAutomationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, trigger_event_key, status, trigger_payload_json) VALUES ($1, $2, 'New contact automation', 'record_created', 'contact', 'evt_bad_status', 'waiting', '{}'::jsonb)`, organizationID, workflowAutomationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, trigger_event_key, trigger_payload_json) VALUES ($1, $2, 'New contact automation', 'record_created', 'contact', '', '{}'::jsonb)`, organizationID, workflowAutomationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, target_entity_id, trigger_event_key, trigger_payload_json) VALUES ($1, $2, 'New contact automation', 'record_created', 'contact', 0, 'evt_bad_target', '{}'::jsonb)`, organizationID, workflowAutomationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, trigger_event_key, trigger_payload_json) VALUES ($1, $2, 'New contact automation', 'record_created', 'contact', 'evt_bad_payload', '[]'::jsonb)`, organizationID, workflowAutomationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, trigger_event_key, actions_total, actions_completed, trigger_payload_json) VALUES ($1, $2, 'New contact automation', 'record_created', 'contact', 'evt_bad_progress', 1, 2, '{}'::jsonb)`, organizationID, workflowAutomationID)
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, trigger_event_key, status, trigger_payload_json) VALUES ($1, $2, 'New contact automation', 'record_created', 'contact', 'evt_bad_terminal', 'succeeded', '{}'::jsonb)`, organizationID, workflowAutomationID)
	if _, err := pool.Exec(ctx, `INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, target_entity_id, trigger_event_key, trigger_payload_json, actions_total) VALUES ($1, $2, 'New contact automation', 'record_created', 'contact', 7, 'contact:7:created', '{"contactId":7}'::jsonb, 1)`, organizationID, workflowAutomationID); err != nil {
		t.Fatalf("insert workflow automation run: %v", err)
	}
	expectExecError(t, ctx, pool, `INSERT INTO workflow_automation_runs (organization_id, automation_id, automation_name, trigger_type, target_entity_type, trigger_event_key, trigger_payload_json) VALUES ($1, $2, 'New contact automation', 'record_created', 'contact', 'contact:7:created', '{}'::jsonb)`, organizationID, workflowAutomationID)
	expectExecError(t, ctx, pool, `INSERT INTO notes (organization_id, entity_type, entity_id, body, created_by_user_id) VALUES ($1, 'task', 1, 'Bad note', $2)`, organizationID, userID)
	expectExecError(t, ctx, pool, `INSERT INTO tasks (organization_id, entity_type, entity_id, title, status, created_by_user_id) VALUES ($1, 'invoice', 1, 'Bad task', 'open', $2)`, organizationID, userID)
	expectExecError(t, ctx, pool, `INSERT INTO tasks (organization_id, entity_type, entity_id, title, status, created_by_user_id) VALUES ($1, 'contact', 1, 'Bad task', 'done', $2)`, organizationID, userID)
	expectExecError(t, ctx, pool, `INSERT INTO activities (organization_id, entity_type, entity_id, action, summary) VALUES ($1, 'invoice', 1, 'created', 'Bad activity')`, organizationID)

	var contactID int64
	if err := pool.QueryRow(ctx, `INSERT INTO contacts (organization_id, first_name, last_name) VALUES ($1, 'Jane', 'Contact') RETURNING id`, organizationID).Scan(&contactID); err != nil {
		t.Fatalf("insert contact: %v", err)
	}
	expectExecError(t, ctx, pool, `UPDATE contacts SET lead_score = 101 WHERE id = $1`, contactID)
	expectExecError(t, ctx, pool, `UPDATE contacts SET lead_grade = 'F' WHERE id = $1`, contactID)
	expectExecError(t, ctx, pool, `UPDATE contacts SET lead_score_breakdown = '{}'::jsonb WHERE id = $1`, contactID)
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
