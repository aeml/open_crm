package db

import (
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
