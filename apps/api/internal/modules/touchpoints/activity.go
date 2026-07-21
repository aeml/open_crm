package touchpoints

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	clientActivityDateLayout = "2006-01-02"
	clientActivityTimeout    = 5 * time.Second
)

var ClientActivitySemantics = []string{
	"Rows are active organization customers or individual client contacts at query time; archived and non-client records are excluded.",
	"Company counts combine direct work with work on contacts currently linked to that company, and duplicate source events count once per client.",
	"From and to dates are inclusive UTC calendar days. Each qualifying touch uses its source-specific occurrence time.",
	"The owner filter uses the client's current retained owner, not a reconstructed historical owner.",
	"Private email and meeting activity counts only when it is visible to the teammate running the report.",
	"No-activity means no qualifying note, completed-task event, successful call or SMS, scheduled meeting, or sent/received visible email in the period. Creation, edits, failures, reminders, and future due dates do not count.",
	"This report does not infer historical health changes. Current client health remains a separate live report until real snapshots exist.",
}

type ClientActivityQuery struct {
	EntityType  string
	FromDate    string
	ToDate      string
	Activity    string
	OwnerUserID int64
	Limit       int
}

type ClientActivityTotals struct {
	ClientsWithoutActivity int `json:"clientsWithoutActivity"`
	ClientsWithActivity    int `json:"clientsWithActivity"`
	TotalClients           int `json:"totalClients"`
	QualifyingTouches      int `json:"qualifyingTouches"`
	NotesAdded             int `json:"notesAdded"`
	TasksCompleted         int `json:"tasksCompleted"`
}

type ClientActivityRecord struct {
	EntityType        string      `json:"entityType"`
	EntityID          int64       `json:"entityId"`
	Label             string      `json:"label"`
	OwnerUserID       int64       `json:"ownerUserId"`
	OwnerUserName     string      `json:"ownerUserName"`
	CreatedAt         time.Time   `json:"createdAt"`
	QualifyingTouches int         `json:"qualifyingTouches"`
	NotesAdded        int         `json:"notesAdded"`
	TasksCompleted    int         `json:"tasksCompleted"`
	ActiveDays        int         `json:"activeDays"`
	LastTouchInPeriod *Touchpoint `json:"lastTouchInPeriod,omitempty"`
}

type ClientActivityReport struct {
	EntityType  string                 `json:"entityType"`
	FromDate    string                 `json:"fromDate"`
	ToDate      string                 `json:"toDate"`
	Activity    string                 `json:"activity"`
	OwnerUserID int64                  `json:"ownerUserId"`
	GeneratedAt time.Time              `json:"generatedAt"`
	Count       int                    `json:"count"`
	Totals      ClientActivityTotals   `json:"totals"`
	Records     []ClientActivityRecord `json:"records"`
	Semantics   []string               `json:"semantics"`
}

func (s *Service) ClientActivity(ctx context.Context, organizationID, viewerUserID int64, query ClientActivityQuery) (ClientActivityReport, error) {
	if s == nil || s.pool == nil {
		return ClientActivityReport{}, fmt.Errorf("touchpoints service not configured")
	}
	from, toExclusive, query, err := normalizeClientActivityQuery(organizationID, viewerUserID, query, time.Now().UTC())
	if err != nil {
		return ClientActivityReport{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, clientActivityTimeout)
	defer cancel()
	if query.OwnerUserID > 0 {
		if err := s.ensureOwner(queryCtx, organizationID, query.OwnerUserID); err != nil {
			if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
				return ClientActivityReport{}, ErrQueryTimeout
			}
			return ClientActivityReport{}, err
		}
	}

	rows, err := s.pool.Query(queryCtx, clientActivitySQL(query.EntityType), organizationID, viewerUserID, from, toExclusive, query.OwnerUserID, query.Activity, query.Limit)
	if err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return ClientActivityReport{}, ErrQueryTimeout
		}
		return ClientActivityReport{}, fmt.Errorf("list %s client period activity: %w", query.EntityType, err)
	}
	defer rows.Close()

	report := ClientActivityReport{
		EntityType: query.EntityType, FromDate: query.FromDate, ToDate: query.ToDate,
		Activity: query.Activity, OwnerUserID: query.OwnerUserID, GeneratedAt: time.Now().UTC(),
		Records: []ClientActivityRecord{}, Semantics: append([]string(nil), ClientActivitySemantics...),
	}
	for rows.Next() {
		var record ClientActivityRecord
		var hasLastTouch bool
		var lastTouch Touchpoint
		if err := rows.Scan(
			&record.EntityID, &record.Label, &record.OwnerUserID, &record.OwnerUserName, &record.CreatedAt,
			&record.QualifyingTouches, &record.NotesAdded, &record.TasksCompleted, &record.ActiveDays,
			&hasLastTouch, &lastTouch.SourceType, &lastTouch.SourceID, &lastTouch.Action, &lastTouch.Summary,
			&lastTouch.OccurredAt, &lastTouch.RecordEntityType, &lastTouch.RecordEntityID, &lastTouch.RecordLabel,
			&report.Count, &report.Totals.TotalClients, &report.Totals.ClientsWithActivity,
			&report.Totals.ClientsWithoutActivity, &report.Totals.QualifyingTouches,
			&report.Totals.NotesAdded, &report.Totals.TasksCompleted,
		); err != nil {
			return ClientActivityReport{}, fmt.Errorf("scan client period activity: %w", err)
		}
		if record.EntityID <= 0 {
			continue
		}
		record.EntityType = query.EntityType
		if hasLastTouch {
			record.LastTouchInPeriod = &lastTouch
		}
		report.Records = append(report.Records, record)
	}
	if err := rows.Err(); err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return ClientActivityReport{}, ErrQueryTimeout
		}
		return ClientActivityReport{}, fmt.Errorf("iterate client period activity: %w", err)
	}
	return report, nil
}

func normalizeClientActivityQuery(organizationID, viewerUserID int64, query ClientActivityQuery, now time.Time) (time.Time, time.Time, ClientActivityQuery, error) {
	query.EntityType = strings.ToLower(strings.TrimSpace(query.EntityType))
	query.Activity = strings.ToLower(strings.TrimSpace(query.Activity))
	query.FromDate = strings.TrimSpace(query.FromDate)
	query.ToDate = strings.TrimSpace(query.ToDate)
	if organizationID <= 0 || viewerUserID <= 0 || (query.EntityType != "company" && query.EntityType != "contact") {
		return time.Time{}, time.Time{}, ClientActivityQuery{}, ErrInvalidInput
	}
	if query.Activity == "" {
		query.Activity = "all"
	}
	if query.Activity != "all" && query.Activity != "with_activity" && query.Activity != "without_activity" {
		return time.Time{}, time.Time{}, ClientActivityQuery{}, fmt.Errorf("%w: activity must be all, with_activity, or without_activity", ErrInvalidInput)
	}
	if query.FromDate == "" && query.ToDate == "" {
		to := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
		from := to.AddDate(0, 0, -29)
		query.FromDate, query.ToDate = from.Format(clientActivityDateLayout), to.Format(clientActivityDateLayout)
	} else if query.FromDate == "" || query.ToDate == "" {
		return time.Time{}, time.Time{}, ClientActivityQuery{}, fmt.Errorf("%w: from and to must be provided together", ErrInvalidInput)
	}
	from, fromErr := time.Parse(clientActivityDateLayout, query.FromDate)
	to, toErr := time.Parse(clientActivityDateLayout, query.ToDate)
	if fromErr != nil || toErr != nil || to.Before(from) || to.Sub(from) > 365*24*time.Hour {
		return time.Time{}, time.Time{}, ClientActivityQuery{}, fmt.Errorf("%w: dates must be an ordered inclusive UTC window of at most 366 days", ErrInvalidInput)
	}
	if query.OwnerUserID < 0 {
		return time.Time{}, time.Time{}, ClientActivityQuery{}, ErrInvalidInput
	}
	if query.Limit == 0 {
		query.Limit = defaultLimit
	}
	if query.Limit < 1 || query.Limit > maximumLimit {
		return time.Time{}, time.Time{}, ClientActivityQuery{}, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidInput, maximumLimit)
	}
	return from, to.AddDate(0, 0, 1), query, nil
}

func clientActivitySQL(entityType string) string {
	return touchpointCTEForRecords(clientActivityRecordEntitiesSQL(entityType)) + `
	, period_touches AS (
		SELECT * FROM touches WHERE occurred_at >= $3 AND occurred_at < $4
	), activity_counts AS (
		SELECT target_id,COUNT(*)::int qualifying_touches,
		       COUNT(*) FILTER (WHERE source_type='note' AND action='note.created')::int notes_added,
		       COUNT(*) FILTER (WHERE source_type='task' AND action='task.completed')::int tasks_completed,
		       COUNT(DISTINCT (occurred_at AT TIME ZONE 'UTC')::date)::int active_days
		FROM period_touches GROUP BY target_id
	), latest AS (
		SELECT target_id,source_type,source_id,action,summary,occurred_at,source_entity_type,source_entity_id,source_entity_label,
		       ROW_NUMBER() OVER (PARTITION BY target_id ORDER BY occurred_at DESC,source_type,source_id DESC) row_number
		FROM period_touches
	), records AS (` + healthRecordListSQL(entityType) + `
	), signals AS (
		SELECT r.id,r.label,r.owner_user_id,r.owner_user_name,r.created_at,
		       COALESCE(a.qualifying_touches,0) qualifying_touches,COALESCE(a.notes_added,0) notes_added,
		       COALESCE(a.tasks_completed,0) tasks_completed,COALESCE(a.active_days,0) active_days,
		       (l.occurred_at IS NOT NULL) has_last_touch,COALESCE(l.source_type,'') source_type,
		       COALESCE(l.source_id,0) source_id,COALESCE(l.action,'') action,COALESCE(l.summary,'') summary,
		       COALESCE(l.occurred_at,r.created_at) occurred_at,COALESCE(l.source_entity_type,'') source_entity_type,
		       COALESCE(l.source_entity_id,0) source_entity_id,COALESCE(l.source_entity_label,'') source_entity_label
		FROM records r
		LEFT JOIN activity_counts a ON a.target_id=r.id
		LEFT JOIN latest l ON l.target_id=r.id AND l.row_number=1
	), totals AS (
		SELECT COUNT(*)::int total_clients,
		       COUNT(*) FILTER (WHERE qualifying_touches>0)::int clients_with_activity,
		       COUNT(*) FILTER (WHERE qualifying_touches=0)::int clients_without_activity,
		       COALESCE(SUM(qualifying_touches),0)::int qualifying_touches,
		       COALESCE(SUM(notes_added),0)::int notes_added,
		       COALESCE(SUM(tasks_completed),0)::int tasks_completed
		FROM signals WHERE ($5::bigint=0 OR owner_user_id=$5)
	), filtered_total AS (
		SELECT COUNT(*)::int count FROM signals
		WHERE ($5::bigint=0 OR owner_user_id=$5)
		  AND ($6='all' OR ($6='with_activity' AND qualifying_touches>0) OR ($6='without_activity' AND qualifying_touches=0))
	)
	SELECT COALESCE(activity.id,0),COALESCE(activity.label,''),COALESCE(activity.owner_user_id,0),COALESCE(activity.owner_user_name,''),
	       COALESCE(activity.created_at,NOW()),COALESCE(activity.qualifying_touches,0),COALESCE(activity.notes_added,0),
	       COALESCE(activity.tasks_completed,0),COALESCE(activity.active_days,0),COALESCE(activity.has_last_touch,FALSE),
	       COALESCE(activity.source_type,''),COALESCE(activity.source_id,0),COALESCE(activity.action,''),COALESCE(activity.summary,''),
	       COALESCE(activity.occurred_at,NOW()),COALESCE(activity.source_entity_type,''),COALESCE(activity.source_entity_id,0),COALESCE(activity.source_entity_label,''),
	       filtered_total.count,totals.total_clients,totals.clients_with_activity,totals.clients_without_activity,
	       totals.qualifying_touches,totals.notes_added,totals.tasks_completed
	FROM totals CROSS JOIN filtered_total
	LEFT JOIN LATERAL (
		SELECT * FROM signals
		WHERE ($5::bigint=0 OR owner_user_id=$5)
		  AND ($6='all' OR ($6='with_activity' AND qualifying_touches>0) OR ($6='without_activity' AND qualifying_touches=0))
		ORDER BY (qualifying_touches=0) DESC,qualifying_touches,COALESCE(occurred_at,created_at),label,id
		LIMIT $7
	) activity ON TRUE`
}

func clientActivityRecordEntitiesSQL(entityType string) string {
	if entityType == "contact" {
		return `SELECT target.id target_id,'contact'::text source_entity_type,target.id source_entity_id,
		       COALESCE(NULLIF(trim(target.first_name||' '||target.last_name),''),'Contact #'||target.id::text) source_entity_label,1 direct_rank
		FROM contacts target
		WHERE target.organization_id=$1 AND target.archived_at IS NULL AND (target.is_client=TRUE OR target.status='customer')`
	}
	return `SELECT target.id target_id,'company'::text source_entity_type,target.id source_entity_id,target.name source_entity_label,1 direct_rank
		FROM companies target
		WHERE target.organization_id=$1 AND target.archived_at IS NULL AND target.status='customer'
		UNION ALL
		SELECT target.id,'contact',contact.id,
		       COALESCE(NULLIF(trim(contact.first_name||' '||contact.last_name),''),'Contact #'||contact.id::text),0
		FROM companies target
		JOIN contact_company_links link ON link.organization_id=$1 AND link.company_id=target.id
		JOIN contacts contact ON contact.organization_id=$1 AND contact.id=link.contact_id AND contact.archived_at IS NULL
		WHERE target.organization_id=$1 AND target.archived_at IS NULL AND target.status='customer'`
}
