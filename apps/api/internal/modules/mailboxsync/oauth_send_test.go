package mailboxsync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func TestOAuthSenderUsesExactGmailMIMEContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/users/me/messages/send" {
			t.Fatalf("unexpected Gmail request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer gmail-access" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type: %q", got)
		}
		var payload struct {
			Raw string `json:"raw"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(payload.Raw)
		if err != nil {
			t.Fatalf("decode MIME: %v", err)
		}
		message := string(raw)
		for _, expected := range []string{"From: \"Revenue Rep\" <rep@acme.test>", "To: lead@buyer.test", "Subject: Follow up", "Message-ID: <sequence-1@crm.acme.test>", "multipart/alternative", "Plain body", "<p>HTML body</p>"} {
			if !strings.Contains(message, expected) {
				t.Fatalf("Gmail MIME missing %q: %s", expected, message)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"gmail-message-1","threadId":"thread-1"}`)
	}))
	defer server.Close()

	sender := NewOAuthSender(OAuthSenderConfig{HTTPClient: server.Client(), GmailBaseURL: server.URL})
	receipt, err := sender.Send(context.Background(), moduleuseremail.SyncCredentials{Provider: "google", FromEmail: "rep@acme.test", FromName: "Revenue Rep", OAuthAccess: "gmail-access"}, moduleemail.Message{To: "lead@buyer.test", Subject: "Follow up", TextBody: "Plain body", HTMLBody: "<p>HTML body</p>", MessageID: "<sequence-1@crm.acme.test>"})
	if err != nil {
		t.Fatalf("send Gmail message: %v", err)
	}
	if receipt.ProviderMessageID != "gmail-message-1" || receipt.ProviderThreadID != "thread-1" {
		t.Fatalf("unexpected Gmail correlation receipt: %#v", receipt)
	}
}

func TestOAuthSenderUsesExactMicrosoftMIMEContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/me/sendMail" {
			t.Fatalf("unexpected Microsoft request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer microsoft-access" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "text/plain" {
			t.Fatalf("unexpected content type: %q", got)
		}
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		raw, err := base64.StdEncoding.DecodeString(string(payload))
		if err != nil {
			t.Fatalf("decode MIME: %v", err)
		}
		if message := string(raw); !strings.Contains(message, "To: lead@buyer.test") || !strings.Contains(message, "Message-ID: <sequence-2@crm.acme.test>") || !strings.Contains(message, "Plain body") {
			t.Fatalf("unexpected Microsoft MIME: %s", message)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sender := NewOAuthSender(OAuthSenderConfig{HTTPClient: server.Client(), MicrosoftBaseURL: server.URL})
	receipt, err := sender.Send(context.Background(), moduleuseremail.SyncCredentials{Provider: "microsoft", FromEmail: "rep@acme.test", OAuthAccess: "microsoft-access"}, moduleemail.Message{To: "lead@buyer.test", Subject: "Follow up", TextBody: "Plain body", MessageID: "<sequence-2@crm.acme.test>"})
	if err != nil {
		t.Fatalf("send Microsoft message: %v", err)
	}
	if receipt != (moduleuseremail.SendReceipt{}) {
		t.Fatalf("Microsoft MIME send should return no provider-specific receipt: %#v", receipt)
	}
}

func TestOAuthSenderRejectsProviderErrorsAndIncompleteSuccess(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		status   int
		body     string
		want     string
	}{
		{name: "gmail error", provider: "google", status: http.StatusForbidden, body: `{"error":{"message":"insufficient scope"}}`, want: "insufficient scope"},
		{name: "gmail missing id", provider: "google", status: http.StatusOK, body: `{}`, want: "outcome is uncertain"},
		{name: "gmail oversized success", provider: "google", status: http.StatusOK, body: strings.Repeat("x", maxOAuthSendResponseBytes+1), want: "outcome is uncertain"},
		{name: "microsoft non-accepted", provider: "microsoft", status: http.StatusOK, body: `{}`, want: "status 200"},
		{name: "microsoft oversized error", provider: "microsoft", status: http.StatusBadGateway, body: strings.Repeat("x", maxOAuthSendResponseBytes+1), want: "status 502"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			config := OAuthSenderConfig{HTTPClient: server.Client(), GmailBaseURL: server.URL, MicrosoftBaseURL: server.URL}
			_, err := NewOAuthSender(config).Send(context.Background(), moduleuseremail.SyncCredentials{Provider: test.provider, FromEmail: "rep@acme.test", OAuthAccess: "access"}, moduleemail.Message{To: "lead@buyer.test", Subject: "Hi", TextBody: "Body"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
	}
}

func TestOAuthSenderHonorsHTTPDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(80 * time.Millisecond)
		_, _ = fmt.Fprint(w, `{"id":"late"}`)
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 10 * time.Millisecond
	_, err := NewOAuthSender(OAuthSenderConfig{HTTPClient: client, GmailBaseURL: server.URL}).Send(context.Background(), moduleuseremail.SyncCredentials{Provider: "google", FromEmail: "rep@acme.test", OAuthAccess: "access"}, moduleemail.Message{To: "lead@buyer.test", Subject: "Hi", TextBody: "Body"})
	if err == nil || !strings.Contains(err.Error(), "outcome is uncertain") {
		t.Fatalf("expected bounded provider error, got %v", err)
	}
}
