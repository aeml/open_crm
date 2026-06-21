package db

import _ "embed"

//go:embed migrations/001_initial_schema.sql
var initialSchemaSQL string

//go:embed migrations/002_company_client_type.sql
var companyClientTypeSQL string

//go:embed migrations/003_contact_client_flag.sql
var contactClientFlagSQL string

//go:embed migrations/004_client_address.sql
var clientAddressSQL string

//go:embed migrations/005_client_structured_address.sql
var clientStructuredAddressSQL string

//go:embed migrations/006_remove_company_domain.sql
var removeCompanyDomainSQL string

//go:embed migrations/007_task_archive.sql
var taskArchiveSQL string

//go:embed migrations/008_user_setup_tokens.sql
var userSetupTokensSQL string

//go:embed migrations/009_database_integrity.sql
var databaseIntegritySQL string

//go:embed migrations/010_saved_views.sql
var savedViewsSQL string

//go:embed migrations/011_audit_events.sql
var auditEventsSQL string

//go:embed migrations/012_user_preferences.sql
var userPreferencesSQL string

//go:embed migrations/013_notifications.sql
var notificationsSQL string

//go:embed migrations/014_billing_plans.sql
var billingPlansSQL string

//go:embed migrations/015_subscription_lifecycle.sql
var subscriptionLifecycleSQL string

//go:embed migrations/016_email_templates.sql
var emailTemplatesSQL string

//go:embed migrations/017_user_email_accounts.sql
var userEmailAccountsSQL string

//go:embed migrations/018_email_messages.sql
var emailMessagesSQL string

//go:embed migrations/019_email_open_tracking.sql
var emailOpenTrackingSQL string

//go:embed migrations/020_email_click_tracking.sql
var emailClickTrackingSQL string

//go:embed migrations/021_email_sequences.sql
var emailSequencesSQL string

//go:embed migrations/022_email_sequence_enrollments.sql
var emailSequenceEnrollmentsSQL string

//go:embed migrations/023_user_email_sync_foundation.sql
var userEmailSyncFoundationSQL string

//go:embed migrations/024_inbound_email_messages.sql
var inboundEmailMessagesSQL string

//go:embed migrations/025_email_message_entity_links.sql
var emailMessageEntityLinksSQL string

//go:embed migrations/026_email_message_visibility.sql
var emailMessageVisibilitySQL string

//go:embed migrations/027_email_suppressions.sql
var emailSuppressionsSQL string

//go:embed migrations/028_email_snippets.sql
var emailSnippetsSQL string

//go:embed migrations/029_email_shared_inbox.sql
var emailSharedInboxSQL string

//go:embed migrations/030_call_logs.sql
var callLogsSQL string

//go:embed migrations/031_call_recording_controls.sql
var callRecordingControlsSQL string

//go:embed migrations/032_sms_foundation.sql
var smsFoundationSQL string

//go:embed migrations/033_calendar_foundation.sql
var calendarFoundationSQL string

//go:embed migrations/034_calendar_booking_links.sql
var calendarBookingLinksSQL string

//go:embed migrations/035_calendar_reminders.sql
var calendarRemindersSQL string

//go:embed migrations/036_product_catalog.sql
var productCatalogSQL string

//go:embed migrations/037_deal_line_items.sql
var dealLineItemsSQL string

//go:embed migrations/038_deal_signature_requests.sql
var dealSignatureRequestsSQL string

//go:embed migrations/039_deal_pipelines.sql
var dealPipelinesSQL string

//go:embed migrations/040_sales_quotas.sql
var salesQuotasSQL string

//go:embed migrations/041_currency_exchange_rates.sql
var currencyExchangeRatesSQL string

//go:embed migrations/042_lead_capture_forms.sql
var leadCaptureFormsSQL string

//go:embed migrations/043_lead_landing_pages.sql
var leadLandingPagesSQL string

//go:embed migrations/044_lead_attribution.sql
var leadAttributionSQL string

//go:embed migrations/045_lead_audiences.sql
var leadAudiencesSQL string

//go:embed migrations/046_marketing_email_campaigns.sql
var marketingEmailCampaignsSQL string

//go:embed migrations/047_lead_nurture_campaigns.sql
var leadNurtureCampaignsSQL string

//go:embed migrations/048_lead_scoring_routing.sql
var leadScoringRoutingSQL string

//go:embed migrations/049_lead_chat_widgets.sql
var leadChatWidgetsSQL string

//go:embed migrations/050_workflow_automations.sql
var workflowAutomationsSQL string

func MigrationFiles() []string {
	return []string{"001_initial_schema.sql", "002_company_client_type.sql", "003_contact_client_flag.sql", "004_client_address.sql", "005_client_structured_address.sql", "006_remove_company_domain.sql", "007_task_archive.sql", "008_user_setup_tokens.sql", "009_database_integrity.sql", "010_saved_views.sql", "011_audit_events.sql", "012_user_preferences.sql", "013_notifications.sql", "014_billing_plans.sql", "015_subscription_lifecycle.sql", "016_email_templates.sql", "017_user_email_accounts.sql", "018_email_messages.sql", "019_email_open_tracking.sql", "020_email_click_tracking.sql", "021_email_sequences.sql", "022_email_sequence_enrollments.sql", "023_user_email_sync_foundation.sql", "024_inbound_email_messages.sql", "025_email_message_entity_links.sql", "026_email_message_visibility.sql", "027_email_suppressions.sql", "028_email_snippets.sql", "029_email_shared_inbox.sql", "030_call_logs.sql", "031_call_recording_controls.sql", "032_sms_foundation.sql", "033_calendar_foundation.sql", "034_calendar_booking_links.sql", "035_calendar_reminders.sql", "036_product_catalog.sql", "037_deal_line_items.sql", "038_deal_signature_requests.sql", "039_deal_pipelines.sql", "040_sales_quotas.sql", "041_currency_exchange_rates.sql", "042_lead_capture_forms.sql", "043_lead_landing_pages.sql", "044_lead_attribution.sql", "045_lead_audiences.sql", "046_marketing_email_campaigns.sql", "047_lead_nurture_campaigns.sql", "048_lead_scoring_routing.sql", "049_lead_chat_widgets.sql", "050_workflow_automations.sql"}
}

func MigrationSQL(name string) string {
	if name == "001_initial_schema.sql" {
		return initialSchemaSQL
	}
	if name == "002_company_client_type.sql" {
		return companyClientTypeSQL
	}
	if name == "003_contact_client_flag.sql" {
		return contactClientFlagSQL
	}
	if name == "004_client_address.sql" {
		return clientAddressSQL
	}
	if name == "005_client_structured_address.sql" {
		return clientStructuredAddressSQL
	}
	if name == "006_remove_company_domain.sql" {
		return removeCompanyDomainSQL
	}
	if name == "007_task_archive.sql" {
		return taskArchiveSQL
	}
	if name == "008_user_setup_tokens.sql" {
		return userSetupTokensSQL
	}
	if name == "009_database_integrity.sql" {
		return databaseIntegritySQL
	}
	if name == "010_saved_views.sql" {
		return savedViewsSQL
	}
	if name == "011_audit_events.sql" {
		return auditEventsSQL
	}
	if name == "012_user_preferences.sql" {
		return userPreferencesSQL
	}
	if name == "013_notifications.sql" {
		return notificationsSQL
	}
	if name == "014_billing_plans.sql" {
		return billingPlansSQL
	}
	if name == "015_subscription_lifecycle.sql" {
		return subscriptionLifecycleSQL
	}
	if name == "016_email_templates.sql" {
		return emailTemplatesSQL
	}
	if name == "017_user_email_accounts.sql" {
		return userEmailAccountsSQL
	}
	if name == "018_email_messages.sql" {
		return emailMessagesSQL
	}
	if name == "019_email_open_tracking.sql" {
		return emailOpenTrackingSQL
	}
	if name == "020_email_click_tracking.sql" {
		return emailClickTrackingSQL
	}
	if name == "021_email_sequences.sql" {
		return emailSequencesSQL
	}
	if name == "022_email_sequence_enrollments.sql" {
		return emailSequenceEnrollmentsSQL
	}
	if name == "023_user_email_sync_foundation.sql" {
		return userEmailSyncFoundationSQL
	}
	if name == "024_inbound_email_messages.sql" {
		return inboundEmailMessagesSQL
	}
	if name == "025_email_message_entity_links.sql" {
		return emailMessageEntityLinksSQL
	}
	if name == "026_email_message_visibility.sql" {
		return emailMessageVisibilitySQL
	}
	if name == "027_email_suppressions.sql" {
		return emailSuppressionsSQL
	}
	if name == "028_email_snippets.sql" {
		return emailSnippetsSQL
	}
	if name == "029_email_shared_inbox.sql" {
		return emailSharedInboxSQL
	}
	if name == "030_call_logs.sql" {
		return callLogsSQL
	}
	if name == "031_call_recording_controls.sql" {
		return callRecordingControlsSQL
	}
	if name == "032_sms_foundation.sql" {
		return smsFoundationSQL
	}
	if name == "033_calendar_foundation.sql" {
		return calendarFoundationSQL
	}
	if name == "034_calendar_booking_links.sql" {
		return calendarBookingLinksSQL
	}
	if name == "035_calendar_reminders.sql" {
		return calendarRemindersSQL
	}
	if name == "036_product_catalog.sql" {
		return productCatalogSQL
	}
	if name == "037_deal_line_items.sql" {
		return dealLineItemsSQL
	}
	if name == "038_deal_signature_requests.sql" {
		return dealSignatureRequestsSQL
	}
	if name == "039_deal_pipelines.sql" {
		return dealPipelinesSQL
	}
	if name == "040_sales_quotas.sql" {
		return salesQuotasSQL
	}
	if name == "041_currency_exchange_rates.sql" {
		return currencyExchangeRatesSQL
	}
	if name == "042_lead_capture_forms.sql" {
		return leadCaptureFormsSQL
	}
	if name == "043_lead_landing_pages.sql" {
		return leadLandingPagesSQL
	}
	if name == "044_lead_attribution.sql" {
		return leadAttributionSQL
	}
	if name == "045_lead_audiences.sql" {
		return leadAudiencesSQL
	}
	if name == "046_marketing_email_campaigns.sql" {
		return marketingEmailCampaignsSQL
	}
	if name == "047_lead_nurture_campaigns.sql" {
		return leadNurtureCampaignsSQL
	}
	if name == "048_lead_scoring_routing.sql" {
		return leadScoringRoutingSQL
	}
	if name == "049_lead_chat_widgets.sql" {
		return leadChatWidgetsSQL
	}
	if name == "050_workflow_automations.sql" {
		return workflowAutomationsSQL
	}
	return ""
}
