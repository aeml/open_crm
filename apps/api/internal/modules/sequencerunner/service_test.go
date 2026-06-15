package sequencerunner

import (
	"context"
	"errors"
	"testing"

	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
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
}

func (f *fakeMailboxSender) Configured() bool { return f.configured }

func (f *fakeMailboxSender) SendAs(_ context.Context, organizationID, userID int64, to, subject, textBody, _ string) error {
	f.orgID = organizationID
	f.userID = userID
	f.to = to
	f.subject = subject
	f.body = textBody
	return f.err
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

func TestServiceRequiresConfiguredMailboxSender(t *testing.T) {
	service := NewService(&fakeSequenceStore{}, &fakeMailboxSender{configured: false}, &fakeMessageStore{})
	if service.Configured() {
		t.Fatalf("service should not be configured without mailbox sender storage")
	}
}
