package app

import (
	"errors"
	"net/http"
	"strings"

	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type sendRecordEmailRequest struct {
	ContactID       int64  `json:"contactId"`
	Subject         string `json:"subject"`
	Body            string `json:"body"`
	TrackEngagement bool   `json:"trackEngagement"`
}

type sendEmailResponse struct {
	Data recordEmailDeliveryView `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type recordEmailDeliveryView struct {
	ID                 int64  `json:"id"`
	EntityType         string `json:"entityType"`
	EntityID           int64  `json:"entityId"`
	ActorUserID        int64  `json:"actorUserId"`
	To                 string `json:"to"`
	Subject            string `json:"subject"`
	Status             string `json:"status"`
	Sent               bool   `json:"sent"`
	LastError          string `json:"lastError,omitempty"`
	OwnedByCurrentUser bool   `json:"ownedByCurrentUser"`
	CanRetry           bool   `json:"canRetry"`
	CanResolve         bool   `json:"canResolve"`
}

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
	if contacts == nil || messages == nil {
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
	var request sendRecordEmailRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	key, ok := recordEmailDeliveryKey(w, r, requestID, state.User.ID, "contact", contactID, contactID, request)
	if !ok || replayRecordEmailDelivery(w, r, requestID, accounts, messages, suppressions, state, key) {
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

	fields := contactMergeFields(detail)
	prepareAndSendRecordEmail(w, r, requestID, accounts, messages, suppressions, state, key, contactID, to, request, fields)
}

func handleSendCompanyEmail(auth authService, companies companiesService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if companies == nil || messages == nil {
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
	var request sendRecordEmailRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	key, ok := recordEmailDeliveryKey(w, r, requestID, state.User.ID, "company", companyID, request.ContactID, request)
	if !ok || replayRecordEmailDelivery(w, r, requestID, accounts, messages, suppressions, state, key) {
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

	recipient, ok := companyEmailRecipient(detail, request.ContactID)
	if !ok {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Company has no linked contact with an email address")
		return
	}

	fields := companyMergeFields(detail, recipient)
	prepareAndSendRecordEmail(w, r, requestID, accounts, messages, suppressions, state, key, recipient.ID, recipient.Email, request, fields)
}

func handleSendDealEmail(auth authService, deals dealsService, contacts contactsService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if deals == nil || messages == nil {
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
	var request sendRecordEmailRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	key, ok := recordEmailDeliveryKey(w, r, requestID, state.User.ID, "deal", dealID, request.ContactID, request)
	if !ok || replayRecordEmailDelivery(w, r, requestID, accounts, messages, suppressions, state, key) {
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
	prepareAndSendRecordEmail(w, r, requestID, accounts, messages, suppressions, state, key, detail.Summary.PrimaryContactID, to, request, fields)
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

func recordEmailDeliveryKey(w http.ResponseWriter, r *http.Request, requestID string, actorUserID int64, entityType string, entityID, requestedContactID int64, request sendRecordEmailRequest) (moduleemailmessages.RecordDeliveryKeyInput, bool) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide an Idempotency-Key header of 16-200 characters")
		return moduleemailmessages.RecordDeliveryKeyInput{}, false
	}
	return moduleemailmessages.RecordDeliveryKeyInput{
		EntityType: entityType, EntityID: entityID, RecipientContactID: requestedContactID,
		ActorUserID: actorUserID, SubjectTemplate: request.Subject, BodyTemplate: request.Body,
		TrackEngagement: request.TrackEngagement, IdempotencyKey: idempotencyKey,
	}, true
}

func replayRecordEmailDelivery(w http.ResponseWriter, r *http.Request, requestID string, accounts userEmailAccountService, messages emailMessagesService, suppressions emailSuppressionsService, state moduleauth.SessionState, key moduleemailmessages.RecordDeliveryKeyInput) bool {
	delivery, found, err := messages.ReplayRecordDelivery(r.Context(), state.Organization.ID, key)
	if err != nil {
		writeRecordEmailDeliveryServiceError(w, requestID, err)
		return true
	}
	if !found {
		return false
	}
	if delivery.Status == "prepared" || delivery.Status == "sending" {
		sendRecordEmailDelivery(w, r, requestID, accounts, messages, suppressions, state, delivery)
		return true
	}
	writeRecordEmailDeliveryResponse(w, http.StatusOK, requestID, state, delivery)
	return true
}

func prepareAndSendRecordEmail(w http.ResponseWriter, r *http.Request, requestID string, accounts userEmailAccountService, messages emailMessagesService, suppressions emailSuppressionsService, state moduleauth.SessionState, key moduleemailmessages.RecordDeliveryKeyInput, resolvedContactID int64, to string, request sendRecordEmailRequest, fields map[string]string) {
	organizationID := state.Organization.ID
	subject := strings.TrimSpace(moduleemailtemplates.Render(request.Subject, fields))
	body := strings.TrimSpace(moduleemailtemplates.Render(request.Body, fields))
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
	if request.TrackEngagement {
		trackingToken = newEmailTrackingToken()
		trackingBaseURL = emailTrackingBaseURL(r)
		trackingURL = emailTrackingURL(trackingBaseURL, trackingToken)
		if trackingURL == "" {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Engagement tracking requires a valid public email URL")
			return
		}
	}
	htmlBody := ""
	var trackedLinks []moduleemailmessages.TrackedLinkInput
	if trackingURL != "" || unsubscribeURL != "" {
		htmlBody, trackedLinks = trackedHTMLBody(body, trackingURL, trackingBaseURL, unsubscribeURL)
	}
	account, err := accounts.GetForUser(r.Context(), organizationID, state.User.ID)
	if err != nil {
		writeRecordEmailAccountError(w, requestID, err)
		return
	}
	rfcMessageID, err := moduleemail.NewMessageID(emailAddressDomain(account.FromEmail))
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to prepare email delivery")
		return
	}
	delivery, err := messages.PrepareRecordDelivery(r.Context(), organizationID, moduleemailmessages.PrepareRecordDeliveryInput{
		Request: key, ResolvedRecipientContactID: resolvedContactID, SenderEmail: account.FromEmail,
		RecipientEmail: to, Subject: subject, TextBody: bodyToSend, HTMLBody: htmlBody,
		ListUnsubscribeURL: moduleemail.OneClickUnsubscribeURL(unsubscribeURL), RFCMessageID: rfcMessageID,
		TrackingToken: trackingToken, TrackedLinks: trackedLinks,
	})
	if err != nil {
		writeRecordEmailDeliveryServiceError(w, requestID, err)
		return
	}
	sendRecordEmailDelivery(w, r, requestID, accounts, messages, suppressions, state, delivery)
}

func sendRecordEmailDelivery(w http.ResponseWriter, r *http.Request, requestID string, accounts userEmailAccountService, messages emailMessagesService, suppressions emailSuppressionsService, state moduleauth.SessionState, delivery moduleemailmessages.RecordDelivery) {
	claimed, shouldSend, err := messages.ClaimRecordDelivery(r.Context(), state.Organization.ID, delivery.ID, state.User.ID)
	if err != nil {
		writeRecordEmailDeliveryServiceError(w, requestID, err)
		return
	}
	if !shouldSend {
		status := http.StatusAccepted
		if claimed.Status == "accepted" || claimed.Status == "failed" || claimed.Status == "uncertain" {
			status = http.StatusOK
		}
		writeRecordEmailDeliveryResponse(w, status, requestID, state, claimed)
		return
	}
	account, err := accounts.GetForUser(r.Context(), state.Organization.ID, claimed.ActorUserID)
	if err != nil {
		_, _ = messages.FailRecordDelivery(r.Context(), state.Organization.ID, claimed.ID, err, false)
		writeRecordEmailAccountError(w, requestID, err)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(account.FromEmail), claimed.SenderEmail) {
		failure := errors.New("connected mailbox sender changed; compose a new email")
		_, _ = messages.FailRecordDelivery(r.Context(), state.Organization.ID, claimed.ID, failure, false)
		platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_SENDER_CHANGED", failure.Error())
		return
	}
	if suppressions != nil {
		suppressed, suppressionErr := suppressions.IsSuppressed(r.Context(), state.Organization.ID, claimed.RecipientEmail)
		if suppressionErr != nil {
			_, _ = messages.FailRecordDelivery(r.Context(), state.Organization.ID, claimed.ID, suppressionErr, false)
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to check email suppression status")
			return
		}
		if suppressed {
			failure := errors.New("recipient is suppressed")
			_, _ = messages.FailRecordDelivery(r.Context(), state.Organization.ID, claimed.ID, failure, false)
			platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_SUPPRESSED", "This recipient has unsubscribed from email")
			return
		}
	}
	receipt, err := accounts.SendMessageAs(r.Context(), state.Organization.ID, claimed.ActorUserID, moduleemail.Message{
		To: claimed.RecipientEmail, Subject: claimed.Subject, TextBody: claimed.TextBody,
		HTMLBody: claimed.HTMLBody, MessageID: claimed.RFCMessageID,
		ListUnsubscribeURL: claimed.ListUnsubscribeURL,
	})
	if err != nil {
		uncertain := errors.Is(err, moduleuseremail.ErrOAuthDeliveryUncertain) || errors.Is(err, moduleemail.ErrDeliveryUncertain)
		if _, recordErr := messages.FailRecordDelivery(r.Context(), state.Organization.ID, claimed.ID, err, uncertain); recordErr != nil {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to record mailbox delivery outcome")
			return
		}
		if uncertain {
			platformweb.WriteError(w, http.StatusBadGateway, requestID, "EMAIL_DELIVERY_UNCERTAIN", "The provider outcome is uncertain. Check your Sent folder before resolving this email")
			return
		}
		writeRecordEmailAccountError(w, requestID, err)
		return
	}
	completed, err := messages.CompleteRecordDelivery(r.Context(), state.Organization.ID, claimed.ID, receipt)
	if err != nil {
		_, _ = messages.FailRecordDelivery(r.Context(), state.Organization.ID, claimed.ID, err, true)
		platformweb.WriteError(w, http.StatusBadGateway, requestID, "EMAIL_DELIVERY_UNCERTAIN", "The mailbox accepted the email but CRM completion was interrupted. Resolve it after checking Sent mail")
		return
	}
	writeRecordEmailDeliveryResponse(w, http.StatusOK, requestID, state, completed)
}

func writeRecordEmailAccountError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleuseremail.ErrNotFound):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "EMAIL_ACCOUNT_REQUIRED", "Connect your email account in Settings before sending email")
	case errors.Is(err, moduleuseremail.ErrOAuthReconnectRequired):
		platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_OAUTH_RECONNECT_REQUIRED", "Reconnect your mailbox in Settings to approve sending email")
	case errors.Is(err, moduleuseremail.ErrOAuthDeliveryUnavailable):
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "OAuth mailbox delivery is not configured on this server")
	default:
		platformweb.WriteError(w, http.StatusBadGateway, requestID, "EMAIL_SEND_FAILED", "Unable to send email through your connected mailbox")
	}
}

func emailAddressDomain(value string) string {
	separator := strings.LastIndexByte(strings.TrimSpace(value), '@')
	if separator < 0 || separator == len(value)-1 {
		return ""
	}
	return value[separator+1:]
}
