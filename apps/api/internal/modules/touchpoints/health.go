package touchpoints

import (
	"context"
	"fmt"
	"strings"
	"time"
)

var HealthSemantics = []string{
	"Needs attention means the client has an overdue open task or its latest viewer-visible qualifying touch is older than the selected threshold.",
	"Watch means follow-up is current and at least one open task is due within the next 7 days. Healthy means neither condition applies.",
	"Company health includes direct client work and work on currently linked people. Individual-client health uses the contact record.",
	"Open tasks without a due date are counted but do not change health. Archived and completed tasks are excluded.",
	"Open-issue health is not inferred because Open CRM does not currently expose an issue record.",
}

type HealthQuery struct {
	EntityType  string
	Status      string
	StaleDays   int
	OwnerUserID int64
	Limit       int
}

type HealthTotals struct {
	Total          int `json:"total"`
	Healthy        int `json:"healthy"`
	Watch          int `json:"watch"`
	NeedsAttention int `json:"needsAttention"`
}

type HealthRecord struct {
	EntityType         string      `json:"entityType"`
	EntityID           int64       `json:"entityId"`
	Label              string      `json:"label"`
	OwnerUserID        int64       `json:"ownerUserId"`
	OwnerUserName      string      `json:"ownerUserName"`
	CreatedAt          time.Time   `json:"createdAt"`
	ReferenceAt        time.Time   `json:"referenceAt"`
	DaysSinceReference int         `json:"daysSinceReference"`
	IsStale            bool        `json:"isStale"`
	LastTouch          *Touchpoint `json:"lastTouch,omitempty"`
	OpenTaskCount      int         `json:"openTaskCount"`
	OverdueTaskCount   int         `json:"overdueTaskCount"`
	DueSoonTaskCount   int         `json:"dueSoonTaskCount"`
	HealthStatus       string      `json:"healthStatus"`
	HealthLabel        string      `json:"healthLabel"`
	HealthReasons      []string    `json:"healthReasons"`
}

type HealthReport struct {
	EntityType  string         `json:"entityType"`
	Status      string         `json:"status"`
	StaleDays   int            `json:"staleDays"`
	OwnerUserID int64          `json:"ownerUserId"`
	CutoffAt    time.Time      `json:"cutoffAt"`
	GeneratedAt time.Time      `json:"generatedAt"`
	Count       int            `json:"count"`
	Totals      HealthTotals   `json:"totals"`
	Records     []HealthRecord `json:"records"`
	Semantics   []string       `json:"semantics"`
}

type taskSignals struct {
	Open    int
	Overdue int
	DueSoon int
}

func (s *Service) Health(ctx context.Context, organizationID, viewerUserID int64, query HealthQuery) (HealthReport, error) {
	if s == nil || s.pool == nil {
		return HealthReport{}, fmt.Errorf("touchpoints service not configured")
	}
	query, err := normalizeHealthQuery(organizationID, viewerUserID, query)
	if err != nil {
		return HealthReport{}, err
	}
	if query.OwnerUserID > 0 {
		if err := s.ensureOwner(ctx, organizationID, query.OwnerUserID); err != nil {
			return HealthReport{}, err
		}
	}

	generatedAt := time.Now().UTC()
	cutoffAt := generatedAt.AddDate(0, 0, -query.StaleDays)
	rows, err := s.pool.Query(ctx, healthSQL(query.EntityType), organizationID, viewerUserID, cutoffAt, query.Status, query.OwnerUserID, query.Limit)
	if err != nil {
		return HealthReport{}, fmt.Errorf("list %s client health: %w", query.EntityType, err)
	}
	defer rows.Close()

	report := HealthReport{
		EntityType: query.EntityType, Status: query.Status, StaleDays: query.StaleDays,
		OwnerUserID: query.OwnerUserID, CutoffAt: cutoffAt, GeneratedAt: generatedAt,
		Records: []HealthRecord{}, Semantics: append([]string(nil), HealthSemantics...),
	}
	for rows.Next() {
		var record HealthRecord
		var hasTouch bool
		var touch Touchpoint
		if err := rows.Scan(
			&record.EntityID, &record.Label, &record.OwnerUserID, &record.OwnerUserName,
			&record.CreatedAt, &record.ReferenceAt, &record.DaysSinceReference, &record.IsStale,
			&hasTouch, &touch.SourceType, &touch.SourceID, &touch.Action, &touch.Summary,
			&touch.OccurredAt, &touch.RecordEntityType, &touch.RecordEntityID, &touch.RecordLabel,
			&record.OpenTaskCount, &record.OverdueTaskCount, &record.DueSoonTaskCount, &record.HealthStatus,
			&report.Count, &report.Totals.Total, &report.Totals.Healthy, &report.Totals.Watch, &report.Totals.NeedsAttention,
		); err != nil {
			return HealthReport{}, fmt.Errorf("scan client health: %w", err)
		}
		if record.EntityID <= 0 {
			continue
		}
		record.EntityType = query.EntityType
		if hasTouch {
			record.LastTouch = &touch
		}
		record.HealthStatus, record.HealthLabel, record.HealthReasons = classifyHealth(record.IsStale, record.DaysSinceReference, taskSignals{
			Open: record.OpenTaskCount, Overdue: record.OverdueTaskCount, DueSoon: record.DueSoonTaskCount,
		})
		report.Records = append(report.Records, record)
	}
	if err := rows.Err(); err != nil {
		return HealthReport{}, fmt.Errorf("iterate client health: %w", err)
	}
	return report, nil
}

func normalizeHealthQuery(organizationID, viewerUserID int64, query HealthQuery) (HealthQuery, error) {
	base, err := normalizeQuery(organizationID, viewerUserID, Query{
		EntityType: query.EntityType, StaleDays: query.StaleDays, OwnerUserID: query.OwnerUserID, Limit: query.Limit,
	})
	if err != nil {
		return HealthQuery{}, err
	}
	query.EntityType, query.StaleDays, query.OwnerUserID, query.Limit = base.EntityType, base.StaleDays, base.OwnerUserID, base.Limit
	query.Status = strings.TrimSpace(strings.ToLower(query.Status))
	if query.Status == "" {
		query.Status = "all"
	}
	if query.Status != "all" && query.Status != "healthy" && query.Status != "watch" && query.Status != "needs_attention" {
		return HealthQuery{}, fmt.Errorf("%w: status must be all, healthy, watch, or needs_attention", ErrInvalidInput)
	}
	return query, nil
}

func classifyHealth(isStale bool, daysSinceReference int, signals taskSignals) (string, string, []string) {
	status, label := "healthy", "Healthy"
	if isStale || signals.Overdue > 0 {
		status, label = "needs_attention", "Needs attention"
	} else if signals.DueSoon > 0 {
		status, label = "watch", "Watch"
	}
	reasons := make([]string, 0, 3)
	if isStale {
		reasons = append(reasons, fmt.Sprintf("No qualifying touch for %d days", daysSinceReference))
	}
	if signals.Overdue > 0 {
		reasons = append(reasons, countLabel(signals.Overdue, "overdue open task", "overdue open tasks"))
	}
	if signals.DueSoon > 0 {
		reasons = append(reasons, countLabel(signals.DueSoon, "open task due in the next 7 days", "open tasks due in the next 7 days"))
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "Follow-up is current and no open task is overdue or due in the next 7 days")
	}
	return status, label, reasons
}

func countLabel(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func (s *Service) loadTaskSignals(ctx context.Context, organizationID, entityID int64, entityType string) (taskSignals, error) {
	var signals taskSignals
	if err := s.pool.QueryRow(ctx, taskSignalsSQL(entityType), organizationID, entityID).Scan(&signals.Open, &signals.Overdue, &signals.DueSoon); err != nil {
		return taskSignals{}, fmt.Errorf("load client task health: %w", err)
	}
	return signals, nil
}

func healthSQL(entityType string) string {
	return touchpointCTE(entityType, false) + `
	, latest AS (
		SELECT target_id,source_type,source_id,action,summary,occurred_at,source_entity_type,source_entity_id,source_entity_label,
		       ROW_NUMBER() OVER (PARTITION BY target_id ORDER BY occurred_at DESC,source_type,source_id DESC) row_number
		FROM touches
	), records AS (` + healthRecordListSQL(entityType) + `
	), task_signals AS (
		SELECT re.target_id,
		       COUNT(DISTINCT task.id) FILTER (WHERE task.status='open')::int open_task_count,
		       COUNT(DISTINCT task.id) FILTER (WHERE task.status='open' AND task.due_at<NOW())::int overdue_task_count,
		       COUNT(DISTINCT task.id) FILTER (WHERE task.status='open' AND task.due_at>=NOW() AND task.due_at<NOW()+INTERVAL '7 days')::int due_soon_task_count
		FROM record_entities re
		JOIN tasks task ON task.organization_id=$1 AND task.entity_type=re.source_entity_type AND task.entity_id=re.source_entity_id AND task.archived_at IS NULL
		GROUP BY re.target_id
	), signals AS (
		SELECT r.id,r.label,r.owner_user_id,r.owner_user_name,r.created_at,
		       COALESCE(l.occurred_at,r.created_at) reference_at,
		       GREATEST(0,FLOOR(EXTRACT(EPOCH FROM (NOW()-COALESCE(l.occurred_at,r.created_at)))/86400))::int days_since_reference,
		       COALESCE(l.occurred_at,r.created_at)<$3 is_stale,
		       l.occurred_at IS NOT NULL has_touch,
		       COALESCE(l.source_type,'') source_type,COALESCE(l.source_id,0) source_id,
		       COALESCE(l.action,'') action,COALESCE(l.summary,'') summary,
		       COALESCE(l.occurred_at,r.created_at) occurred_at,COALESCE(l.source_entity_type,'') source_entity_type,
		       COALESCE(l.source_entity_id,0) source_entity_id,COALESCE(l.source_entity_label,'') source_entity_label,
		       COALESCE(ts.open_task_count,0) open_task_count,COALESCE(ts.overdue_task_count,0) overdue_task_count,
		       COALESCE(ts.due_soon_task_count,0) due_soon_task_count,
		       CASE WHEN COALESCE(ts.overdue_task_count,0)>0 OR COALESCE(l.occurred_at,r.created_at)<$3 THEN 'needs_attention'
		            WHEN COALESCE(ts.due_soon_task_count,0)>0 THEN 'watch' ELSE 'healthy' END health_status
		FROM records r
		LEFT JOIN latest l ON l.target_id=r.id AND l.row_number=1
		LEFT JOIN task_signals ts ON ts.target_id=r.id
	), totals AS (
		SELECT COUNT(*)::int total,
		       COUNT(*) FILTER (WHERE health_status='healthy')::int healthy,
		       COUNT(*) FILTER (WHERE health_status='watch')::int watch,
		       COUNT(*) FILTER (WHERE health_status='needs_attention')::int needs_attention
		FROM signals WHERE ($5::bigint=0 OR owner_user_id=$5)
	), filtered_total AS (
		SELECT COUNT(*)::int count FROM signals WHERE ($4='all' OR health_status=$4) AND ($5::bigint=0 OR owner_user_id=$5)
	)
	SELECT COALESCE(health.id,0),COALESCE(health.label,''),COALESCE(health.owner_user_id,0),COALESCE(health.owner_user_name,''),
	       COALESCE(health.created_at,NOW()),COALESCE(health.reference_at,NOW()),COALESCE(health.days_since_reference,0),COALESCE(health.is_stale,FALSE),
	       COALESCE(health.has_touch,FALSE),COALESCE(health.source_type,''),COALESCE(health.source_id,0),COALESCE(health.action,''),COALESCE(health.summary,''),
	       COALESCE(health.occurred_at,NOW()),COALESCE(health.source_entity_type,''),COALESCE(health.source_entity_id,0),COALESCE(health.source_entity_label,''),
	       COALESCE(health.open_task_count,0),COALESCE(health.overdue_task_count,0),COALESCE(health.due_soon_task_count,0),COALESCE(health.health_status,''),
	       filtered_total.count,totals.total,totals.healthy,totals.watch,totals.needs_attention
	FROM totals CROSS JOIN filtered_total
	LEFT JOIN LATERAL (
		SELECT * FROM signals
		WHERE ($4='all' OR health_status=$4) AND ($5::bigint=0 OR owner_user_id=$5)
		ORDER BY CASE health_status WHEN 'needs_attention' THEN 0 WHEN 'watch' THEN 1 ELSE 2 END,
		         overdue_task_count DESC,reference_at,label,id
		LIMIT $6
	) health ON TRUE`
}

func healthRecordListSQL(entityType string) string {
	if entityType == "contact" {
		return `SELECT target.id,
		       COALESCE(NULLIF(trim(target.first_name||' '||target.last_name),''),'Contact #'||target.id::text) label,
		       COALESCE(target.owner_user_id,0) owner_user_id,
		       COALESCE(NULLIF(trim(COALESCE(owner.first_name,'')||' '||COALESCE(owner.last_name,'')),''),COALESCE(owner.email,'')) owner_user_name,
		       target.created_at
		FROM contacts target
		LEFT JOIN organization_memberships owner_membership ON owner_membership.organization_id=target.organization_id AND owner_membership.user_id=target.owner_user_id
		LEFT JOIN users owner ON owner.id=owner_membership.user_id
		WHERE target.organization_id=$1 AND target.archived_at IS NULL AND (target.is_client=TRUE OR target.status='customer')`
	}
	return `SELECT target.id,target.name label,COALESCE(target.owner_user_id,0) owner_user_id,
		       COALESCE(NULLIF(trim(COALESCE(owner.first_name,'')||' '||COALESCE(owner.last_name,'')),''),COALESCE(owner.email,'')) owner_user_name,
		       target.created_at
		FROM companies target
		LEFT JOIN organization_memberships owner_membership ON owner_membership.organization_id=target.organization_id AND owner_membership.user_id=target.owner_user_id
		LEFT JOIN users owner ON owner.id=owner_membership.user_id
		WHERE target.organization_id=$1 AND target.archived_at IS NULL AND target.status='customer'`
}

func taskSignalsSQL(entityType string) string {
	return `WITH record_entities AS (` + recordEntitiesSQL(entityType, " AND target.id=$2") + `)
	SELECT COUNT(DISTINCT task.id) FILTER (WHERE task.status='open')::int,
	       COUNT(DISTINCT task.id) FILTER (WHERE task.status='open' AND task.due_at<NOW())::int,
	       COUNT(DISTINCT task.id) FILTER (WHERE task.status='open' AND task.due_at>=NOW() AND task.due_at<NOW()+INTERVAL '7 days')::int
	FROM record_entities re
	LEFT JOIN tasks task ON task.organization_id=$1 AND task.entity_type=re.source_entity_type AND task.entity_id=re.source_entity_id AND task.archived_at IS NULL`
}
