package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
)

func TestPendingQuoteApprovalsRequireAdminAndUseSessionTenant(t *testing.T) {
	memberServer := serverWithRole("member", Dependencies{DealsService: &fakeDealsService{}})
	memberRequest := httptest.NewRequest(http.MethodGet, "/api/deal-quote-approvals?status=pending", nil)
	addSessionCookie(memberRequest)
	memberRecorder := httptest.NewRecorder()
	memberServer.ServeHTTP(memberRecorder, memberRequest)
	if memberRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected member pending-approval access to be forbidden, got %d", memberRecorder.Code)
	}

	service := &fakeDealsService{pendingApprovalsResult: []moduledeals.PendingQuoteApproval{{ApprovalID: 8, DealID: 12, QuoteID: 71, QuoteNumber: "Q-12-V1"}}}
	server := serverWithRole("admin", Dependencies{DealsService: service})
	request := httptest.NewRequest(http.MethodGet, "/api/deal-quote-approvals?status=pending", nil)
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastPendingApprovalsOrgID != 42 || !strings.Contains(recorder.Body.String(), `"quoteNumber":"Q-12-V1"`) {
		t.Fatalf("unexpected pending approval list: status=%d org=%d body=%s", recorder.Code, service.lastPendingApprovalsOrgID, recorder.Body.String())
	}
}

func TestQuoteApprovalDecisionForwardsExactScopeAndIdempotency(t *testing.T) {
	service := &fakeDealsService{decideApprovalResult: moduledeals.QuoteVersion{ID: 71, QuoteNumber: "Q-12-V1", Approval: moduledeals.QuoteApproval{Required: true, Status: "approved"}}}
	server := serverWithRole("owner", Dependencies{DealsService: service})
	request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/approval", bytes.NewBufferString(`{"decision":"approved","note":"Reviewed totals"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "approval-decision-key-0001")
	addSessionCookie(request)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.lastApprovalOrgID != 42 || service.lastApprovalDealID != 12 ||
		service.lastApprovalQuoteID != 71 || service.lastApprovalActorID != 5 ||
		service.lastApprovalInput.Decision != "approved" || service.lastApprovalInput.Note != "Reviewed totals" ||
		service.lastApprovalInput.IdempotencyKey != "approval-decision-key-0001" {
		t.Fatalf("unexpected approval decision: status=%d service=%#v body=%s", recorder.Code, service, recorder.Body.String())
	}
}

func TestQuoteApprovalDecisionErrorsAreStable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
		body string
	}{
		{"invalid", moduledeals.ErrInvalidQuote, http.StatusBadRequest, `"code":"BAD_REQUEST"`},
		{"foreign", moduledeals.ErrNotFound, http.StatusNotFound, `"code":"NOT_FOUND"`},
		{"self decision", moduledeals.ErrQuoteApprovalState, http.StatusConflict, `"code":"QUOTE_APPROVAL_STATE"`},
		{"terminal conflict", moduledeals.ErrQuoteApprovalConflict, http.StatusConflict, `"code":"IDEMPOTENCY_CONFLICT"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := serverWithRole("admin", Dependencies{DealsService: &fakeDealsService{decideApprovalErr: test.err}})
			request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/approval", bytes.NewBufferString(`{"decision":"rejected","note":"Fix scope"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "approval-decision-key-0002")
			addSessionCookie(request)
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != test.code || !strings.Contains(recorder.Body.String(), test.body) {
				t.Fatalf("expected %d %s, got %d %s", test.code, test.body, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestQuoteApprovalAndDeliveryGatesMapActionableErrors(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{moduledeals.ErrQuoteApprovalRequired, "QUOTE_APPROVAL_REQUIRED"},
		{moduledeals.ErrQuoteApprovalRejected, "QUOTE_APPROVAL_REJECTED"},
	} {
		server := serverWithRole("member", Dependencies{DealsService: &fakeDealsService{replayQuoteDeliveryErr: test.err}, UserEmailService: &fakeUserEmailService{}})
		request := httptest.NewRequest(http.MethodPost, "/api/deals/12/quotes/71/deliveries", bytes.NewBufferString(`{"subject":"Quote","messageBody":"Review"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "quote-delivery-key-0003")
		addSessionCookie(request)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("expected delivery gate %s, got %d %s", test.code, recorder.Code, recorder.Body.String())
		}
	}
}
