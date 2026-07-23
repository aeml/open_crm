// Package salesreports builds bounded, history-backed sales activity readouts.
package salesreports

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidInput = errors.New("invalid sales activity report filter")

const dateLayout = "2006-01-02"
const activityQueryTimeout = 5 * time.Second

type Query struct {
	FromDate    string
	ToDate      string
	OwnerUserID int64
}

type Totals struct {
	DealsCreated           int    `json:"dealsCreated"`
	StageMoves             int    `json:"stageMoves"`
	DealsWon               int    `json:"dealsWon"`
	DealsLost              int    `json:"dealsLost"`
	ClosedOutcomes         int    `json:"closedOutcomes"`
	WinRatePercent         string `json:"winRatePercent"`
	WonRevenueBase         string `json:"wonRevenueBase"`
	WonRevenueCaptured     int    `json:"wonRevenueCaptured"`
	WonRevenueMissingValue int    `json:"wonRevenueMissingValue"`
	WonRevenueMissingRate  int    `json:"wonRevenueMissingRate"`
	NotesAdded             int    `json:"notesAdded"`
	TasksCreated           int    `json:"tasksCreated"`
	TasksCompleted         int    `json:"tasksCompleted"`
}

type OwnerSummary struct {
	UserID                 int64  `json:"userId"`
	UserName               string `json:"userName"`
	Email                  string `json:"email"`
	Status                 string `json:"status"`
	DealsCreated           int    `json:"dealsCreated"`
	StageMoves             int    `json:"stageMoves"`
	DealsWon               int    `json:"dealsWon"`
	DealsLost              int    `json:"dealsLost"`
	WonRevenueBase         string `json:"wonRevenueBase"`
	WonRevenueCaptured     int    `json:"wonRevenueCaptured"`
	WonRevenueMissingValue int    `json:"wonRevenueMissingValue"`
	WonRevenueMissingRate  int    `json:"wonRevenueMissingRate"`
	NotesAdded             int    `json:"notesAdded"`
	TasksCreated           int    `json:"tasksCreated"`
	TasksCompleted         int    `json:"tasksCompleted"`
}

type StageConversion struct {
	PipelineID             int64  `json:"pipelineId"`
	PipelineName           string `json:"pipelineName"`
	StageID                int64  `json:"stageId"`
	StageName              string `json:"stageName"`
	StagePosition          int    `json:"stagePosition"`
	Entries                int    `json:"entries"`
	Exits                  int    `json:"exits"`
	ForwardExits           int    `json:"forwardExits"`
	WonExits               int    `json:"wonExits"`
	LostExits              int    `json:"lostExits"`
	ForwardExitRatePercent string `json:"forwardExitRatePercent"`
}

type CloseReasonSummary struct {
	Outcome     string `json:"outcome"`
	ReasonCode  string `json:"reasonCode"`
	ReasonLabel string `json:"reasonLabel"`
	Count       int    `json:"count"`
}

type DealEvent struct {
	ID               int64     `json:"id"`
	DealID           int64     `json:"dealId"`
	DealName         string    `json:"dealName"`
	EventType        string    `json:"eventType"`
	ActorName        string    `json:"actorName"`
	OwnerName        string    `json:"ownerName"`
	FromPipelineName string    `json:"fromPipelineName"`
	FromStageName    string    `json:"fromStageName"`
	FromStageOutcome string    `json:"fromStageOutcome"`
	ToPipelineName   string    `json:"toPipelineName"`
	ToStageName      string    `json:"toStageName"`
	ToStageOutcome   string    `json:"toStageOutcome"`
	CloseReasonCode  string    `json:"closeReasonCode"`
	CloseReasonLabel string    `json:"closeReasonLabel"`
	CloseNotes       string    `json:"closeNotes"`
	OccurredAt       time.Time `json:"occurredAt"`
}

type Report struct {
	FromDate                     string               `json:"fromDate"`
	ToDate                       string               `json:"toDate"`
	OwnerUserID                  int64                `json:"ownerUserId"`
	GeneratedAt                  time.Time            `json:"generatedAt"`
	CoverageStartedAt            time.Time            `json:"coverageStartedAt"`
	HistoryComplete              bool                 `json:"historyComplete"`
	BaseCurrency                 string               `json:"baseCurrency"`
	RevenueTrackingStartedAt     time.Time            `json:"revenueTrackingStartedAt"`
	RevenueHistoryComplete       bool                 `json:"revenueHistoryComplete"`
	CloseReasonCoverageStartedAt time.Time            `json:"closeReasonCoverageStartedAt"`
	CloseReasonHistoryComplete   bool                 `json:"closeReasonHistoryComplete"`
	OwnerFilterMeaning           string               `json:"ownerFilterMeaning"`
	OutcomeMeaning               string               `json:"outcomeMeaning"`
	RevenueMeaning               string               `json:"revenueMeaning"`
	CloseReasonMeaning           string               `json:"closeReasonMeaning"`
	StageConversionMeaning       string               `json:"stageConversionMeaning"`
	Totals                       Totals               `json:"totals"`
	Owners                       []OwnerSummary       `json:"owners"`
	Stages                       []StageConversion    `json:"stages"`
	CloseReasons                 []CloseReasonSummary `json:"closeReasons"`
	DealEvents                   []DealEvent          `json:"dealEvents"`
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Activity(ctx context.Context, organizationID int64, query Query) (Report, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return Report{}, fmt.Errorf("sales reports service not configured")
	}
	from, to, query, err := normalizeQuery(query, time.Now().UTC())
	if err != nil {
		return Report{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, activityQueryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Report{}, salesReportQueryError(queryCtx, "begin sales activity snapshot", err)
	}
	defer tx.Rollback(context.Background())
	if query.OwnerUserID > 0 {
		var exists bool
		if err := tx.QueryRow(queryCtx, `SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id=$1 AND user_id=$2)`, organizationID, query.OwnerUserID).Scan(&exists); err != nil {
			return Report{}, salesReportQueryError(queryCtx, "validate sales report owner", err)
		}
		if !exists {
			return Report{}, ErrInvalidInput
		}
	}

	var coverageStartedAt, revenueTrackingStartedAt, closeReasonCoverageStartedAt, organizationCreatedAt time.Time
	var baseCurrency string
	if err := tx.QueryRow(queryCtx, `
		SELECT COALESCE(sales_activity_tracking_started_at,created_at),
		       COALESCE(sales_revenue_tracking_started_at,created_at),
		       COALESCE(deal_close_reason_tracking_started_at,created_at),created_at,
		       COALESCE(NULLIF(base_currency,''),'USD')
		FROM organizations WHERE id=$1
	`, organizationID).Scan(&coverageStartedAt, &revenueTrackingStartedAt, &closeReasonCoverageStartedAt, &organizationCreatedAt, &baseCurrency); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Report{}, ErrInvalidInput
		}
		return Report{}, salesReportQueryError(queryCtx, "load sales reporting coverage", err)
	}
	report := Report{
		FromDate: query.FromDate, ToDate: query.ToDate, OwnerUserID: query.OwnerUserID,
		GeneratedAt: time.Now().UTC(), CoverageStartedAt: coverageStartedAt,
		HistoryComplete:              !coverageStartedAt.After(organizationCreatedAt) || !from.Before(coverageStartedAt),
		BaseCurrency:                 baseCurrency,
		RevenueTrackingStartedAt:     revenueTrackingStartedAt,
		RevenueHistoryComplete:       !revenueTrackingStartedAt.After(organizationCreatedAt) || !from.Before(revenueTrackingStartedAt),
		CloseReasonCoverageStartedAt: closeReasonCoverageStartedAt,
		CloseReasonHistoryComplete:   !closeReasonCoverageStartedAt.After(organizationCreatedAt) || !from.Before(closeReasonCoverageStartedAt),
		OwnerFilterMeaning:           "Deal metrics use the owner saved on the event; notes and tasks use the teammate who performed the activity.",
		OutcomeMeaning:               "A won or lost outcome is a real transition into that outcome; a deal reopened and closed again contributes another outcome.",
		RevenueMeaning:               "Won revenue is the deal value converted to the workspace base currency at each real won transition. Reopened deals won again contribute another outcome. Missing values or event-time exchange rates are counted but never estimated from mutable current data.",
		CloseReasonMeaning:           "Close reasons are fixed pilot options captured at the outcome transition. Not-captured rows predate close-reason tracking; notes remain event-time context.",
		StageConversionMeaning:       "Forward exit rate is forward-or-won stage exits divided by every exit from that stage during the selected period; it is event-based, not a deal-cohort funnel.",
		Owners:                       []OwnerSummary{}, Stages: []StageConversion{}, CloseReasons: []CloseReasonSummary{}, DealEvents: []DealEvent{},
	}
	endExclusive := to.AddDate(0, 0, 1)
	if err := s.loadTotals(queryCtx, tx, organizationID, from, endExclusive, query.OwnerUserID, &report); err != nil {
		return Report{}, salesReportQueryError(queryCtx, "load sales activity totals", err)
	}
	owners, err := s.loadOwners(queryCtx, tx, organizationID, from, endExclusive, query.OwnerUserID)
	if err != nil {
		return Report{}, salesReportQueryError(queryCtx, "load sales activity owners", err)
	}
	report.Owners = owners
	stages, err := s.loadStages(queryCtx, tx, organizationID, from, endExclusive, query.OwnerUserID)
	if err != nil {
		return Report{}, salesReportQueryError(queryCtx, "load sales activity stages", err)
	}
	report.Stages = stages
	closeReasons, err := s.loadCloseReasons(queryCtx, tx, organizationID, from, endExclusive, query.OwnerUserID)
	if err != nil {
		return Report{}, salesReportQueryError(queryCtx, "load sales close reasons", err)
	}
	report.CloseReasons = closeReasons
	events, err := s.loadEvents(queryCtx, tx, organizationID, from, endExclusive, query.OwnerUserID)
	if err != nil {
		return Report{}, salesReportQueryError(queryCtx, "load recent sales events", err)
	}
	report.DealEvents = events
	if err := tx.Commit(queryCtx); err != nil {
		return Report{}, salesReportQueryError(queryCtx, "commit sales activity snapshot", err)
	}
	return report, nil
}

func (s *Service) loadCloseReasons(ctx context.Context, tx pgx.Tx, organizationID int64, from, to time.Time, ownerUserID int64) ([]CloseReasonSummary, error) {
	rows, err := tx.Query(ctx, `
		SELECT to_stage_outcome,
		       CASE WHEN COALESCE(close_reason_code,'')='' THEN 'not_captured' ELSE close_reason_code END,
		       CASE WHEN COALESCE(close_reason_label,'')='' THEN 'Not captured before tracking' ELSE close_reason_label END,
		       COUNT(*)
		FROM deal_stage_events
		WHERE organization_id=$1 AND occurred_at >= $2 AND occurred_at < $3
		  AND ($4::bigint=0 OR owner_user_id=$4)
		  AND to_stage_outcome IN ('won','lost')
		  AND COALESCE(from_stage_outcome,'')<>to_stage_outcome
		GROUP BY to_stage_outcome,2,3
		ORDER BY CASE WHEN to_stage_outcome='won' THEN 0 ELSE 1 END,COUNT(*) DESC,3
	`, organizationID, from, to, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("load close reason summaries: %w", err)
	}
	defer rows.Close()
	result := []CloseReasonSummary{}
	for rows.Next() {
		var item CloseReasonSummary
		if err := rows.Scan(&item.Outcome, &item.ReasonCode, &item.ReasonLabel, &item.Count); err != nil {
			return nil, fmt.Errorf("scan close reason summary: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate close reason summaries: %w", err)
	}
	return result, nil
}

func (s *Service) loadTotals(ctx context.Context, tx pgx.Tx, organizationID int64, from, to time.Time, ownerUserID int64, report *Report) error {
	if err := tx.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type='created'),
			COUNT(*) FILTER (WHERE event_type='stage_changed'),
			COUNT(*) FILTER (WHERE to_stage_outcome='won' AND COALESCE(from_stage_outcome,'')<>'won'),
			COUNT(*) FILTER (WHERE to_stage_outcome='lost' AND COALESCE(from_stage_outcome,'')<>'lost')
		FROM deal_stage_events
		WHERE organization_id=$1 AND occurred_at >= $2 AND occurred_at < $3
		  AND ($4::bigint=0 OR owner_user_id=$4)
	`, organizationID, from, to, ownerUserID).Scan(&report.Totals.DealsCreated, &report.Totals.StageMoves, &report.Totals.DealsWon, &report.Totals.DealsLost); err != nil {
		return fmt.Errorf("load sales deal totals: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE action='note.created'),
			COUNT(*) FILTER (WHERE action IN ('task.created','task.automated')),
			COUNT(*) FILTER (WHERE action='task.completed')
		FROM activities
		WHERE organization_id=$1 AND created_at >= $2 AND created_at < $3
		  AND action IN ('note.created','task.created','task.automated','task.completed')
		  AND ($4::bigint=0 OR actor_user_id=$4)
	`, organizationID, from, to, ownerUserID).Scan(&report.Totals.NotesAdded, &report.Totals.TasksCreated, &report.Totals.TasksCompleted); err != nil {
		return fmt.Errorf("load sales work totals: %w", err)
	}
	report.Totals.ClosedOutcomes = report.Totals.DealsWon + report.Totals.DealsLost
	report.Totals.WinRatePercent = rate(report.Totals.DealsWon, report.Totals.ClosedOutcomes)
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(deal_value_in_base_currency),0)::numeric(24,2)::text,
		       COUNT(*) FILTER (WHERE deal_value_in_base_currency IS NOT NULL),
		       COUNT(*) FILTER (WHERE deal_value_amount IS NULL OR deal_value_currency IS NULL),
		       COUNT(*) FILTER (WHERE deal_value_amount IS NOT NULL AND deal_value_currency IS NOT NULL AND deal_value_in_base_currency IS NULL)
		FROM deal_stage_events
		WHERE organization_id=$1 AND occurred_at >= $2 AND occurred_at < $3
		  AND ($4::bigint=0 OR owner_user_id=$4)
		  AND to_stage_outcome='won' AND COALESCE(from_stage_outcome,'')<>'won'
	`, organizationID, from, to, ownerUserID).Scan(
		&report.Totals.WonRevenueBase, &report.Totals.WonRevenueCaptured,
		&report.Totals.WonRevenueMissingValue, &report.Totals.WonRevenueMissingRate,
	); err != nil {
		return salesReportQueryError(ctx, "load sales revenue totals", err)
	}
	return nil
}

func (s *Service) loadOwners(ctx context.Context, tx pgx.Tx, organizationID int64, from, to time.Time, ownerUserID int64) ([]OwnerSummary, error) {
	rows, err := tx.Query(ctx, `
		WITH deal_metrics AS (
			SELECT owner_user_id AS user_id,
				COUNT(*) FILTER (WHERE event_type='created') AS deals_created,
				COUNT(*) FILTER (WHERE event_type='stage_changed') AS stage_moves,
				COUNT(*) FILTER (WHERE to_stage_outcome='won' AND COALESCE(from_stage_outcome,'')<>'won') AS deals_won,
				COUNT(*) FILTER (WHERE to_stage_outcome='lost' AND COALESCE(from_stage_outcome,'')<>'lost') AS deals_lost
			FROM deal_stage_events
			WHERE organization_id=$1 AND occurred_at >= $2 AND occurred_at < $3 AND owner_user_id IS NOT NULL
			  AND ($4::bigint=0 OR owner_user_id=$4)
			GROUP BY owner_user_id
		), revenue_metrics AS (
			SELECT owner_user_id AS user_id,
				COALESCE(SUM(deal_value_in_base_currency),0)::numeric(24,2) AS won_revenue_base,
				COUNT(*) FILTER (WHERE deal_value_in_base_currency IS NOT NULL) AS won_revenue_captured,
				COUNT(*) FILTER (WHERE deal_value_amount IS NULL OR deal_value_currency IS NULL) AS won_revenue_missing_value,
				COUNT(*) FILTER (WHERE deal_value_amount IS NOT NULL AND deal_value_currency IS NOT NULL AND deal_value_in_base_currency IS NULL) AS won_revenue_missing_rate
			FROM deal_stage_events
			WHERE organization_id=$1 AND occurred_at >= $2 AND occurred_at < $3 AND owner_user_id IS NOT NULL
			  AND ($4::bigint=0 OR owner_user_id=$4)
			  AND to_stage_outcome='won' AND COALESCE(from_stage_outcome,'')<>'won'
			GROUP BY owner_user_id
		), work_metrics AS (
			SELECT actor_user_id AS user_id,
				COUNT(*) FILTER (WHERE action='note.created') AS notes_added,
				COUNT(*) FILTER (WHERE action IN ('task.created','task.automated')) AS tasks_created,
				COUNT(*) FILTER (WHERE action='task.completed') AS tasks_completed
			FROM activities
			WHERE organization_id=$1 AND created_at >= $2 AND created_at < $3 AND actor_user_id IS NOT NULL
			  AND action IN ('note.created','task.created','task.automated','task.completed')
			  AND ($4::bigint=0 OR actor_user_id=$4)
			GROUP BY actor_user_id
		)
		SELECT u.id,TRIM(COALESCE(u.first_name,'')||' '||COALESCE(u.last_name,'')),u.email,
		       COALESCE(m.membership_status,'active'),
		       COALESCE(d.deals_created,0),COALESCE(d.stage_moves,0),COALESCE(d.deals_won,0),COALESCE(d.deals_lost,0),
		       COALESCE(r.won_revenue_base,0)::text,COALESCE(r.won_revenue_captured,0),
		       COALESCE(r.won_revenue_missing_value,0),COALESCE(r.won_revenue_missing_rate,0),
		       COALESCE(w.notes_added,0),COALESCE(w.tasks_created,0),COALESCE(w.tasks_completed,0)
		FROM organization_memberships m
		JOIN users u ON u.id=m.user_id
		LEFT JOIN deal_metrics d ON d.user_id=u.id
		LEFT JOIN revenue_metrics r ON r.user_id=u.id
		LEFT JOIN work_metrics w ON w.user_id=u.id
		WHERE m.organization_id=$1 AND ($4::bigint=0 OR u.id=$4)
		  AND ($4::bigint<>0 OR COALESCE(d.deals_created,0)+COALESCE(d.stage_moves,0)+COALESCE(d.deals_won,0)+COALESCE(d.deals_lost,0)+COALESCE(w.notes_added,0)+COALESCE(w.tasks_created,0)+COALESCE(w.tasks_completed,0)>0)
		ORDER BY COALESCE(d.deals_created,0)+COALESCE(d.stage_moves,0)+COALESCE(w.notes_added,0)+COALESCE(w.tasks_created,0)+COALESCE(w.tasks_completed,0) DESC,u.id
	`, organizationID, from, to, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("load sales owner summaries: %w", err)
	}
	defer rows.Close()
	result := []OwnerSummary{}
	for rows.Next() {
		var item OwnerSummary
		if err := rows.Scan(
			&item.UserID, &item.UserName, &item.Email, &item.Status,
			&item.DealsCreated, &item.StageMoves, &item.DealsWon, &item.DealsLost,
			&item.WonRevenueBase, &item.WonRevenueCaptured, &item.WonRevenueMissingValue, &item.WonRevenueMissingRate,
			&item.NotesAdded, &item.TasksCreated, &item.TasksCompleted,
		); err != nil {
			return nil, fmt.Errorf("scan sales owner summary: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sales owner summaries: %w", err)
	}
	return result, nil
}

func (s *Service) loadStages(ctx context.Context, tx pgx.Tx, organizationID int64, from, to time.Time, ownerUserID int64) ([]StageConversion, error) {
	rows, err := tx.Query(ctx, `
		WITH metrics AS (
			SELECT id,occurred_at,to_pipeline_id AS pipeline_id,to_pipeline_name AS pipeline_name,
			       to_stage_id AS stage_id,to_stage_name AS stage_name,to_stage_position AS stage_position,
			       1 AS entries,0 AS exits,0 AS forward_exits,0 AS won_exits,0 AS lost_exits
			FROM deal_stage_events
			WHERE organization_id=$1 AND occurred_at >= $2 AND occurred_at < $3 AND ($4::bigint=0 OR owner_user_id=$4)
			UNION ALL
			SELECT id,occurred_at,from_pipeline_id,from_pipeline_name,from_stage_id,from_stage_name,from_stage_position,
			       0,1,
			       CASE WHEN to_stage_outcome='won' OR (to_stage_outcome='open' AND to_pipeline_id=from_pipeline_id AND to_stage_position>from_stage_position) THEN 1 ELSE 0 END,
			       CASE WHEN to_stage_outcome='won' AND COALESCE(from_stage_outcome,'')<>'won' THEN 1 ELSE 0 END,
			       CASE WHEN to_stage_outcome='lost' AND COALESCE(from_stage_outcome,'')<>'lost' THEN 1 ELSE 0 END
			FROM deal_stage_events
			WHERE organization_id=$1 AND event_type='stage_changed' AND occurred_at >= $2 AND occurred_at < $3 AND ($4::bigint=0 OR owner_user_id=$4)
		)
		SELECT pipeline_id,(ARRAY_AGG(pipeline_name ORDER BY occurred_at DESC,id DESC))[1],
		       stage_id,(ARRAY_AGG(stage_name ORDER BY occurred_at DESC,id DESC))[1],
		       (ARRAY_AGG(stage_position ORDER BY occurred_at DESC,id DESC))[1],
		       SUM(entries),SUM(exits),SUM(forward_exits),SUM(won_exits),SUM(lost_exits)
		FROM metrics
		GROUP BY pipeline_id,stage_id
		ORDER BY pipeline_id,(ARRAY_AGG(stage_position ORDER BY occurred_at DESC,id DESC))[1],stage_id
	`, organizationID, from, to, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("load stage conversion report: %w", err)
	}
	defer rows.Close()
	result := []StageConversion{}
	for rows.Next() {
		var item StageConversion
		if err := rows.Scan(&item.PipelineID, &item.PipelineName, &item.StageID, &item.StageName, &item.StagePosition, &item.Entries, &item.Exits, &item.ForwardExits, &item.WonExits, &item.LostExits); err != nil {
			return nil, fmt.Errorf("scan stage conversion report: %w", err)
		}
		item.ForwardExitRatePercent = rate(item.ForwardExits, item.Exits)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stage conversion report: %w", err)
	}
	return result, nil
}

func (s *Service) loadEvents(ctx context.Context, tx pgx.Tx, organizationID int64, from, to time.Time, ownerUserID int64) ([]DealEvent, error) {
	rows, err := tx.Query(ctx, `
		SELECT e.id,e.deal_id,e.deal_name,e.event_type,
		       TRIM(COALESCE(actor.first_name,'')||' '||COALESCE(actor.last_name,'')),
		       TRIM(COALESCE(owner_user.first_name,'')||' '||COALESCE(owner_user.last_name,'')),
		       COALESCE(e.from_pipeline_name,''),COALESCE(e.from_stage_name,''),COALESCE(e.from_stage_outcome,''),
		       e.to_pipeline_name,e.to_stage_name,e.to_stage_outcome,
		       COALESCE(e.close_reason_code,''),COALESCE(e.close_reason_label,''),COALESCE(e.close_notes,''),e.occurred_at
		FROM deal_stage_events e
		LEFT JOIN users actor ON actor.id=e.actor_user_id
		LEFT JOIN users owner_user ON owner_user.id=e.owner_user_id
		WHERE e.organization_id=$1 AND e.occurred_at >= $2 AND e.occurred_at < $3
		  AND ($4::bigint=0 OR e.owner_user_id=$4)
		ORDER BY e.occurred_at DESC,e.id DESC
		LIMIT 50
	`, organizationID, from, to, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("load sales deal events: %w", err)
	}
	defer rows.Close()
	result := []DealEvent{}
	for rows.Next() {
		var item DealEvent
		if err := rows.Scan(&item.ID, &item.DealID, &item.DealName, &item.EventType, &item.ActorName, &item.OwnerName, &item.FromPipelineName, &item.FromStageName, &item.FromStageOutcome, &item.ToPipelineName, &item.ToStageName, &item.ToStageOutcome, &item.CloseReasonCode, &item.CloseReasonLabel, &item.CloseNotes, &item.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan sales deal event: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sales deal events: %w", err)
	}
	return result, nil
}

func normalizeQuery(query Query, now time.Time) (time.Time, time.Time, Query, error) {
	query.FromDate = strings.TrimSpace(query.FromDate)
	query.ToDate = strings.TrimSpace(query.ToDate)
	if query.ToDate == "" {
		query.ToDate = now.Format(dateLayout)
	}
	if query.FromDate == "" {
		query.FromDate = now.AddDate(0, 0, -29).Format(dateLayout)
	}
	from, fromErr := time.Parse(dateLayout, query.FromDate)
	to, toErr := time.Parse(dateLayout, query.ToDate)
	if fromErr != nil || toErr != nil || to.Before(from) || to.Sub(from) > 365*24*time.Hour || query.OwnerUserID < 0 {
		return time.Time{}, time.Time{}, Query{}, ErrInvalidInput
	}
	return from.UTC(), to.UTC(), query, nil
}

func rate(numerator, denominator int) string {
	if denominator <= 0 {
		return ""
	}
	return strconv.FormatFloat(float64(numerator)*100/float64(denominator), 'f', 1, 64)
}
