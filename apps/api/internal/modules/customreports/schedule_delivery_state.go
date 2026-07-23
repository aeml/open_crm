package customreports

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) finalizeRecipientAccepted(ctx context.Context, organizationID int64, recipient claimedRecipient, providerMessageID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `
		UPDATE custom_report_recipient_deliveries
		SET status='accepted',provider_message_id=$3,last_error='',accepted_at=NOW(),updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND status='sending'
	`, organizationID, recipient.ID, strings.TrimSpace(providerMessageID))
	if err != nil {
		return fmt.Errorf("retain scheduled report acceptance: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events(organization_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,'report_schedule.delivery_accepted','report_recipient_delivery',$2,'Scheduled saved-report email accepted by provider',jsonb_build_object('deliveryRunId',$3::bigint,'recipientUserId',$4::bigint))
	`, organizationID, recipient.ID, recipient.Run.RunID, recipient.UserID); err != nil {
		return fmt.Errorf("audit scheduled report acceptance: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Service) finalizeRecipientUncertain(ctx context.Context, organizationID, recipientID int64, message string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var runID, userID int64
	err = tx.QueryRow(ctx, `
		UPDATE custom_report_recipient_deliveries
		SET status='uncertain',last_error=$3,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND status='sending'
		RETURNING delivery_run_id,recipient_user_id
	`, organizationID, recipientID, message).Scan(&runID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("retain uncertain scheduled report delivery: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events(organization_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,'report_schedule.delivery_uncertain','report_recipient_delivery',$2,'Scheduled saved-report email requires delivery review',jsonb_build_object('deliveryRunId',$3::bigint,'recipientUserId',$4::bigint))
	`, organizationID, recipientID, runID, userID); err != nil {
		return fmt.Errorf("audit uncertain scheduled report delivery: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Service) returnRecipientPending(ctx context.Context, organizationID, recipientID int64, message string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE custom_report_recipient_deliveries
		SET status='pending',last_error=$3,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND status='sending'
	`, organizationID, recipientID, message)
	if err != nil {
		return fmt.Errorf("release retryable scheduled report delivery: %w", err)
	}
	return nil
}

func (s *Service) finalizeRecipientFailed(ctx context.Context, organizationID, recipientID int64, message string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var runID, userID int64
	err = tx.QueryRow(ctx, `
		UPDATE custom_report_recipient_deliveries
		SET status='failed',last_error=$3,updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND status='sending'
		RETURNING delivery_run_id,recipient_user_id
	`, organizationID, recipientID, message).Scan(&runID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("retain failed scheduled report delivery: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events(organization_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,'report_schedule.delivery_failed','report_recipient_delivery',$2,'Scheduled saved-report email exhausted provider retries',jsonb_build_object('deliveryRunId',$3::bigint,'recipientUserId',$4::bigint))
	`, organizationID, recipientID, runID, userID); err != nil {
		return fmt.Errorf("audit failed scheduled report delivery: %w", err)
	}
	return tx.Commit(ctx)
}

func finalizeDeliveryRun(ctx context.Context, tx pgx.Tx, organizationID, runID int64) error {
	var accepted, uncertain, failed, skipped int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status='accepted')::int,
		       COUNT(*) FILTER (WHERE status='uncertain')::int,
		       COUNT(*) FILTER (WHERE status='failed')::int,
		       COUNT(*) FILTER (WHERE status='skipped')::int
		FROM custom_report_recipient_deliveries
		WHERE organization_id=$1 AND delivery_run_id=$2
	`, organizationID, runID).Scan(&accepted, &uncertain, &failed, &skipped); err != nil {
		return fmt.Errorf("summarize scheduled report delivery: %w", err)
	}
	status := "succeeded"
	message := ""
	if uncertain > 0 || failed > 0 {
		status = "partial"
		message = fmt.Sprintf("%d uncertain, %d failed, and %d skipped recipient deliveries require review.", uncertain, failed, skipped)
		if accepted == 0 && uncertain == 0 {
			status = "failed"
		}
	} else if skipped > 0 && accepted > 0 {
		status = "partial"
		message = fmt.Sprintf("%d recipient deliveries were accepted and %d were skipped after the schedule changed.", accepted, skipped)
	} else if accepted == 0 {
		status = "canceled"
		message = "No currently active scheduled recipient remained."
	}
	result, err := tx.Exec(ctx, `
		UPDATE custom_report_delivery_runs
		SET status=$3,last_error=$4,completed_at=NOW(),updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND status IN ('pending','sending')
	`, organizationID, runID, status, message)
	if err != nil {
		return fmt.Errorf("finalize scheduled report delivery: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events(organization_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,'report_schedule.completed','report_delivery_run',$2,'Completed scheduled saved-report delivery',jsonb_build_object('status',$3::text,'accepted',$4::int,'uncertain',$5::int,'failed',$6::int,'skipped',$7::int))
	`, organizationID, runID, status, accepted, uncertain, failed, skipped)
	if err != nil {
		return fmt.Errorf("audit scheduled report completion: %w", err)
	}
	return nil
}

func cancelStaleDeliveryRun(ctx context.Context, tx pgx.Tx, organizationID, runID int64, message string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE custom_report_recipient_deliveries
		SET status='skipped',last_error=$3,updated_at=NOW()
		WHERE organization_id=$1 AND delivery_run_id=$2 AND status='pending'
	`, organizationID, runID, message); err != nil {
		return fmt.Errorf("cancel stale scheduled recipients: %w", err)
	}
	return finalizeDeliveryRun(ctx, tx, organizationID, runID)
}

func cancelStaleScheduleRuns(ctx context.Context, tx pgx.Tx, organizationID, scheduleID, currentRevision int64) error {
	rows, err := tx.Query(ctx, `
		SELECT run.id
		FROM custom_report_delivery_runs run
		WHERE run.organization_id=$1 AND run.schedule_id=$2 AND run.schedule_revision<>$3
		  AND run.status IN ('pending','sending')
		  AND NOT EXISTS (
			SELECT 1 FROM custom_report_recipient_deliveries recipient
			WHERE recipient.organization_id=run.organization_id AND recipient.delivery_run_id=run.id AND recipient.status='sending'
		  )
		ORDER BY run.id
		FOR UPDATE OF run
	`, organizationID, scheduleID, currentRevision)
	if err != nil {
		return fmt.Errorf("find stale report deliveries: %w", err)
	}
	var runIDs []int64
	for rows.Next() {
		var runID int64
		if err := rows.Scan(&runID); err != nil {
			rows.Close()
			return fmt.Errorf("scan stale report delivery: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate stale report deliveries: %w", err)
	}
	for _, runID := range runIDs {
		if err := cancelStaleDeliveryRun(ctx, tx, organizationID, runID, "Schedule changed before delivery completed."); err != nil {
			return err
		}
	}
	return nil
}

func markDeliveryRun(ctx context.Context, tx pgx.Tx, organizationID, runID int64, status, message string) error {
	result, err := tx.Exec(ctx, `
		UPDATE custom_report_delivery_runs
		SET status=$3,last_error=$4,completed_at=NOW(),updated_at=NOW()
		WHERE organization_id=$1 AND id=$2 AND status IN ('pending','sending')
	`, organizationID, runID, status, message)
	if err != nil {
		return fmt.Errorf("mark scheduled report delivery: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events(organization_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,'report_schedule.completed','report_delivery_run',$2,'Scheduled saved-report delivery stopped',jsonb_build_object('status',$3::text))
	`, organizationID, runID, status)
	return err
}

func markRemainingRecipientDeliveries(ctx context.Context, tx pgx.Tx, organizationID, runID int64, status, message string) error {
	_, err := tx.Exec(ctx, `
		UPDATE custom_report_recipient_deliveries
		SET status=$3,last_error=$4,updated_at=NOW()
		WHERE organization_id=$1 AND delivery_run_id=$2 AND status='pending'
	`, organizationID, runID, status, message)
	return err
}

func isTerminalDeliveryRun(status string) bool {
	switch status {
	case "succeeded", "partial", "failed", "canceled":
		return true
	default:
		return false
	}
}

func (s *Service) deliveryJobResult(ctx context.Context, organizationID, runID int64) (map[string]any, error) {
	var status string
	var accepted, uncertain, failed, skipped int
	err := s.pool.QueryRow(ctx, `
		SELECT run.status,
		       COUNT(*) FILTER (WHERE delivery.status='accepted')::int,
		       COUNT(*) FILTER (WHERE delivery.status='uncertain')::int,
		       COUNT(*) FILTER (WHERE delivery.status='failed')::int,
		       COUNT(*) FILTER (WHERE delivery.status='skipped')::int
		FROM custom_report_delivery_runs run
		LEFT JOIN custom_report_recipient_deliveries delivery ON delivery.organization_id=run.organization_id AND delivery.delivery_run_id=run.id
		WHERE run.organization_id=$1 AND run.id=$2
		GROUP BY run.id,run.status
	`, organizationID, runID).Scan(&status, &accepted, &uncertain, &failed, &skipped)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load scheduled report job result: %w", err)
	}
	return map[string]any{"deliveryRunId": runID, "status": status, "accepted": accepted, "uncertain": uncertain, "failed": failed, "skipped": skipped}, nil
}
