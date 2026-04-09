package onboarding

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/aeml/open_crm/apps/api/internal/db"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	platformauth "github.com/aeml/open_crm/apps/api/internal/platform/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BootstrapInput struct {
	OrganizationName string `json:"organizationName"`
	BusinessType     string `json:"businessType"`
	FirstName        string `json:"firstName"`
	LastName         string `json:"lastName"`
	Email            string `json:"email"`
	Password         string `json:"password"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) BootstrapOrganization(ctx context.Context, input BootstrapInput) (moduleauth.LoginResult, error) {
	if s == nil || s.pool == nil {
		return moduleauth.LoginResult{}, fmt.Errorf("onboarding service not configured")
	}

	input.OrganizationName = strings.TrimSpace(input.OrganizationName)
	input.BusinessType = normalizeBusinessType(input.BusinessType)
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Password = strings.TrimSpace(input.Password)
	if input.OrganizationName == "" || input.FirstName == "" || input.LastName == "" || input.Email == "" || input.Password == "" {
		return moduleauth.LoginResult{}, fmt.Errorf("organization name, owner name, email, and password are required")
	}

	if _, err := moduleorgprofile.BuildDetailForBusinessType(1, input.BusinessType); err != nil {
		return moduleauth.LoginResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("begin bootstrap transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	slug := slugify(input.OrganizationName)
	var organizationID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name, slug, business_type)
		VALUES ($1, $2, $3)
		RETURNING id
	`, input.OrganizationName, slug, input.BusinessType).Scan(&organizationID)
	if err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("insert organization: %w", err)
	}

	passwordHash, err := platformauth.HashPassword(input.Password)
	if err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("hash password: %w", err)
	}

	var userID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, first_name, last_name)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, input.Email, passwordHash, input.FirstName, input.LastName).Scan(&userID)
	if err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("insert owner user: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`, organizationID, userID); err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("insert owner membership: %w", err)
	}

	for _, stage := range db.DefaultDealStagesForBusinessType(input.BusinessType) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO deal_stages (organization_id, name, position, is_closed, is_won)
			VALUES ($1, $2, $3, $4, $5)
		`, organizationID, stage.Name, stage.Position, stage.IsClosed, stage.IsWon); err != nil {
			return moduleauth.LoginResult{}, fmt.Errorf("insert default stage %s: %w", stage.Name, err)
		}
	}

	token, err := platformauth.NewSessionToken()
	if err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("generate session token: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO sessions (user_id, organization_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES ($1, $2, $3, NOW() + INTERVAL '30 days', NOW(), NOW())
	`, userID, organizationID, hashToken(token)); err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("persist session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return moduleauth.LoginResult{}, fmt.Errorf("commit bootstrap transaction: %w", err)
	}

	return moduleauth.LoginResult{
		SessionToken: token,
		State: moduleauth.SessionState{
			User:         moduleauth.User{ID: userID, Email: input.Email, FirstName: input.FirstName, LastName: input.LastName},
			Organization: moduleauth.Organization{ID: organizationID, Name: input.OrganizationName, Slug: slug, BusinessType: input.BusinessType},
			Membership:   moduleauth.Membership{Role: "owner"},
		},
	}, nil
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

func hashToken(token string) string {
	return moduleauth.HashSessionToken(token)
}
