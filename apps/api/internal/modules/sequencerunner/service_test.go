package sequencerunner

import (
	"context"
	"errors"
	"strings"
	"testing"

	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

type fakeSequenceStore struct {
	due              []moduleemailsequences.DueSend
	listLimit        int
	markedOrgID      int64
	markedEnrollID   int64
	markedStep       int
	postponeOrgID    int64
	postponeEnrollID int64
	postponeMinutes  int
	markErr          error
	postponeErr      error
}

type fakeDurableSequenceStore struct {
	*fakeSequenceStore
	delivery            moduleemailsequences.Delivery
	loadErr             error
	prepareErr          error
	claimErr            error
	finalizeSent        int
	finalizeSuppressed  int
	markedUncertain     int
	markUncertainReason error
}

func (f *fakeDurableSequenceStore) LoadScheduledSend(_ context.Context, _, _ int64, _ int) (moduleemailsequences.DueSend, error) {
	if f.loadErr != nil {
		return moduleemailsequences.DueSend{}, f.loadErr
	}
	return f.due[0], nil
}

func (f *fakeDurableSequenceStore) PrepareDelivery(_ context.Context, send moduleemailsequences.DueSend, subject, textBody, htmlBody string) (moduleemailsequences.Delivery, error) {
	if f.prepareErr != nil {
		return moduleemailsequences.Delivery{}, f.prepareErr
	}
	if f.delivery.Status == "" {
		f.delivery = moduleemailsequences.Delivery{ID: 11, OrganizationID: send.OrganizationID, EnrollmentID: send.EnrollmentID, StepOrder: send.CurrentStepOrder, RecipientEmail: send.ContactEmail, Subject: subject, TextBody: textBody, HTMLBody: htmlBody, Status: "queued"}
	}
	return f.delivery, nil
}

func (f *fakeDurableSequenceStore) ClaimDelivery(context.Context, int64, int64, int) (moduleemailsequences.Delivery, error) {
	if f.claimErr != nil {
		return f.delivery, f.claimErr
	}
	f.delivery.Status = "sending"
	return f.delivery, nil
}

func (f *fakeDurableSequenceStore) FinalizeSent(context.Context, int64, int64, int) error {
	f.finalizeSent++
	f.delivery.Status = "sent"
	return f.markErr
}

func (f *fakeDurableSequenceStore) FinalizeSuppressed(context.Context, int64, int64, int) error {
	f.finalizeSuppressed++
	f.delivery.Status = "suppressed"
	return f.markErr
}

func (f *fakeDurableSequenceStore) MarkDeliveryUncertain(_ context.Context, _, _ int64, _ int, failure error) error {
	f.markedUncertain++
	f.markUncertainReason = failure
	f.delivery.Status = "uncertain"
	return nil
}

func (f *fakeSequenceStore) ListDueSends(_ context.Context, limit int) ([]moduleemailsequences.DueSend, error) {
	f.listLimit = limit
	return f.due, nil
}

func (f *fakeSequenceStore) MarkStepSent(_ context.Context, organizationID, enrollmentID int64, currentStepOrder int) error {
	f.markedOrgID = organizationID
	f.markedEnrollID = enrollmentID
	f.markedStep = currentStepOrder
	return f.markErr
}

func (f *fakeSequenceStore) PostponeEnrollment(_ context.Context, organizationID, enrollmentID int64, retryMinutes int) error {
	f.postponeOrgID = organizationID
	f.postponeEnrollID = enrollmentID
	f.postponeMinutes = retryMinutes
	return f.postponeErr
}

type fakeMailboxSender struct {
	configured bool
	err        error
	orgID      int64
	userID     int64
	to         string
	subject    string
	body       string
	htmlBody   string
	calls      int
}

func (f *fakeMailboxSender) Configured() bool { return f.configured }

func (f *fakeMailboxSender) SendAs(_ context.Context, organizationID, userID int64, to, subject, textBody, htmlBody string) error {
	f.calls++
	f.orgID = organizationID
	f.userID = userID
	f.to = to
	f.subject = subject
	f.body = textBody
	f.htmlBody = htmlBody
	return f.err
}

func sequenceJob() modulejobs.Job {
	return modulejobs.Job{OrganizationID: 42, Type: moduleemailsequences.SequenceSendJobType, Payload: map[string]any{"enrollmentId": "9", "stepOrder": "1"}}
}

type fakeSuppressionStore struct {
	suppressed  bool
	isErr       error
	token       string
	tokenErr    error
	lastOrgID   int64
	lastEmail   string
	isCalled    bool
	tokenCalled bool
}

func (f *fakeSuppressionStore) IsSuppressed(_ context.Context, organizationID int64, email string) (bool, error) {
	f.isCalled = true
	f.lastOrgID = organizationID
	f.lastEmail = email
	return f.suppressed, f.isErr
}

func (f *fakeSuppressionStore) UnsubscribeToken(organizationID int64, email string) (string, error) {
	f.tokenCalled = true
	f.lastOrgID = organizationID
	f.lastEmail = email
	return f.token, f.tokenErr
}

type fakeMessageStore struct {
	orgID  int64
	input  moduleemailmessages.RecordInput
	called bool
}

func (f *fakeMessageStore) Record(_ context.Context, organizationID int64, input moduleemailmessages.RecordInput) error {
	f.called = true
	f.orgID = organizationID
	f.input = input
	return nil
}

func dueSend() moduleemailsequences.DueSend {
	return moduleemailsequences.DueSend{
		OrganizationID:   42,
		EnrollmentID:     9,
		SequenceID:       3,
		ContactID:        7,
		EnrolledByUserID: 2,
		CurrentStepOrder: 1,
		ContactFirstName: "Ada",
		ContactLastName:  "Lovelace",
		ContactEmail:     "ada@acme.test",
		ContactJobTitle:  "CTO",
		Subject:          "Hello {{first_name}}",
		Body:             "Hi {{full_name}}, following up.",
	}
}

func TestSendDueSendsAndAdvancesEnrollment(t *testing.T) {
	sequences := &fakeSequenceStore{due: []moduleemailsequences.DueSend{dueSend()}}
	sender := &fakeMailboxSender{configured: true}
	messages := &fakeMessageStore{}
	service := NewService(sequences, sender, messages)

	summary, err := service.SendDue(context.Background(), 5)
	if err != nil {
		t.Fatalf("send due: %v", err)
	}
	if summary.Attempted != 1 || summary.Sent != 1 || summary.Failed != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if sender.orgID != 42 || sender.userID != 2 || sender.to != "ada@acme.test" || sender.subject != "Hello Ada" || sender.body != "Hi Ada Lovelace, following up." {
		t.Fatalf("unexpected send: %#v", sender)
	}
	if sequences.markedOrgID != 42 || sequences.markedEnrollID != 9 || sequences.markedStep != 1 {
		t.Fatalf("expected enrollment to be advanced, got %#v", sequences)
	}
	if !messages.called || messages.input.Status != "sent" || messages.input.EntityType != "contact" || messages.input.EntityID != 7 || messages.input.SentByUserID != 2 {
		t.Fatalf("expected sent email log, got org=%d input=%#v", messages.orgID, messages.input)
	}
}

func TestSendDuePostponesAndRecordsFailedSend(t *testing.T) {
	sequences := &fakeSequenceStore{due: []moduleemailsequences.DueSend{dueSend()}}
	sender := &fakeMailboxSender{configured: true, err: errors.New("smtp offline")}
	messages := &fakeMessageStore{}
	service := NewService(sequences, sender, messages)

	summary, err := service.SendDue(context.Background(), 5)
	if err != nil {
		t.Fatalf("send due should continue after per-send failures: %v", err)
	}
	if summary.Attempted != 1 || summary.Sent != 0 || summary.Failed != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if sequences.markedEnrollID != 0 {
		t.Fatalf("failed send should not advance enrollment")
	}
	if sequences.postponeOrgID != 42 || sequences.postponeEnrollID != 9 || sequences.postponeMinutes != failureRetryDelayMinutes {
		t.Fatalf("expected failed enrollment to be postponed, got %#v", sequences)
	}
	if !messages.called || messages.input.Status != "failed" || messages.input.Error != "smtp offline" {
		t.Fatalf("expected failed email log, got %#v", messages.input)
	}
}

func TestSendDueAdvancesSuppressedRecipientWithoutRetry(t *testing.T) {
	sequences := &fakeSequenceStore{due: []moduleemailsequences.DueSend{dueSend()}}
	sender := &fakeMailboxSender{configured: true}
	messages := &fakeMessageStore{}
	suppressions := &fakeSuppressionStore{suppressed: true}
	service := NewServiceWithSuppressions(sequences, sender, messages, suppressions, "https://crm.example.test")

	summary, err := service.SendDue(context.Background(), 5)
	if err != nil {
		t.Fatalf("send due should continue after suppressed recipients: %v", err)
	}
	if summary.Attempted != 1 || summary.Sent != 0 || summary.Failed != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if sender.to != "" {
		t.Fatalf("suppressed recipient should not be sent email, got send %#v", sender)
	}
	if sequences.markedOrgID != 42 || sequences.markedEnrollID != 9 || sequences.markedStep != 1 {
		t.Fatalf("expected suppressed enrollment to advance, got %#v", sequences)
	}
	if sequences.postponeEnrollID != 0 {
		t.Fatalf("suppressed enrollment should not be retried, got %#v", sequences)
	}
	if !messages.called || messages.input.Status != "failed" || messages.input.Error != "Recipient has unsubscribed from email." {
		t.Fatalf("expected suppressed email log, got %#v", messages.input)
	}
	if !suppressions.isCalled || suppressions.lastOrgID != 42 || suppressions.lastEmail != "ada@acme.test" {
		t.Fatalf("expected suppression check, got %#v", suppressions)
	}
}

func TestSendDueAddsUnsubscribeFooterWhenConfigured(t *testing.T) {
	sequences := &fakeSequenceStore{due: []moduleemailsequences.DueSend{dueSend()}}
	sender := &fakeMailboxSender{configured: true}
	messages := &fakeMessageStore{}
	suppressions := &fakeSuppressionStore{token: "signed.token"}
	service := NewServiceWithSuppressions(sequences, sender, messages, suppressions, "https://crm.example.test/")

	summary, err := service.SendDue(context.Background(), 5)
	if err != nil {
		t.Fatalf("send due: %v", err)
	}
	if summary.Attempted != 1 || summary.Sent != 1 || summary.Failed != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	expectedURL := "https://crm.example.test/api/email-unsubscribe/signed.token"
	if !strings.Contains(sender.body, expectedURL) {
		t.Fatalf("expected text body to include unsubscribe URL %q, got %q", expectedURL, sender.body)
	}
	if !strings.Contains(sender.htmlBody, `<a href="`+expectedURL+`">unsubscribe here</a>`) {
		t.Fatalf("expected HTML body to include unsubscribe link, got %q", sender.htmlBody)
	}
	if messages.input.Body != sender.body {
		t.Fatalf("email log should record sent body with footer, got %q", messages.input.Body)
	}
	if !suppressions.tokenCalled || suppressions.lastOrgID != 42 || suppressions.lastEmail != "ada@acme.test" {
		t.Fatalf("expected unsubscribe token for recipient, got %#v", suppressions)
	}
}

func TestServiceRequiresConfiguredMailboxSender(t *testing.T) {
	service := NewService(&fakeSequenceStore{}, &fakeMailboxSender{configured: false}, &fakeMessageStore{})
	if service.Configured() {
		t.Fatalf("service should not be configured without mailbox sender storage")
	}
}

func TestHandleJobFinalizesDurableDelivery(t *testing.T) {
	sequences := &fakeDurableSequenceStore{fakeSequenceStore: &fakeSequenceStore{due: []moduleemailsequences.DueSend{dueSend()}}}
	sender := &fakeMailboxSender{configured: true}
	messages := &fakeMessageStore{}
	service := NewService(sequences, sender, messages)

	result, err := service.HandleJob(context.Background(), sequenceJob())
	if err != nil || result["status"] != "sent" || sender.calls != 1 || sequences.finalizeSent != 1 || sequences.markedUncertain != 0 {
		t.Fatalf("unexpected durable send result: result=%#v sender=%#v sequences=%#v err=%v", result, sender, sequences, err)
	}
	if !messages.called || messages.input.Status != "sent" {
		t.Fatalf("expected durable send to be logged, got %#v", messages.input)
	}
}

func TestHandleJobDefersPausedSequenceBeforeProviderBoundary(t *testing.T) {
	for _, test := range []struct {
		name     string
		loadErr  error
		claimErr error
	}{
		{name: "paused before load", loadErr: moduleemailsequences.ErrSequencePaused},
		{name: "paused before claim", claimErr: moduleemailsequences.ErrSequencePaused},
	} {
		t.Run(test.name, func(t *testing.T) {
			sequences := &fakeDurableSequenceStore{
				fakeSequenceStore: &fakeSequenceStore{due: []moduleemailsequences.DueSend{dueSend()}},
				loadErr:           test.loadErr,
				claimErr:          test.claimErr,
			}
			sender := &fakeMailboxSender{configured: true}
			service := NewService(sequences, sender, &fakeMessageStore{})

			result, err := service.HandleJob(context.Background(), sequenceJob())
			if result != nil || !errors.Is(err, moduleemailsequences.ErrSequencePaused) || sender.calls != 0 || sequences.finalizeSent != 0 {
				t.Fatalf("expected policy deferral before provider attempt: result=%#v sender=%#v sequences=%#v err=%v", result, sender, sequences, err)
			}
		})
	}
}

func TestHandleJobMakesAmbiguousSMTPFailurePermanent(t *testing.T) {
	sequences := &fakeDurableSequenceStore{fakeSequenceStore: &fakeSequenceStore{due: []moduleemailsequences.DueSend{dueSend()}}}
	sender := &fakeMailboxSender{configured: true, err: errors.New("connection reset after DATA")}
	service := NewService(sequences, sender, &fakeMessageStore{})

	_, err := service.HandleJob(context.Background(), sequenceJob())
	if !errors.Is(err, moduleemailsequences.ErrDeliveryUncertain) || sender.calls != 1 || sequences.markedUncertain != 1 || sequences.finalizeSent != 0 {
		t.Fatalf("expected ambiguous SMTP result to become uncertain once, sender=%#v sequences=%#v err=%v", sender, sequences, err)
	}
	if _, retryErr := service.HandleJob(context.Background(), sequenceJob()); !errors.Is(retryErr, moduleemailsequences.ErrDeliveryUncertain) || sender.calls != 1 {
		t.Fatalf("expected uncertain delivery not to resend, calls=%d err=%v", sender.calls, retryErr)
	}
}

func TestHandleJobQuarantinesARecoveredSendingStateBeforeRetry(t *testing.T) {
	sequences := &fakeDurableSequenceStore{
		fakeSequenceStore: &fakeSequenceStore{due: []moduleemailsequences.DueSend{dueSend()}},
		delivery:          moduleemailsequences.Delivery{ID: 11, Status: "sending"},
	}
	sender := &fakeMailboxSender{configured: true}
	service := NewService(sequences, sender, &fakeMessageStore{})

	_, err := service.HandleJob(context.Background(), sequenceJob())
	if !errors.Is(err, moduleemailsequences.ErrDeliveryUncertain) || sender.calls != 0 || sequences.markedUncertain != 1 || sequences.delivery.Status != "uncertain" {
		t.Fatalf("stale sending state must become operator-recoverable without another provider call: sender=%#v sequences=%#v err=%v", sender, sequences, err)
	}
}

func TestHandleJobFinalizesSuppressedDeliveryWithoutSMTP(t *testing.T) {
	sequences := &fakeDurableSequenceStore{fakeSequenceStore: &fakeSequenceStore{due: []moduleemailsequences.DueSend{dueSend()}}}
	sender := &fakeMailboxSender{configured: true}
	service := NewServiceWithSuppressions(sequences, sender, &fakeMessageStore{}, &fakeSuppressionStore{suppressed: true}, "")

	result, err := service.HandleJob(context.Background(), sequenceJob())
	if err != nil || result["status"] != "suppressed" || sender.calls != 0 || sequences.finalizeSuppressed != 1 {
		t.Fatalf("unexpected suppressed durable result: result=%#v sender=%#v sequences=%#v err=%v", result, sender, sequences, err)
	}
}

func TestSequenceJobIDsRejectsNumericJSONIDs(t *testing.T) {
	if _, _, err := sequenceJobIDs(modulejobs.Job{OrganizationID: 42, Payload: map[string]any{"enrollmentId": float64(9), "stepOrder": "1"}}); !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("expected numeric JSON id to be rejected, got %v", err)
	}
}
