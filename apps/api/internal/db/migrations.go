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

func MigrationFiles() []string {
	return []string{"001_initial_schema.sql", "002_company_client_type.sql", "003_contact_client_flag.sql", "004_client_address.sql", "005_client_structured_address.sql", "006_remove_company_domain.sql", "007_task_archive.sql", "008_user_setup_tokens.sql", "009_database_integrity.sql", "010_saved_views.sql", "011_audit_events.sql", "012_user_preferences.sql", "013_notifications.sql", "014_billing_plans.sql", "015_subscription_lifecycle.sql", "016_email_templates.sql", "017_user_email_accounts.sql", "018_email_messages.sql", "019_email_open_tracking.sql", "020_email_click_tracking.sql", "021_email_sequences.sql", "022_email_sequence_enrollments.sql", "023_user_email_sync_foundation.sql", "024_inbound_email_messages.sql", "025_email_message_entity_links.sql", "026_email_message_visibility.sql", "027_email_suppressions.sql", "028_email_snippets.sql"}
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
	return ""
}
