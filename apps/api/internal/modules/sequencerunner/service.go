// Package sequencerunner sends due email sequence steps through each enrolling
// user's connected mailbox and advances enrollment scheduler state.
package sequencerunner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
)

const (
	defaultBatchLimit        = 10
	defaultInterval          = 15 * time.Minute
	startupDelay             = 2 * time.Minute
	failureRetryDelayMinutes = 60
)

var ErrNotConfigured = errors.New("email sequence runner not configured")

type sequenceStore interface {
	ListDueSends(context.Context, int) ([]moduleemailsequences.DueSend, error)
	MarkStepSent(context.Context, int64, int64, int) error
	PostponeEnrollment(context.Context, int64, int64, int) error
}

type mailboxSender interface {
	Configured() bool
	SendAs(context.Context, int64, int64, string, string, string, string) error
}

type messageStore interface {
	Record(context.Context, int64, moduleemailmessages.RecordInput) error
}

type Summary struct {
	Attempted int
	Sent      int
	Failed    int
}

type Service struct {
	sequences sequenceStore
	sender    mailboxSender
	messages  messageStore
	limit     int
}

func NewService(sequences sequenceStore, sender mailboxSender, messages messageStore) *Service {
	return &Service{sequences: sequences, sender: sender, messages: messages, limit: defaultBatchLimit}
}

func (s *Service) Configured() bool {
	return s != nil && s.sequences != nil && s.sender != nil && s.sender.Configured() && s.messages != nil
}

func (s *Service) SendDue(ctx context.Context, limit int) (Summary, error) {
	if !s.Configured() {
		return Summary{}, ErrNotConfigured
	}
	if limit <= 0 || limit > 100 {
		limit = s.limit
	}
	due, err := s.sequences.ListDueSends(ctx, limit)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{}
	for _, send := range due {
		summary.Attempted++
		if err := s.sendOne(ctx, send); err != nil {
			summary.Failed++
			continue
		}
		summary.Sent++
	}
	return summary, nil
}

func (s *Service) sendOne(ctx context.Context, send moduleemailsequences.DueSend) error {
	subject := strings.TrimSpace(moduleemailtemplates.Render(send.Subject, contactFields(send)))
	body := strings.TrimSpace(moduleemailtemplates.Render(send.Body, contactFields(send)))
	if send.OrganizationID <= 0 || send.EnrollmentID <= 0 || send.EnrolledByUserID <= 0 || send.ContactID <= 0 || strings.TrimSpace(send.ContactEmail) == "" || subject == "" || body == "" {
		_ = s.sequences.PostponeEnrollment(ctx, send.OrganizationID, send.EnrollmentID, failureRetryDelayMinutes)
		return fmt.Errorf("invalid due sequence send")
	}

	if err := s.sender.SendAs(ctx, send.OrganizationID, send.EnrolledByUserID, send.ContactEmail, subject, body, ""); err != nil {
		s.record(ctx, send, subject, body, "failed", err.Error())
		_ = s.sequences.PostponeEnrollment(ctx, send.OrganizationID, send.EnrollmentID, failureRetryDelayMinutes)
		return err
	}
	s.record(ctx, send, subject, body, "sent", "")
	if err := s.sequences.MarkStepSent(ctx, send.OrganizationID, send.EnrollmentID, send.CurrentStepOrder); err != nil {
		return err
	}
	return nil
}

func (s *Service) record(ctx context.Context, send moduleemailsequences.DueSend, subject, body, status, errMsg string) {
	if s.messages == nil {
		return
	}
	_ = s.messages.Record(ctx, send.OrganizationID, moduleemailmessages.RecordInput{
		ToEmail:      send.ContactEmail,
		Subject:      subject,
		Body:         body,
		Status:       status,
		Error:        errMsg,
		EntityType:   "contact",
		EntityID:     send.ContactID,
		SentByUserID: send.EnrolledByUserID,
	})
}

func contactFields(send moduleemailsequences.DueSend) map[string]string {
	fullName := strings.TrimSpace(send.ContactFirstName + " " + send.ContactLastName)
	return map[string]string{
		"first_name": send.ContactFirstName,
		"last_name":  send.ContactLastName,
		"full_name":  fullName,
		"email":      send.ContactEmail,
		"job_title":  send.ContactJobTitle,
	}
}

func (s *Service) RunWorker(ctx context.Context, logger *slog.Logger, interval time.Duration, limit int) {
	if !s.Configured() {
		return
	}
	if interval <= 0 {
		interval = defaultInterval
	}
	if limit <= 0 || limit > 100 {
		limit = defaultBatchLimit
	}
	timer := time.NewTimer(startupDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			summary, err := s.SendDue(ctx, limit)
			if err != nil {
				if logger != nil {
					logger.Warn("email sequence runner failed", "error", err)
				}
			} else if summary.Attempted > 0 && logger != nil {
				logger.Info("email sequence runner completed", "attempted", summary.Attempted, "sent", summary.Sent, "failed", summary.Failed)
			}
			timer.Reset(interval)
		}
	}
}
