package customreports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	"github.com/jackc/pgx/v5"
)

const deliveryAmbiguityWindow = 5 * time.Minute

type preparedDeliveryRun struct {
	OrganizationID int64
	RunID          int64
	ScheduleID     int64
	ReportID       int64
	ReportName     string
	Workspace      string
	ScheduledFor   time.Time
	Filename       string
	Content        []byte
	SHA256         string
	RowCount       int
	Terminal       bool
}

type claimedRecipient struct {
	ID     int64
	UserID int64
	Name   string
	Email  string
	Skip   bool
	Done   bool
	Run    preparedDeliveryRun
}

func (s *Service) HandleScheduledDeliveryJob(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("custom reports service not configured")
	}
	if !s.deliveryAvailable() {
		return nil, ErrDeliveryNotConfigured
	}
	runID, err := deliveryRunID(job)
	if err != nil || job.OrganizationID <= 0 {
		return nil, ErrInvalidInput
	}
	run, err := s.prepareDeliveryRun(ctx, job.OrganizationID, runID)
	if err != nil {
		return nil, err
	}
	if run.Terminal {
		return s.deliveryJobResult(ctx, job.OrganizationID, runID)
	}
	for {
		recipient, err := s.claimScheduledRecipient(ctx, job.OrganizationID, runID)
		if err != nil {
			return nil, err
		}
		if recipient.Done {
			return s.deliveryJobResult(ctx, job.OrganizationID, runID)
		}
		if recipient.Skip {
			continue
		}
		message := scheduledReportMessage(recipient)
		receipt, sendErr := s.deliveryProvider.Send(ctx, message)
		if sendErr == nil {
			if finalErr := s.finalizeRecipientAccepted(ctx, job.OrganizationID, recipient, receipt.ProviderMessageID); finalErr != nil {
				// The provider accepted but CRM finalization failed. Never resend this
				// recipient automatically because the external outcome is now ambiguous.
				if uncertainErr := s.finalizeRecipientUncertain(context.WithoutCancel(ctx), job.OrganizationID, recipient.ID, "Provider accepted the message but CRM could not retain final evidence."); uncertainErr != nil {
					return nil, errors.Join(fmt.Errorf("retain scheduled report provider acceptance: %w", finalErr), uncertainErr)
				}
			}
			continue
		}
		if errors.Is(sendErr, moduleemail.ErrDeliveryUncertain) {
			if err := s.finalizeRecipientUncertain(context.WithoutCancel(ctx), job.OrganizationID, recipient.ID, safeDeliveryError(sendErr)); err != nil {
				return nil, err
			}
			continue
		}
		if job.Attempts < job.MaxAttempts {
			if err := s.returnRecipientPending(context.WithoutCancel(ctx), job.OrganizationID, recipient.ID, safeDeliveryError(sendErr)); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("scheduled report provider rejected delivery: %w", sendErr)
		}
		if err := s.finalizeRecipientFailed(context.WithoutCancel(ctx), job.OrganizationID, recipient.ID, safeDeliveryError(sendErr)); err != nil {
			return nil, err
		}
	}
}

func (s *Service) prepareDeliveryRun(ctx context.Context, organizationID, runID int64) (preparedDeliveryRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return preparedDeliveryRun{}, fmt.Errorf("begin scheduled report generation: %w", err)
	}
	defer tx.Rollback(ctx)
	var run preparedDeliveryRun
	var status string
	var scheduleRevision, currentRevision int64
	var scheduleActive bool
	var artifact []byte
	run.OrganizationID = organizationID
	err = tx.QueryRow(ctx, `
		SELECT run.id,run.schedule_id,run.report_definition_id,definition.name,organization.name,
		       run.scheduled_for,run.status,run.schedule_revision,schedule.revision,schedule.is_active,
		       run.filename,run.artifact,run.content_sha256,run.row_count
		FROM custom_report_delivery_runs run
		JOIN custom_report_schedules schedule ON schedule.organization_id=run.organization_id AND schedule.id=run.schedule_id
		JOIN custom_report_definitions definition ON definition.organization_id=run.organization_id AND definition.id=run.report_definition_id
		JOIN organizations organization ON organization.id=run.organization_id
		WHERE run.organization_id=$1 AND run.id=$2
		FOR UPDATE OF run,schedule
	`, organizationID, runID).Scan(&run.RunID, &run.ScheduleID, &run.ReportID, &run.ReportName, &run.Workspace, &run.ScheduledFor, &status, &scheduleRevision, &currentRevision, &scheduleActive, &run.Filename, &artifact, &run.SHA256, &run.RowCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return preparedDeliveryRun{}, ErrNotFound
	}
	if err != nil {
		return preparedDeliveryRun{}, fmt.Errorf("load scheduled report occurrence: %w", err)
	}
	if isTerminalDeliveryRun(status) {
		run.Terminal = true
		return run, tx.Commit(ctx)
	}
	if !scheduleActive || scheduleRevision != currentRevision {
		if err := cancelStaleDeliveryRun(ctx, tx, organizationID, runID, "Schedule changed before delivery completed."); err != nil {
			return preparedDeliveryRun{}, err
		}
		run.Terminal = true
		return run, tx.Commit(ctx)
	}
	var recipients int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM custom_report_recipient_deliveries WHERE organization_id=$1 AND delivery_run_id=$2`, organizationID, runID).Scan(&recipients); err != nil {
		return preparedDeliveryRun{}, fmt.Errorf("count scheduled report recipients: %w", err)
	}
	if recipients == 0 {
		if err := markDeliveryRun(ctx, tx, organizationID, runID, "canceled", "No active recipients remained when the occurrence was captured."); err != nil {
			return preparedDeliveryRun{}, err
		}
		run.Terminal = true
		return run, tx.Commit(ctx)
	}
	if len(artifact) == 0 {
		definition, file, generateErr := generateCSV(ctx, tx, organizationID, run.ReportID, run.ScheduledFor)
		if generateErr != nil {
			if ScheduledDeliveryPermanentFailure(generateErr) {
				if err := markDeliveryRun(ctx, tx, organizationID, runID, "failed", safeDeliveryError(generateErr)); err != nil {
					return preparedDeliveryRun{}, err
				}
				if err := markRemainingRecipientDeliveries(ctx, tx, organizationID, runID, "failed", safeDeliveryError(generateErr)); err != nil {
					return preparedDeliveryRun{}, err
				}
				if commitErr := tx.Commit(ctx); commitErr != nil {
					return preparedDeliveryRun{}, commitErr
				}
			}
			return preparedDeliveryRun{}, generateErr
		}
		if len(file.Content) > MaxScheduledCSVBytes {
			if err := markDeliveryRun(ctx, tx, organizationID, runID, "failed", ErrScheduledArtifactTooLarge.Error()); err != nil {
				return preparedDeliveryRun{}, err
			}
			if err := markRemainingRecipientDeliveries(ctx, tx, organizationID, runID, "failed", ErrScheduledArtifactTooLarge.Error()); err != nil {
				return preparedDeliveryRun{}, err
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return preparedDeliveryRun{}, commitErr
			}
			return preparedDeliveryRun{}, ErrScheduledArtifactTooLarge
		}
		digest := sha256.Sum256(file.Content)
		run.Filename = file.Filename
		run.Content = file.Content
		run.SHA256 = hex.EncodeToString(digest[:])
		run.RowCount = file.RowCount
		expiresAt := s.currentTime().Add(DeliveryArtifactTTL)
		if _, err := tx.Exec(ctx, `
			UPDATE custom_report_delivery_runs
			SET status='sending',filename=$3,content_sha256=$4,byte_size=$5,row_count=$6,artifact=$7,artifact_expires_at=$8,last_error='',updated_at=NOW()
			WHERE organization_id=$1 AND id=$2
		`, organizationID, runID, run.Filename, run.SHA256, len(run.Content), run.RowCount, run.Content, expiresAt); err != nil {
			return preparedDeliveryRun{}, fmt.Errorf("store scheduled report artifact: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events(organization_id,event_type,entity_type,entity_id,summary,metadata_json)
			VALUES ($1,'report_schedule.generated','report_delivery_run',$2,'Generated scheduled saved-report CSV',jsonb_build_object('scheduleId',$3::bigint,'reportDefinitionId',$4::bigint,'sourceType',$5::text,'rowCount',$6::int,'byteSize',$7::int,'sha256',$8::text))
		`, organizationID, runID, run.ScheduleID, run.ReportID, definition.SourceType, run.RowCount, len(run.Content), run.SHA256); err != nil {
			return preparedDeliveryRun{}, fmt.Errorf("audit scheduled report generation: %w", err)
		}
	} else {
		run.Content = artifact
		if _, err := tx.Exec(ctx, `UPDATE custom_report_delivery_runs SET status='sending',updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, runID); err != nil {
			return preparedDeliveryRun{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return preparedDeliveryRun{}, fmt.Errorf("commit scheduled report generation: %w", err)
	}
	return run, nil
}

func (s *Service) claimScheduledRecipient(ctx context.Context, organizationID, runID int64) (claimedRecipient, error) {
	for {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return claimedRecipient{}, fmt.Errorf("begin scheduled recipient claim: %w", err)
		}
		var runStatus string
		var scheduleActive bool
		var scheduleRevision, currentRevision int64
		err = tx.QueryRow(ctx, `
			SELECT run.status,run.schedule_revision,schedule.revision,schedule.is_active
			FROM custom_report_delivery_runs run
			JOIN custom_report_schedules schedule ON schedule.organization_id=run.organization_id AND schedule.id=run.schedule_id
			WHERE run.organization_id=$1 AND run.id=$2
			FOR UPDATE OF run,schedule
		`, organizationID, runID).Scan(&runStatus, &scheduleRevision, &currentRevision, &scheduleActive)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				err = ErrNotFound
			}
			return claimedRecipient{}, rollbackScheduledRecipientClaim(ctx, tx, err)
		}
		if isTerminalDeliveryRun(runStatus) {
			_ = tx.Commit(ctx)
			return claimedRecipient{Done: true}, nil
		}
		if !scheduleActive || scheduleRevision != currentRevision {
			if err := cancelStaleDeliveryRun(ctx, tx, organizationID, runID, "Schedule changed before delivery completed."); err != nil {
				return claimedRecipient{}, rollbackScheduledRecipientClaim(ctx, tx, err)
			}
			if err := tx.Commit(ctx); err != nil {
				return claimedRecipient{}, err
			}
			return claimedRecipient{Done: true}, nil
		}
		cutoff := s.currentTime().Add(-deliveryAmbiguityWindow)
		if _, err := tx.Exec(ctx, `
			WITH stalled AS (
				UPDATE custom_report_recipient_deliveries
				SET status='uncertain',last_error='Worker stopped while the provider outcome was unknown.',updated_at=NOW()
				WHERE organization_id=$1 AND delivery_run_id=$2 AND status='sending' AND attempted_at <= $3
				RETURNING organization_id,id,delivery_run_id,recipient_user_id
			)
			INSERT INTO audit_events(organization_id,event_type,entity_type,entity_id,summary,metadata_json)
			SELECT organization_id,'report_schedule.delivery_uncertain','report_recipient_delivery',id,
			       'Scheduled saved-report email requires delivery review',
			       jsonb_build_object('deliveryRunId',delivery_run_id,'recipientUserId',recipient_user_id)
			FROM stalled
		`, organizationID, runID, cutoff); err != nil {
			return claimedRecipient{}, rollbackScheduledRecipientClaim(ctx, tx, err)
		}
		var recipient claimedRecipient
		err = tx.QueryRow(ctx, `
			SELECT delivery.id,delivery.recipient_user_id,TRIM(users.first_name||' '||users.last_name),users.email,
			       membership.membership_status='active' AND schedule_recipient.id IS NOT NULL
			FROM custom_report_recipient_deliveries delivery
			JOIN users ON users.id=delivery.recipient_user_id
			JOIN organization_memberships membership ON membership.organization_id=delivery.organization_id AND membership.user_id=delivery.recipient_user_id
			LEFT JOIN custom_report_delivery_runs run ON run.organization_id=delivery.organization_id AND run.id=delivery.delivery_run_id
			LEFT JOIN custom_report_schedule_recipients schedule_recipient ON schedule_recipient.organization_id=delivery.organization_id AND schedule_recipient.schedule_id=run.schedule_id AND schedule_recipient.recipient_user_id=delivery.recipient_user_id
			WHERE delivery.organization_id=$1 AND delivery.delivery_run_id=$2 AND delivery.status='pending'
			ORDER BY delivery.id
			LIMIT 1
			FOR UPDATE OF delivery
		`, organizationID, runID).Scan(&recipient.ID, &recipient.UserID, &recipient.Name, &recipient.Email, &recipient.Skip)
		if errors.Is(err, pgx.ErrNoRows) {
			var sending int
			if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM custom_report_recipient_deliveries WHERE organization_id=$1 AND delivery_run_id=$2 AND status='sending'`, organizationID, runID).Scan(&sending); err != nil {
				return claimedRecipient{}, rollbackScheduledRecipientClaim(ctx, tx, err)
			}
			if sending > 0 {
				return claimedRecipient{}, rollbackScheduledRecipientClaim(ctx, tx, ErrDeliveryInProgress)
			}
			if err := finalizeDeliveryRun(ctx, tx, organizationID, runID); err != nil {
				return claimedRecipient{}, rollbackScheduledRecipientClaim(ctx, tx, err)
			}
			if err := tx.Commit(ctx); err != nil {
				return claimedRecipient{}, err
			}
			return claimedRecipient{Done: true}, nil
		}
		if err != nil {
			return claimedRecipient{}, rollbackScheduledRecipientClaim(ctx, tx, err)
		}
		// The query returns true when the current membership and current schedule
		// recipient are both valid. Flip it into the claim's Skip flag.
		recipient.Skip = !recipient.Skip
		if recipient.Skip {
			if _, err := tx.Exec(ctx, `UPDATE custom_report_recipient_deliveries SET status='skipped',last_error='Recipient is no longer active on this schedule.',updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, recipient.ID); err != nil {
				return claimedRecipient{}, rollbackScheduledRecipientClaim(ctx, tx, err)
			}
			if err := tx.Commit(ctx); err != nil {
				return claimedRecipient{}, err
			}
			return recipient, nil
		}
		if _, err := tx.Exec(ctx, `UPDATE custom_report_recipient_deliveries SET status='sending',attempt_count=attempt_count+1,attempted_at=NOW(),last_error='',updated_at=NOW() WHERE organization_id=$1 AND id=$2`, organizationID, recipient.ID); err != nil {
			return claimedRecipient{}, rollbackScheduledRecipientClaim(ctx, tx, err)
		}
		recipient.Run.OrganizationID = organizationID
		if err := tx.QueryRow(ctx, `
			SELECT run.id,run.schedule_id,run.report_definition_id,definition.name,organization.name,run.scheduled_for,run.filename,run.artifact,run.content_sha256,run.row_count
			FROM custom_report_delivery_runs run
			JOIN custom_report_definitions definition ON definition.organization_id=run.organization_id AND definition.id=run.report_definition_id
			JOIN organizations organization ON organization.id=run.organization_id
			WHERE run.organization_id=$1 AND run.id=$2
		`, organizationID, runID).Scan(&recipient.Run.RunID, &recipient.Run.ScheduleID, &recipient.Run.ReportID, &recipient.Run.ReportName, &recipient.Run.Workspace, &recipient.Run.ScheduledFor, &recipient.Run.Filename, &recipient.Run.Content, &recipient.Run.SHA256, &recipient.Run.RowCount); err != nil {
			return claimedRecipient{}, rollbackScheduledRecipientClaim(ctx, tx, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return claimedRecipient{}, err
		}
		return recipient, nil
	}
}

func rollbackScheduledRecipientClaim(ctx context.Context, tx pgx.Tx, cause error) error {
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
		return errors.Join(cause, fmt.Errorf("rollback scheduled recipient claim: %w", rollbackErr))
	}
	return cause
}

func scheduledReportMessage(recipient claimedRecipient) moduleemail.Message {
	reportName := singleLine(recipient.Run.ReportName)
	workspace := singleLine(recipient.Run.Workspace)
	return moduleemail.Message{
		To:       recipient.Email,
		Subject:  "Scheduled report: " + reportName,
		TextBody: fmt.Sprintf("Hi %s,\n\nYour scheduled Open CRM report %q for %s is attached as CSV. It was generated for %s and contains %d data rows.\n\nCSV SHA-256: %s\n", firstNameOrThere(recipient.Name), reportName, workspace, recipient.Run.ScheduledFor.UTC().Format(time.RFC3339), recipient.Run.RowCount, recipient.Run.SHA256),
		Metadata: map[string]string{
			"open_crm_scheduled_report":  "v1",
			"open_crm_organization_id":   strconv.FormatInt(recipient.Run.OrganizationID, 10),
			"open_crm_delivery_run_id":   strconv.FormatInt(recipient.Run.RunID, 10),
			"open_crm_recipient_user_id": strconv.FormatInt(recipient.UserID, 10),
		},
		Attachments: []moduleemail.Attachment{{Name: recipient.Run.Filename, ContentType: "text/csv; charset=utf-8", Content: recipient.Run.Content}},
	}
}

func firstNameOrThere(name string) string {
	if value := strings.Fields(name); len(value) > 0 {
		return singleLine(value[0])
	}
	return "there"
}

func singleLine(value string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ", "\x00", "").Replace(value))
}

func safeDeliveryError(err error) string {
	if err == nil {
		return ""
	}
	value := singleLine(err.Error())
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}
