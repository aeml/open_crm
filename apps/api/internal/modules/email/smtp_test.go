package email

import (
	"errors"
	"io"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"testing"
)

func TestBuildMessageUsesPlainTextWhenNoHTML(t *testing.T) {
	raw := string(buildMessage("Rep <rep@example.test>", "lead@example.test", "Hi", "Plain body", "", "", "", nil, ""))
	if !strings.Contains(raw, "Content-Type: text/plain") {
		t.Fatalf("expected plain-text content type, got %q", raw)
	}
	if strings.Contains(raw, "multipart/alternative") {
		t.Fatalf("plain message should not be multipart: %q", raw)
	}
}

func TestBuildMessageUsesMultipartWhenHTMLProvided(t *testing.T) {
	raw := string(buildMessage("Rep <rep@example.test>", "lead@example.test", "Hi", "Plain body", "<p>HTML body</p>", "", "", nil, ""))
	if !strings.Contains(raw, "multipart/alternative") {
		t.Fatalf("expected multipart message, got %q", raw)
	}
	if !strings.Contains(raw, "Content-Type: text/plain") || !strings.Contains(raw, "Content-Type: text/html") {
		t.Fatalf("expected text and html parts, got %q", raw)
	}
	if !strings.Contains(raw, "Plain body") || !strings.Contains(raw, "<p>HTML body</p>") {
		t.Fatalf("expected both bodies, got %q", raw)
	}
}

func TestBuildRFC822MessageRejectsHeaderInjection(t *testing.T) {
	for name, msg := range map[string]Message{
		"recipient":   {To: "lead@example.test\r\nBcc: stolen@example.test", Subject: "Hi"},
		"subject":     {To: "lead@example.test", Subject: "Hi\r\nBcc: stolen@example.test"},
		"message id":  {To: "lead@example.test", Subject: "Hi", MessageID: "<safe@example.test>\r\nBcc: stolen@example.test"},
		"in reply to": {To: "lead@example.test", Subject: "Hi", InReplyTo: "<safe@example.test>\r\nBcc: stolen@example.test"},
		"references":  {To: "lead@example.test", Subject: "Hi", References: []string{"<safe@example.test>\r\nBcc: stolen@example.test"}},
		"unsubscribe": {To: "lead@example.test", Subject: "Hi", ListUnsubscribeURL: "https://crm.example.test/unsubscribe\r\nBcc: stolen@example.test"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildRFC822Message("Rep", "rep@example.test", msg); err == nil {
				t.Fatal("expected header injection to be rejected")
			}
		})
	}
}

func TestBuildRFC822MessagePreservesBoundedReplyHeaders(t *testing.T) {
	raw, err := BuildRFC822Message("Revenue Rep", "rep@example.test", Message{
		To: "lead@example.test", Subject: "Re: Follow up", TextBody: "Reply body",
		MessageID: "<reply-2@crm.example.test>", InReplyTo: "<message-1@CLIENT.Example>",
		References: []string{"<root@client.example>", "<message-1@client.example>", "<message-1@client.example>"},
	})
	if err != nil {
		t.Fatalf("build reply message: %v", err)
	}
	message := string(raw)
	for _, expected := range []string{
		"Message-ID: <reply-2@crm.example.test>",
		"In-Reply-To: <message-1@client.example>",
		"References: <root@client.example> <message-1@client.example>",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("reply message missing %q: %s", expected, message)
		}
	}
	if strings.Count(message, "<message-1@client.example>") != 2 {
		t.Fatalf("references must be de-duplicated while retaining in-reply-to: %s", message)
	}
}

func TestBuildRFC822MessageFormatsNamedSenderAndMultipartBody(t *testing.T) {
	raw, err := BuildRFC822Message("Revenue Rep", "rep@example.test", Message{
		To:                 "lead@example.test",
		Subject:            "Follow up",
		TextBody:           "Plain body",
		HTMLBody:           "<p>HTML body</p>",
		MessageID:          "<sequence-1@crm.example.test>",
		ListUnsubscribeURL: "https://crm.example.test/api/email-unsubscribe/signed.token",
	})
	if err != nil {
		t.Fatalf("build message: %v", err)
	}
	message := string(raw)
	for _, expected := range []string{"From: \"Revenue Rep\" <rep@example.test>", "To: lead@example.test", "Subject: Follow up", "Message-ID: <sequence-1@crm.example.test>", "List-Unsubscribe: <https://crm.example.test/api/email-unsubscribe/signed.token>", "List-Unsubscribe-Post: List-Unsubscribe=One-Click", "multipart/alternative", "Plain body", "<p>HTML body</p>"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("message missing %q: %s", expected, message)
		}
	}
}

func TestOneClickUnsubscribeURLRequiresBoundedHTTPSURL(t *testing.T) {
	valid := "https://crm.example.test/api/email-unsubscribe/signed.token?source=header"
	if got := OneClickUnsubscribeURL(valid); got != valid {
		t.Fatalf("expected valid HTTPS URL, got %q", got)
	}
	for name, value := range map[string]string{
		"http":        "http://crm.example.test/unsubscribe",
		"relative":    "/api/email-unsubscribe/token",
		"credentials": "https://user@crm.example.test/unsubscribe",
		"fragment":    "https://crm.example.test/unsubscribe#recipient",
		"control":     "https://crm.example.test/unsub\tscribe",
		"oversized":   "https://crm.example.test/" + strings.Repeat("a", 2048),
	} {
		t.Run(name, func(t *testing.T) {
			if got := OneClickUnsubscribeURL(value); got != "" {
				t.Fatalf("expected URL to be rejected, got %q", got)
			}
		})
	}
}

func TestWriteMessageClassifiesPostDataDisconnectAsUncertain(t *testing.T) {
	client := scriptedSMTPClient(t, func(connection *textproto.Conn) {
		expectSMTPCommand(t, connection, "EHLO ")
		_ = connection.PrintfLine("250-localhost")
		_ = connection.PrintfLine("250 OK")
		expectSMTPCommand(t, connection, "MAIL FROM:")
		_ = connection.PrintfLine("250 OK")
		expectSMTPCommand(t, connection, "RCPT TO:")
		_ = connection.PrintfLine("250 OK")
		expectSMTPCommand(t, connection, "DATA")
		_ = connection.PrintfLine("354 End data")
		_, _ = io.ReadAll(connection.DotReader())
		_ = connection.PrintfLine("250 accepted")
		expectSMTPCommand(t, connection, "QUIT")
	})
	err := writeMessage(client, "rep@example.test", "lead@example.test", []byte("Subject: Hi\r\n\r\nBody\r\n"))
	if !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("post-acceptance disconnect must be uncertain, got %v", err)
	}
}

func TestWriteMessageKeepsRecipientRejectionDefinite(t *testing.T) {
	client := scriptedSMTPClient(t, func(connection *textproto.Conn) {
		expectSMTPCommand(t, connection, "EHLO ")
		_ = connection.PrintfLine("250-localhost")
		_ = connection.PrintfLine("250 OK")
		expectSMTPCommand(t, connection, "MAIL FROM:")
		_ = connection.PrintfLine("250 OK")
		expectSMTPCommand(t, connection, "RCPT TO:")
		_ = connection.PrintfLine("550 recipient rejected")
	})
	err := writeMessage(client, "rep@example.test", "lead@example.test", []byte("Subject: Hi\r\n\r\nBody\r\n"))
	if err == nil || errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("recipient rejection must be definite, got %v", err)
	}
}

func scriptedSMTPClient(t *testing.T, script func(*textproto.Conn)) *smtp.Client {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for SMTP contract test: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		textConnection := textproto.NewConn(connection)
		defer textConnection.Close()
		_ = textConnection.PrintfLine("220 localhost ready")
		script(textConnection)
	}()
	connection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		listener.Close()
		t.Fatalf("dial SMTP contract server: %v", err)
	}
	client, err := smtp.NewClient(connection, "localhost")
	if err != nil {
		connection.Close()
		listener.Close()
		t.Fatalf("create SMTP contract client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = listener.Close()
		<-done
	})
	return client
}

func expectSMTPCommand(t *testing.T, connection *textproto.Conn, prefix string) {
	t.Helper()
	line, err := connection.ReadLine()
	if err != nil {
		t.Errorf("read SMTP command %q: %v", prefix, err)
		return
	}
	if !strings.HasPrefix(line, prefix) {
		t.Errorf("SMTP command = %q, want prefix %q", line, prefix)
	}
}
