package mailboxsync

import (
	"strings"
	"testing"
	"time"

	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestRecentUIDsReturnsOnlyNewLimitedUIDs(t *testing.T) {
	uids := recentUIDs([]string{"7", "8", "9", "10", "11"}, "8", 2)
	if strings.Join(uids, ",") != "10,11" {
		t.Fatalf("unexpected recent UIDs: %#v", uids)
	}
}

func TestParseFetchedMessageDecodesHeadersAndBodies(t *testing.T) {
	raw := strings.Join([]string{
		"From: Customer <customer@acme.test>",
		"To: Rep <rep@acme.test>",
		"Subject: =?UTF-8?Q?Hello_World?=",
		"Date: Fri, 02 Jan 2026 03:04:05 +0000",
		"Content-Type: multipart/alternative; boundary=demo",
		"",
		"--demo",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: quoted-printable",
		"",
		"Line=201",
		"--demo",
		"Content-Type: text/html; charset=utf-8",
		"Content-Transfer-Encoding: base64",
		"",
		"PHA+TGluZSAxPC9wPg==",
		"--demo--",
	}, "\r\n")

	message := parseFetchedMessage([]byte(raw), "123", time.Time{}, moduleuseremail.SyncCredentials{FromEmail: "fallback@acme.test"})

	if message.ProviderMessageID != "123" || message.FromEmail != "customer@acme.test" || message.ToEmail != "rep@acme.test" {
		t.Fatalf("unexpected message envelope: %#v", message)
	}
	if message.Subject != "Hello World" {
		t.Fatalf("subject was not decoded: %q", message.Subject)
	}
	if message.Body != "Line 1" {
		t.Fatalf("plain body was not decoded: %q", message.Body)
	}
	if message.ReceivedAt.IsZero() {
		t.Fatalf("expected received date to be parsed")
	}
}
