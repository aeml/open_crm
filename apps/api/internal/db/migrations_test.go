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

func TestMigrationFilesIncludeEmailTemplateDefinitionManagement(t *testing.T) {
	const name = "116_email_template_definition_management.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	sql := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"lock_timeout",
		"statement_timeout",
		"email_templates_revision_positive",
		"email_snippets_revision_positive",
		"NOT VALID",
		"VALIDATE CONSTRAINT",
		"idx_email_templates_org_name_id",
		"idx_email_snippets_org_name_id",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("email definition management migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("email definition management deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeSavedViewManagement(t *testing.T) {
	const name = "117_saved_view_management.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	sql := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"lock_timeout",
		"statement_timeout",
		"ADD COLUMN revision",
		"saved_views_revision_positive",
		"NOT VALID",
		"VALIDATE CONSTRAINT",
		"idx_saved_views_management",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("saved-view management migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("saved-view management deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeDashboardQueryIndexes(t *testing.T) {
	const name = "118_dashboard_query_indexes.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	sql := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"lock_timeout",
		"statement_timeout",
		"idx_activities_dashboard_recent",
		"idx_contacts_dashboard_recent",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("dashboard query-index migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("dashboard query-index deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeAsyncImportJobs(t *testing.T) {
	const name = "119_async_import_jobs.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	sql := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"lock_timeout",
		"statement_timeout",
		"source_csv BYTEA",
		"source_expires_at",
		"import_batches_source_expiry_check",
		"idx_import_batches_source_expiry",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("async-import migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("async-import deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeDurableCRMExports(t *testing.T) {
	const name = "120_durable_crm_exports.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	sql := MigrationSQL(name)
	for _, expected := range []string{"-- open-crm-deploy: expand", "lock_timeout", "statement_timeout", "CREATE TABLE IF NOT EXISTS crm_exports", "crm_exports_membership_fk", "idx_crm_exports_expiry"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("durable CRM export migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("durable CRM export deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeVersionedDealQuotes(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "093_versioned_deal_quotes.sql") {
		t.Fatal("expected versioned deal quotes migration to be registered")
	}
	sql := MigrationSQL("093_versioned_deal_quotes.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"deal_quotes",
		"deal_quote_line_items",
		"pdf_content BYTEA",
		"pdf_sha256 ~ '^[0-9a-f]{64}$'",
		"idempotency_key_hash",
		"UNIQUE (organization_id, deal_id, version)",
		"deal_quote_line_items_quote_fk",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("versioned deal quotes migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("093_versioned_deal_quotes.sql"); class != "expand" {
		t.Fatalf("versioned deal quotes deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeQuoteExpirationReissue(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "098_quote_expiration_reissue.sql") {
		t.Fatal("expected quote expiration reissue migration to be registered")
	}
	sql := MigrationSQL("098_quote_expiration_reissue.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"reissued_from_quote_id",
		"deal_quotes_reissued_from_fk",
		"deal_quotes_reissued_from_self_check",
		"idx_deal_quotes_one_reissue",
		"idx_deal_quotes_org_expiration",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("quote expiration reissue migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("098_quote_expiration_reissue.sql"); class != "expand" {
		t.Fatalf("quote expiration reissue deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeQuoteFXDisclosure(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "099_quote_fx_disclosure.sql") {
		t.Fatal("expected quote FX disclosure migration to be registered")
	}
	sql := MigrationSQL("099_quote_fx_disclosure.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"quote_base_currency",
		"exchange_rate_to_base",
		"exchange_rate_effective_date",
		"exchange_rate_source",
		"total_in_base_currency",
		"deal_quotes_fx_snapshot_check",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("quote FX disclosure migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("099_quote_fx_disclosure.sql"); class != "expand" {
		t.Fatalf("quote FX disclosure deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeQuoteTemplatesApprovals(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "100_quote_templates_approvals.sql") {
		t.Fatal("expected quote templates and approvals migration to be registered")
	}
	sql := MigrationSQL("100_quote_templates_approvals.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"CREATE TABLE quote_templates",
		"organization_quote_policies",
		"source_quote_template_id",
		"delivery_subject_default",
		"delivery_message_default",
		"deal_quotes_template_snapshot_check",
		"deal_quote_approvals",
		"quote_pdf_sha256",
		"decision_key_hash",
		"NOT VALID",
		"VALIDATE CONSTRAINT deal_quotes_source_template_fk",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("quote templates and approvals migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("100_quote_templates_approvals.sql"); class != "expand" {
		t.Fatalf("quote templates and approvals deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeDealQuoteDeliveries(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "094_deal_quote_deliveries.sql") {
		t.Fatal("expected deal quote deliveries migration to be registered")
	}
	sql := MigrationSQL("094_deal_quote_deliveries.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"deal_quote_deliveries",
		"deal_quote_deliveries_email_fk",
		"access_token_digest",
		"receipt_confirmed_at",
		"deal_quote_deliveries_send_state_check",
		"idx_deal_quote_deliveries_stale_sending",
		"idx_deal_quote_deliveries_one_unresolved_quote",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("deal quote deliveries migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("094_deal_quote_deliveries.sql"); class != "expand" {
		t.Fatalf("deal quote deliveries deployment class = %q", class)
	}
}

func TestMigrationFilesIncludePublicLeadConsentChallenges(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "095_public_lead_consent_challenges.sql") {
		t.Fatal("expected public lead consent challenge migration to be registered")
	}
	sql := MigrationSQL("095_public_lead_consent_challenges.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"consent_text",
		"consent_text_snapshot",
		"lead_capture_submission_challenges",
		"token_digest",
		"request_digest",
		"lead_capture_submission_challenges_consumption_check",
		"idx_lead_capture_submission_challenges_cleanup",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("public lead consent challenge migration missing %q", expected)
		}
	}
	if strings.Contains(sql, "remote_addr_digest") || strings.Contains(sql, "user_agent_digest") {
		t.Fatal("public lead challenge migration must not add network or browser identifiers")
	}
	if class := MigrationDeploymentClass("095_public_lead_consent_challenges.sql"); class != "expand" {
		t.Fatalf("public lead consent challenge deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeQuoteSignatureCeremony(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "096_quote_signature_ceremony.sql") {
		t.Fatal("expected quote signature ceremony migration to be registered")
	}
	sql := MigrationSQL("096_quote_signature_ceremony.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"deal_signature_requests_quote_fk",
		"deal_signature_requests_native_state_check",
		"completion_idempotency_key_hash",
		"certificate_content BYTEA",
		"idx_deal_signature_requests_one_active_quote",
		"deal_quote_deliveries_signature_fk",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("quote signature ceremony migration missing %q", expected)
		}
	}
	if strings.Contains(sql, "remote_addr") || strings.Contains(sql, "user_agent") {
		t.Fatal("quote signature ceremony must not add network or browser identifiers")
	}
	if class := MigrationDeploymentClass("096_quote_signature_ceremony.sql"); class != "expand" {
		t.Fatalf("quote signature ceremony deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeSignedQuoteConversion(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "097_signed_quote_conversion.sql") {
		t.Fatal("expected signed quote conversion migration to be registered")
	}
	sql := MigrationSQL("097_signed_quote_conversion.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"conversion_stage_id",
		"conversion_close_reason_code",
		"conversion_activity_id",
		"conversion_idempotency_key_hash",
		"deal_signature_requests_conversion_shape_check",
		"idx_deal_signature_requests_signed_unconverted",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("signed quote conversion migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("097_signed_quote_conversion.sql"); class != "expand" {
		t.Fatalf("signed quote conversion deployment class = %q", class)
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

func TestMigrationFilesIncludePasswordRecovery(t *testing.T) {
	sql := MigrationSQL("083_password_recovery.sql")
	if sql == "" {
		t.Fatal("expected password recovery migration SQL to be embedded")
	}
	for _, expected := range []string{"password_reset_token_hash", "password_reset_expires_at", "password_reset_delivery_status", "idx_users_password_reset_token_hash"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected password recovery migration to include %q", expected)
		}
	}
}

func TestMigrationFilesIncludeUserInvitationLifecycle(t *testing.T) {
	sql := MigrationSQL("084_user_invitation_lifecycle.sql")
	if sql == "" {
		t.Fatal("expected user invitation lifecycle migration SQL to be embedded")
	}
	for _, expected := range []string{"password_setup_revoked_at", "users_password_setup_terminal_state_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected user invitation lifecycle migration to include %q", expected)
		}
	}
	if class := migrationDeploymentClass("084_user_invitation_lifecycle.sql", sql); class != "expand" {
		t.Fatalf("expected user invitation lifecycle migration to be expand-safe, got %q", class)
	}
}

func TestMigrationFilesIncludePostmarkDeliveryFeedback(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "085_postmark_delivery_feedback.sql") {
		t.Fatal("expected Postmark delivery feedback migration to be registered")
	}
	sql := MigrationSQL("085_postmark_delivery_feedback.sql")
	for _, expected := range []string{
		"system_email_feedback_events",
		"email_verification_delivery_key_hash",
		"password_setup_delivery_key_hash",
		"password_reset_delivery_key_hash",
		"system_email_suppressed_at",
		"payload_sha256",
		"idx_system_email_feedback_unapplied",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("Postmark delivery feedback migration missing %q", expected)
		}
	}
	if strings.Contains(sql, "recipient_email") || strings.Contains(sql, "payload_json") {
		t.Fatal("Postmark feedback ledger must not retain recipient addresses or provider payloads")
	}
	if class := MigrationDeploymentClass("085_postmark_delivery_feedback.sql"); class != "expand" {
		t.Fatalf("Postmark delivery feedback deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeOAuthMailDelivery(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "086_oauth_mail_delivery.sql") {
		t.Fatal("expected OAuth mail delivery migration to be registered")
	}
	sql := MigrationSQL("086_oauth_mail_delivery.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "oauth_scopes", "lock_timeout", "statement_timeout"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("OAuth mail delivery migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("086_oauth_mail_delivery.sql"); class != "expand" {
		t.Fatalf("OAuth mail delivery deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeEmailSequenceApprovals(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "087_email_sequence_approvals.sql") {
		t.Fatal("expected email sequence approvals migration to be registered")
	}
	sql := MigrationSQL("087_email_sequence_approvals.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "revision", "approved_revision", "approved_by_user_id", "approved_at", "status = 'paused'", "NOT VALID", "VALIDATE CONSTRAINT email_sequences_approval_state_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("email sequence approvals migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("087_email_sequence_approvals.sql"); class != "expand" {
		t.Fatalf("email sequence approvals deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeEmailSequenceOutcomes(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "088_email_sequence_outcomes.sql") {
		t.Fatal("expected email sequence outcomes migration to be registered")
	}
	sql := MigrationSQL("088_email_sequence_outcomes.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "completion_reason", "replied_at", "reply_email_message_id", "NOT VALID", "VALIDATE CONSTRAINT email_sequence_enrollments_reply_message_fk"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("email sequence outcomes migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("088_email_sequence_outcomes.sql"); class != "expand" {
		t.Fatalf("email sequence outcomes deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeEmailMessageCorrelation(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "089_email_message_correlation.sql") {
		t.Fatal("expected email message correlation migration to be registered")
	}
	sql := MigrationSQL("089_email_message_correlation.sql")
	for _, expected := range []string{"-- open-crm-deploy: expand", "rfc_message_id", "provider_thread_id", "in_reply_to", "reference_message_ids", "idx_email_sequence_deliveries_org_rfc_message_id", "NOT VALID", "VALIDATE CONSTRAINT email_messages_reference_message_ids_check"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("email message correlation migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("089_email_message_correlation.sql"); class != "expand" {
		t.Fatalf("email message correlation deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeCustomerEmailFeedback(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "090_customer_email_feedback.sql") {
		t.Fatal("expected customer email feedback migration to be registered")
	}
	sql := MigrationSQL("090_customer_email_feedback.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"customer_email_feedback_events",
		"delivery_outcome",
		"delivery_feedback_email_message_id",
		"idx_customer_email_feedback_unapplied",
		"idx_email_messages_org_mailbox_outbound_rfc_message",
		"NOT VALID",
		"VALIDATE CONSTRAINT email_sequence_deliveries_delivery_feedback_message_fk",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("customer email feedback migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("090_customer_email_feedback.sql"); class != "expand" {
		t.Fatalf("customer email feedback deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeEmailEngagementTrackingPrivacy(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "091_email_engagement_tracking_privacy.sql") {
		t.Fatal("expected email engagement tracking privacy migration to be registered")
	}
	sql := MigrationSQL("091_email_engagement_tracking_privacy.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"engagement_tracking_enabled",
		"engagement_tracking_authorized_by_user_id",
		"engagement_tracking_expires_at",
		"engagement_tracking_purged_at",
		"engagement_tracking_expires_at = NOW()",
		"NOT VALID",
		"idx_email_messages_engagement_tracking_retention",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("email engagement tracking privacy migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("091_email_engagement_tracking_privacy.sql"); class != "expand" {
		t.Fatalf("email engagement tracking privacy deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeThreadedReplies(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "092_email_threaded_replies.sql") {
		t.Fatal("expected threaded email replies migration to be registered")
	}
	sql := MigrationSQL("092_email_threaded_replies.sql")
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"thread_root_message_id",
		"email_messages_default_thread_root",
		"email_reply_requests",
		"idempotency_key_hash",
		"idx_email_reply_requests_stale_sending",
		"idx_email_reply_requests_one_unresolved_actor_thread",
		"resolved_roots AS MATERIALIZED",
		"NOT VALID",
		"VALIDATE CONSTRAINT email_messages_thread_root_fk",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("threaded replies migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("092_email_threaded_replies.sql"); class != "expand" {
		t.Fatalf("threaded replies deployment class = %q", class)
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
	unsafeTruncate := "-- open-crm-deploy: expand\nTRUNCATE TABLE audit_events;"
	if err := validateAutomaticMigration("999_unsafe_truncate.sql", unsafeTruncate, false); err == nil || !strings.Contains(err.Error(), "unsafe truncate") {
		t.Fatalf("destructive truncate expand migration was not rejected: %v", err)
	}
	safeTruncateGuard := "-- open-crm-deploy: expand\nCREATE TRIGGER preserve_history BEFORE TRUNCATE ON audit_events FOR EACH STATEMENT EXECUTE FUNCTION reject_truncate();"
	if err := validateAutomaticMigration("999_safe_truncate_guard.sql", safeTruncateGuard, false); err != nil {
		t.Fatalf("truncate-protection trigger was rejected as destructive: %v", err)
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

func TestMigrationFilesIncludeBillingUsageSnapshotsMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "077_billing_usage_snapshots.sql") {
		t.Fatal("expected billing usage snapshots migration to be registered")
	}
	sql := MigrationSQL("077_billing_usage_snapshots.sql")
	for _, expected := range []string{"subscription_current_period_start", "billing_usage_snapshots", "period_basis", "outbound_messages_used", "automation_executions_used", "storage_bytes_used", "idx_email_messages_org_sent_period"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("billing usage snapshots migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass("077_billing_usage_snapshots.sql"); class != "expand" {
		t.Fatalf("billing usage snapshots deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeBillingCapacityReservationsMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "078_billing_capacity_reservations.sql") {
		t.Fatal("capacity reservation migration is missing")
	}
	sql := MigrationSQL("078_billing_capacity_reservations.sql")
	for _, fragment := range []string{"billing_capacity_reservations", "organization_id", "expires_at", "contacts", "deals", "seats"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("capacity reservation migration missing %q", fragment)
		}
	}
	if class := MigrationDeploymentClass("078_billing_capacity_reservations.sql"); class != "expand" {
		t.Fatalf("capacity reservation migration class=%q", class)
	}
}

func TestMigrationFilesIncludeSharedPublicRateLimitsMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "079_shared_public_rate_limits.sql") {
		t.Fatal("shared public rate limits migration is missing")
	}
	sql := MigrationSQL("079_shared_public_rate_limits.sql")
	for _, fragment := range []string{"public_rate_limit_buckets", "client_key_hash BYTEA", "expires_at", "request_count", "idx_public_rate_limit_buckets_expiry"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("shared public rate limits migration missing %q", fragment)
		}
	}
	if strings.Contains(sql, "ip_address") || strings.Contains(sql, "remote_address") {
		t.Fatal("shared public rate limits migration must not retain raw client addresses")
	}
	if class := MigrationDeploymentClass("079_shared_public_rate_limits.sql"); class != "expand" {
		t.Fatalf("shared public rate limits migration class=%q", class)
	}
}

func TestMigrationFilesIncludeBackgroundJobRetentionMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "080_background_job_retention.sql") {
		t.Fatal("background job retention migration is missing")
	}
	sql := MigrationSQL("080_background_job_retention.sql")
	for _, fragment := range []string{"background_jobs", "completed_at ASC", "status = 'succeeded'", "idx_background_jobs_retention_succeeded"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("background job retention migration missing %q", fragment)
		}
	}
	if class := MigrationDeploymentClass("080_background_job_retention.sql"); class != "expand" {
		t.Fatalf("background job retention migration class=%q", class)
	}
}

func TestMigrationFilesIncludeDealAssignmentNotificationsMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "081_deal_assignment_notifications.sql") {
		t.Fatal("deal assignment notification migration is missing")
	}
	sql := MigrationSQL("081_deal_assignment_notifications.sql")
	for _, fragment := range []string{"owner_assignment_version", "deals_owner_assignment_version_check", "DEFAULT 0"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("deal assignment notification migration missing %q", fragment)
		}
	}
	if class := MigrationDeploymentClass("081_deal_assignment_notifications.sql"); class != "expand" {
		t.Fatalf("deal assignment notification migration class=%q", class)
	}
}

func TestMigrationFilesIncludeNotificationRetentionMigration(t *testing.T) {
	if !slices.Contains(MigrationFiles(), "082_notification_retention.sql") {
		t.Fatal("notification retention migration is missing")
	}
	sql := MigrationSQL("082_notification_retention.sql")
	for _, fragment := range []string{"idx_notifications_retention_read", "idx_notifications_retention_unread", "idx_notifications_operational_created", "INCLUDE (organization_id, user_id, event_type)"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("notification retention migration missing %q", fragment)
		}
	}
	if class := MigrationDeploymentClass("082_notification_retention.sql"); class != "expand" {
		t.Fatalf("notification retention migration class=%q", class)
	}
}

func TestMigrationFilesIncludeWorkflowRunOperationsMigrations(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		fragments []string
	}{
		{
			name:      "101_workflow_run_operations.sql",
			fragments: []string{"-- open-crm-deploy: expand", "idx_workflow_automation_runs_active_created", "idx_workflow_automation_runs_terminal_recent", "lock_timeout", "statement_timeout"},
		},
		{
			name:      "102_workflow_run_scheduling.sql",
			fragments: []string{"-- open-crm-deploy: expand", "scheduled_at", "workflow_automation_runs_scheduled_at_present", "idx_workflow_automation_runs_active_scheduled", "lock_timeout", "statement_timeout"},
		},
	} {
		if !slices.Contains(MigrationFiles(), testCase.name) {
			t.Fatalf("workflow run migration %s is missing", testCase.name)
		}
		sql := MigrationSQL(testCase.name)
		for _, fragment := range testCase.fragments {
			if !strings.Contains(sql, fragment) {
				t.Fatalf("workflow run migration %s missing %q", testCase.name, fragment)
			}
		}
		if class := MigrationDeploymentClass(testCase.name); class != "expand" {
			t.Fatalf("workflow run migration %s class=%q", testCase.name, class)
		}
	}
}

func TestMigrationFilesIncludeLeadSubmissionReview(t *testing.T) {
	const name = "103_lead_submission_review.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("lead submission review migration %s is missing", name)
	}
	for _, fragment := range []string{
		"-- open-crm-deploy: expand",
		"review_status",
		"quarantined_contact_at",
		"lead_capture_submission_review_requests",
		"key_digest",
		"lead_capture_submissions_reviewer_membership_fk",
		"idx_lead_capture_submissions_org_id_unique",
		"idx_lead_capture_submissions_org_review_created",
		"idx_lead_capture_submissions_review_created",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(MigrationSQL(name), fragment) {
			t.Fatalf("lead submission review migration missing %q", fragment)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("lead submission review migration class=%q", class)
	}
}

func TestMigrationFilesIncludeAuditRetentionExport(t *testing.T) {
	const name = "104_audit_retention_export.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	sql := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"open_crm_audit_metadata_keys_are_safe",
		"audit_events_metadata_keys_safe",
		"NOT VALID",
		"VALIDATE CONSTRAINT audit_events_metadata_keys_safe",
		"open_crm_protect_audit_event_history",
		"audit_events_protect_history",
		"audit_events_protect_truncate",
		"BEFORE TRUNCATE",
		"audit events are append-only",
		"audit events are retained for the workspace lifetime",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("audit retention/export migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("audit retention/export deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeCustomReportGroupedBarContract(t *testing.T) {
	const name = "105_custom_report_grouped_bar_contract.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	sql := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"visualization_contract",
		"grouped_bar_v1",
		"NOT VALID",
		"VALIDATE CONSTRAINT custom_report_definitions_visualization_contract_check",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("grouped bar contract migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("grouped bar contract deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeRecordEmailDeliveries(t *testing.T) {
	const name = "106_record_email_deliveries.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	sql := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"CREATE TABLE record_email_deliveries",
		"idempotency_key_hash",
		"request_sha256",
		"idx_record_email_deliveries_stale_sending",
		"idx_record_email_deliveries_one_unresolved_actor_entity",
		"status IN ('prepared', 'sending', 'accepted', 'failed', 'uncertain')",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("record email delivery migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("record email delivery deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeRecordEmailTemplateTests(t *testing.T) {
	const name = "107_record_email_template_tests.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	sql := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"ADD COLUMN purpose",
		"ADD COLUMN recipient_user_id",
		"purpose = 'test'",
		"VALIDATE CONSTRAINT record_email_deliveries_purpose_check",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("record email template test migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("record email template test deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeTimelineCursorIndexes(t *testing.T) {
	const name = "108_timeline_cursor_indexes.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	sql := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"idx_notes_org_entity_created_id",
		"idx_activities_org_entity_created_id",
		"created_at DESC, id DESC",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("timeline cursor migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("timeline cursor deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeCompanyLinkedContactPaging(t *testing.T) {
	const name = "109_company_linked_contact_paging.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	content := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"idx_contact_company_links_org_company_contact",
		"organization_id, company_id, contact_id",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("company linked-contact paging migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("company linked-contact paging deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeSharedInboxCursor(t *testing.T) {
	const name = "110_shared_inbox_cursor.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	content := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"idx_email_messages_shared_inbox_cursor",
		"CASE WHEN shared_inbox_status = 'open' THEN 0 ELSE 1 END",
		"COALESCE(received_at, created_at)",
		"id DESC",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("shared inbox cursor migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("shared inbox cursor deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeLeadSubmissionReviewCursor(t *testing.T) {
	const name = "111_lead_submission_review_cursor.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	content := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"idx_lead_capture_submissions_org_created",
		"idx_lead_capture_submissions_org_form_review_created",
		"created_at DESC",
		"id DESC",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("lead submission review cursor migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("lead submission review cursor deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeWorkflowDefinitionPaging(t *testing.T) {
	const name = "112_workflow_definition_paging.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	content := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"idx_workflow_automations_org_management_page",
		"is_active DESC",
		"position ASC",
		"updated_at DESC",
		"id DESC",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("workflow definition paging migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("workflow definition paging deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeCustomReportDefinitionPaging(t *testing.T) {
	const name = "113_custom_report_definition_paging.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	content := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"idx_custom_report_definitions_org_management_page",
		"is_active DESC",
		"updated_at DESC",
		"id DESC",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("custom report definition paging migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("custom report definition paging deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeEmailSequenceEnrollmentHistory(t *testing.T) {
	const name = "114_email_sequence_enrollment_history.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	content := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"idx_email_sequence_enrollments_org_sequence_created",
		"organization_id",
		"sequence_id",
		"created_at DESC",
		"id DESC",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("email sequence enrollment history migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("email sequence enrollment history deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeWorkflowActionOutcomes(t *testing.T) {
	const name = "115_workflow_action_outcomes.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	content := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"workflow_automation_action_outcomes",
		"workflow_action_outcomes_run_fk",
		"idx_workflow_action_outcomes_org_run_position_unique",
		"Historical task action",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("workflow action outcome migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("workflow action outcome deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeWorkflowCausalityNotifications(t *testing.T) {
	const name = "122_workflow_causality_notifications.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	content := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"causation_run_id",
		"causation_action_position",
		"causal_depth",
		"workflow_automation_runs_causation_action_fk",
		"notification_count",
		"idx_workflow_automation_runs_org_causation",
		"NOT VALID",
		"VALIDATE CONSTRAINT",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("workflow causality migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("workflow causality migration deployment class = %q", class)
	}
}

func TestMigrationFilesIncludeWorkflowDealOwnerAssignment(t *testing.T) {
	const name = "123_workflow_deal_owner_assignment.sql"
	if !slices.Contains(MigrationFiles(), name) {
		t.Fatalf("expected %s to be registered", name)
	}
	content := MigrationSQL(name)
	for _, expected := range []string{
		"-- open-crm-deploy: expand",
		"assigned_user_id",
		"assignment_changed",
		"workflow_action_outcomes_assignment_shape_check",
		"workflow_action_outcomes_assigned_membership_fk",
		"NOT VALID",
		"VALIDATE CONSTRAINT",
		"lock_timeout",
		"statement_timeout",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("workflow owner-assignment migration missing %q", expected)
		}
	}
	if class := MigrationDeploymentClass(name); class != "expand" {
		t.Fatalf("workflow owner-assignment migration deployment class = %q", class)
	}
}
