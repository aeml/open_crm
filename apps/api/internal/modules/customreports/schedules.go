package customreports

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	ScheduledDeliveryJobType = "report.schedule.deliver"
	MaxReportSchedules       = 20
	MaxScheduleRecipients    = 10
	MaxDeliveryHistory       = 20
	MaxScheduledCSVBytes     = 5 << 20
	DeliveryArtifactTTL      = 7 * 24 * time.Hour
)

var (
	ErrDeliveryNotConfigured     = errors.New("scheduled report delivery provider is not configured")
	ErrScheduleConflict          = errors.New("scheduled report revision conflict")
	ErrScheduleLimit             = errors.New("scheduled report limit reached")
	ErrDeliveryNotRecoverable    = errors.New("scheduled report delivery is not recoverable")
	ErrDeliveryInProgress        = errors.New("scheduled report delivery is still inside its ambiguity window")
	ErrScheduledArtifactTooLarge = errors.New("scheduled report CSV exceeds the 5 MiB attachment limit; narrow the saved filters")
)

type ScheduleRecipient struct {
	UserID   int64  `json:"userId"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	IsActive bool   `json:"isActive"`
}

type ReportSchedule struct {
	OrganizationID     int64               `json:"-"`
	ID                 int64               `json:"id"`
	ReportDefinitionID int64               `json:"reportDefinitionId"`
	ReportName         string              `json:"reportName"`
	Revision           int64               `json:"revision"`
	Cadence            string              `json:"cadence"`
	WeekdayUTC         *int                `json:"weekdayUtc"`
	HourUTC            int                 `json:"hourUtc"`
	IsActive           bool                `json:"isActive"`
	NextRunAt          *time.Time          `json:"nextRunAt"`
	Recipients         []ScheduleRecipient `json:"recipients"`
	CreatedAt          time.Time           `json:"createdAt"`
	UpdatedAt          time.Time           `json:"updatedAt"`
}

type ReportScheduleInput struct {
	Revision         int64   `json:"revision"`
	Cadence          string  `json:"cadence"`
	WeekdayUTC       *int    `json:"weekdayUtc"`
	HourUTC          int     `json:"hourUtc"`
	RecipientUserIDs []int64 `json:"recipientUserIds"`
	IsActive         bool    `json:"isActive"`
}

type RecipientDelivery struct {
	ID              int64      `json:"id"`
	RecipientUserID int64      `json:"recipientUserId"`
	RecipientName   string     `json:"recipientName"`
	RecipientEmail  string     `json:"recipientEmail"`
	Status          string     `json:"status"`
	AttemptCount    int        `json:"attemptCount"`
	LastError       string     `json:"lastError,omitempty"`
	AttemptedAt     *time.Time `json:"attemptedAt,omitempty"`
	AcceptedAt      *time.Time `json:"acceptedAt,omitempty"`
	ResolvedAt      *time.Time `json:"resolvedAt,omitempty"`
}

type DeliveryRun struct {
	ID                 int64               `json:"id"`
	ScheduleID         int64               `json:"scheduleId"`
	ReportDefinitionID int64               `json:"reportDefinitionId"`
	ReportName         string              `json:"reportName"`
	ScheduleRevision   int64               `json:"scheduleRevision"`
	ScheduledFor       time.Time           `json:"scheduledFor"`
	Status             string              `json:"status"`
	Filename           string              `json:"filename,omitempty"`
	ContentSHA256      string              `json:"contentSha256,omitempty"`
	ByteSize           int64               `json:"byteSize"`
	RowCount           int                 `json:"rowCount"`
	ArtifactExpiresAt  *time.Time          `json:"artifactExpiresAt,omitempty"`
	LastError          string              `json:"lastError,omitempty"`
	CompletedAt        *time.Time          `json:"completedAt,omitempty"`
	Recipients         []RecipientDelivery `json:"recipients"`
	CreatedAt          time.Time           `json:"createdAt"`
}

type ScheduleOverview struct {
	Provider          string           `json:"provider"`
	DeliveryAvailable bool             `json:"deliveryAvailable"`
	Schedules         []ReportSchedule `json:"schedules"`
	DeliveryRuns      []DeliveryRun    `json:"deliveryRuns"`
}

type DeliveryResolutionInput struct {
	Resolution           string `json:"resolution"`
	ConfirmDuplicateRisk bool   `json:"confirmDuplicateRisk"`
}

func (s *Service) ListSchedules(ctx context.Context, organizationID, actorUserID int64) (ScheduleOverview, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || actorUserID <= 0 {
		return ScheduleOverview{}, ErrInvalidInput
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return ScheduleOverview{}, fmt.Errorf("begin scheduled report list: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveReportAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return ScheduleOverview{}, err
	}
	overview := ScheduleOverview{Provider: s.deliveryProviderName(), DeliveryAvailable: s.deliveryAvailable(), Schedules: []ReportSchedule{}, DeliveryRuns: []DeliveryRun{}}
	rows, err := tx.Query(ctx, scheduleSelect+`
		WHERE schedule.organization_id=$1
		ORDER BY schedule.is_active DESC, schedule.updated_at DESC, schedule.id DESC
	`, organizationID)
	if err != nil {
		return ScheduleOverview{}, fmt.Errorf("list report schedules: %w", err)
	}
	for rows.Next() {
		schedule, scanErr := scanSchedule(rows)
		if scanErr != nil {
			rows.Close()
			return ScheduleOverview{}, scanErr
		}
		overview.Schedules = append(overview.Schedules, schedule)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ScheduleOverview{}, fmt.Errorf("iterate saved-report schedules: %w", err)
	}
	for index := range overview.Schedules {
		recipients, loadErr := loadScheduleRecipients(ctx, tx, organizationID, overview.Schedules[index].ID)
		if loadErr != nil {
			return ScheduleOverview{}, loadErr
		}
		overview.Schedules[index].Recipients = recipients
	}
	overview.DeliveryRuns, err = loadDeliveryRuns(ctx, tx, organizationID, MaxDeliveryHistory)
	if err != nil {
		return ScheduleOverview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScheduleOverview{}, fmt.Errorf("commit scheduled report list: %w", err)
	}
	return overview, nil
}

func (s *Service) UpsertSchedule(ctx context.Context, organizationID, reportDefinitionID, actorUserID int64, input ReportScheduleInput) (ReportSchedule, error) {
	if s == nil || s.pool == nil || organizationID <= 0 || reportDefinitionID <= 0 || actorUserID <= 0 {
		return ReportSchedule{}, ErrInvalidInput
	}
	input, err := normalizeScheduleInput(input)
	if err != nil {
		return ReportSchedule{}, err
	}
	if input.IsActive && !s.deliveryAvailable() {
		return ReportSchedule{}, ErrDeliveryNotConfigured
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReportSchedule{}, fmt.Errorf("begin report schedule update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveReportAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return ReportSchedule{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("report-schedules:%d", organizationID)); err != nil {
		return ReportSchedule{}, fmt.Errorf("serialize report schedules: %w", err)
	}
	definition, _, err := loadExecutableDefinition(ctx, tx, organizationID, reportDefinitionID)
	if err != nil {
		return ReportSchedule{}, err
	}
	if err := validateScheduleRecipients(ctx, tx, organizationID, input.RecipientUserIDs); err != nil {
		return ReportSchedule{}, err
	}

	existing, existingRecipients, err := loadScheduleForUpdate(ctx, tx, organizationID, reportDefinitionID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ReportSchedule{}, err
	}
	nextRunAt := scheduleNextRun(s.currentTime(), input)
	eventType := "report_schedule.created"
	summary := "Created scheduled saved-report delivery"
	var schedule ReportSchedule
	if errors.Is(err, pgx.ErrNoRows) {
		if input.Revision != 0 {
			return ReportSchedule{}, ErrScheduleConflict
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM custom_report_schedules WHERE organization_id=$1`, organizationID).Scan(&count); err != nil {
			return ReportSchedule{}, fmt.Errorf("count report schedules: %w", err)
		}
		if count >= MaxReportSchedules {
			return ReportSchedule{}, ErrScheduleLimit
		}
		schedule, err = scanSchedule(tx.QueryRow(ctx, scheduleInsertSQL, organizationID, reportDefinitionID, input.Cadence, input.WeekdayUTC, input.HourUTC, input.IsActive, nextRunAt, actorUserID))
	} else {
		if input.Revision != existing.Revision {
			return ReportSchedule{}, ErrScheduleConflict
		}
		if scheduleInputEquals(existing, existingRecipients, input) {
			return existing, tx.Commit(ctx)
		}
		eventType = "report_schedule.updated"
		summary = "Updated scheduled saved-report delivery"
		schedule, err = scanSchedule(tx.QueryRow(ctx, scheduleUpdateSQL, organizationID, existing.ID, input.Cadence, input.WeekdayUTC, input.HourUTC, input.IsActive, nextRunAt, actorUserID, existing.Revision))
	}
	if err != nil {
		return ReportSchedule{}, mapScheduleSaveError(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM custom_report_schedule_recipients WHERE organization_id=$1 AND schedule_id=$2`, organizationID, schedule.ID); err != nil {
		return ReportSchedule{}, fmt.Errorf("replace report schedule recipients: %w", err)
	}
	for _, userID := range input.RecipientUserIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO custom_report_schedule_recipients(organization_id,schedule_id,recipient_user_id) VALUES ($1,$2,$3)`, organizationID, schedule.ID, userID); err != nil {
			return ReportSchedule{}, mapScheduleSaveError(err)
		}
	}
	if err := cancelStaleScheduleRuns(ctx, tx, organizationID, schedule.ID, schedule.Revision); err != nil {
		return ReportSchedule{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events(organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,$3,'report_schedule',$4,$5,jsonb_build_object('reportDefinitionId',$6::bigint,'reportName',$7::text,'revision',$8::bigint,'cadence',$9::text,'hourUtc',$10::int,'recipientCount',$11::int,'isActive',$12::boolean))
	`, organizationID, actorUserID, eventType, schedule.ID, summary, definition.ID, definition.Name, schedule.Revision, schedule.Cadence, schedule.HourUTC, len(input.RecipientUserIDs), schedule.IsActive); err != nil {
		return ReportSchedule{}, fmt.Errorf("audit report schedule: %w", err)
	}
	schedule.Recipients, err = loadScheduleRecipients(ctx, tx, organizationID, schedule.ID)
	if err != nil {
		return ReportSchedule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReportSchedule{}, fmt.Errorf("commit report schedule update: %w", err)
	}
	return schedule, nil
}

// reconcileScheduledDefinitionWrite gives an active schedule a new revision
// whenever its mutable report definition changes. Untouched queued work is then
// canceled instead of executing a definition different from the reviewed one.
// Ineligible definitions also pause delivery until an admin deliberately resumes.
func reconcileScheduledDefinitionWrite(ctx context.Context, tx pgx.Tx, organizationID, actorUserID int64, definition Definition) error {
	executable := definition.IsActive && isExecutableVisualization(definition)
	reason := "report_definition_updated"
	summary := "Revised scheduled saved-report delivery"
	if !executable {
		reason = "report_definition_not_executable"
		summary = "Paused scheduled saved-report delivery"
	}
	if !definition.IsActive {
		reason = "report_definition_inactive"
	}
	var scheduleID, revision int64
	err := tx.QueryRow(ctx, `
		UPDATE custom_report_schedules
		SET is_active=$4,next_run_at=CASE WHEN $4 THEN next_run_at ELSE NULL END,
		    revision=revision+1,updated_by_user_id=$3,updated_at=NOW()
		WHERE organization_id=$1 AND report_definition_id=$2 AND is_active
		RETURNING id,revision
	`, organizationID, definition.ID, actorUserID, executable).Scan(&scheduleID, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("pause ineligible report schedule: %w", err)
	}
	if err := cancelStaleScheduleRuns(ctx, tx, organizationID, scheduleID, revision); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events(organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'report_schedule.updated','report_schedule',$3,$4,jsonb_build_object('reportDefinitionId',$5::bigint,'reportName',$6::text,'revision',$7::bigint,'isActive',$8::boolean,'reason',$9::text))
	`, organizationID, actorUserID, scheduleID, summary, definition.ID, definition.Name, revision, executable, reason); err != nil {
		return fmt.Errorf("audit scheduled report definition reconciliation: %w", err)
	}
	return nil
}

func requireScheduledDefinitionWrite(ctx context.Context, tx pgx.Tx, organizationID, definitionID, actorUserID int64) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("report-schedules:%d", organizationID)); err != nil {
		return fmt.Errorf("serialize scheduled report definition update: %w", err)
	}
	var activeSchedule bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM custom_report_schedules WHERE organization_id=$1 AND report_definition_id=$2 AND is_active)`, organizationID, definitionID).Scan(&activeSchedule); err != nil {
		return fmt.Errorf("check scheduled report definition: %w", err)
	}
	if activeSchedule {
		return requireActiveReportAdmin(ctx, tx, organizationID, actorUserID)
	}
	return nil
}

func normalizeScheduleInput(input ReportScheduleInput) (ReportScheduleInput, error) {
	input.Cadence = strings.ToLower(strings.TrimSpace(input.Cadence))
	if input.Revision < 0 || input.HourUTC < 0 || input.HourUTC > 23 || (input.Cadence != "daily" && input.Cadence != "weekly") {
		return ReportScheduleInput{}, ErrInvalidInput
	}
	if input.Cadence == "daily" {
		input.WeekdayUTC = nil
	} else if input.WeekdayUTC == nil || *input.WeekdayUTC < 0 || *input.WeekdayUTC > 6 {
		return ReportScheduleInput{}, ErrInvalidInput
	}
	seen := map[int64]bool{}
	recipients := make([]int64, 0, len(input.RecipientUserIDs))
	for _, userID := range input.RecipientUserIDs {
		if userID <= 0 || seen[userID] {
			continue
		}
		seen[userID] = true
		recipients = append(recipients, userID)
	}
	sort.Slice(recipients, func(i, j int) bool { return recipients[i] < recipients[j] })
	if len(recipients) == 0 || len(recipients) > MaxScheduleRecipients {
		return ReportScheduleInput{}, ErrInvalidInput
	}
	input.RecipientUserIDs = recipients
	return input, nil
}

func validateScheduleRecipients(ctx context.Context, tx pgx.Tx, organizationID int64, userIDs []int64) error {
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM organization_memberships WHERE organization_id=$1 AND user_id=ANY($2) AND membership_status='active'`, organizationID, userIDs).Scan(&count); err != nil {
		return fmt.Errorf("validate report schedule recipients: %w", err)
	}
	if count != len(userIDs) {
		return ErrInvalidInput
	}
	return nil
}

func scheduleInputEquals(schedule ReportSchedule, recipients []int64, input ReportScheduleInput) bool {
	return schedule.Cadence == input.Cadence && equalOptionalInt(schedule.WeekdayUTC, input.WeekdayUTC) && schedule.HourUTC == input.HourUTC && schedule.IsActive == input.IsActive && slices.Equal(recipients, input.RecipientUserIDs)
}

func equalOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (s *Service) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) deliveryProviderName() string {
	if s == nil || s.deliveryProvider == nil {
		return "none"
	}
	return strings.TrimSpace(s.deliveryProvider.Name())
}

func (s *Service) deliveryAvailable() bool {
	name := strings.ToLower(s.deliveryProviderName())
	return name != "" && name != "none" && (name != "fake" || s.allowFakeDelivery)
}

func scheduleNextRun(now time.Time, input ReportScheduleInput) *time.Time {
	if !input.IsActive {
		return nil
	}
	now = now.UTC()
	candidate := time.Date(now.Year(), now.Month(), now.Day(), input.HourUTC, 0, 0, 0, time.UTC)
	if input.Cadence == "weekly" {
		days := (int(*input.WeekdayUTC) - int(candidate.Weekday()) + 7) % 7
		candidate = candidate.AddDate(0, 0, days)
	}
	if !candidate.After(now) {
		if input.Cadence == "weekly" {
			candidate = candidate.AddDate(0, 0, 7)
		} else {
			candidate = candidate.AddDate(0, 0, 1)
		}
	}
	return &candidate
}

func mapScheduleSaveError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrScheduleConflict
	}
	return fmt.Errorf("save report schedule: %w", err)
}
