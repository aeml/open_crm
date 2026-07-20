package app

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type sendRecordEmailRequest struct {
	ContactID int64  `json:"contactId"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

type sendEmailResponse struct {
	Data struct {
		Sent bool   `json:"sent"`
		To   string `json:"to"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

var emailBodyURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

// handleSendContactEmail sends a one-to-one email to a contact through the
// sending user's own mailbox (SMTP, Gmail, or Microsoft), so the email
// comes from the user, not the platform. The subject and body may contain
// {{merge_field}} placeholders, rendered server-side from contact data. A note
// is logged on the contact so the send appears in its activity timeline.
func handleSendContactEmail(auth authService, contacts contactsService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}
	if accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account service unavailable")
		return
	}

	contactID, ok := parsePathInt64(w, r, "contactID")
	if !ok {
		return
	}
	detail, err := contacts.GetByID(r.Context(), state.Organization.ID, contactID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load contact")
		return
	}

	to := strings.TrimSpace(detail.Summary.Email)
	if to == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Contact has no email address")
		return
	}

	var request sendRecordEmailRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	fields := contactMergeFields(detail)
	sendRenderedEntityEmail(w, r, requestID, accounts, notes, messages, suppressions, state.Organization.ID, state.User.ID, "contact", contactID, to, request.Subject, request.Body, fields, "Connect your email account in Settings before sending email to contacts")
}

func handleSendCompanyEmail(auth authService, companies companiesService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if companies == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Companies service unavailable")
		return
	}
	if accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account service unavailable")
		return
	}

	companyID, ok := parsePathInt64(w, r, "companyID")
	if !ok {
		return
	}
	detail, err := companies.GetByID(r.Context(), state.Organization.ID, companyID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load company")
		return
	}

	var request sendRecordEmailRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	recipient, ok := companyEmailRecipient(detail, request.ContactID)
	if !ok {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Company has no linked contact with an email address")
		return
	}

	fields := companyMergeFields(detail, recipient)
	sendRenderedEntityEmail(w, r, requestID, accounts, notes, messages, suppressions, state.Organization.ID, state.User.ID, "company", companyID, recipient.Email, request.Subject, request.Body, fields, "Connect your email account in Settings before sending email")
}

func handleSendDealEmail(auth authService, deals dealsService, contacts contactsService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Deals service unavailable")
		return
	}
	if contacts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Contacts service unavailable")
		return
	}
	if accounts == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email account service unavailable")
		return
	}

	dealID, ok := parsePathInt64(w, r, "dealID")
	if !ok {
		return
	}
	detail, err := deals.GetByID(r.Context(), state.Organization.ID, dealID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load deal")
		return
	}

	var request sendRecordEmailRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	if detail.Summary.PrimaryContactID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Deal has no primary contact")
		return
	}
	if request.ContactID > 0 && request.ContactID != detail.Summary.PrimaryContactID {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Recipient must be the deal primary contact")
		return
	}

	contact, err := contacts.GetByID(r.Context(), state.Organization.ID, detail.Summary.PrimaryContactID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load deal primary contact")
		return
	}
	to := strings.TrimSpace(contact.Summary.Email)
	if to == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Deal primary contact has no email address")
		return
	}

	fields := dealMergeFields(detail, contact)
	sendRenderedEntityEmail(w, r, requestID, accounts, notes, messages, suppressions, state.Organization.ID, state.User.ID, "deal", dealID, to, request.Subject, request.Body, fields, "Connect your email account in Settings before sending email")
}

func contactMergeFields(detail modulecontacts.Detail) map[string]string {
	summary := detail.Summary
	fullName := strings.TrimSpace(summary.FirstName + " " + summary.LastName)
	return map[string]string{
		"first_name": summary.FirstName,
		"last_name":  summary.LastName,
		"full_name":  fullName,
		"email":      summary.Email,
		"job_title":  summary.JobTitle,
	}
}

type recordEmailRecipient struct {
	ID        int64
	FirstName string
	LastName  string
	Email     string
}

func companyEmailRecipient(detail modulecompanies.Detail, requestedContactID int64) (recordEmailRecipient, bool) {
	var primary recordEmailRecipient
	var firstWithEmail recordEmailRecipient
	for _, contact := range detail.LinkedContacts {
		recipient := recordEmailRecipient{
			ID:        contact.ID,
			FirstName: contact.FirstName,
			LastName:  contact.LastName,
			Email:     strings.TrimSpace(contact.Email),
		}
		if requestedContactID > 0 && contact.ID == requestedContactID {
			return recipient, recipient.Email != ""
		}
		if recipient.Email == "" {
			continue
		}
		if contact.IsPrimary {
			primary = recipient
		}
		if firstWithEmail.Email == "" {
			firstWithEmail = recipient
		}
	}
	if requestedContactID > 0 {
		return recordEmailRecipient{}, false
	}
	if primary.Email != "" {
		return primary, true
	}
	return firstWithEmail, firstWithEmail.Email != ""
}

func companyMergeFields(detail modulecompanies.Detail, recipient recordEmailRecipient) map[string]string {
	fullName := strings.TrimSpace(recipient.FirstName + " " + recipient.LastName)
	summary := detail.Summary
	return map[string]string{
		"first_name":     recipient.FirstName,
		"last_name":      recipient.LastName,
		"full_name":      fullName,
		"email":          recipient.Email,
		"company_name":   summary.Name,
		"client_name":    summary.Name,
		"client_type":    summary.ClientType,
		"company_status": summary.Status,
		"client_status":  summary.Status,
		"industry":       summary.Industry,
		"phone":          summary.Phone,
		"website":        summary.Website,
	}
}

func dealMergeFields(detail moduledeals.Detail, contact modulecontacts.Detail) map[string]string {
	fields := contactMergeFields(contact)
	fields["deal_name"] = detail.Summary.Name
	fields["deal_stage"] = detail.Summary.StageName
	fields["deal_status"] = detail.Summary.Status
	fields["deal_value"] = detail.Summary.ValueAmount
	fields["deal_currency"] = detail.Summary.ValueCurrency
	fields["expected_close_date"] = detail.Summary.ExpectedCloseDate
	fields["company_name"] = detail.Summary.CompanyName
	fields["primary_contact_name"] = detail.Summary.PrimaryContactName
	return fields
}

func sendRenderedEntityEmail(w http.ResponseWriter, r *http.Request, requestID string, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, organizationID, userID int64, entityType string, entityID int64, to, subjectTemplate, bodyTemplate string, fields map[string]string, accountRequiredMessage string) {
	subject := strings.TrimSpace(moduleemailtemplates.Render(subjectTemplate, fields))
	body := strings.TrimSpace(moduleemailtemplates.Render(bodyTemplate, fields))
	if subject == "" || body == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Subject and body are required")
		return
	}
	if suppressions != nil {
		suppressed, err := suppressions.IsSuppressed(r.Context(), organizationID, to)
		if err != nil {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to check email suppression status")
			return
		}
		if suppressed {
			platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_SUPPRESSED", "This recipient has unsubscribed from email")
			return
		}
	}
	unsubscribeURL := emailUnsubscribeURL(r, suppressions, organizationID, to)
	bodyToSend := textBodyWithUnsubscribe(body, unsubscribeURL)
	trackingToken := ""
	trackingURL := ""
	trackingBaseURL := ""
	if messages != nil {
		trackingToken = newEmailTrackingToken()
		trackingBaseURL = emailTrackingBaseURL(r)
		trackingURL = emailTrackingURL(trackingBaseURL, trackingToken)
	}
	htmlBody := ""
	var trackedLinks []moduleemailmessages.TrackedLinkInput
	if trackingURL != "" || unsubscribeURL != "" {
		htmlBody, trackedLinks = trackedHTMLBody(body, trackingURL, trackingBaseURL, unsubscribeURL)
	}

	receipt, err := accounts.SendMessageAs(r.Context(), organizationID, userID, moduleemail.Message{To: to, Subject: subject, TextBody: bodyToSend, HTMLBody: htmlBody})
	if err != nil {
		if errors.Is(err, moduleuseremail.ErrNotFound) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "EMAIL_ACCOUNT_REQUIRED", accountRequiredMessage)
			return
		}
		if errors.Is(err, moduleuseremail.ErrOAuthReconnectRequired) {
			platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_OAUTH_RECONNECT_REQUIRED", "Reconnect your mailbox in Settings to approve sending email")
			return
		}
		if errors.Is(err, moduleuseremail.ErrOAuthDeliveryUnavailable) {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "OAuth mailbox delivery is not configured on this server")
			return
		}
		if errors.Is(err, moduleuseremail.ErrOAuthDeliveryUncertain) {
			recordEntityEmail(r, messages, organizationID, userID, entityType, entityID, to, subject, bodyToSend, "failed", err.Error(), trackingToken, trackedLinks, moduleuseremail.SendReceipt{})
			platformweb.WriteError(w, http.StatusBadGateway, requestID, "EMAIL_DELIVERY_UNCERTAIN", "The provider outcome is uncertain. Check your Sent folder before retrying")
			return
		}
		recordEntityEmail(r, messages, organizationID, userID, entityType, entityID, to, subject, bodyToSend, "failed", err.Error(), trackingToken, trackedLinks, moduleuseremail.SendReceipt{})
		platformweb.WriteError(w, http.StatusBadGateway, requestID, "EMAIL_SEND_FAILED", "Unable to send email through your connected mailbox")
		return
	}

	recordEntityEmail(r, messages, organizationID, userID, entityType, entityID, to, subject, bodyToSend, "sent", "", trackingToken, trackedLinks, receipt)
	logEntityEmailNote(r, notes, organizationID, userID, entityType, entityID, subject)

	response := sendEmailResponse{}
	response.Data.Sent = true
	response.Data.To = to
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func newEmailTrackingToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func emailTrackingBaseURL(r *http.Request) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func emailTrackingURL(baseURL, token string) string {
	return emailTrackingRouteURL(baseURL, "/api/email-messages/open/", token)
}

func emailClickTrackingURL(baseURL, token string) string {
	return emailTrackingRouteURL(baseURL, "/api/email-messages/click/", token)
}

func emailTrackingRouteURL(baseURL, routePrefix, token string) string {
	token = strings.TrimSpace(token)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if token == "" || baseURL == "" {
		return ""
	}
	return baseURL + routePrefix + url.PathEscape(token)
}

func trackedHTMLBody(textBody, trackingURL, trackingBaseURL, unsubscribeURL string) (string, []moduleemailmessages.TrackedLinkInput) {
	var body strings.Builder
	links := make([]moduleemailmessages.TrackedLinkInput, 0)
	last := 0
	for _, loc := range emailBodyURLPattern.FindAllStringIndex(textBody, -1) {
		if loc[0] < last {
			continue
		}
		candidate := textBody[loc[0]:loc[1]]
		targetURL, trailing := splitEmailBodyURL(candidate)
		if targetURL == "" || !isSafeEmailClickURL(targetURL) {
			continue
		}
		appendEscapedEmailHTML(&body, textBody[last:loc[0]])
		href := targetURL
		if trackingBaseURL != "" && len(links) < 100 {
			clickToken := newEmailTrackingToken()
			clickURL := emailClickTrackingURL(trackingBaseURL, clickToken)
			if clickURL != "" {
				href = clickURL
				links = append(links, moduleemailmessages.TrackedLinkInput{ClickToken: clickToken, TargetURL: targetURL})
			}
		}
		body.WriteString(`<a href="` + html.EscapeString(href) + `">` + html.EscapeString(targetURL) + `</a>`)
		appendEscapedEmailHTML(&body, trailing)
		last = loc[1]
	}
	appendEscapedEmailHTML(&body, textBody[last:])
	if unsubscribeURL != "" {
		body.WriteString(`<p style="margin-top:24px;font-size:12px;color:#666">To stop receiving emails from us, <a href="` + html.EscapeString(unsubscribeURL) + `">unsubscribe here</a>.</p>`)
	}
	trackingPixel := ""
	if trackingURL != "" {
		trackingPixel = `<img src="` + html.EscapeString(trackingURL) + `" width="1" height="1" alt="" style="display:none" />`
	}
	return `<!doctype html><html><body><div>` + body.String() + `</div>` + trackingPixel + `</body></html>`, links
}

func emailUnsubscribeURL(r *http.Request, suppressions emailSuppressionsService, organizationID int64, email string) string {
	if suppressions == nil {
		return ""
	}
	token, err := suppressions.UnsubscribeToken(organizationID, email)
	if err != nil {
		return ""
	}
	return emailTrackingRouteURL(emailTrackingBaseURL(r), "/api/email-unsubscribe/", token)
}

func textBodyWithUnsubscribe(body, unsubscribeURL string) string {
	if unsubscribeURL == "" {
		return body
	}
	return strings.TrimRight(body, " \t\r\n") + "\n\nTo stop receiving emails from us, unsubscribe here: " + unsubscribeURL
}

func appendEscapedEmailHTML(builder *strings.Builder, value string) {
	escaped := html.EscapeString(value)
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\n")
	escaped = strings.ReplaceAll(escaped, "\n", "<br>\n")
	builder.WriteString(escaped)
}

func splitEmailBodyURL(value string) (string, string) {
	targetURL := strings.TrimRight(value, ".,;:!?)]}")
	if targetURL == "" {
		return "", value
	}
	return targetURL, value[len(targetURL):]
}

func logEntityEmailNote(r *http.Request, notes notesService, organizationID, userID int64, entityType string, entityID int64, subject string) {
	if notes == nil {
		return
	}
	_, _ = notes.Create(r.Context(), organizationID, userID, modulenotes.CreateInput{
		EntityType: entityType,
		EntityID:   entityID,
		Body:       "Sent email: " + subject,
	})
}

func recordEntityEmail(r *http.Request, messages emailMessagesService, organizationID, userID int64, entityType string, entityID int64, to, subject, body, status, errMsg, trackingToken string, trackedLinks []moduleemailmessages.TrackedLinkInput, receipt moduleuseremail.SendReceipt) {
	if messages == nil {
		return
	}
	_ = messages.Record(r.Context(), organizationID, moduleemailmessages.RecordInput{
		ToEmail:           to,
		Subject:           subject,
		Body:              body,
		Status:            status,
		Error:             errMsg,
		EntityType:        entityType,
		EntityID:          entityID,
		SentByUserID:      userID,
		TrackingToken:     trackingToken,
		TrackedLinks:      trackedLinks,
		RFCMessageID:      receipt.RFCMessageID,
		ProviderMessageID: receipt.ProviderMessageID,
		ProviderThreadID:  receipt.ProviderThreadID,
	})
}
