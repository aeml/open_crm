// Package touchpoints derives explainable customer follow-up history from
// existing CRM work without treating ordinary record edits as contact.
package touchpoints

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
	ErrInvalidInput = errors.New("invalid touchpoint query")
	ErrNotFound     = errors.New("touchpoint record not found")
)

const (
	defaultStaleDays = 30
	defaultLimit     = 25
	maximumLimit     = 100
)

var Semantics = []string{
	"A touch is a note, a completed-task event, a successful logged/completed call, a sent/received SMS, a scheduled meeting, or a sent/received email visible to the viewer.",
	"Record creation, edits, failed communication, reminders, and future task due dates are not touches.",
	"A record with no touch becomes stale only after its creation time crosses the selected threshold.",
	"Client touch history includes direct client work and work on currently linked people; each entry identifies its source record.",
}

type Query struct {
	EntityType  string
	StaleDays   int
	OwnerUserID int64
	Limit       int
}

type Touchpoint struct {
	SourceType       string    `json:"sourceType"`
	SourceID         int64     `json:"sourceId"`
	Action           string    `json:"action"`
	Summary          string    `json:"summary"`
	OccurredAt       time.Time `json:"occurredAt"`
	RecordEntityType string    `json:"recordEntityType"`
	RecordEntityID   int64     `json:"recordEntityId"`
	RecordLabel      string    `json:"recordLabel"`
}

type Record struct {
	EntityType         string      `json:"entityType"`
	EntityID           int64       `json:"entityId"`
	Label              string      `json:"label"`
	OwnerUserID        int64       `json:"ownerUserId"`
	OwnerUserName      string      `json:"ownerUserName"`
	CreatedAt          time.Time   `json:"createdAt"`
	ReferenceAt        time.Time   `json:"referenceAt"`
	DaysSinceReference int         `json:"daysSinceReference"`
	LastTouch          *Touchpoint `json:"lastTouch,omitempty"`
}

type Report struct {
	EntityType  string    `json:"entityType"`
	StaleDays   int       `json:"staleDays"`
	OwnerUserID int64     `json:"ownerUserId"`
	CutoffAt    time.Time `json:"cutoffAt"`
	GeneratedAt time.Time `json:"generatedAt"`
	Count       int       `json:"count"`
	Records     []Record  `json:"records"`
	Semantics   []string  `json:"semantics"`
}

type Summary struct {
	EntityType         string       `json:"entityType"`
	EntityID           int64        `json:"entityId"`
	Label              string       `json:"label"`
	CreatedAt          time.Time    `json:"createdAt"`
	StaleDays          int          `json:"staleDays"`
	CutoffAt           time.Time    `json:"cutoffAt"`
	ReferenceAt        time.Time    `json:"referenceAt"`
	DaysSinceReference int          `json:"daysSinceReference"`
	IsStale            bool         `json:"isStale"`
	LastTouch          *Touchpoint  `json:"lastTouch,omitempty"`
	Recent             []Touchpoint `json:"recent"`
	OpenTaskCount      int          `json:"openTaskCount"`
	OverdueTaskCount   int          `json:"overdueTaskCount"`
	DueSoonTaskCount   int          `json:"dueSoonTaskCount"`
	HealthStatus       string       `json:"healthStatus"`
	HealthLabel        string       `json:"healthLabel"`
	HealthReasons      []string     `json:"healthReasons"`
	Semantics          []string     `json:"semantics"`
	HealthSemantics    []string     `json:"healthSemantics"`
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Stale(ctx context.Context, organizationID, viewerUserID int64, query Query) (Report, error) {
	if s == nil || s.pool == nil {
		return Report{}, fmt.Errorf("touchpoints service not configured")
	}
	query, err := normalizeQuery(organizationID, viewerUserID, query)
	if err != nil {
		return Report{}, err
	}
	if query.OwnerUserID > 0 {
		if err := s.ensureOwner(ctx, organizationID, query.OwnerUserID); err != nil {
			return Report{}, err
		}
	}

	generatedAt := time.Now().UTC()
	cutoffAt := generatedAt.AddDate(0, 0, -query.StaleDays)
	rows, err := s.pool.Query(ctx, staleSQL(query.EntityType), organizationID, viewerUserID, cutoffAt, query.OwnerUserID, query.Limit)
	if err != nil {
		return Report{}, fmt.Errorf("list stale %s touchpoints: %w", query.EntityType, err)
	}
	defer rows.Close()

	report := Report{
		EntityType:  query.EntityType,
		StaleDays:   query.StaleDays,
		OwnerUserID: query.OwnerUserID,
		CutoffAt:    cutoffAt,
		GeneratedAt: generatedAt,
		Records:     []Record{},
		Semantics:   append([]string(nil), Semantics...),
	}
	for rows.Next() {
		var record Record
		var hasTouch bool
		var touch Touchpoint
		if err := rows.Scan(
			&record.EntityID, &record.Label, &record.OwnerUserID, &record.OwnerUserName,
			&record.CreatedAt, &record.ReferenceAt, &record.DaysSinceReference,
			&hasTouch, &touch.SourceType, &touch.SourceID, &touch.Action, &touch.Summary,
			&touch.OccurredAt, &touch.RecordEntityType, &touch.RecordEntityID, &touch.RecordLabel,
			&report.Count,
		); err != nil {
			return Report{}, fmt.Errorf("scan stale touchpoint: %w", err)
		}
		record.EntityType = query.EntityType
		if hasTouch {
			record.LastTouch = &touch
		}
		report.Records = append(report.Records, record)
	}
	if err := rows.Err(); err != nil {
		return Report{}, fmt.Errorf("iterate stale touchpoints: %w", err)
	}
	return report, nil
}

func (s *Service) Summary(ctx context.Context, organizationID, viewerUserID int64, entityType string, entityID int64, staleDays int) (Summary, error) {
	if s == nil || s.pool == nil {
		return Summary{}, fmt.Errorf("touchpoints service not configured")
	}
	query, err := normalizeQuery(organizationID, viewerUserID, Query{EntityType: entityType, StaleDays: staleDays, Limit: 10})
	if err != nil || entityID <= 0 {
		return Summary{}, ErrInvalidInput
	}
	label, createdAt, err := s.loadRecord(ctx, organizationID, query.EntityType, entityID)
	if err != nil {
		return Summary{}, err
	}
	recent, err := s.listRecent(ctx, organizationID, viewerUserID, query.EntityType, entityID, query.Limit)
	if err != nil {
		return Summary{}, err
	}
	generatedAt := time.Now().UTC()
	cutoffAt := generatedAt.AddDate(0, 0, -query.StaleDays)
	referenceAt := createdAt
	var lastTouch *Touchpoint
	if len(recent) > 0 {
		lastTouch = &recent[0]
		referenceAt = recent[0].OccurredAt
	}
	signals, err := s.loadTaskSignals(ctx, organizationID, entityID, query.EntityType)
	if err != nil {
		return Summary{}, err
	}
	daysSinceReference := max(0, int(generatedAt.Sub(referenceAt).Hours()/24))
	isStale := referenceAt.Before(cutoffAt)
	healthStatus, healthLabel, healthReasons := classifyHealth(isStale, daysSinceReference, signals)
	return Summary{
		EntityType: query.EntityType, EntityID: entityID, Label: label, CreatedAt: createdAt,
		StaleDays: query.StaleDays, CutoffAt: cutoffAt, ReferenceAt: referenceAt, DaysSinceReference: daysSinceReference,
		IsStale: isStale, LastTouch: lastTouch, Recent: recent,
		OpenTaskCount: signals.Open, OverdueTaskCount: signals.Overdue, DueSoonTaskCount: signals.DueSoon,
		HealthStatus: healthStatus, HealthLabel: healthLabel, HealthReasons: healthReasons,
		Semantics: append([]string(nil), Semantics...), HealthSemantics: append([]string(nil), HealthSemantics...),
	}, nil
}

func (s *Service) ensureOwner(ctx context.Context, organizationID, ownerUserID int64) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id=$1 AND user_id=$2)`, organizationID, ownerUserID).Scan(&exists); err != nil {
		return fmt.Errorf("validate touchpoint owner: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: ownerUserId is not a workspace member", ErrInvalidInput)
	}
	return nil
}

func (s *Service) loadRecord(ctx context.Context, organizationID int64, entityType string, entityID int64) (string, time.Time, error) {
	var label string
	var createdAt time.Time
	var sql string
	if entityType == "contact" {
		sql = `SELECT COALESCE(NULLIF(trim(first_name||' '||last_name),''),'Contact #'||id::text),created_at FROM contacts WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL`
	} else {
		sql = `SELECT name,created_at FROM companies WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL`
	}
	if err := s.pool.QueryRow(ctx, sql, organizationID, entityID).Scan(&label, &createdAt); errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, ErrNotFound
	} else if err != nil {
		return "", time.Time{}, fmt.Errorf("load touchpoint record: %w", err)
	}
	return label, createdAt, nil
}

func (s *Service) listRecent(ctx context.Context, organizationID, viewerUserID int64, entityType string, entityID int64, limit int) ([]Touchpoint, error) {
	rows, err := s.pool.Query(ctx, recentSQL(entityType), organizationID, viewerUserID, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent %s touchpoints: %w", entityType, err)
	}
	defer rows.Close()
	recent := make([]Touchpoint, 0)
	for rows.Next() {
		var touch Touchpoint
		if err := rows.Scan(&touch.SourceType, &touch.SourceID, &touch.Action, &touch.Summary, &touch.OccurredAt, &touch.RecordEntityType, &touch.RecordEntityID, &touch.RecordLabel); err != nil {
			return nil, fmt.Errorf("scan recent touchpoint: %w", err)
		}
		recent = append(recent, touch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent touchpoints: %w", err)
	}
	return recent, nil
}

func normalizeQuery(organizationID, viewerUserID int64, query Query) (Query, error) {
	query.EntityType = strings.TrimSpace(strings.ToLower(query.EntityType))
	if organizationID <= 0 || viewerUserID <= 0 || (query.EntityType != "contact" && query.EntityType != "company") {
		return Query{}, ErrInvalidInput
	}
	if query.StaleDays == 0 {
		query.StaleDays = defaultStaleDays
	}
	if query.StaleDays < 7 || query.StaleDays > 365 {
		return Query{}, fmt.Errorf("%w: staleDays must be between 7 and 365", ErrInvalidInput)
	}
	if query.OwnerUserID < 0 {
		return Query{}, ErrInvalidInput
	}
	if query.Limit == 0 {
		query.Limit = defaultLimit
	}
	if query.Limit < 1 || query.Limit > maximumLimit {
		return Query{}, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidInput, maximumLimit)
	}
	return query, nil
}

func staleSQL(entityType string) string {
	return touchpointCTE(entityType, false) + `
	, latest AS (
		SELECT target_id,source_type,source_id,action,summary,occurred_at,source_entity_type,source_entity_id,source_entity_label,
		       ROW_NUMBER() OVER (PARTITION BY target_id ORDER BY occurred_at DESC,source_type,source_id DESC) AS row_number
		FROM touches
	), records AS (` + recordListSQL(entityType) + `)
	SELECT r.id,r.label,r.owner_user_id,r.owner_user_name,r.created_at,
	       COALESCE(l.occurred_at,r.created_at) reference_at,
	       GREATEST(0,FLOOR(EXTRACT(EPOCH FROM (NOW()-COALESCE(l.occurred_at,r.created_at)))/86400))::int days_since_reference,
	       (l.occurred_at IS NOT NULL) has_touch,
	       COALESCE(l.source_type,''),COALESCE(l.source_id,0),COALESCE(l.action,''),COALESCE(l.summary,''),
	       COALESCE(l.occurred_at,r.created_at),COALESCE(l.source_entity_type,''),COALESCE(l.source_entity_id,0),COALESCE(l.source_entity_label,''),
	       COUNT(*) OVER()
	FROM records r
	LEFT JOIN latest l ON l.target_id=r.id AND l.row_number=1
	WHERE COALESCE(l.occurred_at,r.created_at) < $3
	  AND ($4::bigint=0 OR r.owner_user_id=$4)
	ORDER BY COALESCE(l.occurred_at,r.created_at),r.label,r.id
	LIMIT $5`
}

func recentSQL(entityType string) string {
	return touchpointCTE(entityType, true) + `
	SELECT source_type,source_id,action,summary,occurred_at,source_entity_type,source_entity_id,source_entity_label
	FROM touches
	ORDER BY occurred_at DESC,source_type,source_id DESC
	LIMIT $4`
}

func touchpointCTE(entityType string, restricted bool) string {
	restriction := ""
	if restricted {
		restriction = " AND target.id=$3"
	}
	return `WITH record_entities AS (` + recordEntitiesSQL(entityType, restriction) + `
	), all_touches AS (
		SELECT re.target_id,'note'::text source_type,n.id source_id,
		       CASE WHEN n.body ILIKE 'Sent email:%' THEN 'email.sent' ELSE 'note.created' END action,
		       CASE WHEN n.body ILIKE 'Sent email:%' THEN 'Email sent' ELSE 'Note added' END summary,
		       n.created_at occurred_at,re.source_entity_type,re.source_entity_id,re.source_entity_label,re.direct_rank
		FROM record_entities re
		JOIN notes n ON n.organization_id=$1 AND n.entity_type=re.source_entity_type AND n.entity_id=re.source_entity_id
		WHERE NOT (n.body ILIKE 'Sent email:%' AND EXISTS (
			SELECT 1 FROM email_message_entity_links duplicate_link
			JOIN email_messages duplicate_email ON duplicate_email.organization_id=$1 AND duplicate_email.id=duplicate_link.email_message_id
			WHERE duplicate_link.organization_id=$1 AND duplicate_link.entity_type=re.source_entity_type AND duplicate_link.entity_id=re.source_entity_id
			  AND duplicate_email.direction='outbound' AND duplicate_email.status='sent'
			  AND duplicate_email.sent_by_user_id=n.created_by_user_id
			  AND ABS(EXTRACT(EPOCH FROM (duplicate_email.created_at-n.created_at))) < 30
		))
		UNION ALL
		SELECT re.target_id,'task',t.id,'task.completed','Completed task: '||t.title,a.created_at,
		       re.source_entity_type,re.source_entity_id,re.source_entity_label,re.direct_rank
		FROM record_entities re
		JOIN tasks t ON t.organization_id=$1 AND t.entity_type=re.source_entity_type AND t.entity_id=re.source_entity_id
		JOIN activities a ON a.organization_id=$1 AND a.entity_type='task' AND a.entity_id=t.id AND a.action='task.completed'
		UNION ALL
		SELECT re.target_id,'call',call.id,'call.completed',
		       CASE WHEN call.disposition<>'' THEN 'Call: '||call.disposition ELSE 'Call completed' END,
		       COALESCE(call.completed_at,call.created_at),re.source_entity_type,re.source_entity_id,re.source_entity_label,re.direct_rank
		FROM record_entities re
		JOIN call_logs call ON call.organization_id=$1 AND call.entity_type=re.source_entity_type AND call.entity_id=re.source_entity_id
		WHERE call.status='completed'
		UNION ALL
		SELECT re.target_id,'sms',sms.id,
		       CASE WHEN sms.direction='inbound' THEN 'sms.received' ELSE 'sms.sent' END,
		       CASE WHEN sms.direction='inbound' THEN 'SMS received' ELSE 'SMS sent' END,
		       COALESCE(sms.received_at,sms.sent_at,sms.created_at),
		       re.source_entity_type,re.source_entity_id,re.source_entity_label,re.direct_rank
		FROM record_entities re
		JOIN sms_messages sms ON sms.organization_id=$1 AND sms.entity_type=re.source_entity_type AND sms.entity_id=re.source_entity_id
		WHERE sms.status IN ('sent','received')
		UNION ALL
		SELECT re.target_id,'meeting',meeting.id,'meeting.scheduled','Meeting scheduled: '||meeting.title,
		       meeting.created_at,re.source_entity_type,re.source_entity_id,re.source_entity_label,re.direct_rank
		FROM record_entities re
		JOIN calendar_events meeting ON meeting.organization_id=$1 AND meeting.entity_type=re.source_entity_type AND meeting.entity_id=re.source_entity_id
		WHERE meeting.status='scheduled'
		  AND (meeting.visibility='shared' OR meeting.calendar_user_id=$2 OR meeting.created_by_user_id=$2)
		UNION ALL
		SELECT re.target_id,'email',em.id,
		       CASE WHEN em.direction='inbound' THEN 'email.received' ELSE 'email.sent' END,
		       CASE WHEN em.direction='inbound' THEN 'Email received' ELSE 'Email sent' END,
		       COALESCE(em.received_at,em.created_at),re.source_entity_type,re.source_entity_id,re.source_entity_label,re.direct_rank
		FROM record_entities re
		JOIN email_message_entity_links link ON link.organization_id=$1 AND link.entity_type=re.source_entity_type AND link.entity_id=re.source_entity_id
		JOIN email_messages em ON em.organization_id=$1 AND em.id=link.email_message_id
		WHERE em.status IN ('sent','received')
		  AND (em.visibility='shared' OR em.mailbox_user_id=$2 OR em.sent_by_user_id=$2)
	), touches AS (
		SELECT DISTINCT ON (target_id,source_type,source_id,action)
		target_id,source_type,source_id,action,summary,occurred_at,source_entity_type,source_entity_id,source_entity_label
		FROM all_touches
		ORDER BY target_id,source_type,source_id,action,occurred_at DESC,direct_rank DESC
	)`
}

func recordEntitiesSQL(entityType, restriction string) string {
	if entityType == "contact" {
		return `SELECT target.id target_id,'contact'::text source_entity_type,target.id source_entity_id,
		       COALESCE(NULLIF(trim(target.first_name||' '||target.last_name),''),'Contact #'||target.id::text) source_entity_label,1 direct_rank
		FROM contacts target WHERE target.organization_id=$1 AND target.archived_at IS NULL` + restriction
	}
	return `SELECT target.id target_id,'company'::text source_entity_type,target.id source_entity_id,target.name source_entity_label,1 direct_rank
		FROM companies target WHERE target.organization_id=$1 AND target.archived_at IS NULL` + restriction + `
		UNION ALL
		SELECT target.id,'contact',contact.id,
		       COALESCE(NULLIF(trim(contact.first_name||' '||contact.last_name),''),'Contact #'||contact.id::text),0
		FROM companies target
		JOIN contact_company_links link ON link.organization_id=$1 AND link.company_id=target.id
		JOIN contacts contact ON contact.organization_id=$1 AND contact.id=link.contact_id AND contact.archived_at IS NULL
		WHERE target.organization_id=$1 AND target.archived_at IS NULL` + restriction
}

func recordListSQL(entityType string) string {
	if entityType == "contact" {
		return `SELECT target.id,
		       COALESCE(NULLIF(trim(target.first_name||' '||target.last_name),''),'Contact #'||target.id::text) label,
		       COALESCE(target.owner_user_id,0) owner_user_id,
		       COALESCE(NULLIF(trim(COALESCE(owner.first_name,'')||' '||COALESCE(owner.last_name,'')),''),COALESCE(owner.email,'')) owner_user_name,
		       target.created_at
		FROM contacts target
		LEFT JOIN organization_memberships owner_membership ON owner_membership.organization_id=target.organization_id AND owner_membership.user_id=target.owner_user_id
		LEFT JOIN users owner ON owner.id=owner_membership.user_id
		WHERE target.organization_id=$1 AND target.archived_at IS NULL`
	}
	return `SELECT target.id,target.name label,COALESCE(target.owner_user_id,0) owner_user_id,
		       COALESCE(NULLIF(trim(COALESCE(owner.first_name,'')||' '||COALESCE(owner.last_name,'')),''),COALESCE(owner.email,'')) owner_user_name,
		       target.created_at
		FROM companies target
		LEFT JOIN organization_memberships owner_membership ON owner_membership.organization_id=target.organization_id AND owner_membership.user_id=target.owner_user_id
		LEFT JOIN users owner ON owner.id=owner_membership.user_id
		WHERE target.organization_id=$1 AND target.archived_at IS NULL`
}
