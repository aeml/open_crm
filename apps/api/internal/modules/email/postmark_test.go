package email

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestPostmarkConfigured(t *testing.T) {
	if NewPostmarkProvider("", "", "", nil).Configured() {
		t.Errorf("empty provider should not be configured")
	}
	if NewPostmarkProvider("tok", "", "", nil).Configured() {
		t.Errorf("provider without from address should not be configured")
	}
	if !NewPostmarkProvider("tok", "from@acme.test", "outbound", nil).Configured() {
		t.Errorf("provider with token and from should be configured")
	}
}

func TestPostmarkSendRequiresConfiguration(t *testing.T) {
	_, err := NewPostmarkProvider("", "", "", nil).Send(context.Background(), Message{To: "a@b.test", Subject: "Hi"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestPostmarkSendValidatesRecipientAndSubject(t *testing.T) {
	provider := NewPostmarkProvider("tok", "from@acme.test", "outbound", nil)
	if _, err := provider.Send(context.Background(), Message{To: "", Subject: "Hi"}); err == nil {
		t.Errorf("expected error for missing recipient")
	}
	if _, err := provider.Send(context.Background(), Message{To: "a@b.test", Subject: ""}); err == nil {
		t.Errorf("expected error for missing subject")
	}
}

func TestPostmarkSendCarriesCorrelationMetadataAndReturnsMessageID(t *testing.T) {
	provider := NewPostmarkProvider("server-token", "from@acme.test", "outbound", nil)
	var sent postmarkSendRequest
	provider.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("X-Postmark-Server-Token") != "server-token" {
			t.Fatalf("missing Postmark server token header")
		}
		if err := json.NewDecoder(request.Body).Decode(&sent); err != nil {
			t.Fatalf("decode Postmark request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ErrorCode":0,"Message":"OK","MessageID":"postmark-message-correlated"}`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	metadata := map[string]string{"open_crm_system_email": "v1", "open_crm_delivery_key": "delivery-key"}
	result, err := provider.Send(context.Background(), Message{To: "person@example.test", Subject: "Identity", TextBody: "Hello", Metadata: metadata, Attachments: []Attachment{{Name: "pilot.csv", ContentType: "text/csv", Content: []byte("name\nAda\n")}}})
	if err != nil || result.ProviderMessageID != "postmark-message-correlated" {
		t.Fatalf("send correlated Postmark message: result=%#v err=%v", result, err)
	}
	if sent.MessageStream != "outbound" || sent.Metadata["open_crm_system_email"] != "v1" || sent.Metadata["open_crm_delivery_key"] != "delivery-key" {
		t.Fatalf("Postmark request omitted correlation metadata: %#v", sent)
	}
	if len(sent.Attachments) != 1 || sent.Attachments[0].Name != "pilot.csv" || sent.Attachments[0].ContentType != "text/csv" {
		t.Fatalf("Postmark request omitted the bounded attachment: %#v", sent.Attachments)
	}
	decoded, decodeErr := base64.StdEncoding.DecodeString(sent.Attachments[0].Content)
	if decodeErr != nil || string(decoded) != "name\nAda\n" {
		t.Fatalf("Postmark attachment is not exact base64 content: body=%q err=%v", decoded, decodeErr)
	}
}

func TestPostmarkSendRejectsUnsafeOrEmptyAttachmentsBeforeProvider(t *testing.T) {
	provider := NewPostmarkProvider("tok", "from@acme.test", "outbound", nil)
	for _, attachment := range []Attachment{
		{Name: "../secret.csv", ContentType: "text/csv", Content: []byte("x")},
		{Name: "report.csv\r\nX-Header: value", ContentType: "text/csv", Content: []byte("x")},
		{Name: "report.csv", ContentType: "text/csv\r\nX-Header: value", Content: []byte("x")},
		{Name: "empty.csv", ContentType: "text/csv"},
		{Name: "missing-type.csv", Content: []byte("x")},
	} {
		if _, err := provider.Send(context.Background(), Message{To: "person@example.test", Subject: "Report", Attachments: []Attachment{attachment}}); err == nil || !strings.Contains(err.Error(), "invalid attachment") {
			t.Fatalf("unsafe attachment %#v returned %v", attachment, err)
		}
	}
}

func TestPostmarkSendRejectsAcceptedResponseWithoutMessageID(t *testing.T) {
	provider := NewPostmarkProvider("server-token", "from@acme.test", "outbound", nil)
	provider.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ErrorCode":0,"Message":"OK"}`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	if _, err := provider.Send(context.Background(), Message{To: "person@example.test", Subject: "Identity"}); err == nil || !strings.Contains(err.Error(), "missing message id") || !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("missing Postmark correlation reference returned %v", err)
	}
}

func TestPostmarkSendTreatsMalformedAcceptedResponseAsUncertain(t *testing.T) {
	provider := NewPostmarkProvider("server-token", "from@acme.test", "outbound", nil)
	provider.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ErrorCode":`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	if _, err := provider.Send(context.Background(), Message{To: "person@example.test", Subject: "Identity"}); !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("malformed accepted response could cause an automatic duplicate: %v", err)
	}
}

func TestPostmarkName(t *testing.T) {
	if NewPostmarkProvider("tok", "from@acme.test", "outbound", nil).Name() != "postmark" {
		t.Errorf("unexpected provider name")
	}
}

func TestPostmarkSendSurfacesDegradationAndRecoversOnLaterAttempt(t *testing.T) {
	provider := NewPostmarkProvider("tok", "from@acme.test", "outbound", nil)
	attempts := 0
	provider.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader(`{"ErrorCode":500,"Message":"temporarily unavailable"}`)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ErrorCode":0,"Message":"OK","MessageID":"postmark-message-1"}`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}

	message := Message{To: "person@example.test", Subject: "Pilot follow-up", TextBody: "Hello"}
	if _, err := provider.Send(context.Background(), message); err == nil || !strings.Contains(err.Error(), "http 503: temporarily unavailable") {
		t.Fatalf("expected actionable provider degradation, got %v", err)
	}
	if result, err := provider.Send(context.Background(), message); err != nil || result.ProviderMessageID != "postmark-message-1" {
		t.Fatalf("expected later operator/job attempt to recover, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected exactly two caller-controlled attempts, got %d", attempts)
	}
}

func TestPostmarkSendHonorsContextDeadline(t *testing.T) {
	provider := NewPostmarkProvider("tok", "from@acme.test", "outbound", nil)
	provider.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := provider.Send(ctx, Message{To: "person@example.test", Subject: "Pilot follow-up"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected provider request deadline, got %v", err)
	}
	if !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("provider transport interruption must be classified as uncertain, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("provider deadline surfaced after %s", elapsed)
	}
}
