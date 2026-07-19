package duplicateoperations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConflict            = errors.New("duplicate merge conflict")
	ErrIdempotencyConflict = errors.New("idempotency key was already used for a different merge")
	ErrInactiveActor       = errors.New("actor is not an active organization member")
	ErrInvalidInput        = errors.New("invalid duplicate merge input")
	ErrNotFound            = errors.New("duplicate record not found")
)

type Field struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Value        string `json:"value"`
	DisplayValue string `json:"displayValue"`
	Selectable   bool   `json:"selectable"`
}

type Record struct {
	ID        int64          `json:"id"`
	Label     string         `json:"label"`
	Fields    []Field        `json:"fields"`
	Related   map[string]int `json:"related"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type Candidate struct {
	EntityType string   `json:"entityType"`
	Reasons    []string `json:"reasons"`
	First      Record   `json:"first"`
	Second     Record   `json:"second"`
}

type MergeOperation struct {
	ID                     int64          `json:"id"`
	EntityType             string         `json:"entityType"`
	SourceEntityID         int64          `json:"sourceEntityId"`
	SourceLabel            string         `json:"sourceLabel"`
	TargetEntityID         int64          `json:"targetEntityId"`
	TargetLabel            string         `json:"targetLabel"`
	SourceFields           []string       `json:"sourceFields"`
	RelationshipCounts     map[string]int `json:"relationshipCounts"`
	TargetAppliedUpdatedAt time.Time      `json:"targetAppliedUpdatedAt"`
	CreatedAt              time.Time      `json:"createdAt"`
	Replayed               bool           `json:"replayed"`
}

type Review struct {
	Candidates   []Candidate      `json:"candidates"`
	RecentMerges []MergeOperation `json:"recentMerges"`
}

type MergeInput struct {
	OrganizationID  int64
	ActorUserID     int64
	EntityType      string
	SourceEntityID  int64
	TargetEntityID  int64
	SourceFields    []string
	SourceUpdatedAt time.Time
	TargetUpdatedAt time.Time
	IdempotencyKey  string
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Review(ctx context.Context, organizationID int64, entityType string, limit int) (Review, error) {
	if s == nil || s.pool == nil {
		return Review{}, fmt.Errorf("duplicate operations service not configured")
	}
	entityType = normalizeEntityType(entityType)
	if entityType == "" || organizationID <= 0 {
		return Review{}, fmt.Errorf("%w: entityType must be contact or company", ErrInvalidInput)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	pairs, err := s.listPairs(ctx, organizationID, entityType, limit)
	if err != nil {
		return Review{}, err
	}
	ids := uniquePairIDs(pairs)
	records, err := s.loadRecords(ctx, organizationID, entityType, ids)
	if err != nil {
		return Review{}, err
	}
	if err := s.loadRelatedCounts(ctx, organizationID, entityType, ids, records); err != nil {
		return Review{}, err
	}
	candidates := make([]Candidate, 0, len(pairs))
	for _, pair := range pairs {
		first, firstOK := records[pair.FirstID]
		second, secondOK := records[pair.SecondID]
		if firstOK && secondOK {
			candidates = append(candidates, Candidate{EntityType: entityType, Reasons: pair.Reasons, First: first, Second: second})
		}
	}
	merges, err := s.listMerges(ctx, organizationID, entityType, 10)
	if err != nil {
		return Review{}, err
	}
	return Review{Candidates: candidates, RecentMerges: merges}, nil
}

type candidatePair struct {
	FirstID  int64
	SecondID int64
	Reasons  []string
}

func (s *Service) listPairs(ctx context.Context, organizationID int64, entityType string, limit int) ([]candidatePair, error) {
	query := contactPairQuery
	if entityType == "company" {
		query = companyPairQuery
	}
	rows, err := s.pool.Query(ctx, query, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list duplicate %s candidates: %w", entityType, err)
	}
	defer rows.Close()
	pairs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (candidatePair, error) {
		var pair candidatePair
		err := row.Scan(&pair.FirstID, &pair.SecondID, &pair.Reasons)
		return pair, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan duplicate %s candidates: %w", entityType, err)
	}
	return pairs, nil
}

const contactPairQuery = `
	SELECT first.id, second.id,
	       array_remove(ARRAY[
	         CASE WHEN NULLIF(lower(trim(first.email)), '') = NULLIF(lower(trim(second.email)), '') THEN 'matching email' END,
	         CASE WHEN length(regexp_replace(COALESCE(first.phone, ''), '[^0-9]', '', 'g')) >= 7
	                    AND regexp_replace(COALESCE(first.phone, ''), '[^0-9]', '', 'g') = regexp_replace(COALESCE(second.phone, ''), '[^0-9]', '', 'g') THEN 'matching phone' END,
	         CASE WHEN lower(trim(first.first_name)) = lower(trim(second.first_name))
	                    AND lower(trim(first.last_name)) = lower(trim(second.last_name)) THEN 'same name' END
	       ], NULL)::text[] AS reasons
	FROM contacts first
	JOIN contacts second ON second.organization_id = first.organization_id AND second.id > first.id
	WHERE first.organization_id = $1
	  AND first.archived_at IS NULL AND second.archived_at IS NULL
	  AND (
	    (NULLIF(lower(trim(first.email)), '') IS NOT NULL AND lower(trim(first.email)) = lower(trim(second.email))) OR
	    (length(regexp_replace(COALESCE(first.phone, ''), '[^0-9]', '', 'g')) >= 7 AND regexp_replace(COALESCE(first.phone, ''), '[^0-9]', '', 'g') = regexp_replace(COALESCE(second.phone, ''), '[^0-9]', '', 'g')) OR
	    (lower(trim(first.first_name)) = lower(trim(second.first_name)) AND lower(trim(first.last_name)) = lower(trim(second.last_name)))
	  )
	ORDER BY CASE WHEN NULLIF(lower(trim(first.email)), '') = NULLIF(lower(trim(second.email)), '') THEN 1 WHEN regexp_replace(COALESCE(first.phone, ''), '[^0-9]', '', 'g') = regexp_replace(COALESCE(second.phone, ''), '[^0-9]', '', 'g') THEN 2 ELSE 3 END,
	         first.id, second.id
	LIMIT $2`

const companyPairQuery = `
	SELECT first.id, second.id,
	       array_remove(ARRAY[
	         CASE WHEN NULLIF(lower(trim(first.website)), '') = NULLIF(lower(trim(second.website)), '') THEN 'matching website' END,
	         CASE WHEN length(regexp_replace(COALESCE(first.phone, ''), '[^0-9]', '', 'g')) >= 7
	                    AND regexp_replace(COALESCE(first.phone, ''), '[^0-9]', '', 'g') = regexp_replace(COALESCE(second.phone, ''), '[^0-9]', '', 'g') THEN 'matching phone' END,
	         CASE WHEN lower(trim(first.name)) = lower(trim(second.name)) THEN 'same name' END
	       ], NULL)::text[] AS reasons
	FROM companies first
	JOIN companies second ON second.organization_id = first.organization_id AND second.id > first.id
	WHERE first.organization_id = $1
	  AND first.archived_at IS NULL AND second.archived_at IS NULL
	  AND (
	    (NULLIF(lower(trim(first.website)), '') IS NOT NULL AND lower(trim(first.website)) = lower(trim(second.website))) OR
	    (length(regexp_replace(COALESCE(first.phone, ''), '[^0-9]', '', 'g')) >= 7 AND regexp_replace(COALESCE(first.phone, ''), '[^0-9]', '', 'g') = regexp_replace(COALESCE(second.phone, ''), '[^0-9]', '', 'g')) OR
	    lower(trim(first.name)) = lower(trim(second.name))
	  )
	ORDER BY CASE WHEN NULLIF(lower(trim(first.website)), '') = NULLIF(lower(trim(second.website)), '') THEN 1 WHEN regexp_replace(COALESCE(first.phone, ''), '[^0-9]', '', 'g') = regexp_replace(COALESCE(second.phone, ''), '[^0-9]', '', 'g') THEN 2 ELSE 3 END,
	         first.id, second.id
	LIMIT $2`

func uniquePairIDs(pairs []candidatePair) []int64 {
	seen := map[int64]struct{}{}
	for _, pair := range pairs {
		seen[pair.FirstID] = struct{}{}
		seen[pair.SecondID] = struct{}{}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func makeField(key, label, value string) Field {
	display := strings.TrimSpace(value)
	if display == "" {
		display = "Not set"
	}
	return Field{Key: key, Label: label, Value: value, DisplayValue: display, Selectable: true}
}

func makeReadOnlyField(key, label, value string) Field {
	field := makeField(key, label, value)
	field.Selectable = false
	return field
}

func boolString(value bool) string { return strconv.FormatBool(value) }

func decodeOperationJSON(sourceFieldsJSON, countsJSON []byte, operation *MergeOperation) error {
	if err := json.Unmarshal(sourceFieldsJSON, &operation.SourceFields); err != nil {
		return err
	}
	if err := json.Unmarshal(countsJSON, &operation.RelationshipCounts); err != nil {
		return err
	}
	if operation.SourceFields == nil {
		operation.SourceFields = []string{}
	}
	if operation.RelationshipCounts == nil {
		operation.RelationshipCounts = map[string]int{}
	}
	return nil
}
