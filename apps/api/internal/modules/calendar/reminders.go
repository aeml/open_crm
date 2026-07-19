package calendar

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	defaultReminderMinutes = 15
	defaultReminderLimit   = 25
	ReminderJobType        = "calendar.reminder"
)

type ReminderSummary struct {
	Attempted int
	Sent      int
}

type dueReminder struct {
	ID             int64
	OrganizationID int64
	EventID        int64
	UserID         int64
	EntityType     string
	EntityID       int64
	Title          string
	StartAt        time.Time
}

func (s *Service) Configured() bool {
	return s != nil && s.pool != nil
}

func (s *Service) SendDueReminders(ctx context.Context, now time.Time, limit int) (ReminderSummary, error) {
	if !s.Configured() {
		return ReminderSummary{}, fmt.Errorf("calendar service not configured")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if limit <= 0 || limit > 100 {
		limit = defaultReminderLimit
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReminderSummary{}, fmt.Errorf("begin calendar reminder transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	reminders, err := selectDueReminders(ctx, tx, now, limit)
	if err != nil {
		return ReminderSummary{}, err
	}
	summary := ReminderSummary{Attempted: len(reminders)}
	for _, reminder := range reminders {
		if err := deliverReminder(ctx, tx, reminder, now); err != nil {
			return summary, err
		}
		summary.Sent++
	}
	if err := tx.Commit(ctx); err != nil {
		return ReminderSummary{}, fmt.Errorf("commit calendar reminder transaction: %w", err)
	}
	return summary, nil
}

func createDefaultReminder(ctx context.Context, tx pgx.Tx, organizationID, eventID, userID int64, startAt time.Time) error {
	if organizationID <= 0 || eventID <= 0 || userID <= 0 || startAt.IsZero() {
		return ErrInvalidInput
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO calendar_event_reminders (organization_id, calendar_event_id, user_id, reminder_minutes, remind_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (calendar_event_id, user_id, reminder_minutes) DO NOTHING
	`, organizationID, eventID, userID, defaultReminderMinutes, reminderTime(startAt, defaultReminderMinutes))
	if err != nil {
		return fmt.Errorf("create calendar reminder: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO background_jobs (organization_id, job_type, idempotency_key, payload_json, max_attempts, run_at)
		SELECT organization_id, $4, 'reminder:' || id::text, jsonb_build_object('reminderId', id::text), 5, remind_at
		FROM calendar_event_reminders
		WHERE organization_id = $1 AND calendar_event_id = $2 AND user_id = $3 AND reminder_minutes = 15
		ON CONFLICT (organization_id, job_type, idempotency_key) DO NOTHING
	`, organizationID, eventID, userID, ReminderJobType)
	if err != nil {
		return fmt.Errorf("enqueue calendar reminder job: %w", err)
	}
	return nil
}

// DeliverReminderJob performs one idempotent reminder delivery. A duplicate or
// stale job is a successful no-op because the reminder row is the effect ledger.
func (s *Service) DeliverReminderJob(ctx context.Context, organizationID int64, payload map[string]any) (map[string]any, error) {
	if !s.Configured() {
		return nil, fmt.Errorf("calendar service not configured")
	}
	reminderID, err := reminderIDFromPayload(payload)
	if err != nil || organizationID <= 0 {
		return nil, fmt.Errorf("%w: invalid calendar reminder job payload", ErrInvalidInput)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin calendar reminder job: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	reminder, err := selectReminderByID(ctx, tx, organizationID, reminderID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return map[string]any{"delivered": false, "reminderId": strconv.FormatInt(reminderID, 10)}, nil
		}
		return nil, err
	}
	if err := deliverReminder(ctx, tx, reminder, time.Now().UTC()); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit calendar reminder job: %w", err)
	}
	return map[string]any{"delivered": true, "reminderId": strconv.FormatInt(reminderID, 10)}, nil
}

func reminderIDFromPayload(payload map[string]any) (int64, error) {
	value, ok := payload["reminderId"].(string)
	if !ok {
		return 0, fmt.Errorf("calendar reminder id is missing")
	}
	reminderID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || reminderID <= 0 {
		return 0, fmt.Errorf("calendar reminder id is invalid")
	}
	return reminderID, nil
}

func skipPendingReminders(ctx context.Context, tx pgx.Tx, organizationID, eventID int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE calendar_event_reminders
		SET status = 'skipped', updated_at = NOW()
		WHERE organization_id = $1 AND calendar_event_id = $2 AND status = 'pending'
	`, organizationID, eventID)
	if err != nil {
		return fmt.Errorf("skip calendar reminders: %w", err)
	}
	return nil
}

func reminderTime(startAt time.Time, minutes int) time.Time {
	return startAt.Add(-time.Duration(minutes) * time.Minute)
}

func selectDueReminders(ctx context.Context, tx pgx.Tx, now time.Time, limit int) ([]dueReminder, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.organization_id, r.calendar_event_id, r.user_id, e.entity_type, e.entity_id, e.title, e.start_at
		FROM calendar_event_reminders r
		JOIN calendar_events e ON e.organization_id = r.organization_id AND e.id = r.calendar_event_id
		WHERE r.status = 'pending' AND r.remind_at <= $1 AND e.status = 'scheduled'
		ORDER BY r.remind_at ASC, r.id ASC
		LIMIT $2
		FOR UPDATE OF r SKIP LOCKED
	`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due calendar reminders: %w", err)
	}
	defer rows.Close()

	reminders := make([]dueReminder, 0)
	for rows.Next() {
		var reminder dueReminder
		if err := rows.Scan(&reminder.ID, &reminder.OrganizationID, &reminder.EventID, &reminder.UserID, &reminder.EntityType, &reminder.EntityID, &reminder.Title, &reminder.StartAt); err != nil {
			return nil, fmt.Errorf("scan due calendar reminder: %w", err)
		}
		reminders = append(reminders, reminder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due calendar reminders: %w", err)
	}
	return reminders, nil
}

func selectReminderByID(ctx context.Context, tx pgx.Tx, organizationID, reminderID int64) (dueReminder, error) {
	var reminder dueReminder
	err := tx.QueryRow(ctx, `
		SELECT r.id, r.organization_id, r.calendar_event_id, r.user_id, e.entity_type, e.entity_id, e.title, e.start_at
		FROM calendar_event_reminders r
		JOIN calendar_events e ON e.organization_id = r.organization_id AND e.id = r.calendar_event_id
		WHERE r.organization_id = $1 AND r.id = $2
		  AND r.status = 'pending' AND r.remind_at <= NOW() AND e.status = 'scheduled'
		FOR UPDATE OF r
	`, organizationID, reminderID).Scan(&reminder.ID, &reminder.OrganizationID, &reminder.EventID, &reminder.UserID, &reminder.EntityType, &reminder.EntityID, &reminder.Title, &reminder.StartAt)
	if err != nil {
		return dueReminder{}, err
	}
	return reminder, nil
}

func deliverReminder(ctx context.Context, tx pgx.Tx, reminder dueReminder, deliveredAt time.Time) error {
	summary := "Meeting reminder: " + reminder.Title
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (organization_id, user_id, event_type, entity_type, entity_id, summary)
		VALUES ($1, $2, 'meeting.reminder', $3, $4, $5)
	`, reminder.OrganizationID, reminder.UserID, reminder.EntityType, reminder.EntityID, summary); err != nil {
		return fmt.Errorf("create meeting reminder notification: %w", err)
	}
	if err := insertActivity(ctx, tx, reminder.OrganizationID, reminder.EntityType, reminder.EntityID, reminder.UserID, "meeting.reminder_sent", "Meeting reminder sent: "+reminder.Title); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE calendar_event_reminders
		SET status = 'sent', delivered_at = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`, reminder.ID, deliveredAt); err != nil {
		return fmt.Errorf("mark calendar reminder sent: %w", err)
	}
	return nil
}
