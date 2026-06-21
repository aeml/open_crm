// Package leadforms stores public lead capture form definitions and turns
// accepted submissions into normal CRM contacts.
package leadforms

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateSlug     = errors.New("lead capture form slug already exists")
	ErrDuplicatePageSlug = errors.New("lead landing page slug already exists")
	ErrInvalidInput      = errors.New("invalid lead capture form")
	ErrInvalidPage       = errors.New("invalid lead landing page")
	ErrInvalidSubmission = errors.New("invalid lead capture submission")
	ErrNotFound          = errors.New("lead capture form not found")
)

var fieldKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

type Field struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	FieldType string `json:"fieldType"`
	Required  bool   `json:"required"`
	MapTo     string `json:"mapTo"`
}

type Form struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	PublicID        string    `json:"publicId"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Fields          []Field   `json:"fields"`
	SuccessMessage  string    `json:"successMessage"`
	SourceLabel     string    `json:"sourceLabel"`
	IsActive        bool      `json:"isActive"`
	SubmissionCount int       `json:"submissionCount"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Input struct {
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Fields         []Field `json:"fields"`
	SuccessMessage string  `json:"successMessage"`
	SourceLabel    string  `json:"sourceLabel"`
	IsActive       *bool   `json:"isActive"`
}

type SubmissionInput struct {
	Values     map[string]string `json:"values"`
	SourceURL  string            `json:"sourceUrl"`
	RemoteAddr string            `json:"remoteAddr"`
	UserAgent  string            `json:"userAgent"`
}

type Submission struct {
	ID        int64     `json:"id"`
	FormID    int64     `json:"formId"`
	ContactID int64     `json:"contactId"`
	CreatedAt time.Time `json:"createdAt"`
}

type SubmissionResult struct {
	Submission     Submission `json:"submission"`
	SuccessMessage string     `json:"successMessage"`
}

type LandingPage struct {
	ID                      int64     `json:"id"`
	Name                    string    `json:"name"`
	Slug                    string    `json:"slug"`
	PublicID                string    `json:"publicId"`
	Title                   string    `json:"title"`
	Subtitle                string    `json:"subtitle"`
	Body                    string    `json:"body"`
	CTALabel                string    `json:"ctaLabel"`
	Theme                   string    `json:"theme"`
	LeadCaptureFormID       int64     `json:"leadCaptureFormId"`
	LeadCaptureFormName     string    `json:"leadCaptureFormName"`
	LeadCaptureFormPublicID string    `json:"leadCaptureFormPublicId"`
	IsActive                bool      `json:"isActive"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

type LandingPageInput struct {
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	Title             string `json:"title"`
	Subtitle          string `json:"subtitle"`
	Body              string `json:"body"`
	CTALabel          string `json:"ctaLabel"`
	Theme             string `json:"theme"`
	LeadCaptureFormID int64  `json:"leadCaptureFormId"`
	IsActive          *bool  `json:"isActive"`
}

type PublicLandingPage struct {
	Page LandingPage `json:"page"`
	Form Form        `json:"form"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64) ([]Form, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("lead forms service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT f.id, f.name, f.slug, f.public_id, f.title, f.description, f.fields_json,
			f.success_message, f.source_label, f.is_active, COALESCE(sc.submission_count, 0), f.created_at, f.updated_at
		FROM lead_capture_forms f
		LEFT JOIN (
			SELECT form_id, COUNT(*)::int AS submission_count
			FROM lead_capture_submissions
			WHERE organization_id = $1
			GROUP BY form_id
		) sc ON sc.form_id = f.id
		WHERE f.organization_id = $1
		ORDER BY f.updated_at DESC, f.id DESC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list lead capture forms: %w", err)
	}
	defer rows.Close()

	forms := make([]Form, 0)
	for rows.Next() {
		form, err := scanForm(rows)
		if err != nil {
			return nil, err
		}
		forms = append(forms, form)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lead capture forms: %w", err)
	}
	return forms, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Form, error) {
	if s == nil || s.pool == nil {
		return Form{}, fmt.Errorf("lead forms service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Form{}, err
	}

	publicID, err := newPublicID()
	if err != nil {
		return Form{}, err
	}
	fieldsJSON, err := json.Marshal(input.Fields)
	if err != nil {
		return Form{}, fmt.Errorf("encode lead capture fields: %w", err)
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var form Form
	form, err = scanForm(s.pool.QueryRow(ctx, `
		INSERT INTO lead_capture_forms (organization_id, public_id, name, slug, title, description, fields_json, success_message, source_label, is_active, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $11)
		RETURNING id, name, slug, public_id, title, description, fields_json, success_message, source_label, is_active, 0, created_at, updated_at
	`, organizationID, publicID, input.Name, input.Slug, input.Title, input.Description, string(fieldsJSON), input.SuccessMessage, input.SourceLabel, isActive, actorUserID))
	if err != nil {
		return Form{}, mapSaveError(err)
	}
	return form, nil
}

func (s *Service) Update(ctx context.Context, organizationID, formID, actorUserID int64, input Input) (Form, error) {
	if s == nil || s.pool == nil {
		return Form{}, fmt.Errorf("lead forms service not configured")
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Form{}, err
	}
	fieldsJSON, err := json.Marshal(input.Fields)
	if err != nil {
		return Form{}, fmt.Errorf("encode lead capture fields: %w", err)
	}
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var form Form
	form, err = scanForm(s.pool.QueryRow(ctx, `
		UPDATE lead_capture_forms
		SET name = $3,
		    slug = $4,
		    title = $5,
		    description = $6,
		    fields_json = $7::jsonb,
		    success_message = $8,
		    source_label = $9,
		    is_active = COALESCE($10::boolean, is_active),
		    updated_by_user_id = $11,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2
		RETURNING id, name, slug, public_id, title, description, fields_json, success_message, source_label, is_active,
			(SELECT COUNT(*)::int FROM lead_capture_submissions WHERE organization_id = $1 AND form_id = lead_capture_forms.id), created_at, updated_at
	`, organizationID, formID, input.Name, input.Slug, input.Title, input.Description, string(fieldsJSON), input.SuccessMessage, input.SourceLabel, isActive, actorUserID))
	if err != nil {
		return Form{}, mapSaveError(err)
	}
	return form, nil
}

func (s *Service) ListLandingPagesByOrganization(ctx context.Context, organizationID int64) ([]LandingPage, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("lead forms service not configured")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.name, p.slug, p.public_id, p.title, p.subtitle, p.body, p.cta_label, p.theme,
			p.lead_capture_form_id, f.name, f.public_id, p.is_active, p.created_at, p.updated_at
		FROM lead_landing_pages p
		JOIN lead_capture_forms f ON f.organization_id = p.organization_id AND f.id = p.lead_capture_form_id
		WHERE p.organization_id = $1
		ORDER BY p.updated_at DESC, p.id DESC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list lead landing pages: %w", err)
	}
	defer rows.Close()

	pages := make([]LandingPage, 0)
	for rows.Next() {
		page, err := scanLandingPage(rows)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lead landing pages: %w", err)
	}
	return pages, nil
}

func (s *Service) CreateLandingPage(ctx context.Context, organizationID, actorUserID int64, input LandingPageInput) (LandingPage, error) {
	if s == nil || s.pool == nil {
		return LandingPage{}, fmt.Errorf("lead forms service not configured")
	}
	input = normalizeLandingPageInput(input)
	if err := validateLandingPageInput(input); err != nil {
		return LandingPage{}, err
	}
	publicID, err := newLandingPagePublicID()
	if err != nil {
		return LandingPage{}, err
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	page, err := scanLandingPage(s.pool.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO lead_landing_pages (organization_id, public_id, lead_capture_form_id, name, slug, title, subtitle, body, cta_label, theme, is_active, created_by_user_id, updated_by_user_id)
			SELECT $1, $2, f.id, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11
			FROM lead_capture_forms f
			WHERE f.organization_id = $1 AND f.id = $12
			RETURNING *
		)
		SELECT p.id, p.name, p.slug, p.public_id, p.title, p.subtitle, p.body, p.cta_label, p.theme,
			p.lead_capture_form_id, f.name, f.public_id, p.is_active, p.created_at, p.updated_at
		FROM inserted p
		JOIN lead_capture_forms f ON f.organization_id = p.organization_id AND f.id = p.lead_capture_form_id
	`, organizationID, publicID, input.Name, input.Slug, input.Title, input.Subtitle, input.Body, input.CTALabel, input.Theme, isActive, actorUserID, input.LeadCaptureFormID))
	if err != nil {
		return LandingPage{}, mapLandingPageSaveError(err)
	}
	return page, nil
}

func (s *Service) UpdateLandingPage(ctx context.Context, organizationID, pageID, actorUserID int64, input LandingPageInput) (LandingPage, error) {
	if s == nil || s.pool == nil {
		return LandingPage{}, fmt.Errorf("lead forms service not configured")
	}
	input = normalizeLandingPageInput(input)
	if err := validateLandingPageInput(input); err != nil {
		return LandingPage{}, err
	}
	var isActive any
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	page, err := scanLandingPage(s.pool.QueryRow(ctx, `
		WITH updated AS (
			UPDATE lead_landing_pages p
			SET lead_capture_form_id = $3,
			    name = $4,
			    slug = $5,
			    title = $6,
			    subtitle = $7,
			    body = $8,
			    cta_label = $9,
			    theme = $10,
			    is_active = COALESCE($11::boolean, p.is_active),
			    updated_by_user_id = $12,
			    updated_at = NOW()
			WHERE p.organization_id = $1
			  AND p.id = $2
			  AND EXISTS (SELECT 1 FROM lead_capture_forms f WHERE f.organization_id = $1 AND f.id = $3)
			RETURNING *
		)
		SELECT p.id, p.name, p.slug, p.public_id, p.title, p.subtitle, p.body, p.cta_label, p.theme,
			p.lead_capture_form_id, f.name, f.public_id, p.is_active, p.created_at, p.updated_at
		FROM updated p
		JOIN lead_capture_forms f ON f.organization_id = p.organization_id AND f.id = p.lead_capture_form_id
	`, organizationID, pageID, input.LeadCaptureFormID, input.Name, input.Slug, input.Title, input.Subtitle, input.Body, input.CTALabel, input.Theme, isActive, actorUserID))
	if err != nil {
		return LandingPage{}, mapLandingPageSaveError(err)
	}
	return page, nil
}

func (s *Service) GetPublicLandingPage(ctx context.Context, slug string) (PublicLandingPage, error) {
	if s == nil || s.pool == nil {
		return PublicLandingPage{}, fmt.Errorf("lead forms service not configured")
	}
	slug = normalizeSlug(slug)
	if slug == "" {
		return PublicLandingPage{}, ErrNotFound
	}

	page, form, err := scanPublicLandingPage(s.pool.QueryRow(ctx, `
		SELECT p.id, p.name, p.slug, p.public_id, p.title, p.subtitle, p.body, p.cta_label, p.theme,
			p.lead_capture_form_id, f.name, f.public_id, p.is_active, p.created_at, p.updated_at,
			f.id, f.name, f.slug, f.public_id, f.title, f.description, f.fields_json, f.success_message, f.source_label, f.is_active, 0, f.created_at, f.updated_at
		FROM lead_landing_pages p
		JOIN lead_capture_forms f ON f.organization_id = p.organization_id AND f.id = p.lead_capture_form_id
		WHERE p.slug = $1 AND p.is_active = TRUE AND f.is_active = TRUE
	`, slug))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublicLandingPage{}, ErrNotFound
		}
		return PublicLandingPage{}, fmt.Errorf("get public lead landing page: %w", err)
	}
	return PublicLandingPage{Page: page, Form: form}, nil
}

func (s *Service) SubmitByPublicID(ctx context.Context, publicID string, input SubmissionInput) (SubmissionResult, error) {
	if s == nil || s.pool == nil {
		return SubmissionResult{}, fmt.Errorf("lead forms service not configured")
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return SubmissionResult{}, ErrNotFound
	}

	form, organizationID, err := s.getActiveByPublicID(ctx, publicID)
	if err != nil {
		return SubmissionResult{}, err
	}
	contact, payload, err := contactInputFromSubmission(form, input.Values)
	if err != nil {
		return SubmissionResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SubmissionResult{}, fmt.Errorf("begin lead capture submission transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var contactID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO contacts (organization_id, first_name, last_name, email, phone, address_line1, address_line2, city, state, postal_code, country, job_title, status, is_client, owner_user_id)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), 'lead', FALSE, NULL)
		RETURNING id
	`, organizationID, contact.FirstName, contact.LastName, contact.Email, contact.Phone, contact.AddressLine1, contact.AddressLine2, contact.City, contact.State, contact.PostalCode, contact.Country, contact.JobTitle).Scan(&contactID); err != nil {
		return SubmissionResult{}, mapSubmissionSaveError(err)
	}

	metadataJSON, err := json.Marshal(map[string]any{"formId": form.ID, "formName": form.Name, "formPublicId": form.PublicID})
	if err != nil {
		return SubmissionResult{}, fmt.Errorf("encode lead capture activity metadata: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary, metadata_json)
		VALUES ($1, 'contact', $2, NULL, 'lead_form.submitted', $3, $4::jsonb)
	`, organizationID, contactID, "Submitted lead form: "+form.Name, string(metadataJSON)); err != nil {
		return SubmissionResult{}, fmt.Errorf("insert lead capture activity: %w", err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return SubmissionResult{}, fmt.Errorf("encode lead capture payload: %w", err)
	}
	var submission Submission
	if err := tx.QueryRow(ctx, `
		INSERT INTO lead_capture_submissions (organization_id, form_id, contact_id, payload_json, source_url, remote_addr, user_agent)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)
		RETURNING id, form_id, COALESCE(contact_id, 0), created_at
	`, organizationID, form.ID, contactID, string(payloadJSON), trimMax(input.SourceURL, 2048), trimMax(input.RemoteAddr, 255), trimMax(input.UserAgent, 1024)).Scan(&submission.ID, &submission.FormID, &submission.ContactID, &submission.CreatedAt); err != nil {
		return SubmissionResult{}, fmt.Errorf("insert lead capture submission: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return SubmissionResult{}, fmt.Errorf("commit lead capture submission transaction: %w", err)
	}

	return SubmissionResult{Submission: submission, SuccessMessage: form.SuccessMessage}, nil
}

func (s *Service) getActiveByPublicID(ctx context.Context, publicID string) (Form, int64, error) {
	form, organizationID, err := scanFormWithOrganization(s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, slug, public_id, title, description, fields_json, success_message, source_label, is_active, 0, created_at, updated_at
		FROM lead_capture_forms
		WHERE public_id = $1 AND is_active = TRUE
	`, publicID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Form{}, 0, ErrNotFound
		}
		return Form{}, 0, fmt.Errorf("get public lead capture form: %w", err)
	}
	return form, organizationID, nil
}

type contactInput struct {
	FirstName    string
	LastName     string
	Email        string
	Phone        string
	AddressLine1 string
	AddressLine2 string
	City         string
	State        string
	PostalCode   string
	Country      string
	JobTitle     string
}

func contactInputFromSubmission(form Form, values map[string]string) (contactInput, map[string]string, error) {
	normalizedValues := normalizeValues(values)
	contact := contactInput{}
	payload := make(map[string]string, len(normalizedValues))
	for key, value := range normalizedValues {
		payload[key] = value
	}

	for _, field := range form.Fields {
		value := strings.TrimSpace(normalizedValues[field.Key])
		if field.Required && value == "" {
			return contactInput{}, nil, ErrInvalidSubmission
		}
		switch field.MapTo {
		case "firstName":
			contact.FirstName = value
		case "lastName":
			contact.LastName = value
		case "email":
			contact.Email = strings.ToLower(value)
		case "phone":
			contact.Phone = value
		case "addressLine1":
			contact.AddressLine1 = value
		case "addressLine2":
			contact.AddressLine2 = value
		case "city":
			contact.City = value
		case "state":
			contact.State = value
		case "postalCode":
			contact.PostalCode = value
		case "country":
			contact.Country = value
		case "jobTitle":
			contact.JobTitle = value
		}
	}
	if contact.FirstName == "" || contact.LastName == "" {
		return contactInput{}, nil, ErrInvalidSubmission
	}
	return contact, payload, nil
}

type formScanner interface {
	Scan(...any) error
}

func scanForm(scanner formScanner) (Form, error) {
	var form Form
	var fieldsJSON []byte
	if err := scanner.Scan(
		&form.ID,
		&form.Name,
		&form.Slug,
		&form.PublicID,
		&form.Title,
		&form.Description,
		&fieldsJSON,
		&form.SuccessMessage,
		&form.SourceLabel,
		&form.IsActive,
		&form.SubmissionCount,
		&form.CreatedAt,
		&form.UpdatedAt,
	); err != nil {
		return Form{}, fmt.Errorf("scan lead capture form: %w", err)
	}
	if err := decodeFieldsJSON(fieldsJSON, &form); err != nil {
		return Form{}, err
	}
	return form, nil
}

func scanFormWithOrganization(scanner formScanner) (Form, int64, error) {
	var form Form
	var organizationID int64
	var fieldsJSON []byte
	if err := scanner.Scan(
		&form.ID,
		&organizationID,
		&form.Name,
		&form.Slug,
		&form.PublicID,
		&form.Title,
		&form.Description,
		&fieldsJSON,
		&form.SuccessMessage,
		&form.SourceLabel,
		&form.IsActive,
		&form.SubmissionCount,
		&form.CreatedAt,
		&form.UpdatedAt,
	); err != nil {
		return Form{}, 0, err
	}
	if err := decodeFieldsJSON(fieldsJSON, &form); err != nil {
		return Form{}, 0, err
	}
	return form, organizationID, nil
}

func scanLandingPage(scanner formScanner) (LandingPage, error) {
	var page LandingPage
	if err := scanner.Scan(
		&page.ID,
		&page.Name,
		&page.Slug,
		&page.PublicID,
		&page.Title,
		&page.Subtitle,
		&page.Body,
		&page.CTALabel,
		&page.Theme,
		&page.LeadCaptureFormID,
		&page.LeadCaptureFormName,
		&page.LeadCaptureFormPublicID,
		&page.IsActive,
		&page.CreatedAt,
		&page.UpdatedAt,
	); err != nil {
		return LandingPage{}, fmt.Errorf("scan lead landing page: %w", err)
	}
	return page, nil
}

func scanPublicLandingPage(scanner formScanner) (LandingPage, Form, error) {
	var page LandingPage
	var form Form
	var fieldsJSON []byte
	if err := scanner.Scan(
		&page.ID,
		&page.Name,
		&page.Slug,
		&page.PublicID,
		&page.Title,
		&page.Subtitle,
		&page.Body,
		&page.CTALabel,
		&page.Theme,
		&page.LeadCaptureFormID,
		&page.LeadCaptureFormName,
		&page.LeadCaptureFormPublicID,
		&page.IsActive,
		&page.CreatedAt,
		&page.UpdatedAt,
		&form.ID,
		&form.Name,
		&form.Slug,
		&form.PublicID,
		&form.Title,
		&form.Description,
		&fieldsJSON,
		&form.SuccessMessage,
		&form.SourceLabel,
		&form.IsActive,
		&form.SubmissionCount,
		&form.CreatedAt,
		&form.UpdatedAt,
	); err != nil {
		return LandingPage{}, Form{}, err
	}
	if err := decodeFieldsJSON(fieldsJSON, &form); err != nil {
		return LandingPage{}, Form{}, err
	}
	return page, form, nil
}

func decodeFieldsJSON(raw []byte, form *Form) error {
	if len(raw) == 0 {
		raw = []byte("[]")
	}
	if err := json.Unmarshal(raw, &form.Fields); err != nil {
		return fmt.Errorf("decode lead capture fields: %w", err)
	}
	return nil
}

func normalizeInput(input Input) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = normalizeSlug(input.Slug)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.SuccessMessage = strings.TrimSpace(input.SuccessMessage)
	input.SourceLabel = strings.TrimSpace(input.SourceLabel)
	if input.Slug == "" {
		input.Slug = normalizeSlug(input.Name)
	}
	if input.Title == "" {
		input.Title = input.Name
	}
	if input.SuccessMessage == "" {
		input.SuccessMessage = "Thanks. We will be in touch soon."
	}
	if input.SourceLabel == "" {
		input.SourceLabel = "Lead capture form"
	}
	if len(input.Fields) == 0 {
		input.Fields = defaultFields()
	}
	for index := range input.Fields {
		input.Fields[index] = normalizeField(input.Fields[index])
	}
	return input
}

func normalizeField(field Field) Field {
	field.Key = strings.TrimSpace(field.Key)
	field.Label = strings.TrimSpace(field.Label)
	field.FieldType = strings.TrimSpace(strings.ToLower(field.FieldType))
	field.MapTo = strings.TrimSpace(field.MapTo)
	if field.FieldType == "" {
		field.FieldType = "text"
	}
	return field
}

func validateInput(input Input) error {
	if input.Name == "" || input.Slug == "" || input.Title == "" || input.SuccessMessage == "" || input.SourceLabel == "" {
		return ErrInvalidInput
	}
	if len(input.Fields) == 0 || len(input.Fields) > 25 {
		return ErrInvalidInput
	}
	seenKeys := make(map[string]bool, len(input.Fields))
	seenMappings := make(map[string]bool)
	for _, field := range input.Fields {
		if field.Key == "" || field.Label == "" || !fieldKeyPattern.MatchString(field.Key) {
			return ErrInvalidInput
		}
		key := strings.ToLower(field.Key)
		if seenKeys[key] {
			return ErrInvalidInput
		}
		seenKeys[key] = true
		if !isAllowedFieldType(field.FieldType) || !isAllowedMapping(field.MapTo) {
			return ErrInvalidInput
		}
		if field.MapTo != "" {
			if seenMappings[field.MapTo] {
				return ErrInvalidInput
			}
			seenMappings[field.MapTo] = true
		}
	}
	if !seenMappings["firstName"] || !seenMappings["lastName"] {
		return ErrInvalidInput
	}
	return nil
}

func normalizeLandingPageInput(input LandingPageInput) LandingPageInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = normalizeSlug(input.Slug)
	input.Title = strings.TrimSpace(input.Title)
	input.Subtitle = strings.TrimSpace(input.Subtitle)
	input.Body = strings.TrimSpace(input.Body)
	input.CTALabel = strings.TrimSpace(input.CTALabel)
	input.Theme = strings.TrimSpace(strings.ToLower(input.Theme))
	if input.Slug == "" {
		input.Slug = normalizeSlug(input.Name)
	}
	if input.Title == "" {
		input.Title = input.Name
	}
	if input.CTALabel == "" {
		input.CTALabel = "Submit"
	}
	if input.Theme == "" {
		input.Theme = "light"
	}
	return input
}

func validateLandingPageInput(input LandingPageInput) error {
	if input.Name == "" || input.Slug == "" || input.Title == "" || input.CTALabel == "" || input.LeadCaptureFormID <= 0 {
		return ErrInvalidPage
	}
	if !isAllowedLandingPageTheme(input.Theme) {
		return ErrInvalidPage
	}
	return nil
}

func isAllowedLandingPageTheme(theme string) bool {
	switch theme {
	case "light", "blue", "dark":
		return true
	default:
		return false
	}
}

func isAllowedFieldType(fieldType string) bool {
	switch fieldType {
	case "text", "email", "tel", "textarea", "hidden":
		return true
	default:
		return false
	}
}

func isAllowedMapping(mapping string) bool {
	switch mapping {
	case "", "firstName", "lastName", "email", "phone", "addressLine1", "addressLine2", "city", "state", "postalCode", "country", "jobTitle":
		return true
	default:
		return false
	}
}

func defaultFields() []Field {
	return []Field{
		{Key: "firstName", Label: "First name", FieldType: "text", Required: true, MapTo: "firstName"},
		{Key: "lastName", Label: "Last name", FieldType: "text", Required: true, MapTo: "lastName"},
		{Key: "email", Label: "Email", FieldType: "email", Required: true, MapTo: "email"},
		{Key: "phone", Label: "Phone", FieldType: "tel", Required: false, MapTo: "phone"},
		{Key: "message", Label: "How can we help?", FieldType: "textarea", Required: false},
	}
}

func normalizeValues(values map[string]string) map[string]string {
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		normalized[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return normalized
}

func normalizeSlug(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	lastWasDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastWasDash = false
			continue
		}
		if !lastWasDash && builder.Len() > 0 {
			builder.WriteRune('-')
			lastWasDash = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-")
	}
	return slug
}

func newPublicID() (string, error) {
	return newPrefixedPublicID("lf")
}

func newLandingPagePublicID() (string, error) {
	return newPrefixedPublicID("lp")
}

func newPrefixedPublicID(prefix string) (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate %s public id: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(token[:]), nil
}

func trimMax(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	if maxLength <= 0 || len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}

func mapSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrDuplicateSlug
		case "23514", "22P02":
			return ErrInvalidInput
		}
	}
	return fmt.Errorf("save lead capture form: %w", err)
}

func mapLandingPageSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrDuplicatePageSlug
		case "23503":
			return ErrNotFound
		case "23514", "22P02":
			return ErrInvalidPage
		}
	}
	return fmt.Errorf("save lead landing page: %w", err)
}

func mapSubmissionSaveError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23514", "22P02":
			return ErrInvalidSubmission
		}
	}
	return fmt.Errorf("save lead capture submission: %w", err)
}
