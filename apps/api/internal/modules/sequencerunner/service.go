// Package sequencerunner sends due email sequence steps through each enrolling
// user's connected mailbox and advances enrollment scheduler state.
package sequencerunner

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

const (
	defaultBatchLimit        = 10
	failureRetryDelayMinutes = 60
)

var (
	ErrNotConfigured = errors.New("email sequence runner not configured")
	ErrSuppressed    = errors.New("email recipient suppressed")
	ErrInvalidJob    = errors.New("invalid email sequence send job")
)

type sequenceStore interface {
	ListDueSends(context.Context, int) ([]moduleemailsequences.DueSend, error)
	MarkStepSent(context.Context, int64, int64, int) error
	PostponeEnrollment(context.Context, int64, int64, int) error
}

type durableSequenceStore interface {
	LoadScheduledSend(context.Context, int64, int64, int) (moduleemailsequences.DueSend, error)
	PrepareDelivery(context.Context, moduleemailsequences.DueSend, string, string, string) (moduleemailsequences.Delivery, error)
	ClaimDelivery(context.Context, int64, int64, int) (moduleemailsequences.Delivery, error)
	FinalizeSent(context.Context, int64, int64, int) error
	FinalizeSuppressed(context.Context, int64, int64, int) error
	MarkDeliveryUncertain(context.Context, int64, int64, int, error) error
}

type mailboxSender interface {
	Configured() bool
	SendAs(context.Context, int64, int64, string, string, string, string) error
}

type messageStore interface {
	Record(context.Context, int64, moduleemailmessages.RecordInput) error
}

type suppressionStore interface {
	IsSuppressed(context.Context, int64, string) (bool, error)
	UnsubscribeToken(int64, string) (string, error)
}

type Summary struct {
	Attempted int
	Sent      int
	Failed    int
}

type Service struct {
	sequences          sequenceStore
	sender             mailboxSender
	messages           messageStore
	suppressions       suppressionStore
	unsubscribeBaseURL string
	limit              int
}

func NewService(sequences sequenceStore, sender mailboxSender, messages messageStore) *Service {
	return NewServiceWithSuppressions(sequences, sender, messages, nil, "")
}

func NewServiceWithSuppressions(sequences sequenceStore, sender mailboxSender, messages messageStore, suppressions suppressionStore, unsubscribeBaseURL string) *Service {
	return &Service{sequences: sequences, sender: sender, messages: messages, suppressions: suppressions, unsubscribeBaseURL: strings.TrimRight(strings.TrimSpace(unsubscribeBaseURL), "/"), limit: defaultBatchLimit}
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

// HandleJob executes one durable sequence send. Once an SMTP attempt begins,
// any unconfirmed outcome is dead-lettered for operator review rather than
// retried automatically, preventing silent duplicate customer email.
func (s *Service) HandleJob(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}
	store, ok := s.sequences.(durableSequenceStore)
	if !ok {
		return nil, ErrNotConfigured
	}
	enrollmentID, stepOrder, err := sequenceJobIDs(job)
	if err != nil {
		return nil, modulejobs.Permanent(err)
	}
	send, err := store.LoadScheduledSend(ctx, job.OrganizationID, enrollmentID, stepOrder)
	if errors.Is(err, moduleemailsequences.ErrNotFound) {
		return map[string]any{"status": "skipped", "reason": "enrollment is no longer due"}, nil
	}
	if err != nil {
		return nil, err
	}
	subject := strings.TrimSpace(moduleemailtemplates.Render(send.Subject, contactFields(send)))
	body := strings.TrimSpace(moduleemailtemplates.Render(send.Body, contactFields(send)))
	if send.EnrolledByUserID <= 0 || send.ContactID <= 0 || strings.TrimSpace(send.ContactEmail) == "" || subject == "" || body == "" {
		return nil, modulejobs.Permanent(fmt.Errorf("%w: scheduled sequence send is incomplete", ErrInvalidJob))
	}
	unsubscribeURL := s.unsubscribeURL(send.OrganizationID, send.ContactEmail)
	bodyToSend := textBodyWithUnsubscribe(body, unsubscribeURL)
	htmlBody := htmlBodyWithUnsubscribe(body, unsubscribeURL)
	delivery, err := store.PrepareDelivery(ctx, send, subject, bodyToSend, htmlBody)
	if err != nil {
		if errors.Is(err, moduleemailsequences.ErrInvalidInput) {
			return nil, modulejobs.Permanent(err)
		}
		return nil, err
	}
	switch delivery.Status {
	case "sent", "suppressed":
		return map[string]any{"status": delivery.Status, "deliveryId": delivery.ID}, nil
	case "sending", "uncertain":
		return nil, modulejobs.Permanent(moduleemailsequences.ErrDeliveryUncertain)
	}

	if s.suppressions != nil {
		suppressed, err := s.suppressions.IsSuppressed(ctx, send.OrganizationID, delivery.RecipientEmail)
		if err != nil {
			return nil, err
		}
		if suppressed {
			if err := store.FinalizeSuppressed(ctx, send.OrganizationID, send.EnrollmentID, send.CurrentStepOrder); err != nil {
				return nil, err
			}
			s.record(ctx, send, delivery.Subject, delivery.TextBody, "failed", "Recipient has unsubscribed from email.")
			return map[string]any{"status": "suppressed", "deliveryId": delivery.ID}, nil
		}
	}

	delivery, err = store.ClaimDelivery(ctx, send.OrganizationID, send.EnrollmentID, send.CurrentStepOrder)
	if errors.Is(err, moduleemailsequences.ErrDeliveryAlreadyFinalized) {
		return map[string]any{"status": delivery.Status, "deliveryId": delivery.ID}, nil
	}
	if errors.Is(err, moduleemailsequences.ErrDeliveryUncertain) {
		return nil, modulejobs.Permanent(err)
	}
	if err != nil {
		return nil, err
	}
	if err := s.sender.SendAs(ctx, send.OrganizationID, send.EnrolledByUserID, delivery.RecipientEmail, delivery.Subject, delivery.TextBody, delivery.HTMLBody); err != nil {
		_ = store.MarkDeliveryUncertain(ctx, send.OrganizationID, send.EnrollmentID, send.CurrentStepOrder, err)
		s.record(ctx, send, delivery.Subject, delivery.TextBody, "failed", err.Error())
		return nil, modulejobs.Permanent(fmt.Errorf("%w: %v", moduleemailsequences.ErrDeliveryUncertain, err))
	}
	if err := store.FinalizeSent(ctx, send.OrganizationID, send.EnrollmentID, send.CurrentStepOrder); err != nil {
		_ = store.MarkDeliveryUncertain(ctx, send.OrganizationID, send.EnrollmentID, send.CurrentStepOrder, err)
		return nil, modulejobs.Permanent(fmt.Errorf("%w: sent through SMTP but state finalization failed: %v", moduleemailsequences.ErrDeliveryUncertain, err))
	}
	s.record(ctx, send, delivery.Subject, delivery.TextBody, "sent", "")
	return map[string]any{"status": "sent", "deliveryId": delivery.ID}, nil
}

func sequenceJobIDs(job modulejobs.Job) (int64, int, error) {
	if job.OrganizationID <= 0 {
		return 0, 0, ErrInvalidJob
	}
	enrollmentValue, enrollmentOK := job.Payload["enrollmentId"].(string)
	stepValue, stepOK := job.Payload["stepOrder"].(string)
	if !enrollmentOK || !stepOK {
		return 0, 0, ErrInvalidJob
	}
	enrollmentID, enrollmentErr := strconv.ParseInt(enrollmentValue, 10, 64)
	stepOrder, stepErr := strconv.Atoi(stepValue)
	if enrollmentErr != nil || stepErr != nil || enrollmentID <= 0 || stepOrder <= 0 {
		return 0, 0, ErrInvalidJob
	}
	return enrollmentID, stepOrder, nil
}

func (s *Service) sendOne(ctx context.Context, send moduleemailsequences.DueSend) error {
	subject := strings.TrimSpace(moduleemailtemplates.Render(send.Subject, contactFields(send)))
	body := strings.TrimSpace(moduleemailtemplates.Render(send.Body, contactFields(send)))
	if send.OrganizationID <= 0 || send.EnrollmentID <= 0 || send.EnrolledByUserID <= 0 || send.ContactID <= 0 || strings.TrimSpace(send.ContactEmail) == "" || subject == "" || body == "" {
		_ = s.sequences.PostponeEnrollment(ctx, send.OrganizationID, send.EnrollmentID, failureRetryDelayMinutes)
		return fmt.Errorf("invalid due sequence send")
	}
	if s.suppressions != nil {
		suppressed, err := s.suppressions.IsSuppressed(ctx, send.OrganizationID, send.ContactEmail)
		if err != nil {
			s.record(ctx, send, subject, body, "failed", "Unable to check email suppression status.")
			_ = s.sequences.PostponeEnrollment(ctx, send.OrganizationID, send.EnrollmentID, failureRetryDelayMinutes)
			return err
		}
		if suppressed {
			s.record(ctx, send, subject, body, "failed", "Recipient has unsubscribed from email.")
			if err := s.sequences.MarkStepSent(ctx, send.OrganizationID, send.EnrollmentID, send.CurrentStepOrder); err != nil {
				return err
			}
			return ErrSuppressed
		}
	}
	unsubscribeURL := s.unsubscribeURL(send.OrganizationID, send.ContactEmail)
	bodyToSend := textBodyWithUnsubscribe(body, unsubscribeURL)
	htmlBody := htmlBodyWithUnsubscribe(body, unsubscribeURL)

	if err := s.sender.SendAs(ctx, send.OrganizationID, send.EnrolledByUserID, send.ContactEmail, subject, bodyToSend, htmlBody); err != nil {
		s.record(ctx, send, subject, bodyToSend, "failed", err.Error())
		_ = s.sequences.PostponeEnrollment(ctx, send.OrganizationID, send.EnrollmentID, failureRetryDelayMinutes)
		return err
	}
	s.record(ctx, send, subject, bodyToSend, "sent", "")
	if err := s.sequences.MarkStepSent(ctx, send.OrganizationID, send.EnrollmentID, send.CurrentStepOrder); err != nil {
		return err
	}
	return nil
}

func (s *Service) unsubscribeURL(organizationID int64, email string) string {
	if s.suppressions == nil || s.unsubscribeBaseURL == "" {
		return ""
	}
	token, err := s.suppressions.UnsubscribeToken(organizationID, email)
	if err != nil || strings.TrimSpace(token) == "" {
		return ""
	}
	return s.unsubscribeBaseURL + "/api/email-unsubscribe/" + url.PathEscape(token)
}

func textBodyWithUnsubscribe(body, unsubscribeURL string) string {
	if unsubscribeURL == "" {
		return body
	}
	return strings.TrimRight(body, " \t\r\n") + "\n\nTo stop receiving emails from us, unsubscribe here: " + unsubscribeURL
}

func htmlBodyWithUnsubscribe(body, unsubscribeURL string) string {
	if unsubscribeURL == "" {
		return ""
	}
	return `<!doctype html><html><body><div>` + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(htmlEscape(body), "\r\n", "\n"), "\r", "\n"), "\n", "<br>\n") + `</div><p style="margin-top:24px;font-size:12px;color:#666">To stop receiving emails from us, <a href="` + htmlEscape(unsubscribeURL) + `">unsubscribe here</a>.</p></body></html>`
}

func htmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	return strings.ReplaceAll(value, "'", "&#39;")
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
