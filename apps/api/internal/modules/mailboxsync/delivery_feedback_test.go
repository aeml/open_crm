package mailboxsync

import (
	"net/mail"
	"strings"
	"testing"
	"time"

	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestParseFetchedMessageExtractsPermanentDSNFeedback(t *testing.T) {
	raw := strings.Join([]string{
		"From: Mail Delivery System <mailer-daemon@provider.test>",
		"To: Rep <rep@acme.test>",
		"Subject: Delivery Status Notification (Failure)",
		"Message-ID: <dsn-1@provider.test>",
		"Date: Fri, 02 Jan 2026 04:04:05 +0000",
		"MIME-Version: 1.0",
		"Content-Type: multipart/report; report-type=delivery-status; boundary=dsn",
		"",
		"--dsn",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Delivery failed.",
		"--dsn",
		"Content-Type: message/delivery-status",
		"",
		"Reporting-MTA: dns; provider.test",
		"",
		"Final-Recipient: rfc822; lead@buyer.test",
		"Action: failed",
		"Status: 5.1.1",
		"Diagnostic-Code: smtp; 550 mailbox unavailable",
		"",
		"Final-Recipient: rfc822; delayed@buyer.test",
		"Action: delayed",
		"Status: 4.2.0",
		"--dsn",
		"Content-Type: message/rfc822",
		"",
		"From: rep@acme.test",
		"To: lead@buyer.test",
		"Message-ID: <opencrm.1234@crm.example.test>",
		"Subject: Follow up",
		"",
		"Hello",
		"--dsn--",
	}, "\r\n")

	message := parseFetchedMessage([]byte(raw), "dsn-provider-1", time.Time{}, moduleuseremail.SyncCredentials{FromEmail: "rep@acme.test"})
	if message.Body != "Delivery failed." || len(message.DeliveryFeedback) != 1 {
		t.Fatalf("unexpected DSN parse: %#v", message)
	}
	feedback := message.DeliveryFeedback[0]
	if feedback.Type != "bounce" || feedback.OriginalMessageID != "<opencrm.1234@crm.example.test>" || feedback.RecipientEmail != "lead@buyer.test" || feedback.Action != "failed" || feedback.StatusCode != "5.1.1" {
		t.Fatalf("unexpected DSN feedback: %#v", feedback)
	}
}

func TestParseFetchedMessageExtractsARFComplaintFeedback(t *testing.T) {
	raw := strings.Join([]string{
		"From: Feedback Loop <abuse@provider.test>",
		"To: rep@acme.test",
		"Subject: Abuse report",
		"Message-ID: <arf-1@provider.test>",
		"MIME-Version: 1.0",
		"Content-Type: multipart/report; report-type=feedback-report; boundary=arf",
		"",
		"--arf",
		"Content-Type: text/plain",
		"",
		"A recipient reported this message.",
		"--arf",
		"Content-Type: message/feedback-report",
		"",
		"Feedback-Type: abuse",
		"User-Agent: Provider/1.0",
		"Version: 1",
		"Original-Rcpt-To: rfc822; lead@buyer.test",
		"--arf",
		"Content-Type: text/rfc822-headers",
		"",
		"From: rep@acme.test",
		"To: lead@buyer.test",
		"Message-ID: <opencrm.abcd@crm.example.test>",
		"Subject: Follow up",
		"",
		"--arf--",
	}, "\r\n")

	message := parseFetchedMessage([]byte(raw), "arf-provider-1", time.Time{}, moduleuseremail.SyncCredentials{FromEmail: "rep@acme.test"})
	if len(message.DeliveryFeedback) != 1 {
		t.Fatalf("unexpected ARF parse: %#v", message.DeliveryFeedback)
	}
	feedback := message.DeliveryFeedback[0]
	if feedback.Type != "complaint" || feedback.OriginalMessageID != "<opencrm.abcd@crm.example.test>" || feedback.RecipientEmail != "lead@buyer.test" || feedback.Action != "reported" || feedback.StatusCode != "abuse" {
		t.Fatalf("unexpected ARF feedback: %#v", feedback)
	}
}

func TestParseDeliveryFeedbackIgnoresOrdinaryAndNonFailureReports(t *testing.T) {
	ordinary := mail.Header{"Content-Type": []string{"text/plain"}}
	if feedback := parseDeliveryFeedback(ordinary, []byte("hello")); len(feedback) != 0 {
		t.Fatalf("ordinary message produced feedback: %#v", feedback)
	}

	delayed := strings.Join([]string{
		"--status",
		"Content-Type: message/delivery-status",
		"",
		"Reporting-MTA: dns; provider.test",
		"",
		"Final-Recipient: rfc822; lead@buyer.test",
		"Action: delayed",
		"Status: 4.2.0",
		"--status--",
	}, "\r\n")
	header := mail.Header{"Content-Type": []string{"multipart/report; report-type=delivery-status; boundary=status"}}
	if feedback := parseDeliveryFeedback(header, []byte(delayed)); len(feedback) != 0 {
		t.Fatalf("temporary DSN produced terminal feedback: %#v", feedback)
	}
	header = mail.Header{"Content-Type": []string{"multipart/report; report-type=delivery-status; boundary=status"}}
	invalid := strings.Replace(delayed, "Action: delayed\r\nStatus: 4.2.0", "Action: failed\r\nStatus: 5.invalid.1", 1)
	if feedback := parseDeliveryFeedback(header, []byte(invalid)); len(feedback) != 0 {
		t.Fatalf("invalid enhanced status produced feedback: %#v", feedback)
	}
}
