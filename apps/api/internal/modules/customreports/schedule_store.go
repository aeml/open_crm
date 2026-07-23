package customreports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const scheduleColumns = `schedule.organization_id,schedule.id,schedule.report_definition_id,definition.name,schedule.revision,schedule.cadence,schedule.weekday_utc,schedule.hour_utc,schedule.is_active,schedule.next_run_at,schedule.created_at,schedule.updated_at`

const scheduleSelect = `
	SELECT ` + scheduleColumns + `
	FROM custom_report_schedules schedule
	JOIN custom_report_definitions definition
	  ON definition.organization_id=schedule.organization_id
	 AND definition.id=schedule.report_definition_id
`

const scheduleInsertSQL = `
	WITH inserted AS (
		INSERT INTO custom_report_schedules(
			organization_id,report_definition_id,cadence,weekday_utc,hour_utc,is_active,next_run_at,created_by_user_id,updated_by_user_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		RETURNING *
	)
	SELECT inserted.organization_id,inserted.id,inserted.report_definition_id,definition.name,inserted.revision,inserted.cadence,inserted.weekday_utc,inserted.hour_utc,inserted.is_active,inserted.next_run_at,inserted.created_at,inserted.updated_at
	FROM inserted
	JOIN custom_report_definitions definition ON definition.organization_id=inserted.organization_id AND definition.id=inserted.report_definition_id
`

const scheduleUpdateSQL = `
	WITH updated AS (
		UPDATE custom_report_schedules
		SET cadence=$3,weekday_utc=$4,hour_utc=$5,is_active=$6,next_run_at=$7,
		    updated_by_user_id=$8,revision=revision+1,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND revision=$9
		RETURNING *
	)
	SELECT updated.organization_id,updated.id,updated.report_definition_id,definition.name,updated.revision,updated.cadence,updated.weekday_utc,updated.hour_utc,updated.is_active,updated.next_run_at,updated.created_at,updated.updated_at
	FROM updated
	JOIN custom_report_definitions definition ON definition.organization_id=updated.organization_id AND definition.id=updated.report_definition_id
`

func scanSchedule(row rowScanner) (ReportSchedule, error) {
	var schedule ReportSchedule
	var weekday pgtype.Int2
	if err := row.Scan(
		&schedule.OrganizationID, &schedule.ID, &schedule.ReportDefinitionID, &schedule.ReportName,
		&schedule.Revision, &schedule.Cadence, &weekday, &schedule.HourUTC,
		&schedule.IsActive, &schedule.NextRunAt, &schedule.CreatedAt, &schedule.UpdatedAt,
	); err != nil {
		return ReportSchedule{}, err
	}
	if weekday.Valid {
		value := int(weekday.Int16)
		schedule.WeekdayUTC = &value
	}
	schedule.Recipients = []ScheduleRecipient{}
	return schedule, nil
}

func loadScheduleForUpdate(ctx context.Context, tx pgx.Tx, organizationID, reportDefinitionID int64) (ReportSchedule, []int64, error) {
	schedule, err := scanSchedule(tx.QueryRow(ctx, scheduleSelect+`
		WHERE schedule.organization_id=$1 AND schedule.report_definition_id=$2
		FOR UPDATE OF schedule
	`, organizationID, reportDefinitionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReportSchedule{}, nil, pgx.ErrNoRows
		}
		return ReportSchedule{}, nil, fmt.Errorf("load report schedule: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT recipient_user_id
		FROM custom_report_schedule_recipients
		WHERE organization_id=$1 AND schedule_id=$2
		ORDER BY recipient_user_id
	`, organizationID, schedule.ID)
	if err != nil {
		return ReportSchedule{}, nil, fmt.Errorf("load report schedule recipient ids: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ReportSchedule{}, nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ReportSchedule{}, nil, fmt.Errorf("iterate report schedule recipient ids: %w", err)
	}
	schedule.Recipients, err = loadScheduleRecipients(ctx, tx, organizationID, schedule.ID)
	if err != nil {
		return ReportSchedule{}, nil, err
	}
	return schedule, ids, nil
}

func loadScheduleRecipients(ctx context.Context, querier executionQuerier, organizationID, scheduleID int64) ([]ScheduleRecipient, error) {
	rows, err := querier.Query(ctx, `
		SELECT membership.user_id,TRIM(users.first_name||' '||users.last_name),users.email,membership.role,
		       membership.membership_status='active'
		FROM custom_report_schedule_recipients recipient
		JOIN organization_memberships membership
		  ON membership.organization_id=recipient.organization_id
		 AND membership.user_id=recipient.recipient_user_id
		JOIN users ON users.id=membership.user_id
		WHERE recipient.organization_id=$1 AND recipient.schedule_id=$2
		ORDER BY lower(users.email),membership.user_id
	`, organizationID, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("load report schedule recipients: %w", err)
	}
	result := []ScheduleRecipient{}
	for rows.Next() {
		var recipient ScheduleRecipient
		if err := rows.Scan(&recipient.UserID, &recipient.Name, &recipient.Email, &recipient.Role, &recipient.IsActive); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, recipient)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report schedule recipients: %w", err)
	}
	return result, nil
}

func loadDeliveryRuns(ctx context.Context, querier executionQuerier, organizationID int64, limit int) ([]DeliveryRun, error) {
	rows, err := querier.Query(ctx, `
		SELECT run.id,run.schedule_id,run.report_definition_id,definition.name,run.schedule_revision,
		       run.scheduled_for,run.status,run.filename,run.content_sha256,run.byte_size,run.row_count,
		       run.artifact_expires_at,run.last_error,run.completed_at,run.created_at
		FROM custom_report_delivery_runs run
		JOIN custom_report_definitions definition
		  ON definition.organization_id=run.organization_id AND definition.id=run.report_definition_id
		WHERE run.organization_id=$1
		ORDER BY run.created_at DESC,run.id DESC
		LIMIT $2
	`, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("load scheduled report delivery history: %w", err)
	}
	result := []DeliveryRun{}
	for rows.Next() {
		var run DeliveryRun
		if err := rows.Scan(&run.ID, &run.ScheduleID, &run.ReportDefinitionID, &run.ReportName, &run.ScheduleRevision, &run.ScheduledFor, &run.Status, &run.Filename, &run.ContentSHA256, &run.ByteSize, &run.RowCount, &run.ArtifactExpiresAt, &run.LastError, &run.CompletedAt, &run.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		run.Recipients = []RecipientDelivery{}
		result = append(result, run)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scheduled report delivery history: %w", err)
	}
	for index := range result {
		recipients, err := loadRecipientDeliveries(ctx, querier, organizationID, result[index].ID)
		if err != nil {
			return nil, err
		}
		result[index].Recipients = recipients
	}
	return result, nil
}

func loadRecipientDeliveries(ctx context.Context, querier executionQuerier, organizationID, runID int64) ([]RecipientDelivery, error) {
	rows, err := querier.Query(ctx, `
		SELECT delivery.id,delivery.recipient_user_id,TRIM(users.first_name||' '||users.last_name),users.email,
		       delivery.status,delivery.attempt_count,delivery.last_error,delivery.attempted_at,delivery.accepted_at,delivery.resolved_at
		FROM custom_report_recipient_deliveries delivery
		JOIN users ON users.id=delivery.recipient_user_id
		WHERE delivery.organization_id=$1 AND delivery.delivery_run_id=$2
		ORDER BY lower(users.email),delivery.id
	`, organizationID, runID)
	if err != nil {
		return nil, fmt.Errorf("load report recipient deliveries: %w", err)
	}
	result := []RecipientDelivery{}
	for rows.Next() {
		var delivery RecipientDelivery
		if err := rows.Scan(&delivery.ID, &delivery.RecipientUserID, &delivery.RecipientName, &delivery.RecipientEmail, &delivery.Status, &delivery.AttemptCount, &delivery.LastError, &delivery.AttemptedAt, &delivery.AcceptedAt, &delivery.ResolvedAt); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, delivery)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report recipient deliveries: %w", err)
	}
	return result, nil
}

func deliveryRunResult(ctx context.Context, querier executionQuerier, organizationID, runID int64) (DeliveryRun, error) {
	runs, err := loadDeliveryRunsByID(ctx, querier, organizationID, runID)
	if err != nil {
		return DeliveryRun{}, err
	}
	if len(runs) == 0 {
		return DeliveryRun{}, ErrNotFound
	}
	return runs[0], nil
}

func loadDeliveryRunsByID(ctx context.Context, querier executionQuerier, organizationID, runID int64) ([]DeliveryRun, error) {
	var run DeliveryRun
	err := querier.QueryRow(ctx, `
		SELECT run.id,run.schedule_id,run.report_definition_id,definition.name,run.schedule_revision,
		       run.scheduled_for,run.status,run.filename,run.content_sha256,run.byte_size,run.row_count,
		       run.artifact_expires_at,run.last_error,run.completed_at,run.created_at
		FROM custom_report_delivery_runs run
		JOIN custom_report_definitions definition ON definition.organization_id=run.organization_id AND definition.id=run.report_definition_id
		WHERE run.organization_id=$1 AND run.id=$2
	`, organizationID, runID).Scan(&run.ID, &run.ScheduleID, &run.ReportDefinitionID, &run.ReportName, &run.ScheduleRevision, &run.ScheduledFor, &run.Status, &run.Filename, &run.ContentSHA256, &run.ByteSize, &run.RowCount, &run.ArtifactExpiresAt, &run.LastError, &run.CompletedAt, &run.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return []DeliveryRun{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load report delivery run: %w", err)
	}
	recipients, err := loadRecipientDeliveries(ctx, querier, organizationID, runID)
	if err != nil {
		return nil, err
	}
	run.Recipients = recipients
	return []DeliveryRun{run}, nil
}

func nextRunFromSchedule(schedule ReportSchedule, now time.Time) *time.Time {
	return scheduleNextRun(now, ReportScheduleInput{Cadence: schedule.Cadence, WeekdayUTC: schedule.WeekdayUTC, HourUTC: schedule.HourUTC, IsActive: schedule.IsActive})
}
