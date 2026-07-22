package emailtemplates

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Template, error) {
	if s == nil || s.pool == nil {
		return Template{}, fmt.Errorf("email templates service not configured")
	}
	if organizationID <= 0 || actorUserID <= 0 {
		return Template{}, ErrInvalidInput
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Template{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Template{}, fmt.Errorf("begin email template create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockDefinitionWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Template{}, err
	}
	if err := requireDefinitionCapacity(ctx, tx, organizationID, "template"); err != nil {
		return Template{}, err
	}
	var template Template
	err = tx.QueryRow(ctx, `
		INSERT INTO email_templates (organization_id,name,subject,body)
		VALUES ($1,$2,$3,$4)
		RETURNING id,name,subject,body,revision,created_at,updated_at
	`, organizationID, input.Name, input.Subject, input.Body).Scan(
		&template.ID, &template.Name, &template.Subject, &template.Body, &template.Revision, &template.CreatedAt, &template.UpdatedAt,
	)
	if err != nil {
		return Template{}, mapSaveError(err)
	}
	if err := auditDefinition(ctx, tx, organizationID, actorUserID, template.ID, template.Name, template.Revision, "template", "created"); err != nil {
		return Template{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Template{}, fmt.Errorf("commit email template create: %w", err)
	}
	return template, nil
}

func (s *Service) Update(ctx context.Context, organizationID, templateID, actorUserID int64, input Input) (Template, error) {
	if s == nil || s.pool == nil {
		return Template{}, fmt.Errorf("email templates service not configured")
	}
	if organizationID <= 0 || templateID <= 0 || actorUserID <= 0 || input.ExpectedRevision <= 0 {
		return Template{}, ErrInvalidInput
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Template{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Template{}, fmt.Errorf("begin email template update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockDefinitionWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Template{}, err
	}
	if err := requireExpectedRevision(ctx, tx, "email_templates", organizationID, templateID, input.ExpectedRevision); err != nil {
		return Template{}, err
	}
	var template Template
	err = tx.QueryRow(ctx, `
		UPDATE email_templates
		SET name=$3,subject=$4,body=$5,revision=revision+1,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND revision=$6
		RETURNING id,name,subject,body,revision,created_at,updated_at
	`, organizationID, templateID, input.Name, input.Subject, input.Body, input.ExpectedRevision).Scan(
		&template.ID, &template.Name, &template.Subject, &template.Body, &template.Revision, &template.CreatedAt, &template.UpdatedAt,
	)
	if err != nil {
		return Template{}, mapCASWriteError(err)
	}
	if err := auditDefinition(ctx, tx, organizationID, actorUserID, template.ID, template.Name, template.Revision, "template", "updated"); err != nil {
		return Template{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Template{}, fmt.Errorf("commit email template update: %w", err)
	}
	return template, nil
}

func (s *Service) Delete(ctx context.Context, organizationID, templateID, actorUserID int64, expectedRevision int) error {
	return s.deleteDefinition(ctx, organizationID, templateID, actorUserID, expectedRevision, "template")
}

func (s *Service) CreateSnippet(ctx context.Context, organizationID, actorUserID int64, input SnippetInput) (Snippet, error) {
	if s == nil || s.pool == nil {
		return Snippet{}, fmt.Errorf("email templates service not configured")
	}
	if organizationID <= 0 || actorUserID <= 0 {
		return Snippet{}, ErrInvalidInput
	}
	input = normalizeSnippetInput(input)
	if err := validateSnippetInput(input); err != nil {
		return Snippet{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Snippet{}, fmt.Errorf("begin email snippet create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockDefinitionWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Snippet{}, err
	}
	if err := requireDefinitionCapacity(ctx, tx, organizationID, "snippet"); err != nil {
		return Snippet{}, err
	}
	var snippet Snippet
	err = tx.QueryRow(ctx, `
		INSERT INTO email_snippets (organization_id,name,body)
		VALUES ($1,$2,$3)
		RETURNING id,name,body,revision,created_at,updated_at
	`, organizationID, input.Name, input.Body).Scan(
		&snippet.ID, &snippet.Name, &snippet.Body, &snippet.Revision, &snippet.CreatedAt, &snippet.UpdatedAt,
	)
	if err != nil {
		return Snippet{}, mapSaveError(err)
	}
	if err := auditDefinition(ctx, tx, organizationID, actorUserID, snippet.ID, snippet.Name, snippet.Revision, "snippet", "created"); err != nil {
		return Snippet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Snippet{}, fmt.Errorf("commit email snippet create: %w", err)
	}
	return snippet, nil
}

func (s *Service) UpdateSnippet(ctx context.Context, organizationID, snippetID, actorUserID int64, input SnippetInput) (Snippet, error) {
	if s == nil || s.pool == nil {
		return Snippet{}, fmt.Errorf("email templates service not configured")
	}
	if organizationID <= 0 || snippetID <= 0 || actorUserID <= 0 || input.ExpectedRevision <= 0 {
		return Snippet{}, ErrInvalidInput
	}
	input = normalizeSnippetInput(input)
	if err := validateSnippetInput(input); err != nil {
		return Snippet{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Snippet{}, fmt.Errorf("begin email snippet update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockDefinitionWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return Snippet{}, err
	}
	if err := requireExpectedRevision(ctx, tx, "email_snippets", organizationID, snippetID, input.ExpectedRevision); err != nil {
		return Snippet{}, err
	}
	var snippet Snippet
	err = tx.QueryRow(ctx, `
		UPDATE email_snippets
		SET name=$3,body=$4,revision=revision+1,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND revision=$5
		RETURNING id,name,body,revision,created_at,updated_at
	`, organizationID, snippetID, input.Name, input.Body, input.ExpectedRevision).Scan(
		&snippet.ID, &snippet.Name, &snippet.Body, &snippet.Revision, &snippet.CreatedAt, &snippet.UpdatedAt,
	)
	if err != nil {
		return Snippet{}, mapCASWriteError(err)
	}
	if err := auditDefinition(ctx, tx, organizationID, actorUserID, snippet.ID, snippet.Name, snippet.Revision, "snippet", "updated"); err != nil {
		return Snippet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Snippet{}, fmt.Errorf("commit email snippet update: %w", err)
	}
	return snippet, nil
}

func (s *Service) DeleteSnippet(ctx context.Context, organizationID, snippetID, actorUserID int64, expectedRevision int) error {
	return s.deleteDefinition(ctx, organizationID, snippetID, actorUserID, expectedRevision, "snippet")
}

func (s *Service) deleteDefinition(ctx context.Context, organizationID, definitionID, actorUserID int64, expectedRevision int, kind string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("email templates service not configured")
	}
	if organizationID <= 0 || definitionID <= 0 || actorUserID <= 0 || expectedRevision <= 0 {
		return ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email %s delete: %w", kind, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockDefinitionWriter(ctx, tx, organizationID, actorUserID); err != nil {
		return err
	}
	name, err := lockDefinitionForDelete(ctx, tx, organizationID, definitionID, expectedRevision, kind)
	if err != nil {
		return err
	}
	var tag pgconn.CommandTag
	switch kind {
	case "template":
		tag, err = tx.Exec(ctx, `DELETE FROM email_templates WHERE organization_id=$1 AND id=$2 AND revision=$3`, organizationID, definitionID, expectedRevision)
	case "snippet":
		tag, err = tx.Exec(ctx, `DELETE FROM email_snippets WHERE organization_id=$1 AND id=$2 AND revision=$3`, organizationID, definitionID, expectedRevision)
	default:
		return fmt.Errorf("unsupported email definition kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("delete email %s: %w", kind, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	if err := auditDefinition(ctx, tx, organizationID, actorUserID, definitionID, name, expectedRevision, kind, "deleted"); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email %s delete: %w", kind, err)
	}
	return nil
}

func requireExpectedRevision(ctx context.Context, tx pgx.Tx, table string, organizationID, definitionID int64, expectedRevision int) error {
	var revision int
	var err error
	switch table {
	case "email_templates":
		err = tx.QueryRow(ctx, `SELECT revision FROM email_templates WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, definitionID).Scan(&revision)
	case "email_snippets":
		err = tx.QueryRow(ctx, `SELECT revision FROM email_snippets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, definitionID).Scan(&revision)
	default:
		return fmt.Errorf("unsupported email definition table %q", table)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock email definition: %w", err)
	}
	if revision != expectedRevision {
		return ErrConflict
	}
	return nil
}

func lockDefinitionForDelete(ctx context.Context, tx pgx.Tx, organizationID, definitionID int64, expectedRevision int, kind string) (string, error) {
	var name string
	var revision int
	var err error
	switch kind {
	case "template":
		err = tx.QueryRow(ctx, `SELECT name,revision FROM email_templates WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, definitionID).Scan(&name, &revision)
	case "snippet":
		err = tx.QueryRow(ctx, `SELECT name,revision FROM email_snippets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, definitionID).Scan(&name, &revision)
	default:
		return "", fmt.Errorf("unsupported email definition kind %q", kind)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock email %s for delete: %w", kind, err)
	}
	if revision != expectedRevision {
		return "", ErrConflict
	}
	return name, nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)
	return input
}

func validateInput(input Input) error {
	if input.Name == "" || utf8.RuneCountInString(input.Name) > MaxTemplateNameLength ||
		input.Subject == "" || utf8.RuneCountInString(input.Subject) > MaxTemplateSubjectLen ||
		input.Body == "" || utf8.RuneCountInString(input.Body) > MaxTemplateBodyLength {
		return ErrInvalidInput
	}
	return nil
}

func normalizeSnippetInput(input SnippetInput) SnippetInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Body = strings.TrimSpace(input.Body)
	return input
}

func validateSnippetInput(input SnippetInput) error {
	if input.Name == "" || utf8.RuneCountInString(input.Name) > MaxSnippetNameLength ||
		input.Body == "" || utf8.RuneCountInString(input.Body) > MaxSnippetBodyLength {
		return ErrInvalidInput
	}
	return nil
}

func mapCASWriteError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	return mapSaveError(err)
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateName
	}
	return fmt.Errorf("save email definition: %w", err)
}
