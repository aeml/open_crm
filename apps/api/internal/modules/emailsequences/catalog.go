package emailsequences

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type ListQuery struct {
	Search   string
	Status   string
	Page     int
	PageSize int
}

type ListPage struct {
	Sequences []Sequence `json:"sequences"`
	Page      int        `json:"page"`
	PageSize  int        `json:"pageSize"`
	Total     int        `json:"total"`
}

type sequenceQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// ListByOrganization returns one stable definition page and exact filtered
// total from the same snapshot. Outcome aggregation is limited to the selected
// definitions instead of rescanning every retained sequence on every page.
func (s *Service) ListByOrganization(ctx context.Context, organizationID int64, query ListQuery) (ListPage, error) {
	if s == nil || s.pool == nil || organizationID <= 0 {
		return ListPage{}, fmt.Errorf("email sequences service not configured")
	}
	query, page, err := normalizeListQuery(query)
	if err != nil {
		return ListPage{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ListPage{}, fmt.Errorf("begin email sequence list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	args := []any{organizationID}
	filter := ""
	if query.Status != "all" {
		args = append(args, query.Status)
		filter += fmt.Sprintf(" AND seq.status=$%d", len(args))
	}
	if query.Search != "" {
		args = append(args, "%"+escapeSequenceLike(strings.ToLower(query.Search))+"%")
		filter += fmt.Sprintf(" AND LOWER(seq.name) LIKE $%d ESCAPE E'\\\\'", len(args))
	}

	result := ListPage{Sequences: []Sequence{}, Page: page.Number, PageSize: page.Size}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM email_sequences seq WHERE seq.organization_id=$1`+filter, args...).Scan(&result.Total); err != nil {
		return ListPage{}, fmt.Errorf("count email sequences: %w", err)
	}
	args = append(args, page.Size, page.Offset)
	selection := `
		SELECT seq.id,seq.organization_id,seq.name,seq.description,seq.status,seq.revision,
		       seq.approved_revision,seq.approved_by_user_id,seq.approved_at,seq.created_by_user_id,
		       seq.created_at,seq.updated_at
		FROM email_sequences seq
		WHERE seq.organization_id=$1` + filter + `
		ORDER BY LOWER(seq.name),seq.id
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))
	result.Sequences, err = querySequenceDetails(ctx, tx, selection, args...)
	if err != nil {
		return ListPage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ListPage{}, fmt.Errorf("commit email sequence list: %w", err)
	}
	return result, nil
}

func getSequenceByID(ctx context.Context, querier sequenceQuerier, organizationID, sequenceID int64) (Sequence, error) {
	selection := `
		SELECT seq.id,seq.organization_id,seq.name,seq.description,seq.status,seq.revision,
		       seq.approved_revision,seq.approved_by_user_id,seq.approved_at,seq.created_by_user_id,
		       seq.created_at,seq.updated_at
		FROM email_sequences seq
		WHERE seq.organization_id=$1 AND seq.id=$2`
	sequences, err := querySequenceDetails(ctx, querier, selection, organizationID, sequenceID)
	if err != nil {
		return Sequence{}, err
	}
	if len(sequences) == 0 {
		return Sequence{}, ErrNotFound
	}
	return sequences[0], nil
}

func querySequenceDetails(ctx context.Context, querier sequenceQuerier, selection string, args ...any) ([]Sequence, error) {
	rows, err := querier.Query(ctx, `
		WITH selected_sequences AS MATERIALIZED (`+selection+`
		), enrollment_outcomes AS (
			SELECT enrollment.organization_id,enrollment.sequence_id,
			       COUNT(*) AS enrolled,
			       COUNT(*) FILTER (WHERE enrollment.status='active') AS active,
			       COUNT(*) FILTER (WHERE enrollment.status='paused') AS paused,
			       COUNT(*) FILTER (WHERE enrollment.status='completed' AND enrollment.completion_reason='replied') AS replied,
			       COUNT(*) FILTER (WHERE enrollment.status='completed' AND enrollment.completion_reason='finished') AS cadence_finished,
			       COUNT(*) FILTER (WHERE enrollment.status='completed' AND enrollment.completion_reason='suppressed') AS suppressed_exits,
			       COUNT(*) FILTER (WHERE enrollment.status='completed' AND enrollment.completion_reason IS NULL) AS unclassified_completed,
			       COUNT(*) FILTER (WHERE enrollment.status='cancelled') AS cancelled
			FROM email_sequence_enrollments enrollment
			JOIN selected_sequences selected
			  ON selected.organization_id=enrollment.organization_id AND selected.id=enrollment.sequence_id
			GROUP BY enrollment.organization_id,enrollment.sequence_id
		), delivery_outcomes AS (
			SELECT enrollment.organization_id,enrollment.sequence_id,
			       COUNT(*) FILTER (WHERE delivery.status='sent') AS provider_accepted,
			       COUNT(*) FILTER (WHERE delivery.delivery_outcome='bounced') AS bounced_messages,
			       COUNT(*) FILTER (WHERE delivery.delivery_outcome='complaint') AS complaints,
			       COUNT(*) FILTER (WHERE delivery.status='suppressed') AS suppressed_messages,
			       COUNT(*) FILTER (WHERE delivery.status='queued') AS queued_messages,
			       COUNT(*) FILTER (WHERE delivery.status='uncertain') AS needs_review
			FROM email_sequence_enrollments enrollment
			JOIN selected_sequences selected
			  ON selected.organization_id=enrollment.organization_id AND selected.id=enrollment.sequence_id
			JOIN email_sequence_deliveries delivery
			  ON delivery.organization_id=enrollment.organization_id AND delivery.enrollment_id=enrollment.id
			GROUP BY enrollment.organization_id,enrollment.sequence_id
		)
		SELECT seq.id,seq.name,seq.description,seq.status,seq.revision,COALESCE(seq.approved_revision,0),
		       COALESCE(seq.approved_by_user_id,0),seq.approved_at,COALESCE(seq.created_by_user_id,0),seq.created_at,seq.updated_at,
		       COALESCE(enrollment.enrolled,0),COALESCE(enrollment.active,0),COALESCE(enrollment.paused,0),
		       COALESCE(enrollment.replied,0),COALESCE(enrollment.cadence_finished,0),COALESCE(enrollment.suppressed_exits,0),
		       COALESCE(enrollment.unclassified_completed,0),COALESCE(enrollment.cancelled,0),
		       COALESCE(delivery.provider_accepted,0),COALESCE(delivery.bounced_messages,0),COALESCE(delivery.complaints,0),
		       COALESCE(delivery.suppressed_messages,0),COALESCE(delivery.queued_messages,0),COALESCE(delivery.needs_review,0),
		       COALESCE(step.id,0),COALESCE(step.step_order,0),COALESCE(step.delay_days,0),COALESCE(step.subject,''),COALESCE(step.body,'')
		FROM selected_sequences seq
		LEFT JOIN enrollment_outcomes enrollment
		  ON enrollment.organization_id=seq.organization_id AND enrollment.sequence_id=seq.id
		LEFT JOIN delivery_outcomes delivery
		  ON delivery.organization_id=seq.organization_id AND delivery.sequence_id=seq.id
		LEFT JOIN email_sequence_steps step ON step.sequence_id=seq.id
		ORDER BY LOWER(seq.name),seq.id,step.step_order
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list email sequence details: %w", err)
	}
	defer rows.Close()

	sequences := make([]Sequence, 0)
	indexByID := map[int64]int{}
	for rows.Next() {
		var sequence Sequence
		var step Step
		var approvedAt pgtype.Timestamptz
		if err := rows.Scan(&sequence.ID, &sequence.Name, &sequence.Description, &sequence.Status, &sequence.Revision, &sequence.ApprovedRevision,
			&sequence.ApprovedByUserID, &approvedAt, &sequence.CreatedByUserID, &sequence.CreatedAt, &sequence.UpdatedAt,
			&sequence.Outcomes.Enrolled, &sequence.Outcomes.Active, &sequence.Outcomes.Paused, &sequence.Outcomes.Replied,
			&sequence.Outcomes.CadenceFinished, &sequence.Outcomes.SuppressedExits, &sequence.Outcomes.UnclassifiedCompleted,
			&sequence.Outcomes.Cancelled, &sequence.Outcomes.ProviderAccepted, &sequence.Outcomes.BouncedMessages,
			&sequence.Outcomes.Complaints, &sequence.Outcomes.SuppressedMessages, &sequence.Outcomes.QueuedMessages,
			&sequence.Outcomes.NeedsReview, &step.ID, &step.StepOrder, &step.DelayDays, &step.Subject, &step.Body); err != nil {
			return nil, fmt.Errorf("scan email sequence: %w", err)
		}
		if approvedAt.Valid {
			value := approvedAt.Time
			sequence.ApprovedAt = &value
		}
		index, exists := indexByID[sequence.ID]
		if !exists {
			sequence.Steps = []Step{}
			sequences = append(sequences, sequence)
			index = len(sequences) - 1
			indexByID[sequence.ID] = index
		}
		if step.ID > 0 {
			sequences[index].Steps = append(sequences[index].Steps, step)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email sequences: %w", err)
	}
	return sequences, nil
}

func normalizeListQuery(query ListQuery) (ListQuery, platformpagination.Page, error) {
	query.Search = strings.TrimSpace(query.Search)
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))
	if query.Status == "" {
		query.Status = "all"
	}
	if utf8.RuneCountInString(query.Search) > MaxListSearchLength ||
		(query.Status != "all" && query.Status != "draft" && query.Status != "active" && query.Status != "paused") {
		return ListQuery{}, platformpagination.Page{}, ErrInvalidInput
	}
	page, err := platformpagination.Normalize(query.Page, query.PageSize, DefaultListPageSize)
	if err != nil {
		return ListQuery{}, platformpagination.Page{}, ErrInvalidInput
	}
	query.Page, query.PageSize = page.Number, page.Size
	return query, page, nil
}

func escapeSequenceLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
