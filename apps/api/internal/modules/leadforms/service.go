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
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateSlug     = errors.New("lead capture form slug already exists")
	ErrDuplicatePageSlug = errors.New("lead landing page slug already exists")
	ErrStaleLandingPage  = errors.New("lead landing page revision is stale")
	ErrStaleWidget       = errors.New("lead chat widget revision is stale")
	ErrInvalidInput      = errors.New("invalid lead capture form")
	ErrInvalidPage       = errors.New("invalid lead landing page")
	ErrInvalidSubmission = errors.New("invalid lead capture submission")
	ErrConsentRequired   = errors.New("lead capture consent is required")
	ErrChallengeInvalid  = errors.New("lead capture submission challenge is invalid")
	ErrChallengeNotReady = errors.New("lead capture submission challenge is not ready")
	ErrStaleRevision     = errors.New("lead capture form revision is stale")
	ErrInvalidMapping    = errors.New("lead capture field mapping is invalid")
	ErrFormUnavailable   = errors.New("lead capture form configuration is unavailable")
	ErrInvalidWidget     = errors.New("invalid lead chat widget")
	ErrNotFound          = errors.New("lead capture form not found")
	ErrQueryTimeout      = errors.New("lead capture operation timed out")
)

const leadFormOperationTimeout = 5 * time.Second

var fieldKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

type Field struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	FieldType string   `json:"fieldType"`
	Required  bool     `json:"required"`
	MapTo     string   `json:"mapTo"`
	Options   []string `json:"options,omitempty"`
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
	ConsentText     string    `json:"consentText"`
	IsActive        bool      `json:"isActive"`
	Revision        int       `json:"revision"`
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
	ConsentText    string  `json:"consentText"`
	IsActive       *bool   `json:"isActive"`
	Revision       int     `json:"revision"`
}

type Attribution struct {
	LeadSource  string `json:"leadSource"`
	UTMSource   string `json:"utmSource"`
	UTMMedium   string `json:"utmMedium"`
	UTMCampaign string `json:"utmCampaign"`
	UTMTerm     string `json:"utmTerm"`
	UTMContent  string `json:"utmContent"`
}

type SubmissionInput struct {
	Values         map[string]string `json:"values"`
	SourceURL      string            `json:"sourceUrl"`
	Attribution    Attribution       `json:"attribution"`
	ChallengeToken string            `json:"challengeToken"`
	ConsentGranted bool              `json:"consentGranted"`
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
	Replayed       bool       `json:"-"`
}

type SubmissionChallenge struct {
	Token        string    `json:"token"`
	ConsentText  string    `json:"consentText"`
	FormRevision int       `json:"formRevision"`
	NotBefore    time.Time `json:"notBefore"`
	ExpiresAt    time.Time `json:"expiresAt"`
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
	Revision                int       `json:"revision"`
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
	Revision          int    `json:"revision"`
}

type PublicLandingPage struct {
	Page LandingPage `json:"page"`
	Form Form        `json:"form"`
}

type ChatWidget struct {
	ID                      int64     `json:"id"`
	Name                    string    `json:"name"`
	PublicID                string    `json:"publicId"`
	Title                   string    `json:"title"`
	WelcomeMessage          string    `json:"welcomeMessage"`
	PromptLabel             string    `json:"promptLabel"`
	CTALabel                string    `json:"ctaLabel"`
	Theme                   string    `json:"theme"`
	Position                string    `json:"position"`
	LeadCaptureFormID       int64     `json:"leadCaptureFormId"`
	LeadCaptureFormName     string    `json:"leadCaptureFormName"`
	LeadCaptureFormPublicID string    `json:"leadCaptureFormPublicId"`
	IsActive                bool      `json:"isActive"`
	Revision                int       `json:"revision"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

type ChatWidgetInput struct {
	Name              string `json:"name"`
	Title             string `json:"title"`
	WelcomeMessage    string `json:"welcomeMessage"`
	PromptLabel       string `json:"promptLabel"`
	CTALabel          string `json:"ctaLabel"`
	Theme             string `json:"theme"`
	Position          string `json:"position"`
	LeadCaptureFormID int64  `json:"leadCaptureFormId"`
	IsActive          *bool  `json:"isActive"`
	Revision          int    `json:"revision"`
}

type PublicChatWidget struct {
	Widget ChatWidget `json:"widget"`
	Form   Form       `json:"form"`
}

type Service struct {
	pool                 *pgxpool.Pool
	enforceHostedBilling bool
	capacity             modulebilling.CapacityManager
	now                  func() time.Time
	operationTimeout     time.Duration
}

func NewService(pool *pgxpool.Pool, enforceHostedBilling ...bool) *Service {
	enforce := len(enforceHostedBilling) > 0 && enforceHostedBilling[0]
	return &Service{pool: pool, enforceHostedBilling: enforce, now: time.Now, operationTimeout: leadFormOperationTimeout}
}

func NewServiceWithCapacity(pool *pgxpool.Pool, capacity modulebilling.CapacityManager, enforceHostedBilling bool) *Service {
	return &Service{pool: pool, enforceHostedBilling: enforceHostedBilling, capacity: capacity, now: time.Now, operationTimeout: leadFormOperationTimeout}
}

func (s *Service) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := s.operationTimeout
	if timeout <= 0 {
		timeout = leadFormOperationTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func IsQueryTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrQueryTimeout)
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input Input) (Form, error) {
	if s == nil || s.pool == nil {
		return Form{}, fmt.Errorf("lead forms service not configured")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	input = normalizeInput(input)
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Form{}, fmt.Errorf("begin lead capture form create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireLeadFormAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return Form{}, err
	}
	input.Fields, err = hydrateFormFields(ctx, tx, organizationID, input.Fields, isActive)
	if err != nil {
		return Form{}, err
	}
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
	var form Form
	form, err = scanForm(tx.QueryRow(ctx, `
		INSERT INTO lead_capture_forms (organization_id, public_id, name, slug, title, description, fields_json, success_message, source_label, consent_text, is_active, created_by_user_id, updated_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $12)
		RETURNING id, name, slug, public_id, title, description, fields_json, success_message, source_label, consent_text, is_active, COALESCE(revision, 1), 0, created_at, updated_at
	`, organizationID, publicID, input.Name, input.Slug, input.Title, input.Description, string(fieldsJSON), input.SuccessMessage, input.SourceLabel, input.ConsentText, isActive, actorUserID))
	if err != nil {
		return Form{}, mapSaveError(err)
	}
	if err := auditLeadFormDefinition(ctx, tx, organizationID, actorUserID, form, "lead_form.created", "Created lead capture form", 0); err != nil {
		return Form{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Form{}, fmt.Errorf("commit lead capture form create: %w", err)
	}
	return form, nil
}

func (s *Service) Update(ctx context.Context, organizationID, formID, actorUserID int64, input Input) (Form, error) {
	if s == nil || s.pool == nil {
		return Form{}, fmt.Errorf("lead forms service not configured")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	input = normalizeInput(input)
	if input.Revision <= 0 {
		return Form{}, ErrInvalidInput
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Form{}, fmt.Errorf("begin lead capture form update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireLeadFormAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return Form{}, err
	}
	var currentRevision int
	var currentActive bool
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(revision, 1), is_active
		FROM lead_capture_forms
		WHERE organization_id=$1 AND id=$2
		FOR UPDATE
	`, organizationID, formID).Scan(&currentRevision, &currentActive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Form{}, ErrNotFound
		}
		return Form{}, fmt.Errorf("lock lead capture form: %w", err)
	}
	if input.Revision != currentRevision {
		return Form{}, ErrStaleRevision
	}
	targetActive := currentActive
	if input.IsActive != nil {
		targetActive = *input.IsActive
	}
	input.Fields, err = hydrateFormFields(ctx, tx, organizationID, input.Fields, targetActive)
	if err != nil {
		return Form{}, err
	}
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
	form, err = scanForm(tx.QueryRow(ctx, `
		UPDATE lead_capture_forms
		SET name = $3,
		    slug = $4,
		    title = $5,
		    description = $6,
		    fields_json = $7::jsonb,
		    success_message = $8,
		    source_label = $9,
		    consent_text = $10,
		    is_active = COALESCE($11::boolean, is_active),
		    updated_by_user_id = $12,
		    revision = COALESCE(revision, 1) + 1,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND COALESCE(revision, 1) = $13
		RETURNING id, name, slug, public_id, title, description, fields_json, success_message, source_label, consent_text, is_active, COALESCE(revision, 1),
			(SELECT COUNT(*)::int FROM lead_capture_submissions WHERE organization_id = $1 AND form_id = lead_capture_forms.id), created_at, updated_at
	`, organizationID, formID, input.Name, input.Slug, input.Title, input.Description, string(fieldsJSON), input.SuccessMessage, input.SourceLabel, input.ConsentText, isActive, actorUserID, currentRevision))
	if err != nil {
		return Form{}, mapSaveError(err)
	}
	if err := auditLeadFormDefinition(ctx, tx, organizationID, actorUserID, form, "lead_form.updated", "Updated lead capture form", currentRevision); err != nil {
		return Form{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Form{}, fmt.Errorf("commit lead capture form update: %w", err)
	}
	return form, nil
}

func (s *Service) GetPublicLandingPage(ctx context.Context, slug string) (PublicLandingPage, error) {
	if s == nil || s.pool == nil {
		return PublicLandingPage{}, fmt.Errorf("lead forms service not configured")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	slug = normalizeSlug(slug)
	if slug == "" {
		return PublicLandingPage{}, ErrNotFound
	}

	page, form, err := scanPublicLandingPage(s.pool.QueryRow(ctx, `
		SELECT p.id, p.name, p.slug, p.public_id, p.title, p.subtitle, p.body, p.cta_label, p.theme,
			p.lead_capture_form_id, f.name, f.public_id, p.is_active, COALESCE(p.revision, 1), p.created_at, p.updated_at,
			f.id, f.name, f.slug, f.public_id, f.title, f.description, f.fields_json, f.success_message, f.source_label, f.consent_text, f.is_active, COALESCE(f.revision, 1), 0, f.created_at, f.updated_at
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
	var organizationID int64
	if err := s.pool.QueryRow(ctx, `SELECT organization_id FROM lead_capture_forms WHERE id=$1 AND is_active=TRUE`, form.ID).Scan(&organizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublicLandingPage{}, ErrNotFound
		}
		return PublicLandingPage{}, fmt.Errorf("load public lead form workspace: %w", err)
	}
	form.Fields, err = hydrateFormFields(ctx, s.pool, organizationID, form.Fields, true)
	if err != nil {
		if errors.Is(err, ErrInvalidMapping) {
			return PublicLandingPage{}, ErrFormUnavailable
		}
		return PublicLandingPage{}, err
	}
	return PublicLandingPage{Page: page, Form: form}, nil
}

func (s *Service) GetPublicChatWidget(ctx context.Context, publicID string) (PublicChatWidget, error) {
	if s == nil || s.pool == nil {
		return PublicChatWidget{}, fmt.Errorf("lead forms service not configured")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return PublicChatWidget{}, ErrNotFound
	}

	widget, form, err := scanPublicChatWidget(s.pool.QueryRow(ctx, `
		SELECT w.id, w.name, w.public_id, w.title, w.welcome_message, w.prompt_label, w.cta_label, w.theme, w.position,
			w.lead_capture_form_id, f.name, f.public_id, w.is_active, COALESCE(w.revision, 1), w.created_at, w.updated_at,
			f.id, f.name, f.slug, f.public_id, f.title, f.description, f.fields_json, f.success_message, f.source_label, f.consent_text, f.is_active, COALESCE(f.revision, 1), 0, f.created_at, f.updated_at
		FROM lead_chat_widgets w
		JOIN lead_capture_forms f ON f.organization_id = w.organization_id AND f.id = w.lead_capture_form_id
		WHERE w.public_id = $1 AND w.is_active = TRUE AND f.is_active = TRUE
	`, publicID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublicChatWidget{}, ErrNotFound
		}
		return PublicChatWidget{}, fmt.Errorf("get public lead chat widget: %w", err)
	}
	var organizationID int64
	if err := s.pool.QueryRow(ctx, `SELECT organization_id FROM lead_capture_forms WHERE id=$1 AND is_active=TRUE`, form.ID).Scan(&organizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublicChatWidget{}, ErrNotFound
		}
		return PublicChatWidget{}, fmt.Errorf("load public lead widget workspace: %w", err)
	}
	form.Fields, err = hydrateFormFields(ctx, s.pool, organizationID, form.Fields, true)
	if err != nil {
		if errors.Is(err, ErrInvalidMapping) {
			return PublicChatWidget{}, ErrFormUnavailable
		}
		return PublicChatWidget{}, err
	}
	return PublicChatWidget{Widget: widget, Form: form}, nil
}

func (s *Service) SubmitByPublicID(ctx context.Context, publicID string, input SubmissionInput) (SubmissionResult, error) {
	if s == nil || s.pool == nil {
		return SubmissionResult{}, fmt.Errorf("lead forms service not configured")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return SubmissionResult{}, ErrNotFound
	}

	form, organizationID, err := s.getActiveByPublicID(ctx, publicID)
	if err != nil {
		return SubmissionResult{}, err
	}
	if !input.ConsentGranted {
		return SubmissionResult{}, ErrConsentRequired
	}
	preflightChallenge, err := s.loadSubmissionChallenge(ctx, organizationID, form.ID, input.ChallengeToken)
	if err != nil {
		return SubmissionResult{}, err
	}
	if result, replayed, err := s.replayedSubmission(ctx, s.pool, organizationID, form.ID, preflightChallenge, input, form.SuccessMessage); err != nil {
		return SubmissionResult{}, err
	} else if replayed {
		return result, nil
	}
	if preflightChallenge.formRevision != form.Revision {
		return SubmissionResult{}, ErrChallengeInvalid
	}
	form.Fields, err = hydrateFormFields(ctx, s.pool, organizationID, form.Fields, true)
	if err != nil {
		if errors.Is(err, ErrInvalidMapping) {
			return SubmissionResult{}, ErrFormUnavailable
		}
		return SubmissionResult{}, err
	}
	if _, err := prepareLeadContact(ctx, s.pool, organizationID, form, input.Values); err != nil {
		return SubmissionResult{}, err
	}
	preflightNow := s.currentTime()
	if preflightNow.Before(preflightChallenge.notBefore) {
		return SubmissionResult{}, ErrChallengeNotReady
	}
	if !preflightNow.Before(preflightChallenge.expiresAt) {
		return SubmissionResult{}, ErrChallengeInvalid
	}
	reservation, err := modulebilling.ReserveCapacity(ctx, s.capacity, organizationID, modulebilling.ResourceContacts, 1)
	if err != nil {
		return SubmissionResult{}, err
	}
	defer modulebilling.CancelReservation(s.capacity, reservation)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SubmissionResult{}, fmt.Errorf("begin lead capture submission transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := modulebilling.LockCapacityEffect(ctx, tx, reservation); err != nil {
		return SubmissionResult{}, err
	}
	form, lockedOrganizationID, err := s.getActiveByPublicIDTx(ctx, tx, publicID)
	if err != nil {
		return SubmissionResult{}, err
	}
	if lockedOrganizationID != organizationID {
		return SubmissionResult{}, ErrNotFound
	}
	challenge, err := s.lockSubmissionChallenge(ctx, tx, organizationID, form.ID, input.ChallengeToken)
	if err != nil {
		return SubmissionResult{}, err
	}
	if result, replayed, err := s.replayedSubmission(ctx, tx, organizationID, form.ID, challenge, input, form.SuccessMessage); err != nil {
		return SubmissionResult{}, err
	} else if replayed {
		return result, nil
	}
	if challenge.formRevision != form.Revision {
		return SubmissionResult{}, ErrChallengeInvalid
	}
	form.Fields, err = hydrateFormFields(ctx, tx, organizationID, form.Fields, true)
	if err != nil {
		if errors.Is(err, ErrInvalidMapping) {
			return SubmissionResult{}, ErrFormUnavailable
		}
		return SubmissionResult{}, err
	}
	prepared, err := prepareLeadContact(ctx, tx, organizationID, form, input.Values)
	if err != nil {
		return SubmissionResult{}, err
	}
	sourceURL := trimMax(input.SourceURL, 2048)
	attribution := normalizeAttribution(form, input, sourceURL)
	requestDigest, err := submissionRequestDigest(form, prepared.Payload, sourceURL, attribution, challenge.consentText)
	if err != nil {
		return SubmissionResult{}, err
	}
	now := s.currentTime()
	if now.Before(challenge.notBefore) {
		return SubmissionResult{}, ErrChallengeNotReady
	}
	if !now.Before(challenge.expiresAt) {
		return SubmissionResult{}, ErrChallengeInvalid
	}

	var planKey, subscriptionStatus, providerStatus string
	var trialEndsAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT plan, subscription_status, trial_ends_at, COALESCE(billing_provider_status, '')
		FROM organizations WHERE id = $1 FOR UPDATE
	`, organizationID).Scan(&planKey, &subscriptionStatus, &trialEndsAt, &providerStatus); err != nil {
		return SubmissionResult{}, fmt.Errorf("load lead capture subscription policy: %w", err)
	}
	if s.enforceHostedBilling {
		if err := modulebilling.CheckWritable(subscriptionStatus, trialEndsAt, providerStatus); err != nil {
			return SubmissionResult{}, err
		}
		// Compatibility for isolated hosted-policy tests that do not inject the
		// shared capacity manager. Production uses the durable reservation above.
		if s.capacity == nil {
			var activeContacts int
			if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE organization_id=$1 AND archived_at IS NULL`, organizationID).Scan(&activeContacts); err != nil {
				return SubmissionResult{}, fmt.Errorf("load lead capture contact usage: %w", err)
			}
			if !modulebilling.CanCreateMore(modulebilling.LimitUsage{Used: activeContacts, Limit: modulebilling.PlanByKey(planKey).ContactLimit}) {
				return SubmissionResult{}, modulebilling.ErrLimitReached
			}
		}
	}

	var contactID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO contacts (organization_id, first_name, last_name, email, phone, address_line1, address_line2, city, state, postal_code, country, job_title, status, is_client, owner_user_id, lead_source, first_source_url, utm_source, utm_medium, utm_campaign, utm_term, utm_content, custom_fields)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), 'lead', FALSE, NULL, $13, $14, $15, $16, $17, $18, $19, $20::jsonb)
		RETURNING id
	`, organizationID, prepared.Contact.FirstName, prepared.Contact.LastName, prepared.Contact.Email, prepared.Contact.Phone, prepared.Contact.AddressLine1, prepared.Contact.AddressLine2, prepared.Contact.City, prepared.Contact.State, prepared.Contact.PostalCode, prepared.Contact.Country, prepared.Contact.JobTitle, attribution.LeadSource, sourceURL, attribution.UTMSource, attribution.UTMMedium, attribution.UTMCampaign, attribution.UTMTerm, attribution.UTMContent, string(prepared.CustomFieldsJSON)).Scan(&contactID); err != nil {
		return SubmissionResult{}, mapSubmissionSaveError(err)
	}

	metadataJSON, err := json.Marshal(map[string]any{
		"formId":           form.ID,
		"formName":         form.Name,
		"formPublicId":     form.PublicID,
		"formRevision":     form.Revision,
		"customFieldCount": countCustomFieldMappings(form.Fields),
		"sourceUrl":        sourceURL,
		"attribution":      attribution,
		"consentRecorded":  true,
	})
	if err != nil {
		return SubmissionResult{}, fmt.Errorf("encode lead capture activity metadata: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary, metadata_json)
		VALUES ($1, 'contact', $2, NULL, 'lead_form.submitted', $3, $4::jsonb)
	`, organizationID, contactID, "Submitted lead form: "+form.Name, string(metadataJSON)); err != nil {
		return SubmissionResult{}, fmt.Errorf("insert lead capture activity: %w", err)
	}

	payloadJSON, err := json.Marshal(prepared.Payload)
	if err != nil {
		return SubmissionResult{}, fmt.Errorf("encode lead capture payload: %w", err)
	}
	var submission Submission
	if err := tx.QueryRow(ctx, `
		INSERT INTO lead_capture_submissions (
			organization_id, form_id, contact_id, payload_json, source_url,
			remote_addr, user_agent, lead_source, utm_source, utm_medium,
			utm_campaign, utm_term, utm_content, consent_text_snapshot, consented_at,
			form_revision, field_mapping_snapshot_json
		)
		VALUES ($1, $2, $3, $4::jsonb, $5, '', '', $6, $7, $8, $9, $10, $11, $12, $13, $14, $15::jsonb)
		RETURNING id, form_id, COALESCE(contact_id, 0), created_at
	`, organizationID, form.ID, contactID, string(payloadJSON), sourceURL, attribution.LeadSource, attribution.UTMSource, attribution.UTMMedium, attribution.UTMCampaign, attribution.UTMTerm, attribution.UTMContent, challenge.consentText, now, form.Revision, string(prepared.MappingSnapshotJSON)).Scan(&submission.ID, &submission.FormID, &submission.ContactID, &submission.CreatedAt); err != nil {
		return SubmissionResult{}, fmt.Errorf("insert lead capture submission: %w", err)
	}
	if err := moduleworkflowautomations.CaptureLeadFormSubmitted(ctx, tx, moduleworkflowautomations.LeadFormSubmittedEvent{
		OrganizationID: organizationID,
		FormID:         form.ID,
		FormPublicID:   form.PublicID,
		SubmissionID:   submission.ID,
		ContactID:      contactID,
	}); err != nil {
		return SubmissionResult{}, fmt.Errorf("capture lead follow-up workflows: %w", err)
	}
	command, err := tx.Exec(ctx, `
		UPDATE lead_capture_submission_challenges
		SET request_digest = $4, submission_id = $5, consumed_at = $6
		WHERE organization_id = $1 AND form_id = $2 AND token_digest = $3 AND consumed_at IS NULL
	`, organizationID, form.ID, submissionChallengeDigest(input.ChallengeToken), requestDigest, submission.ID, now)
	if err != nil {
		return SubmissionResult{}, fmt.Errorf("consume lead submission challenge: %w", err)
	}
	if command.RowsAffected() != 1 {
		return SubmissionResult{}, ErrChallengeInvalid
	}
	if err := modulebilling.ConsumeCapacity(ctx, s.capacity, tx, reservation); err != nil {
		return SubmissionResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return SubmissionResult{}, fmt.Errorf("commit lead capture submission transaction: %w", err)
	}

	return SubmissionResult{Submission: submission, SuccessMessage: form.SuccessMessage}, nil
}

func (s *Service) getActiveByPublicID(ctx context.Context, publicID string) (Form, int64, error) {
	form, organizationID, err := scanFormWithOrganization(s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, slug, public_id, title, description, fields_json, success_message, source_label, consent_text, is_active, COALESCE(revision, 1), 0, created_at, updated_at
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

func (s *Service) getActiveByPublicIDTx(ctx context.Context, tx pgx.Tx, publicID string) (Form, int64, error) {
	form, organizationID, err := scanFormWithOrganization(tx.QueryRow(ctx, `
		SELECT id, organization_id, name, slug, public_id, title, description, fields_json, success_message, source_label, consent_text, is_active, COALESCE(revision, 1), 0, created_at, updated_at
		FROM lead_capture_forms
		WHERE public_id = $1 AND is_active = TRUE
		FOR SHARE
	`, publicID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Form{}, 0, ErrNotFound
		}
		return Form{}, 0, fmt.Errorf("lock public lead capture form: %w", err)
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
	normalizedValues, err := normalizeValues(values)
	if err != nil {
		return contactInput{}, nil, err
	}
	configured := make(map[string]Field, len(form.Fields))
	for _, field := range form.Fields {
		configured[field.Key] = field
	}
	for key := range normalizedValues {
		if _, ok := configured[key]; !ok {
			return contactInput{}, nil, ErrInvalidSubmission
		}
	}
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
		if !validSubmissionFieldValue(field, value) {
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
		&form.ConsentText,
		&form.IsActive,
		&form.Revision,
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
		&form.ConsentText,
		&form.IsActive,
		&form.Revision,
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
		&page.Revision,
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
		&page.Revision,
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
		&form.ConsentText,
		&form.IsActive,
		&form.Revision,
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

func scanChatWidget(scanner formScanner) (ChatWidget, error) {
	var widget ChatWidget
	if err := scanner.Scan(
		&widget.ID,
		&widget.Name,
		&widget.PublicID,
		&widget.Title,
		&widget.WelcomeMessage,
		&widget.PromptLabel,
		&widget.CTALabel,
		&widget.Theme,
		&widget.Position,
		&widget.LeadCaptureFormID,
		&widget.LeadCaptureFormName,
		&widget.LeadCaptureFormPublicID,
		&widget.IsActive,
		&widget.Revision,
		&widget.CreatedAt,
		&widget.UpdatedAt,
	); err != nil {
		return ChatWidget{}, fmt.Errorf("scan lead chat widget: %w", err)
	}
	return widget, nil
}

func scanPublicChatWidget(scanner formScanner) (ChatWidget, Form, error) {
	var widget ChatWidget
	var form Form
	var fieldsJSON []byte
	if err := scanner.Scan(
		&widget.ID,
		&widget.Name,
		&widget.PublicID,
		&widget.Title,
		&widget.WelcomeMessage,
		&widget.PromptLabel,
		&widget.CTALabel,
		&widget.Theme,
		&widget.Position,
		&widget.LeadCaptureFormID,
		&widget.LeadCaptureFormName,
		&widget.LeadCaptureFormPublicID,
		&widget.IsActive,
		&widget.Revision,
		&widget.CreatedAt,
		&widget.UpdatedAt,
		&form.ID,
		&form.Name,
		&form.Slug,
		&form.PublicID,
		&form.Title,
		&form.Description,
		&fieldsJSON,
		&form.SuccessMessage,
		&form.SourceLabel,
		&form.ConsentText,
		&form.IsActive,
		&form.Revision,
		&form.SubmissionCount,
		&form.CreatedAt,
		&form.UpdatedAt,
	); err != nil {
		return ChatWidget{}, Form{}, err
	}
	if err := decodeFieldsJSON(fieldsJSON, &form); err != nil {
		return ChatWidget{}, Form{}, err
	}
	return widget, form, nil
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
	input.ConsentText = strings.TrimSpace(input.ConsentText)
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
	if input.ConsentText == "" {
		input.ConsentText = "I agree to be contacted about this request."
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
	options := make([]string, 0, len(field.Options))
	seenOptions := make(map[string]bool, len(field.Options))
	for _, raw := range field.Options {
		option := strings.TrimSpace(raw)
		key := strings.ToLower(option)
		if option == "" || seenOptions[key] {
			continue
		}
		seenOptions[key] = true
		options = append(options, option)
	}
	field.Options = options
	if field.FieldType == "" {
		field.FieldType = "text"
	}
	return field
}

func validateInput(input Input) error {
	if input.Name == "" || input.Slug == "" || input.Title == "" || input.SuccessMessage == "" || input.SourceLabel == "" || input.ConsentText == "" || len(input.ConsentText) > 1000 {
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
		if len(field.Label) > 200 || !isAllowedFieldType(field.FieldType) || !isAllowedMapping(field.MapTo) {
			return ErrInvalidInput
		}
		if field.FieldType == "select" {
			if len(field.Options) == 0 || len(field.Options) > 25 {
				return ErrInvalidInput
			}
			for _, option := range field.Options {
				if len(option) > 100 {
					return ErrInvalidInput
				}
			}
		} else if len(field.Options) > 0 {
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
	for _, field := range input.Fields {
		if (field.MapTo == "firstName" || field.MapTo == "lastName") && !field.Required {
			return ErrInvalidInput
		}
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
	if input.Name == "" || len(input.Name) > 100 ||
		input.Slug == "" || len(input.Slug) > 80 ||
		input.Title == "" || len(input.Title) > 200 ||
		len(input.Subtitle) > 500 || len(input.Body) > 10000 ||
		input.CTALabel == "" || len(input.CTALabel) > 100 || input.LeadCaptureFormID <= 0 {
		return ErrInvalidPage
	}
	if !isAllowedLandingPageTheme(input.Theme) {
		return ErrInvalidPage
	}
	return nil
}

func normalizeChatWidgetInput(input ChatWidgetInput) ChatWidgetInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Title = strings.TrimSpace(input.Title)
	input.WelcomeMessage = strings.TrimSpace(input.WelcomeMessage)
	input.PromptLabel = strings.TrimSpace(input.PromptLabel)
	input.CTALabel = strings.TrimSpace(input.CTALabel)
	input.Theme = strings.TrimSpace(strings.ToLower(input.Theme))
	input.Position = strings.TrimSpace(strings.ToLower(input.Position))
	if input.Title == "" {
		input.Title = input.Name
	}
	if input.WelcomeMessage == "" {
		input.WelcomeMessage = "Hi. Tell us a little about what you need and we will follow up."
	}
	if input.PromptLabel == "" {
		input.PromptLabel = "Chat with us"
	}
	if input.CTALabel == "" {
		input.CTALabel = "Send"
	}
	if input.Theme == "" {
		input.Theme = "light"
	}
	if input.Position == "" {
		input.Position = "bottom-right"
	}
	return input
}

func validateChatWidgetInput(input ChatWidgetInput) error {
	if input.Name == "" || len(input.Name) > 100 ||
		input.Title == "" || len(input.Title) > 200 || len(input.WelcomeMessage) > 2000 ||
		input.PromptLabel == "" || len(input.PromptLabel) > 100 ||
		input.CTALabel == "" || len(input.CTALabel) > 100 || input.LeadCaptureFormID <= 0 {
		return ErrInvalidWidget
	}
	if !isAllowedLandingPageTheme(input.Theme) || !isAllowedWidgetPosition(input.Position) {
		return ErrInvalidWidget
	}
	return nil
}

func isAllowedWidgetPosition(position string) bool {
	switch position {
	case "bottom-right", "bottom-left", "inline":
		return true
	default:
		return false
	}
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
	case "text", "email", "tel", "textarea", "hidden", "number", "date", "checkbox", "boolean", "select":
		return true
	default:
		return false
	}
}

func isAllowedMapping(mapping string) bool {
	if strings.HasPrefix(mapping, customFieldMappingPrefix) {
		key := strings.TrimPrefix(mapping, customFieldMappingPrefix)
		return customFieldMappingKeyPattern.MatchString(key)
	}
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

func normalizeValues(values map[string]string) (map[string]string, error) {
	if len(values) > 25 {
		return nil, ErrInvalidSubmission
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || len(key) > 100 {
			return nil, ErrInvalidSubmission
		}
		if _, exists := normalized[key]; exists {
			return nil, ErrInvalidSubmission
		}
		value = strings.TrimSpace(value)
		if len(value) > 4000 {
			return nil, ErrInvalidSubmission
		}
		normalized[key] = value
	}
	return normalized, nil
}

func validSubmissionFieldValue(field Field, value string) bool {
	if value == "" {
		return true
	}
	if field.FieldType == "textarea" {
		return len(value) <= 4000
	}
	if len(value) > 500 {
		return false
	}
	switch field.FieldType {
	case "email":
		if len(value) > 320 {
			return false
		}
		parsed, err := mail.ParseAddress(value)
		return err == nil && strings.EqualFold(parsed.Address, value)
	case "number":
		number, err := strconv.ParseFloat(value, 64)
		return err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
	case "date":
		parsed, err := time.Parse("2006-01-02", value)
		return err == nil && parsed.Format("2006-01-02") == value
	case "checkbox", "boolean":
		return value == "true" || value == "false"
	case "select":
		for _, option := range field.Options {
			if value == option {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func normalizeAttribution(form Form, input SubmissionInput, sourceURL string) Attribution {
	attribution := attributionFromSourceURL(sourceURL)
	attribution = mergeAttribution(attribution, input.Attribution)
	attribution.LeadSource = trimMax(attribution.LeadSource, 255)
	if attribution.LeadSource == "" {
		attribution.LeadSource = trimMax(form.SourceLabel, 255)
	}
	attribution.UTMSource = trimMax(attribution.UTMSource, 255)
	attribution.UTMMedium = trimMax(attribution.UTMMedium, 255)
	attribution.UTMCampaign = trimMax(attribution.UTMCampaign, 255)
	attribution.UTMTerm = trimMax(attribution.UTMTerm, 255)
	attribution.UTMContent = trimMax(attribution.UTMContent, 255)
	return attribution
}

func attributionFromSourceURL(sourceURL string) Attribution {
	parsed, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil {
		return Attribution{}
	}
	values := parsed.Query()
	return Attribution{
		LeadSource:  firstQueryValue(values, "lead_source", "leadSource"),
		UTMSource:   firstQueryValue(values, "utm_source", "utmSource"),
		UTMMedium:   firstQueryValue(values, "utm_medium", "utmMedium"),
		UTMCampaign: firstQueryValue(values, "utm_campaign", "utmCampaign"),
		UTMTerm:     firstQueryValue(values, "utm_term", "utmTerm"),
		UTMContent:  firstQueryValue(values, "utm_content", "utmContent"),
	}
}

func firstQueryValue(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func mergeAttribution(base, override Attribution) Attribution {
	if value := strings.TrimSpace(override.LeadSource); value != "" {
		base.LeadSource = value
	}
	if value := strings.TrimSpace(override.UTMSource); value != "" {
		base.UTMSource = value
	}
	if value := strings.TrimSpace(override.UTMMedium); value != "" {
		base.UTMMedium = value
	}
	if value := strings.TrimSpace(override.UTMCampaign); value != "" {
		base.UTMCampaign = value
	}
	if value := strings.TrimSpace(override.UTMTerm); value != "" {
		base.UTMTerm = value
	}
	if value := strings.TrimSpace(override.UTMContent); value != "" {
		base.UTMContent = value
	}
	return base
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

func newChatWidgetPublicID() (string, error) {
	return newPrefixedPublicID("cw")
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

func mapChatWidgetSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return ErrNotFound
		case "23514", "22P02":
			return ErrInvalidWidget
		}
	}
	return fmt.Errorf("save lead chat widget: %w", err)
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
