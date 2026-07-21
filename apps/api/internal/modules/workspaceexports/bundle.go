package workspaceexports

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	maxArtifactBytes     int64 = 50 * 1024 * 1024
	maxUncompressedBytes int64 = 200 * 1024 * 1024
	maxDatasetRowBytes         = 10 * 1024 * 1024
)

type bundle struct {
	Filename      string
	Content       []byte
	ContentSHA256 string
	DatasetCounts map[string]int64
}

type dataset struct {
	name  string
	query string
}

type manifest struct {
	FormatVersion               int              `json:"formatVersion"`
	GeneratedAt                 time.Time        `json:"generatedAt"`
	OrganizationID              int64            `json:"organizationId"`
	DatasetFormat               string           `json:"datasetFormat"`
	DatasetCounts               map[string]int64 `json:"datasetCounts"`
	OmittedPrivateEmailMessages int64            `json:"omittedPrivateEmailMessages"`
	OmittedPrivateEmailReplies  int64            `json:"omittedPrivateEmailReplyRequests"`
	SecurityExclusions          []string         `json:"securityExclusions"`
	ExternalFiles               string           `json:"externalFiles"`
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Service) buildBundle(ctx context.Context, organizationID int64) (bundle, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return bundle{}, fmt.Errorf("begin repeatable workspace export snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '3min'`); err != nil {
		return bundle{}, fmt.Errorf("bound workspace export query: %w", err)
	}
	if err := checkSchemaCoverage(ctx, tx); err != nil {
		return bundle{}, err
	}

	var organizationSlug string
	if err := tx.QueryRow(ctx, `SELECT slug FROM organizations WHERE id=$1`, organizationID).Scan(&organizationSlug); errors.Is(err, pgx.ErrNoRows) {
		return bundle{}, ErrNotFound
	} else if err != nil {
		return bundle{}, fmt.Errorf("load workspace export organization: %w", err)
	}
	var omittedPrivateMessages int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM email_messages WHERE organization_id=$1 AND visibility <> 'shared'`, organizationID).Scan(&omittedPrivateMessages); err != nil {
		return bundle{}, fmt.Errorf("count private workspace email: %w", err)
	}
	var omittedPrivateReplies int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM email_reply_requests WHERE organization_id=$1 AND visibility <> 'shared'`, organizationID).Scan(&omittedPrivateReplies); err != nil {
		return bundle{}, fmt.Errorf("count private workspace email replies: %w", err)
	}

	temporary, err := os.CreateTemp("", "open-crm-workspace-export-*.zip")
	if err != nil {
		return bundle{}, fmt.Errorf("create temporary workspace export: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()

	bounded := &boundedWriter{writer: temporary, remaining: maxArtifactBytes}
	archive := zip.NewWriter(bounded)
	counts := make(map[string]int64, len(portableDatasets))
	var uncompressedBytes int64
	for _, current := range portableDatasets {
		count, err := writeDataset(ctx, tx, archive, current, organizationID, &uncompressedBytes)
		if err != nil {
			_ = archive.Close()
			return bundle{}, err
		}
		counts[current.name] = count
	}

	generatedAt := s.now().UTC()
	manifestValue := manifest{
		FormatVersion:               1,
		GeneratedAt:                 generatedAt,
		OrganizationID:              organizationID,
		DatasetFormat:               "newline-delimited JSON (one PostgreSQL row per line)",
		DatasetCounts:               counts,
		OmittedPrivateEmailMessages: omittedPrivateMessages,
		OmittedPrivateEmailReplies:  omittedPrivateReplies,
		SecurityExclusions: []string{
			"password hashes, password setup/reset tokens and delivery state, email verification tokens, and sessions",
			"SMTP/IMAP passwords, OAuth access/refresh tokens, sync cursors, and email tracking tokens",
			"Stripe customer/subscription references, signed invoice links, Checkout requests, and webhook processing ledgers",
			"background-job locks, idempotency payloads, workspace bootstrap receipts, and prior export artifacts",
			"email messages marked private and their entity/link metadata",
			"connected-mailbox delivery-feedback correlation ledgers",
		},
		ExternalFiles: "Open CRM currently stores recording and invoice references, not uploaded attachment bodies. Referenced external files are not embedded in this bundle.",
	}
	manifestJSON, err := json.MarshalIndent(manifestValue, "", "  ")
	if err != nil {
		_ = archive.Close()
		return bundle{}, fmt.Errorf("encode workspace export manifest: %w", err)
	}
	if err := writeArchiveFile(archive, "manifest.json", append(manifestJSON, '\n'), &uncompressedBytes); err != nil {
		_ = archive.Close()
		return bundle{}, err
	}
	if err := writeArchiveFile(archive, "README.txt", []byte(bundleReadme), &uncompressedBytes); err != nil {
		_ = archive.Close()
		return bundle{}, err
	}
	if err := archive.Close(); err != nil {
		if errors.Is(err, ErrArtifactTooLarge) || errors.Is(bounded.err, ErrArtifactTooLarge) {
			return bundle{}, ErrArtifactTooLarge
		}
		return bundle{}, fmt.Errorf("finalize workspace export archive: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return bundle{}, fmt.Errorf("sync workspace export archive: %w", err)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return bundle{}, fmt.Errorf("rewind workspace export archive: %w", err)
	}
	content, err := io.ReadAll(io.LimitReader(temporary, maxArtifactBytes+1))
	if err != nil {
		return bundle{}, fmt.Errorf("read workspace export archive: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return bundle{}, fmt.Errorf("close workspace export archive: %w", err)
	}
	if int64(len(content)) > maxArtifactBytes {
		return bundle{}, ErrArtifactTooLarge
	}
	if err := tx.Commit(ctx); err != nil {
		return bundle{}, fmt.Errorf("complete repeatable workspace export snapshot: %w", err)
	}
	digest := sha256.Sum256(content)
	return bundle{
		Filename:      portableFilename(organizationSlug, generatedAt),
		Content:       content,
		ContentSHA256: hex.EncodeToString(digest[:]),
		DatasetCounts: counts,
	}, nil
}

func writeDataset(ctx context.Context, tx pgx.Tx, archive *zip.Writer, current dataset, organizationID int64, uncompressedBytes *int64) (int64, error) {
	entry, err := archive.CreateHeader(&zip.FileHeader{Name: "data/" + current.name + ".ndjson", Method: zip.Deflate})
	if err != nil {
		return 0, fmt.Errorf("create workspace export dataset %s: %w", current.name, err)
	}
	rows, err := tx.Query(ctx, current.query, organizationID)
	if err != nil {
		return 0, fmt.Errorf("query workspace export dataset %s: %w", current.name, err)
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return 0, fmt.Errorf("scan workspace export dataset %s: %w", current.name, err)
		}
		if len(raw) > maxDatasetRowBytes {
			return 0, fmt.Errorf("%w: dataset %s contains a row larger than 10 MiB", ErrDatasetTooLarge, current.name)
		}
		if *uncompressedBytes+int64(len(raw))+1 > maxUncompressedBytes {
			return 0, ErrDatasetTooLarge
		}
		if _, err := entry.Write(raw); err != nil {
			if errors.Is(err, ErrArtifactTooLarge) {
				return 0, ErrArtifactTooLarge
			}
			return 0, fmt.Errorf("write workspace export dataset %s: %w", current.name, err)
		}
		if _, err := entry.Write([]byte{'\n'}); err != nil {
			if errors.Is(err, ErrArtifactTooLarge) {
				return 0, ErrArtifactTooLarge
			}
			return 0, fmt.Errorf("write workspace export dataset %s newline: %w", current.name, err)
		}
		*uncompressedBytes += int64(len(raw)) + 1
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate workspace export dataset %s: %w", current.name, err)
	}
	return count, nil
}

func writeArchiveFile(archive *zip.Writer, name string, content []byte, uncompressedBytes *int64) error {
	if *uncompressedBytes+int64(len(content)) > maxUncompressedBytes {
		return ErrDatasetTooLarge
	}
	entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return fmt.Errorf("create workspace export file %s: %w", name, err)
	}
	if _, err := entry.Write(content); err != nil {
		if errors.Is(err, ErrArtifactTooLarge) {
			return ErrArtifactTooLarge
		}
		return fmt.Errorf("write workspace export file %s: %w", name, err)
	}
	*uncompressedBytes += int64(len(content))
	return nil
}

type boundedWriter struct {
	writer    io.Writer
	remaining int64
	err       error
}

func (w *boundedWriter) Write(content []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if int64(len(content)) > w.remaining {
		w.err = ErrArtifactTooLarge
		return 0, w.err
	}
	written, err := w.writer.Write(content)
	w.remaining -= int64(written)
	if err != nil {
		w.err = err
	}
	return written, err
}

func portableFilename(slug string, generatedAt time.Time) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	slug = strings.Trim(slugUnsafe.ReplaceAllString(slug, "-"), "-")
	if slug == "" {
		slug = "workspace"
	}
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-")
	}
	return fmt.Sprintf("open-crm-%s-%s.zip", slug, generatedAt.Format("20060102T150405Z"))
}

func checkSchemaCoverage(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT table_name
		FROM information_schema.columns
		WHERE table_schema=current_schema() AND column_name='organization_id'
		ORDER BY table_name
	`)
	if err != nil {
		return fmt.Errorf("inspect workspace export schema coverage: %w", err)
	}
	defer rows.Close()
	missing := make([]string, 0)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("scan workspace export schema coverage: %w", err)
		}
		if _, ok := classifiedOrganizationTables[table]; !ok {
			missing = append(missing, table)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate workspace export schema coverage: %w", err)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%w: classify tables %s", ErrUnclassifiedDataset, strings.Join(missing, ", "))
	}
	return nil
}

var ordinaryOrganizationTables = []string{
	"activities",
	"audit_events",
	"bulk_operation_rows",
	"bulk_operations",
	"calendar_availability_blocks",
	"calendar_booking_links",
	"calendar_event_reminders",
	"calendar_events",
	"call_logs",
	"client_review_schedules",
	"companies",
	"contact_company_links",
	"contacts",
	"custom_field_definitions",
	"custom_report_definitions",
	"deal_line_items",
	"deal_quote_line_items",
	"deal_pipelines",
	"deal_signature_requests",
	"deal_stage_events",
	"deal_stages",
	"deals",
	"duplicate_merge_operations",
	"email_sequence_deliveries",
	"email_sequence_enrollments",
	"email_sequences",
	"email_snippets",
	"email_suppressions",
	"email_templates",
	"import_batch_rows",
	"import_batches",
	"lead_audiences",
	"lead_capture_forms",
	"lead_capture_submissions",
	"lead_chat_widgets",
	"lead_landing_pages",
	"lead_nurture_campaigns",
	"lead_scoring_rules",
	"marketing_email_campaigns",
	"note_mentions",
	"notes",
	"notifications",
	"organization_exchange_rates",
	"product_catalog_items",
	"record_followers",
	"sales_quotas",
	"saved_views",
	"sms_messages",
	"sms_suppressions",
	"task_reminders",
	"tasks",
	"workflow_automation_runs",
	"workflow_automations",
}

var portableDatasets = buildPortableDatasets()

func buildPortableDatasets() []dataset {
	datasets := []dataset{
		{name: "organization", query: `
			SELECT to_jsonb(o) - ARRAY[
				'stripe_customer_id','stripe_subscription_id','billing_last_event_id',
				'billing_last_invoice_event_id','billing_last_reconciliation_error'
			]::text[]
			FROM organizations o WHERE id=$1`},
		{name: "members", query: `
			SELECT jsonb_build_object(
				'id',m.id,'user_id',u.id,'email',u.email,'first_name',u.first_name,'last_name',u.last_name,
				'role',m.role,'membership_status',m.membership_status,'created_at',m.created_at,
				'status_changed_at',m.status_changed_at,'status_changed_by_user_id',m.status_changed_by_user_id,
				'email_verified_at',u.email_verified_at,'preferences',u.preferences,'user_created_at',u.created_at,'user_updated_at',u.updated_at
			)
			FROM organization_memberships m JOIN users u ON u.id=m.user_id
			WHERE m.organization_id=$1 ORDER BY m.id`},
		{name: "billing_invoices", query: `
			SELECT to_jsonb(i) - ARRAY['hosted_invoice_url','invoice_pdf_url','provider_subscription_id']::text[]
			FROM billing_invoices i WHERE organization_id=$1 ORDER BY id`},
		{name: "deal_quotes", query: `
			SELECT to_jsonb(q) - ARRAY['idempotency_key_hash','request_sha256']::text[]
			FROM deal_quotes q WHERE organization_id=$1 ORDER BY id`},
		{name: "email_messages_shared", query: `
			SELECT to_jsonb(m) - ARRAY['tracking_token','provider_message_id','provider_thread_id','rfc_message_id','in_reply_to','reference_message_ids','delivery_feedback_email_message_id']::text[]
			FROM email_messages m WHERE organization_id=$1 AND visibility='shared' ORDER BY id`},
		{name: "email_message_entity_links_shared", query: `
			SELECT to_jsonb(link)
			FROM email_message_entity_links link
			JOIN email_messages message ON message.id=link.email_message_id AND message.organization_id=link.organization_id
			WHERE link.organization_id=$1 AND message.visibility='shared' ORDER BY link.id`},
		{name: "email_message_links_shared", query: `
			SELECT to_jsonb(link) - 'click_token'
			FROM email_message_links link
			JOIN email_messages message ON message.id=link.email_message_id
			WHERE message.organization_id=$1 AND message.visibility='shared' ORDER BY link.id`},
		{name: "email_reply_requests_shared", query: `
			SELECT jsonb_build_object(
				'id',id,'organization_id',organization_id,'source_message_id',source_message_id,
				'thread_root_message_id',thread_root_message_id,'actor_user_id',actor_user_id,
				'sender_email',sender_email,'recipient_email',recipient_email,'subject',subject,'body',body,
				'visibility',visibility,'status',status,'outbound_email_message_id',outbound_email_message_id,
				'last_error',last_error,'claimed_at',claimed_at,'finalized_at',finalized_at,
				'created_at',created_at,'updated_at',updated_at
			)
			FROM email_reply_requests WHERE organization_id=$1 AND visibility='shared' ORDER BY id`},
		{name: "email_sequence_steps", query: `
			SELECT to_jsonb(step)
			FROM email_sequence_steps step
			JOIN email_sequences sequence ON sequence.id=step.sequence_id
			WHERE sequence.organization_id=$1 ORDER BY step.id`},
		{name: "calendar_booking_link_members", query: `
			SELECT to_jsonb(member)
			FROM calendar_booking_link_members member
			JOIN calendar_booking_links link ON link.id=member.booking_link_id
			WHERE link.organization_id=$1 ORDER BY member.booking_link_id,member.position,member.user_id`},
		{name: "email_account_configuration", query: `
			SELECT jsonb_build_object(
				'id',id,'organization_id',organization_id,'user_id',user_id,'from_email',from_email,'from_name',from_name,
				'smtp_host',smtp_host,'smtp_port',smtp_port,'smtp_username',smtp_username,'smtp_use_tls',smtp_use_tls,
				'imap_host',imap_host,'imap_port',imap_port,'imap_username',imap_username,'imap_use_tls',imap_use_tls,
				'provider',provider,'auth_method',auth_method,'sync_enabled',sync_enabled,'sync_status',sync_status,
				'last_sync_at',last_sync_at,'last_sync_error',last_sync_error,'oauth_token_expires_at',oauth_token_expires_at,
				'created_at',created_at,'updated_at',updated_at
			)
			FROM user_email_accounts WHERE organization_id=$1 ORDER BY id`},
	}
	for _, table := range ordinaryOrganizationTables {
		table := table
		datasets = append(datasets, dataset{
			name:  table,
			query: "SELECT to_jsonb(export_row) FROM (SELECT * FROM " + table + " WHERE organization_id=$1 ORDER BY id) export_row",
		})
	}
	sort.Slice(datasets, func(i, j int) bool { return datasets[i].name < datasets[j].name })
	return datasets
}

var classifiedOrganizationTables = buildClassifiedOrganizationTables()

func buildClassifiedOrganizationTables() map[string]struct{} {
	classified := make(map[string]struct{}, len(ordinaryOrganizationTables)+16)
	for _, table := range ordinaryOrganizationTables {
		classified[table] = struct{}{}
	}
	for _, table := range []string{
		"organization_memberships",
		"billing_invoices",
		"deal_quotes",
		"email_messages",
		"email_message_entity_links",
		"email_reply_requests",
		"user_email_accounts",
		"background_jobs",
		"billing_checkout_requests",
		"billing_webhook_events",
		"billing_usage_snapshots",
		"billing_capacity_reservations",
		"sessions",
		"system_email_feedback_events",
		"customer_email_feedback_events",
		"workspace_bootstrap_requests",
		"workspace_exports",
	} {
		classified[table] = struct{}{}
	}
	return classified
}

const bundleReadme = `Open CRM portable workspace export

This archive is a consistent, repeatable-read snapshot of portable workspace
data. Each file under data/ is newline-delimited JSON (NDJSON): one database
record per line. manifest.json lists record counts and security exclusions.

Archived records, configuration, CRM history, audit history, compliance
suppressions, and shared communication content are included. Authentication
secrets, provider credentials, session material, private mailbox messages, and
internal delivery/billing job ledgers are deliberately excluded. External file
references are preserved where they are ordinary record fields, but Open CRM
does not currently store uploaded attachment bodies for inclusion.

Keep this archive secure: it contains customer and business data. Verify its
SHA-256 checksum against the value shown in Open CRM before relying on it.
`
