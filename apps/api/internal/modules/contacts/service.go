package contacts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Summary struct {
	ID        int64  `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	JobTitle  string `json:"jobTitle"`
	Status    string `json:"status"`
	IsClient  bool   `json:"isClient"`
}

type ListQuery struct {
	Search   string
	Page     int
	PageSize int
}

type ListMeta struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}

type ListResult struct {
	Contacts []Summary
	Meta     ListMeta
}

type CreateInput struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	JobTitle  string `json:"jobTitle"`
	Status    string `json:"status"`
	IsClient  bool   `json:"isClient"`
}

type UpdateInput struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	JobTitle  string `json:"jobTitle"`
	Status    string `json:"status"`
	IsClient  bool   `json:"isClient"`
}

type NoteEntry struct{}

type TaskEntry struct{}

type ActivityEntry struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}

type Detail struct {
	Summary    Summary
	Notes      []NoteEntry
	Tasks      []TaskEntry
	Activities []ActivityEntry
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (ListResult, error) {
	if s == nil || s.pool == nil {
		return ListResult{}, fmt.Errorf("contacts service not configured")
	}

	query.Search = strings.TrimSpace(query.Search)
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	offset := (query.Page - 1) * query.PageSize

	filter := ""
	args := []any{organizationID}
	if query.Search != "" {
		filter = " AND (first_name ILIKE $2 OR last_name ILIKE $2 OR email ILIKE $2 OR phone ILIKE $2 OR job_title ILIKE $2)"
		args = append(args, "%"+query.Search+"%")
	}

	countSQL := `SELECT COUNT(*) FROM contacts WHERE organization_id = $1 AND archived_at IS NULL` + filter
	var total int
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("count contacts: %w", err)
	}

	args = append(args, query.PageSize, offset)
	limitArg := len(args) - 1
	offsetArg := len(args)
	rows, err := s.pool.Query(ctx, `
		SELECT id, first_name, last_name, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(address, ''), COALESCE(job_title, ''), COALESCE(status, ''), is_client
		FROM contacts
		WHERE organization_id = $1 AND archived_at IS NULL`+filter+`
		ORDER BY last_name ASC, first_name ASC, id ASC
		LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg), args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()

	contacts := make([]Summary, 0)
	for rows.Next() {
		var contact Summary
		if err := rows.Scan(&contact.ID, &contact.FirstName, &contact.LastName, &contact.Email, &contact.Phone, &contact.Address, &contact.JobTitle, &contact.Status, &contact.IsClient); err != nil {
			return ListResult{}, fmt.Errorf("scan contact: %w", err)
		}
		contacts = append(contacts, contact)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate contacts: %w", err)
	}

	return ListResult{
		Contacts: contacts,
		Meta: ListMeta{
			Page:     query.Page,
			PageSize: query.PageSize,
			Total:    total,
		},
	}, nil
}

func (s *Service) GetByID(ctx context.Context, organizationID, contactID int64) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("contacts service not configured")
	}

	detail := Detail{
		Notes:      []NoteEntry{},
		Tasks:      []TaskEntry{},
		Activities: []ActivityEntry{},
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT id, first_name, last_name, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(address, ''), COALESCE(job_title, ''), COALESCE(status, ''), is_client
		FROM contacts
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, contactID).Scan(&detail.Summary.ID, &detail.Summary.FirstName, &detail.Summary.LastName, &detail.Summary.Email, &detail.Summary.Phone, &detail.Summary.Address, &detail.Summary.JobTitle, &detail.Summary.Status, &detail.Summary.IsClient); err != nil {
		return Detail{}, fmt.Errorf("get contact: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, action, summary, created_at
		FROM activities
		WHERE organization_id = $1 AND entity_type = 'contact' AND entity_id = $2
		ORDER BY created_at DESC, id DESC
	`, organizationID, contactID)
	if err != nil {
		return Detail{}, fmt.Errorf("list contact activities: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var activity ActivityEntry
		if err := rows.Scan(&activity.ID, &activity.Action, &activity.Summary, &activity.CreatedAt); err != nil {
			return Detail{}, fmt.Errorf("scan activity: %w", err)
		}
		detail.Activities = append(detail.Activities, activity)
	}
	if err := rows.Err(); err != nil {
		return Detail{}, fmt.Errorf("iterate activities: %w", err)
	}

	return detail, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID int64, input CreateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("contacts service not configured")
	}

	input = normalizeCreateInput(input)
	if input.FirstName == "" || input.LastName == "" {
		return Detail{}, fmt.Errorf("first name and last name are required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin create contact transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var contactID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO contacts (organization_id, first_name, last_name, email, phone, address, job_title, status, is_client, owner_user_id)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), $9, $10)
		RETURNING id
	`, organizationID, input.FirstName, input.LastName, input.Email, input.Phone, input.Address, input.JobTitle, input.Status, input.IsClient, actorUserID).Scan(&contactID); err != nil {
		return Detail{}, fmt.Errorf("insert contact: %w", err)
	}

	if err := insertActivity(ctx, tx, organizationID, contactID, actorUserID, "contact.created", "Contact created"); err != nil {
		return Detail{}, fmt.Errorf("insert create activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit create contact transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, contactID)
}

func (s *Service) Update(ctx context.Context, organizationID, contactID, actorUserID int64, input UpdateInput) (Detail, error) {
	if s == nil || s.pool == nil {
		return Detail{}, fmt.Errorf("contacts service not configured")
	}

	input = normalizeUpdateInput(input)
	if input.FirstName == "" || input.LastName == "" {
		return Detail{}, fmt.Errorf("first name and last name are required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("begin update contact transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE contacts
		SET first_name = $3,
		    last_name = $4,
		    email = NULLIF($5, ''),
		    phone = NULLIF($6, ''),
		    address = NULLIF($7, ''),
		    job_title = NULLIF($8, ''),
		    status = NULLIF($9, ''),
		    is_client = $10,
		    updated_at = NOW()
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, contactID, input.FirstName, input.LastName, input.Email, input.Phone, input.Address, input.JobTitle, input.Status, input.IsClient); err != nil {
		return Detail{}, fmt.Errorf("update contact: %w", err)
	}

	if err := insertActivity(ctx, tx, organizationID, contactID, actorUserID, "contact.updated", "Contact updated"); err != nil {
		return Detail{}, fmt.Errorf("insert update activity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("commit update contact transaction: %w", err)
	}

	return s.GetByID(ctx, organizationID, contactID)
}

func (s *Service) Archive(ctx context.Context, organizationID, contactID, actorUserID int64) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("contacts service not configured")
	}

	_, err := s.pool.Exec(ctx, `
		UPDATE contacts
		SET archived_at = NOW(), updated_at = NOW(), owner_user_id = COALESCE(owner_user_id, $3)
		WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL
	`, organizationID, contactID, actorUserID)
	if err != nil {
		return fmt.Errorf("archive contact: %w", err)
	}
	return nil
}

type activityExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertActivity(ctx context.Context, executor activityExecutor, organizationID, entityID, actorUserID int64, action, summary string) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary)
		VALUES ($1, 'contact', $2, $3, $4, $5)
	`, organizationID, entityID, actorUserID, action, summary)
	return err
}

func normalizeCreateInput(input CreateInput) CreateInput {
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Phone = strings.TrimSpace(input.Phone)
	input.Address = strings.TrimSpace(input.Address)
	input.JobTitle = strings.TrimSpace(input.JobTitle)
	input.Status = strings.TrimSpace(strings.ToLower(input.Status))
	return input
}

func normalizeUpdateInput(input UpdateInput) UpdateInput {
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Phone = strings.TrimSpace(input.Phone)
	input.Address = strings.TrimSpace(input.Address)
	input.JobTitle = strings.TrimSpace(input.JobTitle)
	input.Status = strings.TrimSpace(strings.ToLower(input.Status))
	return input
}
