package salesreports

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const funnelQueryTimeout = 5 * time.Second

var ErrQueryTimeout = errors.New("sales report query timed out")

var FunnelSemantics = []string{
	"The cohort contains deals created in the selected pipeline and exact entry stage during the inclusive UTC cohort dates. A deal is counted once.",
	"The owner filter uses the owner saved on the deal-creation event, so later reassignment does not rewrite cohort membership.",
	"Outcomes and current-stage counts use each cohort deal's latest retained stage event through the inclusive UTC as-of date.",
	"Stage reach counts an exact visit to that stable stage ID. Skipped stages are not inferred, and a deal that re-enters a stage still counts once in reach.",
	"Forward-or-won exits use the stage positions and outcomes saved on the exit event. A move to another pipeline is not credited as forward progress in this pipeline.",
	"Median time to reach uses each deal's first exact visit. Median time in stage uses completed visits; re-entry can contribute another completed visit. Durations are elapsed 24-hour days, not calendar-day labels.",
	"Cohorts observed for less time can show lower conversion simply because they are less mature. Results do not predict eventual conversion.",
	"Stage labels and ordering are the pipeline's current configuration; historical entry, exit, owner, position, and outcome math remains event-time evidence.",
}

type FunnelQuery struct {
	PipelineID   int64
	EntryStageID int64
	FromDate     string
	ToDate       string
	AsOfDate     string
	OwnerUserID  int64
}

type FunnelTotals struct {
	CohortDeals       int    `json:"cohortDeals"`
	OpenDeals         int    `json:"openDeals"`
	WonDeals          int    `json:"wonDeals"`
	LostDeals         int    `json:"lostDeals"`
	ClosedDeals       int    `json:"closedDeals"`
	MovedOutOpenDeals int    `json:"movedOutOpenDeals"`
	WinRatePercent    string `json:"winRatePercent"`
	MedianDaysToWin   string `json:"medianDaysToWin"`
}

type FunnelStage struct {
	StageID                    int64  `json:"stageId"`
	StageName                  string `json:"stageName"`
	StagePosition              int    `json:"stagePosition"`
	StageOutcome               string `json:"stageOutcome"`
	ReachedDeals               int    `json:"reachedDeals"`
	ReachRatePercent           string `json:"reachRatePercent"`
	CurrentlyInStageDeals      int    `json:"currentlyInStageDeals"`
	ExitedDeals                int    `json:"exitedDeals"`
	ForwardOrWonDeals          int    `json:"forwardOrWonDeals"`
	ForwardExitRatePercent     string `json:"forwardExitRatePercent"`
	LostExitDeals              int    `json:"lostExitDeals"`
	MedianDaysToReach          string `json:"medianDaysToReach"`
	MedianDaysInCompletedVisit string `json:"medianDaysInCompletedVisit"`
}

type FunnelReport struct {
	PipelineID        int64         `json:"pipelineId"`
	PipelineName      string        `json:"pipelineName"`
	EntryStageID      int64         `json:"entryStageId"`
	EntryStageName    string        `json:"entryStageName"`
	FromDate          string        `json:"fromDate"`
	ToDate            string        `json:"toDate"`
	AsOfDate          string        `json:"asOfDate"`
	OwnerUserID       int64         `json:"ownerUserId"`
	GeneratedAt       time.Time     `json:"generatedAt"`
	CoverageStartedAt time.Time     `json:"coverageStartedAt"`
	HistoryComplete   bool          `json:"historyComplete"`
	Totals            FunnelTotals  `json:"totals"`
	Stages            []FunnelStage `json:"stages"`
	Semantics         []string      `json:"semantics"`
}

func (s *Service) Funnel(ctx context.Context, organizationID int64, query FunnelQuery) (FunnelReport, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return FunnelReport{}, fmt.Errorf("sales reports service not configured")
	}
	from, toExclusive, asOfExclusive, query, err := normalizeFunnelQuery(query, time.Now().UTC())
	if err != nil {
		return FunnelReport{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, funnelQueryTimeout)
	defer cancel()

	if query.OwnerUserID > 0 {
		var exists bool
		if err := s.pool.QueryRow(queryCtx, `SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id=$1 AND user_id=$2)`, organizationID, query.OwnerUserID).Scan(&exists); err != nil {
			return FunnelReport{}, salesReportQueryError(queryCtx, "validate funnel owner", err)
		}
		if !exists {
			return FunnelReport{}, ErrInvalidInput
		}
	}

	var pipelineName, entryStageName string
	if err := s.pool.QueryRow(queryCtx, `
		SELECT pipeline.name,stage.name
		FROM deal_pipelines pipeline
		JOIN deal_stages stage ON stage.organization_id=pipeline.organization_id AND stage.pipeline_id=pipeline.id AND stage.id=$3
		WHERE pipeline.organization_id=$1 AND pipeline.id=$2
	`, organizationID, query.PipelineID, query.EntryStageID).Scan(&pipelineName, &entryStageName); errors.Is(err, pgx.ErrNoRows) {
		return FunnelReport{}, ErrInvalidInput
	} else if err != nil {
		return FunnelReport{}, salesReportQueryError(queryCtx, "validate funnel pipeline and entry stage", err)
	}

	var coverageStartedAt, organizationCreatedAt time.Time
	if err := s.pool.QueryRow(queryCtx, `
		SELECT COALESCE(sales_activity_tracking_started_at,created_at),created_at
		FROM organizations WHERE id=$1
	`, organizationID).Scan(&coverageStartedAt, &organizationCreatedAt); errors.Is(err, pgx.ErrNoRows) {
		return FunnelReport{}, ErrInvalidInput
	} else if err != nil {
		return FunnelReport{}, salesReportQueryError(queryCtx, "load funnel coverage", err)
	}

	report := FunnelReport{
		PipelineID: query.PipelineID, PipelineName: pipelineName,
		EntryStageID: query.EntryStageID, EntryStageName: entryStageName,
		FromDate: query.FromDate, ToDate: query.ToDate, AsOfDate: query.AsOfDate,
		OwnerUserID: query.OwnerUserID, GeneratedAt: time.Now().UTC(), CoverageStartedAt: coverageStartedAt,
		HistoryComplete: !coverageStartedAt.After(organizationCreatedAt) || !from.Before(coverageStartedAt),
		Stages:          []FunnelStage{}, Semantics: append([]string(nil), FunnelSemantics...),
	}

	rows, err := s.pool.Query(queryCtx, funnelSQL, organizationID, query.PipelineID, query.EntryStageID, from, toExclusive, query.OwnerUserID, asOfExclusive)
	if err != nil {
		return FunnelReport{}, salesReportQueryError(queryCtx, "load pipeline funnel", err)
	}
	defer rows.Close()
	for rows.Next() {
		var stage FunnelStage
		var medianDaysToReach, medianDaysInStage, medianDaysToWin float64
		if err := rows.Scan(
			&stage.StageID, &stage.StageName, &stage.StagePosition, &stage.StageOutcome,
			&stage.ReachedDeals, &stage.CurrentlyInStageDeals, &stage.ExitedDeals,
			&stage.ForwardOrWonDeals, &stage.LostExitDeals, &medianDaysToReach, &medianDaysInStage,
			&report.Totals.CohortDeals, &report.Totals.OpenDeals, &report.Totals.WonDeals,
			&report.Totals.LostDeals, &report.Totals.MovedOutOpenDeals, &medianDaysToWin,
		); err != nil {
			return FunnelReport{}, fmt.Errorf("scan pipeline funnel: %w", err)
		}
		stage.ReachRatePercent = rate(stage.ReachedDeals, report.Totals.CohortDeals)
		stage.ForwardExitRatePercent = rate(stage.ForwardOrWonDeals, stage.ExitedDeals)
		stage.MedianDaysToReach = durationDays(medianDaysToReach)
		stage.MedianDaysInCompletedVisit = durationDays(medianDaysInStage)
		report.Totals.MedianDaysToWin = durationDays(medianDaysToWin)
		report.Stages = append(report.Stages, stage)
	}
	if err := rows.Err(); err != nil {
		return FunnelReport{}, salesReportQueryError(queryCtx, "iterate pipeline funnel", err)
	}
	report.Totals.ClosedDeals = report.Totals.WonDeals + report.Totals.LostDeals
	report.Totals.WinRatePercent = rate(report.Totals.WonDeals, report.Totals.ClosedDeals)
	return report, nil
}

func normalizeFunnelQuery(query FunnelQuery, now time.Time) (time.Time, time.Time, time.Time, FunnelQuery, error) {
	query.FromDate = strings.TrimSpace(query.FromDate)
	query.ToDate = strings.TrimSpace(query.ToDate)
	query.AsOfDate = strings.TrimSpace(query.AsOfDate)
	if query.PipelineID <= 0 || query.EntryStageID <= 0 || query.OwnerUserID < 0 {
		return time.Time{}, time.Time{}, time.Time{}, FunnelQuery{}, ErrInvalidInput
	}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if query.FromDate == "" && query.ToDate == "" {
		query.FromDate = today.AddDate(0, 0, -29).Format(dateLayout)
		query.ToDate = today.Format(dateLayout)
	} else if query.FromDate == "" || query.ToDate == "" {
		return time.Time{}, time.Time{}, time.Time{}, FunnelQuery{}, fmt.Errorf("%w: from and to must be provided together", ErrInvalidInput)
	}
	if query.AsOfDate == "" {
		query.AsOfDate = today.Format(dateLayout)
	}
	from, fromErr := time.Parse(dateLayout, query.FromDate)
	to, toErr := time.Parse(dateLayout, query.ToDate)
	asOf, asOfErr := time.Parse(dateLayout, query.AsOfDate)
	if fromErr != nil || toErr != nil || asOfErr != nil || to.Before(from) || asOf.Before(to) || asOf.After(today) || asOf.Sub(from) > 365*24*time.Hour {
		return time.Time{}, time.Time{}, time.Time{}, FunnelQuery{}, fmt.Errorf("%w: dates must be an ordered inclusive UTC observation window of at most 366 days ending no later than today", ErrInvalidInput)
	}
	return from, to.AddDate(0, 0, 1), asOf.AddDate(0, 0, 1), query, nil
}

func salesReportQueryError(ctx context.Context, operation string, err error) error {
	if errors.Is(err, ErrQueryTimeout) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrQueryTimeout
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func durationDays(value float64) string {
	if value < 0 {
		return ""
	}
	return strconv.FormatFloat(value, 'f', 1, 64)
}

const funnelSQL = `
	WITH cohort_candidates AS (
		SELECT event.id event_id,event.deal_id,event.occurred_at created_at
		FROM deal_stage_events event
		WHERE event.organization_id=$1 AND event.event_type='created'
		  AND event.to_pipeline_id=$2 AND event.to_stage_id=$3
		  AND event.occurred_at >= $4 AND event.occurred_at < $5
		  AND ($6::bigint=0 OR event.owner_user_id=$6)
	), cohort AS (
		SELECT DISTINCT ON (deal_id) event_id,deal_id,created_at
		FROM cohort_candidates ORDER BY deal_id,created_at,event_id
	), scoped_events AS (
		SELECT event.*
		FROM deal_stage_events event
		JOIN cohort ON cohort.deal_id=event.deal_id
		WHERE event.organization_id=$1 AND event.occurred_at < $7
		  AND (event.occurred_at,event.id) >= (cohort.created_at,cohort.event_id)
	), sequenced AS (
		SELECT event.*,
		       LEAD(event.occurred_at) OVER deal_events next_occurred_at,
		       LEAD(event.from_stage_id) OVER deal_events next_from_stage_id,
		       LEAD(event.from_stage_position) OVER deal_events next_from_stage_position,
		       LEAD(event.to_pipeline_id) OVER deal_events next_to_pipeline_id,
		       LEAD(event.to_stage_position) OVER deal_events next_to_stage_position,
		       LEAD(event.to_stage_outcome) OVER deal_events next_to_stage_outcome,
		       ROW_NUMBER() OVER (PARTITION BY event.deal_id ORDER BY event.occurred_at DESC,event.id DESC) latest_rank
		FROM scoped_events event
		WINDOW deal_events AS (PARTITION BY event.deal_id ORDER BY event.occurred_at,event.id)
	), latest AS (
		SELECT * FROM sequenced WHERE latest_rank=1
	), visits AS (
		SELECT deal_id,to_stage_id stage_id,occurred_at entered_at,
		       CASE WHEN next_from_stage_id=to_stage_id THEN next_occurred_at END exited_at,
		       CASE WHEN next_from_stage_id=to_stage_id AND (
		         next_to_stage_outcome='won' OR
		         (next_to_stage_outcome='open' AND next_to_pipeline_id=to_pipeline_id AND next_to_stage_position>next_from_stage_position)
		       ) THEN TRUE ELSE FALSE END forward_or_won,
		       CASE WHEN next_from_stage_id=to_stage_id AND next_to_stage_outcome='lost' THEN TRUE ELSE FALSE END lost_exit
		FROM sequenced
	), first_visits AS (
		SELECT deal_id,stage_id,MIN(entered_at) first_entered_at
		FROM visits GROUP BY deal_id,stage_id
	), reach_summary AS (
		SELECT visit.stage_id,COUNT(*)::int reached_deals,
		       COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (visit.first_entered_at-cohort.created_at))/86400.0),-1) median_days_to_reach
		FROM first_visits visit JOIN cohort ON cohort.deal_id=visit.deal_id
		GROUP BY visit.stage_id
	), visit_summary AS (
		SELECT stage_id,
		       COUNT(DISTINCT deal_id) FILTER (WHERE exited_at IS NOT NULL)::int exited_deals,
		       COUNT(DISTINCT deal_id) FILTER (WHERE forward_or_won)::int forward_or_won_deals,
		       COUNT(DISTINCT deal_id) FILTER (WHERE lost_exit)::int lost_exit_deals,
		       COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (exited_at-entered_at))/86400.0) FILTER (WHERE exited_at IS NOT NULL),-1) median_days_in_stage
		FROM visits GROUP BY stage_id
	), current_summary AS (
		SELECT to_stage_id stage_id,COUNT(*)::int current_deals FROM latest GROUP BY to_stage_id
	), totals AS (
		SELECT COUNT(cohort.deal_id)::int cohort_deals,
		       COUNT(cohort.deal_id) FILTER (WHERE latest.to_stage_outcome='open')::int open_deals,
		       COUNT(cohort.deal_id) FILTER (WHERE latest.to_stage_outcome='won')::int won_deals,
		       COUNT(cohort.deal_id) FILTER (WHERE latest.to_stage_outcome='lost')::int lost_deals,
		       COUNT(cohort.deal_id) FILTER (WHERE latest.to_stage_outcome='open' AND latest.to_pipeline_id<>$2)::int moved_out_open_deals,
		       COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (latest.occurred_at-cohort.created_at))/86400.0) FILTER (WHERE latest.to_stage_outcome='won'),-1) median_days_to_win
		FROM cohort LEFT JOIN latest ON latest.deal_id=cohort.deal_id
	)
	SELECT stage.id,stage.name,stage.position,
	       CASE WHEN stage.is_closed AND stage.is_won THEN 'won' WHEN stage.is_closed THEN 'lost' ELSE 'open' END stage_outcome,
	       COALESCE(reach.reached_deals,0),COALESCE(current_stage.current_deals,0),COALESCE(visit.exited_deals,0),
	       COALESCE(visit.forward_or_won_deals,0),COALESCE(visit.lost_exit_deals,0),
	       COALESCE(reach.median_days_to_reach,-1),COALESCE(visit.median_days_in_stage,-1),
	       totals.cohort_deals,totals.open_deals,totals.won_deals,totals.lost_deals,totals.moved_out_open_deals,totals.median_days_to_win
	FROM deal_stages stage CROSS JOIN totals
	LEFT JOIN reach_summary reach ON reach.stage_id=stage.id
	LEFT JOIN visit_summary visit ON visit.stage_id=stage.id
	LEFT JOIN current_summary current_stage ON current_stage.stage_id=stage.id
	WHERE stage.organization_id=$1 AND stage.pipeline_id=$2
	ORDER BY stage.position,stage.id`
