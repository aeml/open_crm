package db

import (
	"slices"
	"strings"
	"testing"
)

func TestMigrationFilesIncludeInitialSchema(t *testing.T) {
	files := MigrationFiles()
	if len(files) == 0 {
		t.Fatal("expected at least one migration file")
	}

	found := false
	for _, file := range files {
		if file == "001_initial_schema.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected initial schema migration to be registered")
	}
}

func TestMigrationFilesIncludeBackgroundJobs(t *testing.T) {
	sql := MigrationSQL("056_background_jobs.sql")
	if sql == "" {
		t.Fatal("expected background jobs migration SQL to be embedded")
	}
	for _, expected := range []string{"background_jobs", "idempotency_key", "lease_expires_at", "idx_background_jobs_claim", "status IN ('pending', 'retryable')"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected background jobs migration to include %q", expected)
		}
	}
}

func TestMigrationFilesIncludeMailboxSyncJobs(t *testing.T) {
	sql := MigrationSQL("057_mailbox_sync_jobs.sql")
	if sql == "" || !strings.Contains(sql, "next_sync_at") || !strings.Contains(sql, "idx_user_email_accounts_next_sync") {
		t.Fatalf("expected mailbox sync jobs migration to be embedded, got %q", sql)
	}
}

func TestMigrationFilesIncludeSequenceDeliveryJobs(t *testing.T) {
	sql := MigrationSQL("058_sequence_delivery_jobs.sql")
	if sql == "" {
		t.Fatal("expected sequence delivery jobs migration SQL to be embedded")
	}
	for _, expected := range []string{"email_sequence_deliveries", "uncertain", "email_sequence.send", "idx_email_sequence_deliveries_org_status"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected sequence delivery jobs migration to include %q", expected)
		}
	}
}

func TestMigrationFilesIncludeUserLifecycle(t *testing.T) {
	sql := MigrationSQL("059_user_lifecycle.sql")
	if sql == "" {
		t.Fatal("expected user lifecycle migration SQL to be embedded")
	}
	for _, expected := range []string{"membership_status", "organization_memberships_status_check", "status_changed_by_user_id", "idx_organization_memberships_org_status"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected user lifecycle migration to include %q", expected)
		}
	}
}

func TestMigrationFilesIncludeCollaboration(t *testing.T) {
	sql := MigrationSQL("060_collaboration.sql")
	if sql == "" {
		t.Fatal("expected collaboration migration SQL to be embedded")
	}
	for _, expected := range []string{"record_followers", "note_mentions", "organization_memberships(organization_id, user_id)", "idx_notes_org_id_unique", "idempotency_key", "idx_notifications_idempotency"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected collaboration migration to include %q", expected)
		}
	}
}

func TestMigrationFilesIncludeImportBatches(t *testing.T) {
	sql := MigrationSQL("061_import_batches.sql")
	if sql == "" {
		t.Fatal("expected import batches migration SQL to be embedded")
	}
	for _, expected := range []string{"import_batches", "import_batch_rows", "idempotency_key", "source_sha256", "rollback_skipped", "idx_import_batch_rows_errors"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected import batches migration to include %q", expected)
		}
	}
}

func TestMigrationFilesIncludeBulkOperations(t *testing.T) {
	sql := MigrationSQL("062_bulk_operations.sql")
	if sql == "" {
		t.Fatal("expected bulk operations migration SQL to be embedded")
	}
	for _, expected := range []string{"bulk_operations", "bulk_operation_rows", "idempotency_key", "request_sha256", "applied_entity_updated_at", "rollback_skipped"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected bulk operations migration to include %q", expected)
		}
	}
}

func TestMigrationFilesIncludeDuplicateMerges(t *testing.T) {
	sql := MigrationSQL("063_duplicate_merges.sql")
	if sql == "" {
		t.Fatal("expected duplicate merges migration SQL to be embedded")
	}
	for _, expected := range []string{"duplicate_merge_operations", "idempotency_key", "request_sha256", "source_fields", "relationship_counts", "idx_duplicate_merge_operations_source"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected duplicate merges migration to include %q", expected)
		}
	}
}

func TestAutomaticMigrationCompatibilityPolicy(t *testing.T) {
	for _, name := range MigrationFiles() {
		if name < deploymentClassBaseline {
			if class := MigrationDeploymentClass(name); class != "legacy" {
				t.Fatalf("historical migration %s class = %q", name, class)
			}
			continue
		}
		class := MigrationDeploymentClass(name)
		if class != "expand" && class != "contract" {
			t.Fatalf("migration %s has invalid deployment class %q", name, class)
		}
		if err := validateAutomaticMigration(name, MigrationSQL(name), false); err != nil {
			t.Fatalf("migration %s is not safe for automatic deploy: %v", name, err)
		}
	}

	unsafeExpand := "-- open-crm-deploy: expand\nALTER TABLE contacts DROP COLUMN email;"
	if err := validateAutomaticMigration("999_unsafe.sql", unsafeExpand, false); err == nil || !strings.Contains(err.Error(), "unsafe drop") {
		t.Fatalf("unsafe expand migration was not rejected: %v", err)
	}
	safeConstraint := "-- open-crm-deploy: expand\nALTER TABLE contacts ADD CONSTRAINT contacts_future_check CHECK (email <> '') NOT VALID;"
	if err := validateAutomaticMigration("999_safe_constraint.sql", safeConstraint, false); err != nil {
		t.Fatalf("safe not-valid expand constraint was rejected: %v", err)
	}
	unsafeConstraint := "-- open-crm-deploy: expand\nALTER TABLE contacts ADD CONSTRAINT contacts_blocking_check CHECK (email <> '');"
	if err := validateAutomaticMigration("999_unsafe_constraint.sql", unsafeConstraint, false); err == nil || !strings.Contains(err.Error(), "unsafe new constraint") {
		t.Fatalf("blocking expand constraint was not rejected: %v", err)
	}
	contract := "-- open-crm-deploy: contract\nALTER TABLE contacts DROP COLUMN email;"
	if err := validateAutomaticMigration("999_contract.sql", contract, false); err == nil || !strings.Contains(err.Error(), "maintenance window") {
		t.Fatalf("unapproved contract migration was not rejected: %v", err)
	}
	if err := validateAutomaticMigration("999_contract.sql", contract, true); err != nil {
		t.Fatalf("explicitly approved contract migration was rejected: %v", err)
	}
}

func TestMigrationFilesIncludeTaskArchiveMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "007_task_archive.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected task archive migration to be registered")
	}

	if MigrationSQL("007_task_archive.sql") == "" {
		t.Fatal("expected task archive migration SQL to be embedded")
	}
}

func TestMigrationFilesIncludeUserSetupTokensMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "008_user_setup_tokens.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected user setup tokens migration to be registered")
	}

	if MigrationSQL("008_user_setup_tokens.sql") == "" {
		t.Fatal("expected user setup tokens migration SQL to be embedded")
	}
}

func TestMigrationFilesIncludeDatabaseIntegrityMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "009_database_integrity.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected database integrity migration to be registered")
	}

	sql := MigrationSQL("009_database_integrity.sql")
	if sql == "" {
		t.Fatal("expected database integrity migration SQL to be embedded")
	}
	for _, expected := range []string{"organizations_business_type_check", "organization_memberships_role_check", "deals_value_amount_nonnegative_check", "tasks_entity_type_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected database integrity migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeSavedViewsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "010_saved_views.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected saved views migration to be registered")
	}

	sql := MigrationSQL("010_saved_views.sql")
	if sql == "" {
		t.Fatal("expected saved views migration SQL to be embedded")
	}
	for _, expected := range []string{"saved_views", "idx_saved_views_org_user_entity_name", "idx_saved_views_default_per_entity"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected saved views migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeEmailMessageEntityLinksMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "025_email_message_entity_links.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected email message entity links migration to be registered")
	}

	sql := MigrationSQL("025_email_message_entity_links.sql")
	if sql == "" {
		t.Fatal("expected email message entity links migration SQL to be embedded")
	}
	for _, expected := range []string{"email_message_entity_links", "idx_email_message_entity_links_entity", "ON CONFLICT"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected email message entity links migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeEmailMessageVisibilityMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "026_email_message_visibility.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected email message visibility migration to be registered")
	}

	sql := MigrationSQL("026_email_message_visibility.sql")
	if sql == "" {
		t.Fatal("expected email message visibility migration SQL to be embedded")
	}
	for _, expected := range []string{"visibility", "email_messages_visibility_check", "direction = 'inbound'"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected email message visibility migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeEmailSuppressionsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "027_email_suppressions.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected email suppressions migration to be registered")
	}

	sql := MigrationSQL("027_email_suppressions.sql")
	if sql == "" {
		t.Fatal("expected email suppressions migration SQL to be embedded")
	}
	for _, expected := range []string{"email_suppressions", "idx_email_suppressions_org_email", "email_suppressions_reason_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected email suppressions migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeEmailSnippetsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "028_email_snippets.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected email snippets migration to be registered")
	}

	sql := MigrationSQL("028_email_snippets.sql")
	if sql == "" {
		t.Fatal("expected email snippets migration SQL to be embedded")
	}
	for _, expected := range []string{"email_snippets", "idx_email_snippets_org_name", "organization_id"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected email snippets migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeEmailSharedInboxMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "029_email_shared_inbox.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected email shared inbox migration to be registered")
	}

	sql := MigrationSQL("029_email_shared_inbox.sql")
	if sql == "" {
		t.Fatal("expected email shared inbox migration SQL to be embedded")
	}
	for _, expected := range []string{"shared_inbox_status", "shared_inbox_assigned_to_user_id", "idx_email_messages_shared_inbox"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected email shared inbox migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeCallLogsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "030_call_logs.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected call logs migration to be registered")
	}

	sql := MigrationSQL("030_call_logs.sql")
	if sql == "" {
		t.Fatal("expected call logs migration SQL to be embedded")
	}
	for _, expected := range []string{"call_logs", "idx_call_logs_org_entity_created", "call_logs_status_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected call logs migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeCallRecordingControlsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "031_call_recording_controls.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected call recording controls migration to be registered")
	}

	sql := MigrationSQL("031_call_recording_controls.sql")
	if sql == "" {
		t.Fatal("expected call recording controls migration SQL to be embedded")
	}
	for _, expected := range []string{"recording_status", "recording_consent", "recording_retention_until", "idx_call_logs_recording_retention"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected call recording controls migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeSMSFoundationMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "032_sms_foundation.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected sms foundation migration to be registered")
	}

	sql := MigrationSQL("032_sms_foundation.sql")
	if sql == "" {
		t.Fatal("expected sms foundation migration SQL to be embedded")
	}
	for _, expected := range []string{"sms_messages", "sms_suppressions", "idx_sms_messages_org_entity_created", "idx_sms_suppressions_org_phone"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected sms foundation migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeCalendarFoundationMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "033_calendar_foundation.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected calendar foundation migration to be registered")
	}

	sql := MigrationSQL("033_calendar_foundation.sql")
	if sql == "" {
		t.Fatal("expected calendar foundation migration SQL to be embedded")
	}
	for _, expected := range []string{"calendar_events", "calendar_availability_blocks", "idx_calendar_events_org_entity_start", "idx_calendar_availability_org_user"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected calendar foundation migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeCalendarBookingLinksMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "034_calendar_booking_links.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected calendar booking links migration to be registered")
	}

	sql := MigrationSQL("034_calendar_booking_links.sql")
	if sql == "" {
		t.Fatal("expected calendar booking links migration SQL to be embedded")
	}
	for _, expected := range []string{"calendar_booking_links", "calendar_booking_link_members", "idx_calendar_booking_links_org_slug", "idx_calendar_booking_link_members_link"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected calendar booking links migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeCalendarRemindersMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "035_calendar_reminders.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected calendar reminders migration to be registered")
	}

	sql := MigrationSQL("035_calendar_reminders.sql")
	if sql == "" {
		t.Fatal("expected calendar reminders migration SQL to be embedded")
	}
	for _, expected := range []string{"calendar_event_reminders", "idx_calendar_event_reminders_due", "idx_calendar_event_reminders_org_event", "idx_calendar_event_reminders_user_status"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected calendar reminders migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeProductCatalogMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "036_product_catalog.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected product catalog migration to be registered")
	}

	sql := MigrationSQL("036_product_catalog.sql")
	if sql == "" {
		t.Fatal("expected product catalog migration SQL to be embedded")
	}
	for _, expected := range []string{"product_catalog_items", "idx_product_catalog_items_org_sku", "idx_product_catalog_items_org_active_name", "product_catalog_items_type_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected product catalog migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeDealLineItemsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "037_deal_line_items.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected deal line items migration to be registered")
	}

	sql := MigrationSQL("037_deal_line_items.sql")
	if sql == "" {
		t.Fatal("expected deal line items migration SQL to be embedded")
	}
	for _, expected := range []string{"deal_line_items", "idx_deal_line_items_org_deal_position", "deal_line_items_discount_lte_subtotal", "deal_line_items_tax_rate_range"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected deal line items migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeDealSignatureRequestsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "038_deal_signature_requests.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected deal signature requests migration to be registered")
	}

	sql := MigrationSQL("038_deal_signature_requests.sql")
	if sql == "" {
		t.Fatal("expected deal signature requests migration SQL to be embedded")
	}
	for _, expected := range []string{"deal_signature_requests", "idx_deal_signature_requests_org_deal_created", "idx_deal_signature_requests_org_status", "deal_signature_requests_status_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected deal signature requests migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeDealPipelinesMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "039_deal_pipelines.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected deal pipelines migration to be registered")
	}

	sql := MigrationSQL("039_deal_pipelines.sql")
	if sql == "" {
		t.Fatal("expected deal pipelines migration SQL to be embedded")
	}
	for _, expected := range []string{"deal_pipelines", "pipeline_id", "idx_deal_pipelines_org_position_unique", "idx_deal_stages_org_pipeline_position_unique"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected deal pipelines migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeSalesQuotasMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "040_sales_quotas.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected sales quotas migration to be registered")
	}

	sql := MigrationSQL("040_sales_quotas.sql")
	if sql == "" {
		t.Fatal("expected sales quotas migration SQL to be embedded")
	}
	for _, expected := range []string{"sales_quotas", "idx_sales_quotas_org_user_period_unique", "sales_quotas_amount_nonnegative_check", "sales_quotas_currency_code_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected sales quotas migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeStageProbabilitiesMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "065_stage_probabilities.sql") {
		t.Fatal("expected stage probabilities migration to be registered")
	}

	sql := MigrationSQL("065_stage_probabilities.sql")
	for _, expected := range []string{"probability_percent", "positioned_stages", "BETWEEN 0 AND 100", "open-crm-deploy: expand"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected stage probabilities migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeTaskRemindersMigration(t *testing.T) {
	files := MigrationFiles()
	found := false
	for _, file := range files {
		if file == "066_task_reminders.sql" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected task reminders migration to be registered")
	}
	if sql := MigrationSQL("066_task_reminders.sql"); !strings.Contains(sql, "CREATE TABLE task_reminders") || !strings.Contains(sql, "'task.reminder'") {
		t.Fatal("expected durable task reminder schema and job backfill")
	}
}

func TestMigrationFilesIncludeCurrencyExchangeRatesMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "041_currency_exchange_rates.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected currency exchange rates migration to be registered")
	}

	sql := MigrationSQL("041_currency_exchange_rates.sql")
	if sql == "" {
		t.Fatal("expected currency exchange rates migration SQL to be embedded")
	}
	for _, expected := range []string{"base_currency", "organization_exchange_rates", "idx_org_exchange_rates_unique_effective", "organization_exchange_rates_rate_positive_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected currency exchange rates migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeLeadCaptureFormsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "042_lead_capture_forms.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected lead capture forms migration to be registered")
	}

	sql := MigrationSQL("042_lead_capture_forms.sql")
	if sql == "" {
		t.Fatal("expected lead capture forms migration SQL to be embedded")
	}
	for _, expected := range []string{"lead_capture_forms", "lead_capture_submissions", "idx_lead_capture_forms_org_slug_unique", "lead_capture_forms_fields_json_array_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected lead capture forms migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeLeadLandingPagesMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "043_lead_landing_pages.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected lead landing pages migration to be registered")
	}

	sql := MigrationSQL("043_lead_landing_pages.sql")
	if sql == "" {
		t.Fatal("expected lead landing pages migration SQL to be embedded")
	}
	for _, expected := range []string{"lead_landing_pages", "idx_lead_landing_pages_slug_unique", "lead_landing_pages_theme_check", "lead_landing_pages_form_org_fk"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected lead landing pages migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeLeadAttributionMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "044_lead_attribution.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected lead attribution migration to be registered")
	}

	sql := MigrationSQL("044_lead_attribution.sql")
	if sql == "" {
		t.Fatal("expected lead attribution migration SQL to be embedded")
	}
	for _, expected := range []string{"lead_source", "utm_campaign", "idx_contacts_org_lead_source", "idx_lead_capture_submissions_org_attribution"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected lead attribution migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeLeadAudiencesMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "045_lead_audiences.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected lead audiences migration to be registered")
	}

	sql := MigrationSQL("045_lead_audiences.sql")
	if sql == "" {
		t.Fatal("expected lead audiences migration SQL to be embedded")
	}
	for _, expected := range []string{"lead_audiences", "filters_json", "idx_lead_audiences_org_name_unique", "lead_audiences_filters_json_object_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected lead audiences migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeMarketingEmailCampaignsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "046_marketing_email_campaigns.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected marketing email campaigns migration to be registered")
	}

	sql := MigrationSQL("046_marketing_email_campaigns.sql")
	if sql == "" {
		t.Fatal("expected marketing email campaigns migration SQL to be embedded")
	}
	for _, expected := range []string{"marketing_email_campaigns", "audience_id", "scheduled_at", "recipient_count", "idx_marketing_email_campaigns_org_scheduled"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected marketing email campaigns migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeLeadNurtureCampaignsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "047_lead_nurture_campaigns.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected lead nurture campaigns migration to be registered")
	}

	sql := MigrationSQL("047_lead_nurture_campaigns.sql")
	if sql == "" {
		t.Fatal("expected lead nurture campaigns migration SQL to be embedded")
	}
	for _, expected := range []string{"lead_nurture_campaigns", "sequence_id", "eligible_count", "email_sequences_org_id_unique", "idx_lead_nurture_campaigns_org_sequence"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected lead nurture campaigns migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeLeadScoringRoutingMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "048_lead_scoring_routing.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected lead scoring routing migration to be registered")
	}

	sql := MigrationSQL("048_lead_scoring_routing.sql")
	if sql == "" {
		t.Fatal("expected lead scoring routing migration SQL to be embedded")
	}
	for _, expected := range []string{"lead_scoring_rules", "lead_score", "assign_to_user_id", "idx_lead_scoring_rules_org_active_position", "contacts_lead_grade_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected lead scoring routing migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeLeadChatWidgetsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "049_lead_chat_widgets.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected lead chat widgets migration to be registered")
	}

	sql := MigrationSQL("049_lead_chat_widgets.sql")
	if sql == "" {
		t.Fatal("expected lead chat widgets migration SQL to be embedded")
	}
	for _, expected := range []string{"lead_chat_widgets", "public_id", "lead_chat_widgets_form_org_fk", "lead_chat_widgets_position_check", "idx_lead_chat_widgets_org_active"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected lead chat widgets migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeWorkflowAutomationsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "050_workflow_automations.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected workflow automations migration to be registered")
	}

	sql := MigrationSQL("050_workflow_automations.sql")
	if sql == "" {
		t.Fatal("expected workflow automations migration SQL to be embedded")
	}
	for _, expected := range []string{"workflow_automations", "trigger_type", "trigger_config_json", "idx_workflow_automations_org_active_position", "workflow_automations_trigger_type_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected workflow automations migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeWorkflowAutomationConditionsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "051_workflow_automation_conditions.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected workflow automation conditions migration to be registered")
	}

	sql := MigrationSQL("051_workflow_automation_conditions.sql")
	if sql == "" {
		t.Fatal("expected workflow automation conditions migration SQL to be embedded")
	}
	for _, expected := range []string{"condition_logic", "conditions_json", "workflow_automations_condition_logic_check", "workflow_automations_conditions_json_array_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected workflow automation conditions migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeWorkflowAutomationActionsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "052_workflow_automation_actions.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected workflow automation actions migration to be registered")
	}

	sql := MigrationSQL("052_workflow_automation_actions.sql")
	if sql == "" {
		t.Fatal("expected workflow automation actions migration SQL to be embedded")
	}
	for _, expected := range []string{"actions_json", "workflow_automations_actions_json_array_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected workflow automation actions migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeWorkflowAutomationRunsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "053_workflow_automation_runs.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected workflow automation runs migration to be registered")
	}

	sql := MigrationSQL("053_workflow_automation_runs.sql")
	if sql == "" {
		t.Fatal("expected workflow automation runs migration SQL to be embedded")
	}
	for _, expected := range []string{"workflow_automation_runs", "trigger_event_key", "workflow_automation_runs_status_check", "idx_workflow_automation_runs_org_automation_event_unique"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected workflow automation runs migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeCustomReportDefinitionsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "054_custom_report_definitions.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected custom report definitions migration to be registered")
	}

	sql := MigrationSQL("054_custom_report_definitions.sql")
	if sql == "" {
		t.Fatal("expected custom report definitions migration SQL to be embedded")
	}
	for _, expected := range []string{"custom_report_definitions", "columns_json", "filters_json", "aggregation_json", "idx_custom_report_definitions_org_name_unique"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected custom report definitions migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeCustomReportVisualizationsMigration(t *testing.T) {
	files := MigrationFiles()

	found := false
	for _, file := range files {
		if file == "055_custom_report_visualizations.sql" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected custom report visualizations migration to be registered")
	}

	sql := MigrationSQL("055_custom_report_visualizations.sql")
	if sql == "" {
		t.Fatal("expected custom report visualizations migration SQL to be embedded")
	}
	for _, expected := range []string{"visualization_type", "custom_report_definitions_visualization_type_check", "idx_custom_report_definitions_org_visualization"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected custom report visualizations migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeCustomFieldsMigration(t *testing.T) {
	files := MigrationFiles()
	found := false
	for _, file := range files {
		if file == "064_custom_fields.sql" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected custom fields migration to be registered")
	}

	sql := MigrationSQL("064_custom_fields.sql")
	for _, expected := range []string{"custom_field_definitions", "contacts_custom_fields_object_check", "companies_custom_fields_object_check", "idx_contacts_custom_fields_gin", "idx_companies_custom_fields_gin"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected custom fields migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeSalesActivityReportingMigration(t *testing.T) {
	files := MigrationFiles()
	found := false
	for _, file := range files {
		if file == "067_sales_activity_reporting.sql" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected sales activity reporting migration to be registered")
	}

	sql := MigrationSQL("067_sales_activity_reporting.sql")
	for _, expected := range []string{"sales_activity_tracking_started_at", "deal_stage_events", "from_stage_outcome", "to_stage_outcome", "idx_deal_stage_events_org_owner_occurred"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected sales activity reporting migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeDealCloseReviewsMigration(t *testing.T) {
	files := MigrationFiles()
	found := false
	for _, file := range files {
		if file == "068_deal_close_reviews.sql" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected deal close reviews migration to be registered")
	}

	sql := MigrationSQL("068_deal_close_reviews.sql")
	for _, expected := range []string{"deal_close_reason_tracking_started_at", "close_reason_code", "closed_by_user_id", "idx_deal_stage_events_org_outcome_reason"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected deal close reviews migration to include %s", expected)
		}
	}
}

func TestMigrationFilesIncludeSalesReportQueryIndexesMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "069_sales_report_query_indexes.sql") {
		t.Fatal("expected sales report query indexes migration to be registered")
	}

	sql := MigrationSQL("069_sales_report_query_indexes.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "idx_activities_sales_report_org_created", "idx_activities_sales_report_org_actor_created", "WHERE action IN"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected sales report query indexes migration to include %s", expected)
		}
	}
	if class := MigrationDeploymentClass("069_sales_report_query_indexes.sql"); class != "expand" {
		t.Fatalf("expected sales report query indexes migration to be expand-safe, got %q", class)
	}
}

func TestMigrationFilesIncludeWonDealCustomerHandoffMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "070_won_deal_customer_handoff.sql") {
		t.Fatal("expected won-deal customer handoff migration to be registered")
	}

	sql := MigrationSQL("070_won_deal_customer_handoff.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "idx_deals_org_company_active_updated", "idx_deals_org_primary_contact_active_updated", "UPDATE companies", "UPDATE contacts", "deal.company_id IS NULL"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected won-deal customer handoff migration to include %s", expected)
		}
	}
	if class := MigrationDeploymentClass("070_won_deal_customer_handoff.sql"); class != "expand" {
		t.Fatalf("expected won-deal customer handoff migration to be expand-safe, got %q", class)
	}
}

func TestMigrationFilesIncludeClientHealthQueryIndexesMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "071_client_health_query_indexes.sql") {
		t.Fatal("expected client-health query indexes migration to be registered")
	}

	sql := MigrationSQL("071_client_health_query_indexes.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "idx_tasks_org_open_entity_due", "idx_companies_org_customer_owner_name", "idx_contacts_org_client_owner_name", "lock_timeout", "statement_timeout"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected client-health query indexes migration to include %s", expected)
		}
	}
	if class := MigrationDeploymentClass("071_client_health_query_indexes.sql"); class != "expand" {
		t.Fatalf("expected client-health query indexes migration to be expand-safe, got %q", class)
	}
}

func TestMigrationFilesIncludeClientReviewSchedulesMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "072_client_review_schedules.sql") {
		t.Fatal("expected client-review schedules migration to be registered")
	}

	sql := MigrationSQL("072_client_review_schedules.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "client_review_schedules", "client_review_schedules_entity_unique", "client_review_schedules_task_org_fk", "idx_client_review_schedules_org_active_due", "lock_timeout", "statement_timeout"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected client-review schedules migration to include %s", expected)
		}
	}
	if class := MigrationDeploymentClass("072_client_review_schedules.sql"); class != "expand" {
		t.Fatalf("expected client-review schedules migration to be expand-safe, got %q", class)
	}
}

func TestMigrationFilesIncludeVerifiedWorkspaceSignupMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "073_verified_workspace_signup.sql") {
		t.Fatal("expected verified workspace signup migration to be registered")
	}
	sql := MigrationSQL("073_verified_workspace_signup.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "email_verified_at", "email_verification_token_hash", "trial_started_at", "workspace_bootstrap_requests", "request_sha256"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("verified workspace signup migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("073_verified_workspace_signup.sql"); class != "expand" {
		t.Fatalf("verified workspace signup deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeStripeBillingLifecycleMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "074_stripe_billing_lifecycle.sql") {
		t.Fatal("expected Stripe billing lifecycle migration to be registered")
	}
	sql := MigrationSQL("074_stripe_billing_lifecycle.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "stripe_customer_id", "billing_checkout_requests", "billing_webhook_events", "billing_invoices", "payload_sha256"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("Stripe billing lifecycle migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("074_stripe_billing_lifecycle.sql"); class != "expand" {
		t.Fatalf("Stripe billing lifecycle deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeBillingReconciliationMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "075_billing_reconciliation.sql") {
		t.Fatal("expected billing reconciliation migration to be registered")
	}
	sql := MigrationSQL("075_billing_reconciliation.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "billing_last_reconciliation_attempt_at", "billing_last_reconciled_at", "billing_last_reconciliation_error", "idx_organizations_billing_reconciliation_due", "lock_timeout", "statement_timeout"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("billing reconciliation migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("075_billing_reconciliation.sql"); class != "expand" {
		t.Fatalf("billing reconciliation deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeWorkspaceExportsMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "076_workspace_exports.sql") {
		t.Fatal("expected workspace exports migration to be registered")
	}
	sql := MigrationSQL("076_workspace_exports.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "workspace_exports", "artifact BYTEA", "dataset_counts", "workspace_exports_ready_shape_check", "idx_workspace_exports_expiry"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("workspace exports migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("076_workspace_exports.sql"); class != "expand" {
		t.Fatalf("workspace exports deployment class = %q", class)
	}
}
