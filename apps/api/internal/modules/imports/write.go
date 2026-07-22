package imports

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (s *Service) processRows(ctx context.Context, connection *pgxpool.Conn, organizationID, actorUserID, batchID int64, entityType string, rows []PreviewRow) error {
	tx, err := connection.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin import checkpoint: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return err
	}
	definitions, err := modulecustomfields.LoadDefinitions(ctx, tx, organizationID, customFieldEntityType(entityType), false)
	if err != nil {
		return err
	}
	successCount := 0
	errorCount := 0
	for _, row := range rows {
		status, err := processImportRow(ctx, tx, organizationID, actorUserID, batchID, entityType, definitions, row)
		if err != nil {
			return err
		}
		if status == "imported" {
			successCount++
		} else {
			errorCount++
		}
	}
	command, err := tx.Exec(ctx, `
		UPDATE import_batches
		SET processed_rows = processed_rows + $3,
		    success_rows = success_rows + $4,
		    error_rows = error_rows + $5,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND status = 'processing'
	`, organizationID, batchID, len(rows), successCount, errorCount)
	if err != nil {
		return fmt.Errorf("advance import batch progress: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit import checkpoint: %w", err)
	}
	return nil
}

func processImportRow(ctx context.Context, tx pgx.Tx, organizationID, actorUserID, batchID int64, entityType string, definitions []modulecustomfields.Definition, row PreviewRow) (string, error) {
	if len(row.Errors) > 0 {
		return persistInvalidImportRow(ctx, tx, organizationID, batchID, row)
	}
	switch entityType {
	case "contacts":
		return persistImportedContact(ctx, tx, organizationID, actorUserID, batchID, definitions, row)
	case "companies":
		return persistImportedCompany(ctx, tx, organizationID, actorUserID, batchID, definitions, row)
	default:
		return "", fmt.Errorf("unsupported import entity type %q", entityType)
	}
}

func persistInvalidImportRow(ctx context.Context, tx pgx.Tx, organizationID, batchID int64, row PreviewRow) (string, error) {
	issues := row.Errors
	if issues == nil {
		issues = []PreviewIssue{}
	}
	issuesJSON, err := json.Marshal(issues)
	if err != nil {
		return "", fmt.Errorf("encode import row errors: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO import_batch_rows (organization_id, import_batch_id, row_number, status, errors_json)
		VALUES ($1, $2, $3, 'error', $4::jsonb)
	`, organizationID, batchID, row.RowNumber, string(issuesJSON)); err != nil {
		return "", fmt.Errorf("record invalid import row: %w", err)
	}
	return "error", nil
}

func persistImportedContact(ctx context.Context, tx pgx.Tx, organizationID, actorUserID, batchID int64, definitions []modulecustomfields.Definition, row PreviewRow) (string, error) {
	values := row.Values
	firstName := strings.TrimSpace(values["first_name"])
	lastName := strings.TrimSpace(values["last_name"])
	email := strings.ToLower(strings.TrimSpace(values["email"]))
	phone := strings.TrimSpace(values["phone"])
	isClient, _ := strconv.ParseBool(strings.ToLower(strings.TrimSpace(values["is_client"])))
	customFieldsJSON, err := importCustomFieldsJSON(definitions, row)
	if err != nil {
		return "", fmt.Errorf("prepare contact import row %d custom fields: %w", row.RowNumber, err)
	}
	var status string
	if err := tx.QueryRow(ctx, `
		WITH duplicate AS (
			SELECT id
			FROM contacts
			WHERE organization_id = $1 AND archived_at IS NULL
			  AND (
				(lower(first_name) = lower($5) AND lower(last_name) = lower($6)
				 AND COALESCE(NULLIF(lower(email), ''), '__empty__') = COALESCE(NULLIF(lower($7), ''), '__empty__'))
				OR ($9 <> '' AND regexp_replace(COALESCE(phone, ''), '[^0-9]', '', 'g') = $9)
				OR ($7 <> '' AND lower(email) = lower($7))
			  )
			LIMIT 1
		), inserted AS (
			INSERT INTO contacts (
				organization_id, first_name, last_name, email, phone,
				address_line1, address_line2, city, state, postal_code, country,
				job_title, status, is_client, owner_user_id, custom_fields
			)
			SELECT $1, $5, $6, NULLIF($7, ''), NULLIF($8, ''),
			       NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, ''),
			       NULLIF($16, ''), NULLIF($17, ''), $18, $2, $19::jsonb
			WHERE NOT EXISTS (SELECT 1 FROM duplicate)
			RETURNING id, updated_at
		), activity AS (
			INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
			SELECT $1, 'contact', id, $2, 'contact.imported', 'Contact imported' FROM inserted
		)
		INSERT INTO import_batch_rows (
			organization_id, import_batch_id, row_number, status,
			entity_id, imported_entity_updated_at, errors_json
		)
		SELECT $1::bigint, $3::bigint, $4::integer, 'imported', id, updated_at, '[]'::jsonb FROM inserted
		UNION ALL
		SELECT $1::bigint, $3::bigint, $4::integer, 'error', NULL::bigint, NULL::timestamptz,
		       jsonb_build_array(jsonb_build_object('message', 'Possible duplicate contact already exists (record ' || id || ')'))
		FROM duplicate
		RETURNING status
	`, organizationID, actorUserID, batchID, row.RowNumber,
		firstName, lastName, email, phone, normalizePhoneDigits(phone),
		strings.TrimSpace(values["address_line1"]), strings.TrimSpace(values["address_line2"]),
		strings.TrimSpace(values["city"]), strings.TrimSpace(values["state"]), strings.TrimSpace(values["postal_code"]), strings.TrimSpace(values["country"]),
		strings.TrimSpace(values["job_title"]), strings.ToLower(strings.TrimSpace(values["status"])), isClient,
		customFieldsJSON,
	).Scan(&status); err != nil {
		return "", fmt.Errorf("persist contact import row %d: %w", row.RowNumber, err)
	}
	return status, nil
}

func persistImportedCompany(ctx context.Context, tx pgx.Tx, organizationID, actorUserID, batchID int64, definitions []modulecustomfields.Definition, row PreviewRow) (string, error) {
	values := row.Values
	name := strings.TrimSpace(values["name"])
	phone := strings.TrimSpace(values["phone"])
	website := normalizeWebsite(strings.TrimSpace(values["website"]))
	clientType := strings.ToLower(strings.TrimSpace(values["client_type"]))
	if clientType == "" {
		clientType = "organization"
	}
	customFieldsJSON, err := importCustomFieldsJSON(definitions, row)
	if err != nil {
		return "", fmt.Errorf("prepare company import row %d custom fields: %w", row.RowNumber, err)
	}
	var status string
	if err := tx.QueryRow(ctx, `
		WITH duplicate AS (
			SELECT id
			FROM companies
			WHERE organization_id = $1 AND archived_at IS NULL
			  AND (
				lower(name) = lower($5)
				OR ($7 <> '' AND regexp_replace(COALESCE(phone, ''), '[^0-9]', '', 'g') = $7)
				OR ($8 <> '' AND lower(website) = lower($8))
			  )
			LIMIT 1
		), inserted AS (
			INSERT INTO companies (
				organization_id, name, client_type, address_line1, address_line2,
				city, state, postal_code, country, industry, phone, website, status, owner_user_id, custom_fields
			)
			SELECT $1, $5, $6, NULLIF($9, ''), NULLIF($10, ''),
			       NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, ''),
			       NULLIF($16, ''), NULLIF($8, ''), NULLIF($17, ''), $2, $18::jsonb
			WHERE NOT EXISTS (SELECT 1 FROM duplicate)
			RETURNING id, updated_at
		), activity AS (
			INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
			SELECT $1, 'company', id, $2, 'company.imported', 'Company imported' FROM inserted
		)
		INSERT INTO import_batch_rows (
			organization_id, import_batch_id, row_number, status,
			entity_id, imported_entity_updated_at, errors_json
		)
		SELECT $1::bigint, $3::bigint, $4::integer, 'imported', id, updated_at, '[]'::jsonb FROM inserted
		UNION ALL
		SELECT $1::bigint, $3::bigint, $4::integer, 'error', NULL::bigint, NULL::timestamptz,
		       jsonb_build_array(jsonb_build_object('message', 'Possible duplicate company already exists (record ' || id || ')'))
		FROM duplicate
		RETURNING status
	`, organizationID, actorUserID, batchID, row.RowNumber,
		name, clientType, normalizePhoneDigits(phone), website,
		strings.TrimSpace(values["address_line1"]), strings.TrimSpace(values["address_line2"]),
		strings.TrimSpace(values["city"]), strings.TrimSpace(values["state"]), strings.TrimSpace(values["postal_code"]), strings.TrimSpace(values["country"]),
		strings.TrimSpace(values["industry"]), phone, strings.ToLower(strings.TrimSpace(values["status"])),
		customFieldsJSON,
	).Scan(&status); err != nil {
		return "", fmt.Errorf("persist company import row %d: %w", row.RowNumber, err)
	}
	return status, nil
}

func importCustomFieldsJSON(definitions []modulecustomfields.Definition, row PreviewRow) ([]byte, error) {
	values := modulecustomfields.Values{}
	for _, definition := range definitions {
		value := strings.TrimSpace(row.Values["custom:"+definition.FieldKey])
		if value == "" {
			if definition.Required {
				return nil, fmt.Errorf("%s is required", definition.Label)
			}
			continue
		}
		raw, err := modulecustomfields.RawValueFromString(definition, value)
		if err != nil {
			return nil, err
		}
		values[definition.FieldKey] = raw
	}
	return modulecustomfields.EncodeValues(values)
}

func normalizePhoneDigits(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if character >= '0' && character <= '9' {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func normalizeWebsite(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return strings.ToLower(value)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path == "/" {
		parsed.Path = ""
	}
	return parsed.String()
}
