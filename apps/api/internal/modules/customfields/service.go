package customfields

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) List(ctx context.Context, organizationID int64, entityType string, includeArchived bool) ([]Definition, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("custom fields service not configured")
	}
	return LoadDefinitions(ctx, s.pool, organizationID, entityType, includeArchived)
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, raw CreateInput) (Definition, error) {
	if s == nil || s.pool == nil {
		return Definition{}, fmt.Errorf("custom fields service not configured")
	}
	input, err := normalizeCreateInput(raw)
	if err != nil {
		return Definition{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Definition{}, fmt.Errorf("begin custom field create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return Definition{}, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM custom_field_definitions WHERE organization_id=$1 AND entity_type=$2 AND archived_at IS NULL`, organizationID, input.EntityType).Scan(&count); err != nil {
		return Definition{}, fmt.Errorf("count custom fields: %w", err)
	}
	if count >= MaxDefinitionsPerEntity {
		return Definition{}, fmt.Errorf("%w: at most %d active fields are allowed per record type", ErrConflict, MaxDefinitionsPerEntity)
	}
	if input.FieldKey == "" {
		input.FieldKey, err = availableFieldKey(ctx, tx, organizationID, input.EntityType, slugKey(input.Label))
		if err != nil {
			return Definition{}, err
		}
	}
	optionsJSON, _ := json.Marshal(input.Options)
	definition, err := scanDefinition(tx.QueryRow(ctx, `
		INSERT INTO custom_field_definitions (organization_id,created_by_user_id,entity_type,field_key,label,data_type,options_json,is_required,show_in_list,position)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10)
		RETURNING id,entity_type,field_key,label,data_type,options_json,is_required,show_in_list,position,created_by_user_id,created_at,updated_at,archived_at
	`, organizationID, actorUserID, input.EntityType, input.FieldKey, input.Label, input.DataType, optionsJSON, input.Required, input.ShowInList, input.Position))
	if err != nil {
		if isUniqueViolation(err) {
			return Definition{}, fmt.Errorf("%w: a field with that key or label already exists", ErrConflict)
		}
		return Definition{}, fmt.Errorf("create custom field: %w", err)
	}
	if err := auditDefinition(ctx, tx, organizationID, actorUserID, definition, "custom_field.created", "Created custom field"); err != nil {
		return Definition{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Definition{}, fmt.Errorf("commit custom field create: %w", err)
	}
	return definition, nil
}

func (s *Service) Update(ctx context.Context, organizationID, actorUserID, definitionID int64, raw UpdateInput) (Definition, error) {
	if s == nil || s.pool == nil {
		return Definition{}, fmt.Errorf("custom fields service not configured")
	}
	input, err := normalizeUpdateInput(raw)
	if err != nil {
		return Definition{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Definition{}, fmt.Errorf("begin custom field update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return Definition{}, err
	}
	current, err := loadDefinitionForUpdate(ctx, tx, organizationID, definitionID)
	if err != nil {
		return Definition{}, err
	}
	if current.ArchivedAt != nil {
		return Definition{}, ErrNotFound
	}
	if err := validateOptions(current.DataType, input.Options); err != nil {
		return Definition{}, err
	}
	if current.DataType == "select" {
		if err := ensureUsedOptionsRemain(ctx, tx, organizationID, current, input.Options); err != nil {
			return Definition{}, err
		}
	}
	optionsJSON, _ := json.Marshal(input.Options)
	definition, err := scanDefinition(tx.QueryRow(ctx, `
		UPDATE custom_field_definitions SET label=$3,options_json=$4::jsonb,is_required=$5,show_in_list=$6,position=$7,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
		RETURNING id,entity_type,field_key,label,data_type,options_json,is_required,show_in_list,position,created_by_user_id,created_at,updated_at,archived_at
	`, organizationID, definitionID, input.Label, optionsJSON, input.Required, input.ShowInList, input.Position))
	if err != nil {
		if isUniqueViolation(err) {
			return Definition{}, fmt.Errorf("%w: a field with that label already exists", ErrConflict)
		}
		return Definition{}, fmt.Errorf("update custom field: %w", err)
	}
	if err := auditDefinition(ctx, tx, organizationID, actorUserID, definition, "custom_field.updated", "Updated custom field"); err != nil {
		return Definition{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Definition{}, fmt.Errorf("commit custom field update: %w", err)
	}
	return definition, nil
}

func (s *Service) Archive(ctx context.Context, organizationID, actorUserID, definitionID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("custom fields service not configured")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin custom field archive: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveActor(ctx, tx, organizationID, actorUserID); err != nil {
		return err
	}
	definition, err := scanDefinition(tx.QueryRow(ctx, `
		UPDATE custom_field_definitions SET archived_at=NOW(),updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
		RETURNING id,entity_type,field_key,label,data_type,options_json,is_required,show_in_list,position,created_by_user_id,created_at,updated_at,archived_at
	`, organizationID, definitionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("archive custom field: %w", err)
	}
	if err := auditDefinition(ctx, tx, organizationID, actorUserID, definition, "custom_field.archived", "Archived custom field"); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit custom field archive: %w", err)
	}
	return nil
}

type definitionScanner interface{ Scan(...any) error }

func scanDefinition(row definitionScanner) (Definition, error) {
	var definition Definition
	var optionsJSON []byte
	if err := row.Scan(&definition.ID, &definition.EntityType, &definition.FieldKey, &definition.Label, &definition.DataType, &optionsJSON, &definition.Required, &definition.ShowInList, &definition.Position, &definition.CreatedByUserID, &definition.CreatedAt, &definition.UpdatedAt, &definition.ArchivedAt); err != nil {
		return Definition{}, err
	}
	if err := json.Unmarshal(optionsJSON, &definition.Options); err != nil {
		return Definition{}, fmt.Errorf("decode custom field options: %w", err)
	}
	if definition.Options == nil {
		definition.Options = []string{}
	}
	return definition, nil
}

func normalizeCreateInput(input CreateInput) (CreateInput, error) {
	if err := validateOptionEntries(input.Options); err != nil {
		return CreateInput{}, err
	}
	input.EntityType = normalizeEntityType(input.EntityType)
	input.FieldKey = normalizeFieldKey(input.FieldKey)
	input.Label = strings.TrimSpace(input.Label)
	input.DataType = strings.ToLower(strings.TrimSpace(input.DataType))
	input.Options = normalizeOptions(input.Options)
	if input.EntityType == "" || input.Label == "" || len(input.Label) > 100 || !contains([]string{"text", "number", "date", "boolean", "select"}, input.DataType) || input.Position < 0 || input.Position > 1000 {
		return CreateInput{}, fmt.Errorf("%w: valid entity type, label, data type, and position are required", ErrInvalidInput)
	}
	if input.FieldKey != "" && !fieldKeyPattern.MatchString(input.FieldKey) {
		return CreateInput{}, fmt.Errorf("%w: field key must use 2-40 lowercase letters, numbers, or underscores", ErrInvalidInput)
	}
	if err := validateOptions(input.DataType, input.Options); err != nil {
		return CreateInput{}, err
	}
	return input, nil
}

func normalizeUpdateInput(input UpdateInput) (UpdateInput, error) {
	if err := validateOptionEntries(input.Options); err != nil {
		return UpdateInput{}, err
	}
	input.Label = strings.TrimSpace(input.Label)
	input.Options = normalizeOptions(input.Options)
	if input.Label == "" || len(input.Label) > 100 || input.Position < 0 || input.Position > 1000 {
		return UpdateInput{}, fmt.Errorf("%w: valid label and position are required", ErrInvalidInput)
	}
	return input, nil
}

func validateOptionEntries(options []string) error {
	for _, raw := range options {
		value := strings.TrimSpace(raw)
		if value == "" || len(value) > 100 {
			return fmt.Errorf("%w: options must be 1-100 characters", ErrInvalidInput)
		}
	}
	return nil
}

func validateOptions(dataType string, options []string) error {
	if dataType != "select" && len(options) > 0 {
		return fmt.Errorf("%w: only select fields can have options", ErrInvalidInput)
	}
	if dataType == "select" && (len(options) < 1 || len(options) > 25) {
		return fmt.Errorf("%w: select fields require 1-25 options", ErrInvalidInput)
	}
	return nil
}

func normalizeOptions(values []string) []string {
	result := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 100 {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func slugKey(label string) string {
	var builder strings.Builder
	underscore := false
	for _, character := range strings.ToLower(label) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
			underscore = false
		} else if builder.Len() > 0 && !underscore {
			builder.WriteByte('_')
			underscore = true
		}
	}
	value := strings.Trim(builder.String(), "_")
	value = regexp.MustCompile(`[^a-z0-9_]`).ReplaceAllString(value, "")
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		value = "field_" + value
	}
	if len(value) < 2 {
		value += "_field"
	}
	if len(value) > 36 {
		value = strings.TrimRight(value[:36], "_")
	}
	return value
}

func availableFieldKey(ctx context.Context, tx pgx.Tx, organizationID int64, entityType, base string) (string, error) {
	for suffix := 0; suffix < 100; suffix++ {
		candidate := base
		if suffix > 0 {
			candidate = fmt.Sprintf("%s_%d", strings.TrimRight(base, "_"), suffix+1)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM custom_field_definitions WHERE organization_id=$1 AND entity_type=$2 AND field_key=$3)`, organizationID, entityType, candidate).Scan(&exists); err != nil {
			return "", fmt.Errorf("check custom field key: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: unable to allocate a stable field key", ErrConflict)
}

func loadDefinitionForUpdate(ctx context.Context, tx pgx.Tx, organizationID, definitionID int64) (Definition, error) {
	definition, err := scanDefinition(tx.QueryRow(ctx, `
		SELECT id,entity_type,field_key,label,data_type,options_json,is_required,show_in_list,position,created_by_user_id,created_at,updated_at,archived_at
		FROM custom_field_definitions WHERE organization_id=$1 AND id=$2 FOR UPDATE
	`, organizationID, definitionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Definition{}, ErrNotFound
	}
	return definition, err
}

func ensureUsedOptionsRemain(ctx context.Context, tx pgx.Tx, organizationID int64, definition Definition, options []string) error {
	table := "contacts"
	if definition.EntityType == "company" {
		table = "companies"
	}
	var invalid bool
	query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE organization_id=$1 AND custom_fields ? $2 AND NOT ((custom_fields ->> $2) = ANY($3::text[])))`, table)
	if err := tx.QueryRow(ctx, query, organizationID, definition.FieldKey, options).Scan(&invalid); err != nil {
		return fmt.Errorf("validate used custom field options: %w", err)
	}
	if invalid {
		return fmt.Errorf("%w: an option still used by records cannot be removed", ErrConflict)
	}
	return nil
}

func requireActiveActor(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64) error {
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id=$1 AND user_id=$2 AND membership_status='active')`, organizationID, actorUserID).Scan(&active); err != nil {
		return fmt.Errorf("validate custom field actor: %w", err)
	}
	if !active {
		return ErrInactiveActor
	}
	return nil
}

func auditDefinition(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, definition Definition, eventType, summary string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,$3,'custom_field',$4,$5,jsonb_build_object('recordType',$6::text,'fieldKey',$7::text,'dataType',$8::text))
	`, organizationID, actorUserID, eventType, definition.ID, summary, definition.EntityType, definition.FieldKey, definition.DataType)
	if err != nil {
		return fmt.Errorf("audit custom field: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
