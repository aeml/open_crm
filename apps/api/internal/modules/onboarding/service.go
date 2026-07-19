package onboarding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/aeml/open_crm/apps/api/internal/db"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	platformauth "github.com/aeml/open_crm/apps/api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	verificationTokenTTL = 24 * time.Hour
	verificationCooldown = time.Minute
	trialLength          = 14 * 24 * time.Hour
)

var (
	ErrInvalidInput             = errors.New("invalid workspace signup input")
	ErrIdempotencyConflict      = errors.New("idempotency key already used for another signup")
	ErrAccountExists            = errors.New("account already exists")
	ErrInvalidVerificationToken = errors.New("invalid email verification token")
	ErrAlreadyVerified          = errors.New("email already verified")
	ErrVerificationDelivery     = errors.New("verification email delivery failed")
)

type BootstrapInput struct {
	OrganizationName string `json:"organizationName"`
	BusinessType     string `json:"businessType"`
	FirstName        string `json:"firstName"`
	LastName         string `json:"lastName"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	IdempotencyKey   string `json:"idempotencyKey"`
}

type BootstrapResult struct {
	Email                string `json:"email"`
	VerificationRequired bool   `json:"verificationRequired"`
	VerificationLink     string `json:"verificationLink,omitempty"`
	Created              bool   `json:"created"`
}

type ResendResult struct {
	VerificationLink string `json:"verificationLink,omitempty"`
}

type VerificationMailer interface {
	ProviderName() string
	VerificationLink(token string) string
	SendEmailVerification(context.Context, string, string, string) error
}

type Service struct {
	pool   *pgxpool.Pool
	mailer VerificationMailer
	now    func() time.Time
}

func NewService(pool *pgxpool.Pool, mailers ...VerificationMailer) *Service {
	service := &Service{pool: pool, now: time.Now}
	if len(mailers) > 0 {
		service.mailer = mailers[0]
	}
	return service
}

func (s *Service) BootstrapOrganization(ctx context.Context, input BootstrapInput) (BootstrapResult, error) {
	if s == nil || s.pool == nil {
		return BootstrapResult{}, fmt.Errorf("onboarding service not configured")
	}
	input = normalizeBootstrapInput(input)
	if err := validateBootstrapInput(input); err != nil {
		return BootstrapResult{}, err
	}
	if _, err := moduleorgprofile.BuildDetailForBusinessType(1, input.BusinessType); err != nil {
		return BootstrapResult{}, ErrInvalidInput
	}

	requestHash, err := bootstrapRequestHash(input)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("hash bootstrap request: %w", err)
	}
	keyHash := hashValue(input.IdempotencyKey)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("begin bootstrap transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "workspace-bootstrap:"+keyHash); err != nil {
		return BootstrapResult{}, fmt.Errorf("lock bootstrap request: %w", err)
	}
	if existing, token, found, err := s.existingBootstrap(ctx, tx, keyHash, requestHash, input.Password); err != nil {
		return BootstrapResult{}, err
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return BootstrapResult{}, fmt.Errorf("commit bootstrap replay: %w", err)
		}
		return s.deliverVerification(ctx, existing, token)
	}

	slugBase := slugify(input.OrganizationName)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "workspace-slug:"+slugBase); err != nil {
		return BootstrapResult{}, fmt.Errorf("lock workspace slug: %w", err)
	}
	slug := slugBase
	var slugExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organizations WHERE slug=$1)`, slug).Scan(&slugExists); err != nil {
		return BootstrapResult{}, fmt.Errorf("check workspace slug: %w", err)
	}
	if slugExists {
		slug = slugBase + "-" + keyHash[:8]
	}

	var organizationID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name, slug, business_type, subscription_status, trial_started_at, trial_ends_at)
		VALUES ($1, $2, $3, 'trialing', NULL, NULL)
		RETURNING id
	`, input.OrganizationName, slug, input.BusinessType).Scan(&organizationID)
	if err != nil {
		return BootstrapResult{}, mapBootstrapInsertError(err)
	}

	passwordHash, err := platformauth.HashPassword(input.Password)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("hash password: %w", err)
	}
	verificationToken, err := platformauth.NewSessionToken()
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("generate verification token: %w", err)
	}
	var userID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO users (
			email, password_hash, first_name, last_name,
			email_verified_at, email_verification_token_hash,
			email_verification_expires_at, email_verification_sent_at
		)
		VALUES ($1, $2, $3, $4, NULL, $5, $6, NULL)
		RETURNING id
	`, input.Email, passwordHash, input.FirstName, input.LastName, hashValue(verificationToken), s.now().Add(verificationTokenTTL)).Scan(&userID)
	if err != nil {
		return BootstrapResult{}, mapBootstrapInsertError(err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`, organizationID, userID); err != nil {
		return BootstrapResult{}, fmt.Errorf("insert owner membership: %w", err)
	}

	var pipelineID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO deal_pipelines (organization_id, name, position, is_default, created_by_user_id, updated_by_user_id)
		VALUES ($1, 'Sales pipeline', 1, TRUE, $2, $2)
		RETURNING id
	`, organizationID, userID).Scan(&pipelineID); err != nil {
		return BootstrapResult{}, fmt.Errorf("insert default pipeline: %w", err)
	}
	for _, stage := range db.DefaultDealStagesForBusinessType(input.BusinessType) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO deal_stages (organization_id, pipeline_id, name, position, is_closed, is_won, probability_percent)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, organizationID, pipelineID, stage.Name, stage.Position, stage.IsClosed, stage.IsWon, stage.ProbabilityPercent); err != nil {
			return BootstrapResult{}, fmt.Errorf("insert default stage %s: %w", stage.Name, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO workspace_bootstrap_requests (idempotency_key_hash, request_sha256, organization_id, user_id)
		VALUES ($1, $2, $3, $4)
	`, keyHash, requestHash, organizationID, userID); err != nil {
		return BootstrapResult{}, fmt.Errorf("persist bootstrap idempotency: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		VALUES ($1, $2, 'workspace.provisioned', 'organization', $1, 'Workspace provisioned pending owner email verification', jsonb_build_object('businessType', $3::text))
	`, organizationID, userID, input.BusinessType); err != nil {
		return BootstrapResult{}, fmt.Errorf("record workspace provisioning audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return BootstrapResult{}, fmt.Errorf("commit bootstrap transaction: %w", err)
	}

	return s.deliverVerification(ctx, BootstrapResult{Email: input.Email, VerificationRequired: true, Created: true}, verificationToken)
}

func (s *Service) existingBootstrap(ctx context.Context, tx pgx.Tx, keyHash, requestHash, password string) (BootstrapResult, string, bool, error) {
	var storedHash, email, passwordHash string
	var userID int64
	var verifiedAt, sentAt *time.Time
	err := tx.QueryRow(ctx, `
		SELECT request.request_sha256, request.user_id, users.email, users.password_hash,
		       users.email_verified_at, users.email_verification_sent_at
		FROM workspace_bootstrap_requests request
		JOIN users ON users.id = request.user_id
		WHERE request.idempotency_key_hash = $1
		FOR UPDATE OF request, users
	`, keyHash).Scan(&storedHash, &userID, &email, &passwordHash, &verifiedAt, &sentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BootstrapResult{}, "", false, nil
	}
	if err != nil {
		return BootstrapResult{}, "", false, fmt.Errorf("load bootstrap replay: %w", err)
	}
	if storedHash != requestHash || !platformauth.CheckPassword(passwordHash, password) {
		return BootstrapResult{}, "", true, ErrIdempotencyConflict
	}
	if verifiedAt != nil {
		return BootstrapResult{}, "", true, ErrAlreadyVerified
	}
	if sentAt != nil && sentAt.After(s.now().Add(-verificationCooldown)) {
		return BootstrapResult{Email: email, VerificationRequired: true, Created: false}, "", true, nil
	}
	token, err := s.issueVerificationToken(ctx, tx, userID)
	if err != nil {
		return BootstrapResult{}, "", true, err
	}
	if _, err := tx.Exec(ctx, `UPDATE workspace_bootstrap_requests SET updated_at=NOW() WHERE idempotency_key_hash=$1`, keyHash); err != nil {
		return BootstrapResult{}, "", true, fmt.Errorf("update bootstrap replay: %w", err)
	}
	return BootstrapResult{Email: email, VerificationRequired: true, Created: false}, token, true, nil
}

func (s *Service) VerifyEmail(ctx context.Context, token string) (moduleauth.LoginResult, error) {
	if s == nil || s.pool == nil {
		return moduleauth.LoginResult{}, fmt.Errorf("onboarding service not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return moduleauth.LoginResult{}, ErrInvalidVerificationToken
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("begin verification transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var state moduleauth.SessionState
	err = tx.QueryRow(ctx, `
		SELECT users.id, users.email, users.first_name, users.last_name,
		       organizations.id, organizations.name, organizations.slug, organizations.business_type,
		       memberships.role
		FROM users
		JOIN organization_memberships memberships ON memberships.user_id=users.id
		JOIN organizations ON organizations.id=memberships.organization_id
		WHERE users.email_verification_token_hash=$1
		  AND users.email_verification_expires_at > NOW()
		  AND users.email_verified_at IS NULL
		  AND COALESCE(memberships.membership_status, 'active')='active'
		ORDER BY memberships.id ASC
		LIMIT 1
		FOR UPDATE OF users, organizations, memberships
	`, hashValue(token)).Scan(
		&state.User.ID, &state.User.Email, &state.User.FirstName, &state.User.LastName,
		&state.Organization.ID, &state.Organization.Name, &state.Organization.Slug, &state.Organization.BusinessType,
		&state.Membership.Role,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return moduleauth.LoginResult{}, ErrInvalidVerificationToken
	}
	if err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("load email verification: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET email_verified_at=NOW(), email_verification_token_hash=NULL,
		    email_verification_expires_at=NULL, email_verification_sent_at=NULL, updated_at=NOW()
		WHERE id=$1
	`, state.User.ID); err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("mark email verified: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE organizations
		SET trial_started_at=COALESCE(trial_started_at, NOW()),
		    trial_ends_at=COALESCE(trial_ends_at, NOW() + ($2::bigint * INTERVAL '1 microsecond')),
		    updated_at=NOW()
		WHERE id=$1 AND subscription_status='trialing'
	`, state.Organization.ID, trialLength.Microseconds()); err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("start verified workspace trial: %w", err)
	}

	sessionToken, err := platformauth.NewSessionToken()
	if err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("generate verified session token: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO sessions (user_id, organization_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES ($1, $2, $3, NOW() + INTERVAL '30 days', NOW(), NOW())
	`, state.User.ID, state.Organization.ID, hashValue(sessionToken)); err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("persist verified session: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary)
		VALUES ($1, $2, 'user.email_verified', 'user', $2, 'Workspace owner email verified and trial started')
	`, state.Organization.ID, state.User.ID); err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("record email verification audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("commit email verification: %w", err)
	}
	return moduleauth.LoginResult{SessionToken: sessionToken, State: state}, nil
}

func (s *Service) ResendVerification(ctx context.Context, email string) (ResendResult, error) {
	if s == nil || s.pool == nil {
		return ResendResult{}, fmt.Errorf("onboarding service not configured")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > 320 {
		return ResendResult{}, ErrInvalidInput
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return ResendResult{}, fmt.Errorf("begin verification resend: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "verification-resend:"+email); err != nil {
		return ResendResult{}, fmt.Errorf("lock verification resend: %w", err)
	}

	var userID int64
	var sentAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT users.id, users.email_verification_sent_at
		FROM users
		WHERE users.email=$1 AND users.email_verified_at IS NULL
		  AND EXISTS (
		    SELECT 1 FROM organization_memberships membership
		    WHERE membership.user_id=users.id AND COALESCE(membership.membership_status, 'active')='active'
		  )
		FOR UPDATE
	`, email).Scan(&userID, &sentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return ResendResult{}, fmt.Errorf("commit empty verification resend: %w", err)
		}
		return ResendResult{}, nil
	}
	if err != nil {
		return ResendResult{}, fmt.Errorf("load verification resend: %w", err)
	}
	if sentAt != nil && sentAt.After(s.now().Add(-verificationCooldown)) {
		if err := tx.Commit(ctx); err != nil {
			return ResendResult{}, fmt.Errorf("commit throttled verification resend: %w", err)
		}
		return ResendResult{}, nil
	}
	token, err := s.issueVerificationToken(ctx, tx, userID)
	if err != nil {
		return ResendResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ResendResult{}, fmt.Errorf("commit verification resend: %w", err)
	}
	result, err := s.deliverVerification(ctx, BootstrapResult{Email: email, VerificationRequired: true}, token)
	if err != nil {
		return ResendResult{}, err
	}
	return ResendResult{VerificationLink: result.VerificationLink}, nil
}

func (s *Service) issueVerificationToken(ctx context.Context, tx pgx.Tx, userID int64) (string, error) {
	token, err := platformauth.NewSessionToken()
	if err != nil {
		return "", fmt.Errorf("generate verification token: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET email_verification_token_hash=$2, email_verification_expires_at=$3,
		    email_verification_sent_at=NULL, updated_at=NOW()
		WHERE id=$1 AND email_verified_at IS NULL
	`, userID, hashValue(token), s.now().Add(verificationTokenTTL)); err != nil {
		return "", fmt.Errorf("persist verification token: %w", err)
	}
	return token, nil
}

func (s *Service) deliverVerification(ctx context.Context, result BootstrapResult, token string) (BootstrapResult, error) {
	if token == "" {
		return result, nil
	}
	if s.mailer == nil {
		return result, ErrVerificationDelivery
	}
	var firstName string
	if err := s.pool.QueryRow(ctx, `SELECT first_name FROM users WHERE email=$1 AND email_verified_at IS NULL`, result.Email).Scan(&firstName); err != nil {
		return result, fmt.Errorf("load verification recipient: %w", err)
	}
	if err := s.mailer.SendEmailVerification(ctx, result.Email, firstName, token); err != nil {
		return result, fmt.Errorf("%w: %v", ErrVerificationDelivery, err)
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE users SET email_verification_sent_at=NOW(), updated_at=NOW()
		WHERE email=$1 AND email_verification_token_hash=$2 AND email_verified_at IS NULL
	`, result.Email, hashValue(token)); err != nil {
		return result, fmt.Errorf("record verification delivery: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(s.mailer.ProviderName()), "fake") {
		result.VerificationLink = s.mailer.VerificationLink(token)
	}
	return result, nil
}

func normalizeBootstrapInput(input BootstrapInput) BootstrapInput {
	input.OrganizationName = strings.TrimSpace(input.OrganizationName)
	input.BusinessType = normalizeBusinessType(input.BusinessType)
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Password = strings.TrimSpace(input.Password)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	return input
}

func validateBootstrapInput(input BootstrapInput) error {
	if input.OrganizationName == "" || len(input.OrganizationName) > 200 ||
		input.FirstName == "" || len(input.FirstName) > 100 ||
		input.LastName == "" || len(input.LastName) > 100 ||
		input.Email == "" || len(input.Email) > 320 || !validEmailAddress(input.Email) ||
		len(input.Password) < 12 || len(input.Password) > 1024 ||
		len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 200 {
		return ErrInvalidInput
	}
	return nil
}

func validEmailAddress(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}

func bootstrapRequestHash(input BootstrapInput) (string, error) {
	payload, err := json.Marshal(struct {
		OrganizationName string `json:"organizationName"`
		BusinessType     string `json:"businessType"`
		FirstName        string `json:"firstName"`
		LastName         string `json:"lastName"`
		Email            string `json:"email"`
	}{input.OrganizationName, input.BusinessType, input.FirstName, input.LastName, input.Email})
	if err != nil {
		return "", err
	}
	return hashValue(string(payload)), nil
}

func mapBootstrapInsertError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		if strings.Contains(postgresError.ConstraintName, "users_email") {
			return ErrAccountExists
		}
		return ErrIdempotencyConflict
	}
	return fmt.Errorf("persist workspace signup: %w", err)
}

func normalizeBusinessType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "general"
	}
	return value
}

func slugify(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "workspace"
	}
	return slug
}

func hashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
