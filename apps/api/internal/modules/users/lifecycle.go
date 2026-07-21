package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	modulebilling "github.com/aeml/open_crm/apps/api/internal/modules/billing"
	modulenotifications "github.com/aeml/open_crm/apps/api/internal/modules/notifications"
	moduletaskreminders "github.com/aeml/open_crm/apps/api/internal/modules/taskreminders"
)

const lifecycleAttempts = 4

type SetStatusInput struct {
	Status           string `json:"status"`
	ReassignToUserID int64  `json:"reassignToUserId"`
}

type LifecycleResult struct {
	User                UserSummary `json:"user"`
	Reassigned          WorkCounts  `json:"reassigned"`
	SessionsInvalidated int64       `json:"sessionsInvalidated"`
	Changed             bool        `json:"changed"`
}

type membershipRecord struct {
	Role      string
	Status    string
	Email     string
	FirstName string
	LastName  string
}

func (s *Service) SetStatus(ctx context.Context, organizationID, userID, actorUserID int64, input SetStatusInput) (LifecycleResult, error) {
	if s == nil || s.pool == nil {
		return LifecycleResult{}, fmt.Errorf("users service not configured")
	}
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status != MembershipStatusActive && input.Status != MembershipStatusDisabled {
		return LifecycleResult{}, ErrInvalidStatus
	}
	if input.Status == MembershipStatusActive && input.ReassignToUserID != 0 {
		return LifecycleResult{}, ErrInvalidReassignment
	}
	var reservation modulebilling.CapacityReservation
	if input.Status == MembershipStatusActive {
		var currentStatus string
		err := s.pool.QueryRow(ctx, `
			SELECT COALESCE(membership_status, 'active')
			FROM organization_memberships
			WHERE organization_id=$1 AND user_id=$2
		`, organizationID, userID).Scan(&currentStatus)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return LifecycleResult{}, fmt.Errorf("load membership status for capacity: %w", err)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return LifecycleResult{}, ErrNotFound
		}
		if currentStatus == MembershipStatusActive {
			user, err := loadUserSummary(ctx, s.pool, organizationID, userID)
			if err != nil {
				return LifecycleResult{}, err
			}
			return LifecycleResult{User: user}, nil
		}
		if currentStatus == MembershipStatusDisabled {
			reservation, err = modulebilling.ReserveCapacity(ctx, s.capacity, organizationID, modulebilling.ResourceSeats, 1)
			if err != nil {
				return LifecycleResult{}, err
			}
			defer modulebilling.CancelReservation(s.capacity, reservation)
		}
	}

	var lastErr error
	for attempt := 0; attempt < lifecycleAttempts; attempt++ {
		result, err := s.setStatusOnce(ctx, organizationID, userID, actorUserID, input, reservation)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryableLifecycleTransaction(err) || ctx.Err() != nil {
			return LifecycleResult{}, err
		}
	}
	return LifecycleResult{}, fmt.Errorf("update user lifecycle after %d attempts: %w", lifecycleAttempts, lastErr)
}

func (s *Service) setStatusOnce(ctx context.Context, organizationID, userID, actorUserID int64, input SetStatusInput, reservation modulebilling.CapacityReservation) (LifecycleResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return LifecycleResult{}, fmt.Errorf("begin user lifecycle transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := modulebilling.LockCapacityEffect(ctx, tx, reservation); err != nil {
		return LifecycleResult{}, err
	}

	target, err := lockMembership(ctx, tx, organizationID, userID)
	if err != nil {
		return LifecycleResult{}, err
	}
	if target.Status == input.Status {
		user, err := loadUserSummary(ctx, tx, organizationID, userID)
		if err != nil {
			return LifecycleResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return LifecycleResult{}, fmt.Errorf("commit unchanged user lifecycle: %w", err)
		}
		return LifecycleResult{User: user}, nil
	}

	result := LifecycleResult{Changed: true}
	if input.Status == MembershipStatusDisabled {
		if actorUserID == userID {
			return LifecycleResult{}, ErrCannotChangeOwnStatus
		}
		if target.Role == "owner" {
			ownerCount, err := lockAndCountActiveOwners(ctx, tx, organizationID)
			if err != nil {
				return LifecycleResult{}, err
			}
			if ownerCount <= 1 {
				return LifecycleResult{}, ErrLastActiveOwner
			}
		}
		if err := validateReplacement(ctx, tx, organizationID, userID, input.ReassignToUserID); err != nil {
			return LifecycleResult{}, err
		}
		result.Reassigned, err = reassignOperationalWork(ctx, tx, organizationID, userID, input.ReassignToUserID, actorUserID)
		if err != nil {
			return LifecycleResult{}, err
		}
		if err := stopDisabledUserEffects(ctx, tx, organizationID, userID); err != nil {
			return LifecycleResult{}, err
		}
		invalidated, err := tx.Exec(ctx, `DELETE FROM sessions WHERE organization_id = $1 AND user_id = $2`, organizationID, userID)
		if err != nil {
			return LifecycleResult{}, fmt.Errorf("invalidate disabled user sessions: %w", err)
		}
		result.SessionsInvalidated = invalidated.RowsAffected()
	}

	updated, err := tx.Exec(ctx, `
		UPDATE organization_memberships
		SET membership_status = $3, status_changed_at = NOW(), status_changed_by_user_id = $4
		WHERE organization_id = $1 AND user_id = $2
	`, organizationID, userID, input.Status, actorUserID)
	if err != nil {
		return LifecycleResult{}, fmt.Errorf("update membership status: %w", err)
	}
	if updated.RowsAffected() != 1 {
		return LifecycleResult{}, ErrNotFound
	}
	if err := recordLifecycleAudit(ctx, tx, organizationID, userID, actorUserID, target, input, result); err != nil {
		return LifecycleResult{}, err
	}
	result.User, err = loadUserSummary(ctx, tx, organizationID, userID)
	if err != nil {
		return LifecycleResult{}, err
	}
	if input.Status == MembershipStatusActive {
		if err := modulebilling.ConsumeCapacity(ctx, s.capacity, tx, reservation); err != nil {
			return LifecycleResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return LifecycleResult{}, fmt.Errorf("commit user lifecycle: %w", err)
	}
	return result, nil
}

func retryableLifecycleTransaction(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && (postgresError.Code == "40001" || postgresError.Code == "40P01")
}

func (s *Service) updateRole(ctx context.Context, organizationID, userID int64, role string) (UserSummary, error) {
	if s == nil || s.pool == nil {
		return UserSummary{}, fmt.Errorf("users service not configured")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role != "owner" && role != "admin" && role != "member" && role != "viewer" {
		return UserSummary{}, ErrInvalidRole
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return UserSummary{}, fmt.Errorf("begin role update transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	target, err := lockMembership(ctx, tx, organizationID, userID)
	if err != nil {
		return UserSummary{}, err
	}
	if target.Role == "owner" && role != "owner" && target.Status == MembershipStatusActive {
		ownerCount, err := lockAndCountActiveOwners(ctx, tx, organizationID)
		if err != nil {
			return UserSummary{}, err
		}
		if ownerCount <= 1 {
			return UserSummary{}, ErrLastActiveOwner
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE organization_memberships SET role = $3
		WHERE organization_id = $1 AND user_id = $2
	`, organizationID, userID, role); err != nil {
		return UserSummary{}, fmt.Errorf("update user role: %w", err)
	}
	user, err := loadUserSummary(ctx, tx, organizationID, userID)
	if err != nil {
		return UserSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UserSummary{}, fmt.Errorf("commit role update: %w", err)
	}
	return user, nil
}

func lockMembership(ctx context.Context, tx pgx.Tx, organizationID, userID int64) (membershipRecord, error) {
	var membership membershipRecord
	err := tx.QueryRow(ctx, `
		SELECT om.role, COALESCE(om.membership_status, 'active'), u.email, u.first_name, u.last_name
		FROM organization_memberships om
		JOIN users u ON u.id = om.user_id
		WHERE om.organization_id = $1 AND om.user_id = $2
		FOR UPDATE OF om
	`, organizationID, userID).Scan(&membership.Role, &membership.Status, &membership.Email, &membership.FirstName, &membership.LastName)
	if errors.Is(err, pgx.ErrNoRows) {
		return membershipRecord{}, ErrNotFound
	}
	if err != nil {
		return membershipRecord{}, fmt.Errorf("lock organization membership: %w", err)
	}
	return membership, nil
}

func lockAndCountActiveOwners(ctx context.Context, tx pgx.Tx, organizationID int64) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT id FROM organization_memberships
		WHERE organization_id = $1 AND role = 'owner' AND COALESCE(membership_status, 'active') = 'active'
		FOR UPDATE
	`, organizationID)
	if err != nil {
		return 0, fmt.Errorf("lock active owners: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate active owners: %w", err)
	}
	return count, nil
}

func validateReplacement(ctx context.Context, tx pgx.Tx, organizationID, disabledUserID, replacementUserID int64) error {
	if replacementUserID == 0 {
		return nil
	}
	if replacementUserID == disabledUserID {
		return ErrInvalidReassignment
	}
	var active bool
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(membership_status, 'active') = 'active'
		FROM organization_memberships
		WHERE organization_id = $1 AND user_id = $2
		FOR SHARE
	`, organizationID, replacementUserID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidReassignment
	}
	if err != nil {
		return fmt.Errorf("validate work reassignment member: %w", err)
	}
	if !active {
		return ErrInvalidReassignment
	}
	return nil
}

func reassignOperationalWork(ctx context.Context, tx pgx.Tx, organizationID, userID, replacementUserID, actorUserID int64) (WorkCounts, error) {
	var replacement any
	if replacementUserID > 0 {
		replacement = replacementUserID
	}
	updates := []struct {
		name  string
		query string
		count *int64
	}{
		{"contacts", `UPDATE contacts SET owner_user_id = $3, updated_at = NOW() WHERE organization_id = $1 AND owner_user_id = $2 AND archived_at IS NULL`, nil},
		{"companies", `UPDATE companies SET owner_user_id = $3, updated_at = NOW() WHERE organization_id = $1 AND owner_user_id = $2 AND archived_at IS NULL`, nil},
		{"shared inbox", `UPDATE email_messages SET shared_inbox_assigned_to_user_id = $3, shared_inbox_updated_at = NOW() WHERE organization_id = $1 AND shared_inbox_assigned_to_user_id = $2 AND direction = 'inbound' AND visibility = 'shared' AND shared_inbox_status = 'open'`, nil},
		{"lead routing rules", `UPDATE lead_scoring_rules SET assign_to_user_id = $3, updated_at = NOW() WHERE organization_id = $1 AND assign_to_user_id = $2 AND is_active = TRUE`, nil},
		{"calendar events", `UPDATE calendar_events SET calendar_user_id = $3, updated_at = NOW() WHERE organization_id = $1 AND calendar_user_id = $2 AND status = 'scheduled' AND end_at > NOW()`, nil},
	}
	counts := WorkCounts{}
	updates[0].count = &counts.Contacts
	updates[1].count = &counts.Companies
	updates[2].count = &counts.SharedInbox
	updates[3].count = &counts.LeadRoutingRules
	updates[4].count = &counts.CalendarEvents
	for _, update := range updates {
		tag, err := tx.Exec(ctx, update.query, organizationID, userID, replacement)
		if err != nil {
			return WorkCounts{}, fmt.Errorf("reassign %s: %w", update.name, err)
		}
		*update.count = tag.RowsAffected()
	}
	dealRows, err := tx.Query(ctx, `
		UPDATE deals
		SET owner_user_id=$3,
		    owner_assignment_version=COALESCE(owner_assignment_version,0)+1,
		    updated_at=NOW()
		WHERE organization_id=$1 AND owner_user_id=$2 AND archived_at IS NULL
		RETURNING id
	`, organizationID, userID, replacement)
	if err != nil {
		return WorkCounts{}, fmt.Errorf("reassign deals: %w", err)
	}
	dealIDs := []int64{}
	for dealRows.Next() {
		var dealID int64
		if err := dealRows.Scan(&dealID); err != nil {
			dealRows.Close()
			return WorkCounts{}, fmt.Errorf("scan reassigned deal: %w", err)
		}
		dealIDs = append(dealIDs, dealID)
	}
	if err := dealRows.Err(); err != nil {
		dealRows.Close()
		return WorkCounts{}, fmt.Errorf("iterate reassigned deals: %w", err)
	}
	dealRows.Close()
	counts.Deals = int64(len(dealIDs))
	if err := modulenotifications.RecordDealAssignments(ctx, tx, organizationID, dealIDs, actorUserID); err != nil {
		return WorkCounts{}, err
	}
	taskRows, err := tx.Query(ctx, `
		UPDATE tasks
		SET assigned_to_user_id=$3,reminder_version=COALESCE(reminder_version,0)+1,updated_at=NOW()
		WHERE organization_id=$1 AND assigned_to_user_id=$2 AND archived_at IS NULL
		RETURNING id
	`, organizationID, userID, replacement)
	if err != nil {
		return WorkCounts{}, fmt.Errorf("reassign tasks: %w", err)
	}
	taskIDs := []int64{}
	for taskRows.Next() {
		var taskID int64
		if err := taskRows.Scan(&taskID); err != nil {
			taskRows.Close()
			return WorkCounts{}, fmt.Errorf("scan reassigned task: %w", err)
		}
		taskIDs = append(taskIDs, taskID)
	}
	if err := taskRows.Err(); err != nil {
		taskRows.Close()
		return WorkCounts{}, fmt.Errorf("iterate reassigned tasks: %w", err)
	}
	taskRows.Close()
	counts.Tasks = int64(len(taskIDs))
	if err := moduletaskreminders.LoadAndSync(ctx, tx, organizationID, taskIDs, actorUserID, replacementUserID > 0); err != nil {
		return WorkCounts{}, fmt.Errorf("refresh reassigned task reminders: %w", err)
	}
	return counts, nil
}

func stopDisabledUserEffects(ctx context.Context, tx pgx.Tx, organizationID, userID int64) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM record_followers
		WHERE organization_id = $1 AND user_id = $2
	`, organizationID, userID); err != nil {
		return fmt.Errorf("remove disabled user record subscriptions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE calendar_event_reminders reminder
		SET status = 'skipped', updated_at = NOW()
		WHERE reminder.organization_id = $1 AND reminder.user_id = $2 AND reminder.status = 'pending'
	`, organizationID, userID); err != nil {
		return fmt.Errorf("skip disabled user reminders: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE task_reminders
		SET status='skipped',updated_at=NOW()
		WHERE organization_id=$1 AND user_id=$2 AND status='pending'
	`, organizationID, userID); err != nil {
		return fmt.Errorf("skip disabled user task reminders: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE user_email_accounts
		SET sync_enabled = FALSE, sync_status = 'disabled', last_sync_error = '', updated_at = NOW()
		WHERE organization_id = $1 AND user_id = $2
	`, organizationID, userID); err != nil {
		return fmt.Errorf("disable user mailbox sync: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE deal_quote_deliveries
		SET status = CASE status WHEN 'sending' THEN 'uncertain' ELSE 'failed' END,
		    last_error = CASE status
		      WHEN 'sending' THEN 'The sender was disabled while the mailbox provider outcome may be unknown.'
		      ELSE 'The sender was disabled before quote delivery.'
		    END,
		    finalized_at = NOW(), updated_at = NOW()
		WHERE organization_id = $1 AND actor_user_id = $2
		  AND status IN ('prepared', 'sending')
	`, organizationID, userID); err != nil {
		return fmt.Errorf("quiesce disabled user quote deliveries: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE background_jobs
		SET status = 'succeeded', result_json = '{"status":"skipped","reason":"member_disabled"}'::jsonb,
		    completed_at = NOW(), updated_at = NOW(), last_error = ''
		WHERE organization_id = $1
		  AND status IN ('pending', 'retryable')
		  AND (
			(job_type = 'mailbox.sync' AND payload_json->>'userId' = ($2::bigint)::text)
			OR (job_type = 'calendar.reminder' AND idempotency_key IN (
				SELECT 'reminder:' || id::text FROM calendar_event_reminders
				WHERE organization_id = $1 AND user_id = $2::bigint AND status = 'skipped'
			))
			OR (job_type = 'task.reminder' AND idempotency_key IN (
				SELECT 'task-reminder:' || id::text FROM task_reminders
				WHERE organization_id=$1 AND user_id=$2::bigint AND status='skipped'
			))
		  )
	`, organizationID, userID); err != nil {
		return fmt.Errorf("quiesce disabled user jobs: %w", err)
	}
	return nil
}

func recordLifecycleAudit(ctx context.Context, tx pgx.Tx, organizationID, userID, actorUserID int64, target membershipRecord, input SetStatusInput, result LifecycleResult) error {
	replacement := "unassigned"
	if input.ReassignToUserID > 0 {
		replacement = strconv.FormatInt(input.ReassignToUserID, 10)
	}
	metadata, err := json.Marshal(map[string]string{
		"email":                    target.Email,
		"status":                   input.Status,
		"reassignToUserId":         replacement,
		"contactsReassigned":       strconv.FormatInt(result.Reassigned.Contacts, 10),
		"companiesReassigned":      strconv.FormatInt(result.Reassigned.Companies, 10),
		"dealsReassigned":          strconv.FormatInt(result.Reassigned.Deals, 10),
		"tasksReassigned":          strconv.FormatInt(result.Reassigned.Tasks, 10),
		"sharedInboxReassigned":    strconv.FormatInt(result.Reassigned.SharedInbox, 10),
		"leadRoutingReassigned":    strconv.FormatInt(result.Reassigned.LeadRoutingRules, 10),
		"calendarEventsReassigned": strconv.FormatInt(result.Reassigned.CalendarEvents, 10),
		"sessionsInvalidated":      strconv.FormatInt(result.SessionsInvalidated, 10),
	})
	if err != nil {
		return fmt.Errorf("encode user lifecycle audit: %w", err)
	}
	eventType := "user.reactivated"
	summary := fmt.Sprintf("Reactivated %s", target.Email)
	if input.Status == MembershipStatusDisabled {
		eventType = "user.disabled"
		summary = fmt.Sprintf("Disabled %s and reassigned %d active work items", target.Email, result.Reassigned.Total())
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, event_type, entity_type, entity_id, summary, metadata_json)
		VALUES ($1, $2, $3, 'user', $4, $5, $6::jsonb)
	`, organizationID, actorUserID, eventType, userID, summary, string(metadata)); err != nil {
		return fmt.Errorf("record user lifecycle audit: %w", err)
	}
	return nil
}

type lifecycleQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadUserSummary(ctx context.Context, query lifecycleQueryRower, organizationID, userID int64) (UserSummary, error) {
	var user UserSummary
	err := query.QueryRow(ctx, `
		SELECT u.id, u.email, u.first_name, u.last_name, om.role,
			COALESCE(om.membership_status, 'active'), om.status_changed_at,
			(SELECT COUNT(*) FROM contacts record WHERE record.organization_id = om.organization_id AND record.owner_user_id = u.id AND record.archived_at IS NULL),
			(SELECT COUNT(*) FROM companies record WHERE record.organization_id = om.organization_id AND record.owner_user_id = u.id AND record.archived_at IS NULL),
			(SELECT COUNT(*) FROM deals record WHERE record.organization_id = om.organization_id AND record.owner_user_id = u.id AND record.archived_at IS NULL),
			(SELECT COUNT(*) FROM tasks record WHERE record.organization_id = om.organization_id AND record.assigned_to_user_id = u.id AND record.archived_at IS NULL),
			(SELECT COUNT(*) FROM email_messages record WHERE record.organization_id = om.organization_id AND record.shared_inbox_assigned_to_user_id = u.id AND record.direction = 'inbound' AND record.visibility = 'shared' AND record.shared_inbox_status = 'open'),
			(SELECT COUNT(*) FROM lead_scoring_rules record WHERE record.organization_id = om.organization_id AND record.assign_to_user_id = u.id AND record.is_active = TRUE),
			(SELECT COUNT(*) FROM calendar_events record WHERE record.organization_id = om.organization_id AND record.calendar_user_id = u.id AND record.status = 'scheduled' AND record.end_at > NOW()),
			(u.password_setup_token_hash IS NOT NULL AND u.password_setup_consumed_at IS NULL AND u.password_setup_revoked_at IS NULL),
			CASE
				WHEN u.password_setup_consumed_at IS NOT NULL THEN 'accepted'
				WHEN u.password_setup_revoked_at IS NOT NULL THEN 'revoked'
				WHEN u.password_setup_token_hash IS NOT NULL AND u.password_setup_expires_at <= NOW() THEN 'expired'
				WHEN u.password_setup_token_hash IS NOT NULL THEN 'pending'
				ELSE ''
			END,
			u.password_setup_expires_at,
			COALESCE(u.password_setup_delivery_status, '')
		FROM organization_memberships om
		JOIN users u ON u.id = om.user_id
		WHERE om.organization_id = $1 AND om.user_id = $2
	`, organizationID, userID).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.Role,
		&user.Status, &user.StatusChangedAt,
		&user.OwnedWork.Contacts, &user.OwnedWork.Companies, &user.OwnedWork.Deals,
		&user.OwnedWork.Tasks, &user.OwnedWork.SharedInbox, &user.OwnedWork.LeadRoutingRules,
		&user.OwnedWork.CalendarEvents, &user.SetupPending,
		&user.InvitationStatus, &user.InvitationExpiresAt, &user.InvitationDeliveryStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserSummary{}, ErrNotFound
	}
	if err != nil {
		return UserSummary{}, fmt.Errorf("load organization user summary: %w", err)
	}
	return user, nil
}
