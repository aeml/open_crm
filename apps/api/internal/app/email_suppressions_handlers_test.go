package app

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleemailsuppressions "github.com/aeml/open_crm/apps/api/internal/modules/emailsuppressions"
)

type fakeEmailSuppressionsService struct {
	suppressed        bool
	isErr             error
	token             string
	tokenErr          error
	validateErr       error
	unsubscribeResult moduleemailsuppressions.Suppression
	unsubscribeErr    error
	isCalled          bool
	tokenCalled       bool
	validateCalled    bool
	unsubscribeCalled bool
	lastOrgID         int64
	lastEmail         string
	lastToken         string
}

func (f *fakeEmailSuppressionsService) IsSuppressed(_ context.Context, organizationID int64, email string) (bool, error) {
	f.isCalled = true
	f.lastOrgID = organizationID
	f.lastEmail = email
	return f.suppressed, f.isErr
}

func (f *fakeEmailSuppressionsService) UnsubscribeToken(organizationID int64, email string) (string, error) {
	f.tokenCalled = true
	f.lastOrgID = organizationID
	f.lastEmail = email
	return f.token, f.tokenErr
}

func (f *fakeEmailSuppressionsService) ValidateUnsubscribeToken(token string) error {
	f.validateCalled = true
	f.lastToken = token
	return f.validateErr
}

func (f *fakeEmailSuppressionsService) UnsubscribeByToken(_ context.Context, token string) (moduleemailsuppressions.Suppression, error) {
	f.unsubscribeCalled = true
	f.lastToken = token
	return f.unsubscribeResult, f.unsubscribeErr
}

func TestEmailUnsubscribeGETValidatesWithoutMutating(t *testing.T) {
	suppressions := &fakeEmailSuppressionsService{}
	server := NewServer(config.Env{}, Dependencies{EmailSuppressionsService: suppressions})

	request := httptest.NewRequest(http.MethodGet, "/api/email-unsubscribe/signed.token", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if !suppressions.validateCalled || suppressions.lastToken != "signed.token" || suppressions.unsubscribeCalled {
		t.Fatalf("expected read-only unsubscribe validation, got %#v", suppressions)
	}
	if !strings.Contains(recorder.Body.String(), `method="post"`) || !strings.Contains(recorder.Body.String(), `name="List-Unsubscribe" value="One-Click"`) {
		t.Fatalf("expected unsubscribe confirmation, got %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "lead@example.test") {
		t.Fatalf("confirmation must not disclose the signed recipient: %q", recorder.Body.String())
	}
	assertUnsubscribeSecurityHeaders(t, recorder)
}

func TestEmailUnsubscribePOSTConsumesExactConfirmation(t *testing.T) {
	suppressions := &fakeEmailSuppressionsService{}
	server := NewServer(config.Env{}, Dependencies{EmailSuppressionsService: suppressions})
	body := url.Values{"List-Unsubscribe": {"One-Click"}}.Encode()
	request := httptest.NewRequest(http.MethodPost, "/api/email-unsubscribe/signed.token", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !suppressions.unsubscribeCalled || suppressions.lastToken != "signed.token" {
		t.Fatalf("expected token to be consumed once: status=%d service=%#v body=%s", recorder.Code, suppressions, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "You will no longer receive these emails") || strings.Contains(recorder.Body.String(), "lead@example.test") {
		t.Fatalf("expected generic unsubscribe result, got %q", recorder.Body.String())
	}
	assertUnsubscribeSecurityHeaders(t, recorder)
}

func TestEmailUnsubscribePOSTAcceptsRFC8058Multipart(t *testing.T) {
	suppressions := &fakeEmailSuppressionsService{}
	server := NewServer(config.Env{}, Dependencies{EmailSuppressionsService: suppressions})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("List-Unsubscribe", "One-Click"); err != nil {
		t.Fatalf("write multipart form: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart form: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/email-unsubscribe/signed.token", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !suppressions.unsubscribeCalled {
		t.Fatalf("expected multipart unsubscribe: status=%d service=%#v body=%s", recorder.Code, suppressions, recorder.Body.String())
	}
}

func TestEmailUnsubscribePOSTRejectsMalformedConfirmation(t *testing.T) {
	for name, testCase := range map[string][2]string{
		"wrong value": {"application/x-www-form-urlencoded", "List-Unsubscribe=Not-One-Click"},
		"extra field": {"application/x-www-form-urlencoded", "List-Unsubscribe=One-Click&recipient=lead%40example.test"},
		"json":        {"application/json", `{"List-Unsubscribe":"One-Click"}`},
		"missing":     {"application/x-www-form-urlencoded", ""},
	} {
		t.Run(name, func(t *testing.T) {
			suppressions := &fakeEmailSuppressionsService{}
			server := NewServer(config.Env{}, Dependencies{EmailSuppressionsService: suppressions})
			request := httptest.NewRequest(http.MethodPost, "/api/email-unsubscribe/signed.token", strings.NewReader(testCase[1]))
			request.Header.Set("Content-Type", testCase[0])
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || suppressions.unsubscribeCalled {
				t.Fatalf("expected rejected confirmation: status=%d service=%#v body=%s", recorder.Code, suppressions, recorder.Body.String())
			}
		})
	}
}

func TestEmailUnsubscribePOSTRejectsOversizedConfirmation(t *testing.T) {
	suppressions := &fakeEmailSuppressionsService{}
	server := NewServer(config.Env{}, Dependencies{EmailSuppressionsService: suppressions})
	body := "List-Unsubscribe=One-Click&padding=" + strings.Repeat("a", 64<<10)
	request := httptest.NewRequest(http.MethodPost, "/api/email-unsubscribe/signed.token", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge || suppressions.unsubscribeCalled {
		t.Fatalf("expected oversized confirmation rejection: status=%d service=%#v body=%s", recorder.Code, suppressions, recorder.Body.String())
	}
}

func TestEmailUnsubscribeRejectsInvalidToken(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			suppressions := &fakeEmailSuppressionsService{validateErr: moduleemailsuppressions.ErrInvalidToken, unsubscribeErr: moduleemailsuppressions.ErrInvalidToken}
			server := NewServer(config.Env{}, Dependencies{EmailSuppressionsService: suppressions})

			var body *strings.Reader
			if method == http.MethodPost {
				body = strings.NewReader("List-Unsubscribe=One-Click")
			} else {
				body = strings.NewReader("")
			}
			request := httptest.NewRequest(method, "/api/email-unsubscribe/bad", body)
			if method == http.MethodPost {
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", recorder.Code)
			}
		})
	}
}

func assertUnsubscribeSecurityHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	for name, expected := range map[string]string{
		"Cache-Control":          "no-store",
		"Referrer-Policy":        "no-referrer",
		"X-Content-Type-Options": "nosniff",
		"X-Robots-Tag":           "noindex, nofollow",
	} {
		if got := recorder.Header().Get(name); got != expected {
			t.Fatalf("expected %s=%q, got %q", name, expected, got)
		}
	}
}
