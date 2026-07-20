package mailboxsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestMicrosoftGraphFetcherFetchesNewMessagesAfterCursor(t *testing.T) {
	requests := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.String())
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if strings.HasSuffix(r.URL.Path, "/$value") {
			if got := r.Header.Get("Accept"); got != "message/rfc822" {
				t.Fatalf("unexpected MIME accept header: %q", got)
			}
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/graph/v1.0/me/messages/"), "/$value")
			_, _ = fmt.Fprint(w, graphRawMessage(id))
			return
		}
		if r.URL.Path != "/graph/v1.0/me/mailFolders/inbox/messages" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Prefer"); got != `outlook.body-content-type="text"` {
			t.Fatalf("unexpected prefer header: %q", got)
		}
		if r.URL.Query().Get("$top") != "3" || r.URL.Query().Get("$orderby") != "receivedDateTime desc" {
			t.Fatalf("unexpected graph query: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.Query().Get("$select"), "internetMessageHeaders") {
			t.Fatalf("Graph reply headers were not selected: %s", r.URL.RawQuery)
		}

		_, _ = fmt.Fprint(w, `{"value":[{"id":"graph-12","conversationId":"conversation-12","internetMessageId":"<graph-12@acme.test>","subject":"Subject 12","receivedDateTime":"2026-01-01T00:02:00Z","body":{"contentType":"text","content":"Body 12"},"from":{"emailAddress":{"address":"customer-12@acme.test"}},"toRecipients":[{"emailAddress":{"address":"rep@acme.test"}}]},{"id":"graph-11","conversationId":"conversation-11","internetMessageId":"<graph-11@acme.test>","internetMessageHeaders":[{"name":"In-Reply-To","value":"<sequence-11@crm.example.test>"},{"name":"References","value":"<older@crm.example.test> <sequence-11@crm.example.test>"}],"subject":"Subject 11","receivedDateTime":"2026-01-01T00:01:00Z","body":{"contentType":"text","content":"Body 11"},"from":{"emailAddress":{"address":"customer-11@acme.test"}},"toRecipients":[{"emailAddress":{"address":"rep@acme.test"}}]},{"id":"graph-10","conversationId":"conversation-10","internetMessageId":"<graph-10@acme.test>","subject":"Subject 10","receivedDateTime":"2026-01-01T00:00:00Z","body":{"contentType":"text","content":"Body 10"},"from":{"emailAddress":{"address":"customer-10@acme.test"}},"toRecipients":[{"emailAddress":{"address":"rep@acme.test"}}]}]}`)
	}))
	defer server.Close()

	fetcher := &MicrosoftGraphFetcher{HTTPClient: server.Client(), BaseURL: server.URL + "/graph/v1.0"}
	messages, err := fetcher.Fetch(context.Background(), moduleuseremail.SyncCredentials{FromEmail: "rep@acme.test", OAuthAccess: "access-token", SyncCursor: "<graph-10@acme.test>"}, 3)
	if err != nil {
		t.Fatalf("fetch graph messages: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected two messages after cursor, got %#v", messages)
	}
	if messages[0].ProviderMessageID != "<graph-11@acme.test>" || messages[1].ProviderMessageID != "<graph-12@acme.test>" {
		t.Fatalf("expected oldest-to-newest messages, got %#v", messages)
	}
	if messages[0].FromEmail != "customer-11@acme.test" || messages[0].ToEmail != "rep@acme.test" || messages[0].Subject != "Subject 11" || messages[0].Body != "Body 11" {
		t.Fatalf("unexpected parsed message: %#v", messages[0])
	}
	if messages[0].ProviderThreadID != "conversation-11" || !messages[0].ReceivedAt.Equal(time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)) {
		t.Fatalf("unexpected thread/date metadata: %#v", messages[0])
	}
	if messages[0].RFCMessageID != "<graph-11@acme.test>" || messages[0].InReplyTo != "<sequence-11@crm.example.test>" || strings.Join(messages[0].ReferenceMessageIDs, ",") != "<older@crm.example.test>,<sequence-11@crm.example.test>" {
		t.Fatalf("unexpected Graph reply correlation: %#v", messages[0])
	}
	if len(requests) != 3 || !strings.HasSuffix(requests[1], "/graph-11/$value") || !strings.HasSuffix(requests[2], "/graph-12/$value") {
		t.Fatalf("expected one list and two MIME requests, got %#v", requests)
	}
}

func graphRawMessage(id string) string {
	number := strings.TrimPrefix(id, "graph-")
	lines := []string{
		"From: Customer <customer-" + number + "@acme.test>",
		"To: Rep <rep@acme.test>",
		"Subject: Subject " + number,
		"Message-ID: <graph-" + number + "@acme.test>",
		"Date: Thu, 01 Jan 2026 00:0" + number[len(number)-1:] + ":00 +0000",
		"Content-Type: text/plain; charset=utf-8",
	}
	if id == "graph-11" {
		lines = append(lines,
			"In-Reply-To: <sequence-11@crm.example.test>",
			"References: <older@crm.example.test> <sequence-11@crm.example.test>",
		)
	}
	lines = append(lines, "", "Body "+number)
	return strings.Join(lines, "\r\n")
}

func TestMicrosoftGraphFetcherRequiresAccessToken(t *testing.T) {
	_, err := (&MicrosoftGraphFetcher{}).Fetch(context.Background(), moduleuseremail.SyncCredentials{}, 10)
	if err == nil || !strings.Contains(err.Error(), "access token") {
		t.Fatalf("expected access token error, got %v", err)
	}
}

func TestMicrosoftGraphFetcherFailsClosedWhenRawMIMEIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/$value") {
			http.Error(w, `{"error":{"message":"MIME temporarily unavailable"}}`, http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `{"value":[{"id":"graph-20","internetMessageId":"<graph-20@acme.test>","receivedDateTime":"2026-01-01T00:20:00Z"}]}`)
	}))
	defer server.Close()

	fetcher := &MicrosoftGraphFetcher{HTTPClient: server.Client(), BaseURL: server.URL}
	_, err := fetcher.Fetch(context.Background(), moduleuseremail.SyncCredentials{FromEmail: "rep@acme.test", OAuthAccess: "access-token"}, 1)
	if err == nil || !strings.Contains(err.Error(), "MIME temporarily unavailable") {
		t.Fatalf("expected raw MIME failure to stop sync, got %v", err)
	}
}

func TestGraphMessageKeyFallsBackToID(t *testing.T) {
	key := graphMessageKey(graphMessage{ID: "graph-id"})
	if key != "graph-id" {
		t.Fatalf("unexpected graph message key: %q", key)
	}
}
