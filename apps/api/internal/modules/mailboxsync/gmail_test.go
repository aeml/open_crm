package mailboxsync

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestGmailFetcherFetchesNewMessagesAfterCursor(t *testing.T) {
	requests := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.String())
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("unexpected auth header: %q", got)
		}

		switch {
		case r.URL.Path == "/gmail/v1/users/me/messages":
			if r.URL.Query().Get("maxResults") != "3" || r.URL.Query().Get("q") != "in:inbox" {
				t.Fatalf("unexpected list query: %s", r.URL.RawQuery)
			}
			_, _ = fmt.Fprint(w, `{"messages":[{"id":"gmail-12","threadId":"thread-12"},{"id":"gmail-11","threadId":"thread-11"},{"id":"gmail-10","threadId":"thread-10"}]}`)
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/"):
			id := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/")
			_, _ = fmt.Fprintf(w, `{"id":%q,"threadId":"thread-%s","internalDate":"1767225600000","raw":%q}`, id, strings.TrimPrefix(id, "gmail-"), gmailRawMessage(id))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher := &GmailFetcher{HTTPClient: server.Client(), BaseURL: server.URL + "/gmail/v1"}
	messages, err := fetcher.Fetch(context.Background(), moduleuseremail.SyncCredentials{FromEmail: "rep@acme.test", OAuthAccess: "access-token", SyncCursor: "gmail-10"}, 3)
	if err != nil {
		t.Fatalf("fetch gmail messages: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected two messages after cursor, got %#v", messages)
	}
	if messages[0].ProviderMessageID != "gmail-11" || messages[1].ProviderMessageID != "gmail-12" {
		t.Fatalf("expected oldest-to-newest messages, got %#v", messages)
	}
	if messages[0].FromEmail != "customer-gmail-11@acme.test" || messages[0].ToEmail != "rep@acme.test" || messages[0].Subject != "Subject gmail-11" {
		t.Fatalf("unexpected parsed message: %#v", messages[0])
	}
	if messages[0].ProviderThreadID != "thread-11" || !messages[0].ReceivedAt.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected thread/date metadata: %#v", messages[0])
	}
	if messages[0].RFCMessageID != "<incoming-gmail-11@buyer.test>" || messages[0].InReplyTo != "<sequence-11@crm.example.test>" || strings.Join(messages[0].ReferenceMessageIDs, ",") != "<older@crm.example.test>,<sequence-11@crm.example.test>" {
		t.Fatalf("unexpected Gmail reply correlation: %#v", messages[0])
	}
	if len(requests) != 3 {
		t.Fatalf("expected one list and two get requests, got %#v", requests)
	}
}

func TestGmailFetcherRequiresAccessToken(t *testing.T) {
	_, err := (&GmailFetcher{}).Fetch(context.Background(), moduleuseremail.SyncCredentials{}, 10)
	if err == nil || !strings.Contains(err.Error(), "access token") {
		t.Fatalf("expected access token error, got %v", err)
	}
}

func TestDecodeGmailRawMessageAcceptsPaddedURLBase64(t *testing.T) {
	encoded := base64.URLEncoding.EncodeToString([]byte("hello"))
	decoded, err := decodeGmailRawMessage(encoded)
	if err != nil {
		t.Fatalf("decode raw message: %v", err)
	}
	if string(decoded) != "hello" {
		t.Fatalf("unexpected decoded value: %q", decoded)
	}
}

func gmailRawMessage(id string) string {
	raw := strings.Join([]string{
		"From: Customer <customer-" + id + "@acme.test>",
		"To: Rep <rep@acme.test>",
		"Subject: Subject " + id,
		"Message-ID: <incoming-" + id + "@buyer.test>",
		"In-Reply-To: <sequence-" + strings.TrimPrefix(id, "gmail-") + "@crm.example.test>",
		"References: <older@crm.example.test> <sequence-" + strings.TrimPrefix(id, "gmail-") + "@crm.example.test>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Body " + id,
	}, "\r\n")
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
