package calendar

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ListAvailability(ctx context.Context, organizationID, userID int64) ([]AvailabilityBlock, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("calendar service not configured")
	}
	if organizationID <= 0 || userID <= 0 {
		return nil, ErrInvalidInput
	}
	queryCtx, cancel := context.WithTimeout(ctx, calendarCatalogQueryTimeout)
	defer cancel()
	rows, err := s.pool.Query(queryCtx, `
		SELECT id, day_of_week, start_minute, end_minute, timezone, created_at, updated_at
		FROM calendar_availability_blocks
		WHERE organization_id = $1 AND user_id = $2
		ORDER BY day_of_week ASC, start_minute ASC, id ASC
	`, organizationID, userID)
	if err != nil {
		return nil, mapCalendarQueryError("list calendar availability", err)
	}
	defer rows.Close()
	blocks := make([]AvailabilityBlock, 0)
	for rows.Next() {
		var block AvailabilityBlock
		if err := rows.Scan(&block.ID, &block.DayOfWeek, &block.StartMinute, &block.EndMinute, &block.Timezone, &block.CreatedAt, &block.UpdatedAt); err != nil {
			return nil, mapCalendarQueryError("scan calendar availability", err)
		}
		blocks = append(blocks, block)
	}
	if err := rows.Err(); err != nil {
		return nil, mapCalendarQueryError("iterate calendar availability", err)
	}
	return blocks, nil
}

func (s *Service) SetAvailability(ctx context.Context, organizationID, userID int64, input AvailabilityInput) ([]AvailabilityBlock, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("calendar service not configured")
	}
	if organizationID <= 0 || userID <= 0 || len(input.Blocks) > MaxAvailabilityBlocksPerUser {
		return nil, ErrInvalidInput
	}
	blocks := normalizeAvailabilityBlocks(input.Blocks)
	seen := make(map[string]struct{}, len(blocks))
	for _, block := range blocks {
		if block.DayOfWeek < 0 || block.DayOfWeek > 6 || block.StartMinute < 0 || block.EndMinute > 1440 || block.StartMinute >= block.EndMinute || block.Timezone == "" || utf8.RuneCountInString(block.Timezone) > MaxCalendarTimezoneLength {
			return nil, ErrInvalidInput
		}
		key := fmt.Sprintf("%d/%d/%d", block.DayOfWeek, block.StartMinute, block.EndMinute)
		if _, duplicate := seen[key]; duplicate {
			return nil, ErrInvalidInput
		}
		seen[key] = struct{}{}
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].DayOfWeek != blocks[j].DayOfWeek {
			return blocks[i].DayOfWeek < blocks[j].DayOfWeek
		}
		if blocks[i].StartMinute != blocks[j].StartMinute {
			return blocks[i].StartMinute < blocks[j].StartMinute
		}
		return blocks[i].EndMinute < blocks[j].EndMinute
	})

	queryCtx, cancel := context.WithTimeout(ctx, calendarCatalogQueryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, mapCalendarQueryError("begin calendar availability transaction", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := lockCalendarWriter(queryCtx, tx, organizationID, userID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(queryCtx, `DELETE FROM calendar_availability_blocks WHERE organization_id = $1 AND user_id = $2`, organizationID, userID); err != nil {
		return nil, mapCalendarQueryError("clear calendar availability", err)
	}
	result := make([]AvailabilityBlock, 0, len(blocks))
	for _, block := range blocks {
		var saved AvailabilityBlock
		if err := tx.QueryRow(queryCtx, `
			INSERT INTO calendar_availability_blocks (organization_id, user_id, day_of_week, start_minute, end_minute, timezone)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, day_of_week, start_minute, end_minute, timezone, created_at, updated_at
		`, organizationID, userID, block.DayOfWeek, block.StartMinute, block.EndMinute, block.Timezone).Scan(
			&saved.ID, &saved.DayOfWeek, &saved.StartMinute, &saved.EndMinute, &saved.Timezone, &saved.CreatedAt, &saved.UpdatedAt,
		); err != nil {
			return nil, mapCalendarQueryError("insert calendar availability", err)
		}
		result = append(result, saved)
	}
	if err := tx.Commit(queryCtx); err != nil {
		return nil, mapCalendarQueryError("commit calendar availability transaction", err)
	}
	return result, nil
}

func normalizeAvailabilityBlocks(blocks []AvailabilityBlockInput) []AvailabilityBlockInput {
	out := make([]AvailabilityBlockInput, 0, len(blocks))
	for _, block := range blocks {
		block.Timezone = strings.TrimSpace(block.Timezone)
		if block.Timezone == "" {
			block.Timezone = "UTC"
		}
		out = append(out, block)
	}
	return out
}
