package collaboration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidInput = errors.New("invalid collaboration input")
	ErrNotFound     = errors.New("collaboration record not found")
)

type Follower struct {
	UserID    int64     `json:"userId"`
	UserName  string    `json:"userName"`
	UserEmail string    `json:"userEmail"`
	CreatedAt time.Time `json:"createdAt"`
}

type Followers struct {
	EntityType string     `json:"entityType"`
	EntityID   int64      `json:"entityId"`
	Following  bool       `json:"following"`
	Followers  []Follower `json:"followers"`
}

type DigestQuery struct {
	Scope       string
	Days        int
	ActorUserID int64
}

type DigestActivity struct {
	ID          int64     `json:"id"`
	Action      string    `json:"action"`
	Summary     string    `json:"summary"`
	EntityType  string    `json:"entityType"`
	EntityID    int64     `json:"entityId"`
	EntityLabel string    `json:"entityLabel"`
	ActorUserID int64     `json:"actorUserId"`
	ActorName   string    `json:"actorName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Digest struct {
	Scope           string           `json:"scope"`
	Days            int              `json:"days"`
	From            time.Time        `json:"from"`
	To              time.Time        `json:"to"`
	TotalActivities int              `json:"totalActivities"`
	ActiveRecords   int              `json:"activeRecords"`
	ActivePeople    int              `json:"activePeople"`
	Activities      []DigestActivity `json:"activities"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Followers(ctx context.Context, organizationID, userID int64, entityType string, entityID int64) (Followers, error) {
	if s == nil || s.pool == nil {
		return Followers{}, fmt.Errorf("collaboration service not configured")
	}
	entityType = strings.TrimSpace(entityType)
	if !supportedRecord(entityType) || entityID <= 0 {
		return Followers{}, ErrInvalidInput
	}
	if err := ensureActiveMember(ctx, s.pool, organizationID, userID); err != nil {
		return Followers{}, err
	}
	if err := ensureRecord(ctx, s.pool, organizationID, entityType, entityID); err != nil {
		return Followers{}, err
	}

	result := Followers{EntityType: entityType, EntityID: entityID, Followers: []Follower{}}
	rows, err := s.pool.Query(ctx, `
		SELECT rf.user_id,
		       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.email),
		       u.email,
		       rf.created_at
		FROM record_followers rf
		JOIN users u ON u.id = rf.user_id
		JOIN organization_memberships om
		  ON om.organization_id = rf.organization_id
		 AND om.user_id = rf.user_id
		 AND COALESCE(om.membership_status, 'active') = 'active'
		WHERE rf.organization_id = $1 AND rf.entity_type = $2 AND rf.entity_id = $3
		ORDER BY rf.created_at, rf.id
	`, organizationID, entityType, entityID)
	if err != nil {
		return Followers{}, fmt.Errorf("list record followers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var follower Follower
		if err := rows.Scan(&follower.UserID, &follower.UserName, &follower.UserEmail, &follower.CreatedAt); err != nil {
			return Followers{}, fmt.Errorf("scan record follower: %w", err)
		}
		result.Following = result.Following || follower.UserID == userID
		result.Followers = append(result.Followers, follower)
	}
	if err := rows.Err(); err != nil {
		return Followers{}, fmt.Errorf("iterate record followers: %w", err)
	}
	return result, nil
}

func (s *Service) SetFollowing(ctx context.Context, organizationID, userID int64, entityType string, entityID int64, following bool) (Followers, error) {
	if s == nil || s.pool == nil {
		return Followers{}, fmt.Errorf("collaboration service not configured")
	}
	entityType = strings.TrimSpace(entityType)
	if !supportedRecord(entityType) || entityID <= 0 || userID <= 0 {
		return Followers{}, ErrInvalidInput
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Followers{}, fmt.Errorf("begin follow transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := ensureActiveMember(ctx, tx, organizationID, userID); err != nil {
		return Followers{}, err
	}
	if err := ensureRecord(ctx, tx, organizationID, entityType, entityID); err != nil {
		return Followers{}, err
	}
	if following {
		_, err = tx.Exec(ctx, `
			INSERT INTO record_followers (organization_id, entity_type, entity_id, user_id, created_by_user_id)
			VALUES ($1, $2, $3, $4, $4)
			ON CONFLICT (organization_id, entity_type, entity_id, user_id) DO NOTHING
		`, organizationID, entityType, entityID, userID)
	} else {
		_, err = tx.Exec(ctx, `
			DELETE FROM record_followers
			WHERE organization_id = $1 AND entity_type = $2 AND entity_id = $3 AND user_id = $4
		`, organizationID, entityType, entityID, userID)
	}
	if err != nil {
		return Followers{}, fmt.Errorf("update record following: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Followers{}, fmt.Errorf("commit follow transaction: %w", err)
	}
	return s.Followers(ctx, organizationID, userID, entityType, entityID)
}

func (s *Service) ActivityDigest(ctx context.Context, organizationID, userID int64, query DigestQuery) (Digest, error) {
	if s == nil || s.pool == nil {
		return Digest{}, fmt.Errorf("collaboration service not configured")
	}
	query.Scope = strings.TrimSpace(query.Scope)
	if query.Scope == "" {
		query.Scope = "following"
	}
	if query.Scope != "following" && query.Scope != "team" {
		return Digest{}, ErrInvalidInput
	}
	if query.Days == 0 {
		query.Days = 7
	}
	if query.Days != 1 && query.Days != 7 && query.Days != 30 {
		return Digest{}, ErrInvalidInput
	}
	if err := ensureActiveMember(ctx, s.pool, organizationID, userID); err != nil {
		return Digest{}, err
	}

	now := time.Now().UTC()
	result := Digest{
		Scope:      query.Scope,
		Days:       query.Days,
		From:       now.AddDate(0, 0, -query.Days),
		To:         now,
		Activities: []DigestActivity{},
	}
	if err := s.pool.QueryRow(ctx, digestBaseSQL+`
		SELECT COUNT(*), COUNT(DISTINCT (entity_type, entity_id)), COUNT(DISTINCT actor_user_id)
		FROM filtered
	`, organizationID, userID, query.Scope, query.Days, query.ActorUserID).Scan(
		&result.TotalActivities,
		&result.ActiveRecords,
		&result.ActivePeople,
	); err != nil {
		return Digest{}, fmt.Errorf("summarize activity digest: %w", err)
	}

	rows, err := s.pool.Query(ctx, digestBaseSQL+`
		SELECT id, action, summary, entity_type, entity_id,
		       CASE entity_type
		         WHEN 'contact' THEN COALESCE((SELECT NULLIF(TRIM(c.first_name || ' ' || c.last_name), '') FROM contacts c WHERE c.organization_id = $1 AND c.id = entity_id), 'Contact')
		         WHEN 'company' THEN COALESCE((SELECT c.name FROM companies c WHERE c.organization_id = $1 AND c.id = entity_id), 'Company')
		         WHEN 'deal' THEN COALESCE((SELECT d.name FROM deals d WHERE d.organization_id = $1 AND d.id = entity_id), 'Deal')
		         WHEN 'task' THEN COALESCE((SELECT t.title FROM tasks t WHERE t.organization_id = $1 AND t.id = entity_id), 'Task')
		         ELSE entity_type
		       END,
		       COALESCE(actor_user_id, 0),
		       actor_name,
		       created_at
		FROM filtered
		ORDER BY created_at DESC, id DESC
		LIMIT 50
	`, organizationID, userID, query.Scope, query.Days, query.ActorUserID)
	if err != nil {
		return Digest{}, fmt.Errorf("load activity digest: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var activity DigestActivity
		if err := rows.Scan(
			&activity.ID,
			&activity.Action,
			&activity.Summary,
			&activity.EntityType,
			&activity.EntityID,
			&activity.EntityLabel,
			&activity.ActorUserID,
			&activity.ActorName,
			&activity.CreatedAt,
		); err != nil {
			return Digest{}, fmt.Errorf("scan activity digest: %w", err)
		}
		result.Activities = append(result.Activities, activity)
	}
	if err := rows.Err(); err != nil {
		return Digest{}, fmt.Errorf("iterate activity digest: %w", err)
	}
	return result, nil
}

const digestBaseSQL = `
	WITH filtered AS (
		SELECT a.id, a.action, a.summary, a.entity_type, a.entity_id, a.actor_user_id,
		       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.email, 'System') AS actor_name,
		       a.created_at
		FROM activities a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.organization_id = $1
		  AND a.created_at >= NOW() - ($4 * INTERVAL '1 day')
		  AND ($5::bigint = 0 OR a.actor_user_id = $5)
		  AND ($3 = 'team' OR EXISTS (
			SELECT 1 FROM record_followers rf
			WHERE rf.organization_id = a.organization_id
			  AND rf.entity_type = a.entity_type
			  AND rf.entity_id = a.entity_id
			  AND rf.user_id = $2
		  ))
	)
`

type recordExecutor interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func ensureRecord(ctx context.Context, executor recordExecutor, organizationID int64, entityType string, entityID int64) error {
	var exists bool
	var query string
	switch entityType {
	case "contact":
		query = `SELECT EXISTS(SELECT 1 FROM contacts WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL)`
	case "company":
		query = `SELECT EXISTS(SELECT 1 FROM companies WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL)`
	case "deal":
		query = `SELECT EXISTS(SELECT 1 FROM deals WHERE organization_id = $1 AND id = $2 AND archived_at IS NULL)`
	default:
		return ErrInvalidInput
	}
	if err := executor.QueryRow(ctx, query, organizationID, entityID).Scan(&exists); err != nil {
		return fmt.Errorf("verify collaboration record: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func ensureActiveMember(ctx context.Context, executor recordExecutor, organizationID, userID int64) error {
	var active bool
	if err := executor.QueryRow(ctx, `
		SELECT TRUE FROM organization_memberships
		WHERE organization_id = $1 AND user_id = $2
		  AND COALESCE(membership_status, 'active') = 'active'
		FOR SHARE
	`, organizationID, userID).Scan(&active); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("verify active collaboration member: %w", err)
	}
	if !active {
		return ErrNotFound
	}
	return nil
}

func supportedRecord(entityType string) bool {
	return entityType == "contact" || entityType == "company" || entityType == "deal"
}
