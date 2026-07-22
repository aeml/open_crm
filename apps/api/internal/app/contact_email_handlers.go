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
	Purpose            string `json:"purpose"`
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
func handleSendContactEmail(auth authService, contacts contactsService, customFields customFieldsService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	handleContactEmail(auth, contacts, customFields, accounts, notes, messages, suppressions, "record", w, r)
}

func handleTestContactEmail(auth authService, contacts contactsService, customFields customFieldsService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	handleContactEmail(auth, contacts, customFields, accounts, notes, messages, suppressions, "test", w, r)
}

func handleContactEmail(auth authService, contacts contactsService, customFields customFieldsService, accounts userEmailAccountService, _ notesService, messages emailMessagesService, suppressions emailSuppressionsService, purpose string, w http.ResponseWriter, r *http.Request) {
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
	key, ok := recordEmailDeliveryKey(w, r, requestID, purpose, state.User.ID, "contact", contactID, contactID, request)
	if !ok || replayRecordEmailDelivery(w, r, requestID, accounts, messages, suppressions, state, key) {
		return
	}
	composition, err := loadContactEmailComposition(r.Context(), state.Organization.ID, contactID, contacts, customFields)
	if err != nil {
		writeRecordEmailCompositionError(w, requestID, err)
		return
	}
	prepareAndSendRecordEmail(w, r, requestID, accounts, messages, suppressions, state, key, composition.RecipientContactID, composition.RecipientEmail, request, composition.Fields)
}

func handleSendCompanyEmail(auth authService, companies companiesService, contacts contactsService, customFields customFieldsService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	handleCompanyEmail(auth, companies, contacts, customFields, accounts, notes, messages, suppressions, "record", w, r)
}

func handleTestCompanyEmail(auth authService, companies companiesService, contacts contactsService, customFields customFieldsService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	handleCompanyEmail(auth, companies, contacts, customFields, accounts, notes, messages, suppressions, "test", w, r)
}

func handleCompanyEmail(auth authService, companies companiesService, contacts contactsService, customFields customFieldsService, accounts userEmailAccountService, _ notesService, messages emailMessagesService, suppressions emailSuppressionsService, purpose string, w http.ResponseWriter, r *http.Request) {
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
	key, ok := recordEmailDeliveryKey(w, r, requestID, purpose, state.User.ID, "company", companyID, request.ContactID, request)
	if !ok || replayRecordEmailDelivery(w, r, requestID, accounts, messages, suppressions, state, key) {
		return
	}
	composition, err := loadCompanyEmailComposition(r.Context(), state.Organization.ID, companyID, request.ContactID, companies, contacts, customFields)
	if err != nil {
		writeRecordEmailCompositionError(w, requestID, err)
		return
	}
	prepareAndSendRecordEmail(w, r, requestID, accounts, messages, suppressions, state, key, composition.RecipientContactID, composition.RecipientEmail, request, composition.Fields)
}

func handleSendDealEmail(auth authService, deals dealsService, contacts contactsService, companies companiesService, customFields customFieldsService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	handleDealEmail(auth, deals, contacts, companies, customFields, accounts, notes, messages, suppressions, "record", w, r)
}

func handleTestDealEmail(auth authService, deals dealsService, contacts contactsService, companies companiesService, customFields customFieldsService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	handleDealEmail(auth, deals, contacts, companies, customFields, accounts, notes, messages, suppressions, "test", w, r)
}

func handleDealEmail(auth authService, deals dealsService, contacts contactsService, companies companiesService, customFields customFieldsService, accounts userEmailAccountService, _ notesService, messages emailMessagesService, suppressions emailSuppressionsService, purpose string, w http.ResponseWriter, r *http.Request) {
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
	key, ok := recordEmailDeliveryKey(w, r, requestID, purpose, state.User.ID, "deal", dealID, request.ContactID, request)
	if !ok || replayRecordEmailDelivery(w, r, requestID, accounts, messages, suppressions, state, key) {
		return
	}
	composition, err := loadDealEmailComposition(r.Context(), state.Organization.ID, dealID, request.ContactID, deals, contacts, companies, customFields)
	if err != nil {
		writeRecordEmailCompositionError(w, requestID, err)
		return
	}
	prepareAndSendRecordEmail(w, r, requestID, accounts, messages, suppressions, state, key, composition.RecipientContactID, composition.RecipientEmail, request, composition.Fields)
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

func recordEmailDeliveryKey(w http.ResponseWriter, r *http.Request, requestID, purpose string, actorUserID int64, entityType string, entityID, requestedContactID int64, request sendRecordEmailRequest) (moduleemailmessages.RecordDeliveryKeyInput, bool) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide an Idempotency-Key header of 16-200 characters")
		return moduleemailmessages.RecordDeliveryKeyInput{}, false
	}
	trackEngagement := request.TrackEngagement
	if purpose == "test" {
		trackEngagement = false
	}
	return moduleemailmessages.RecordDeliveryKeyInput{
		Purpose: purpose, EntityType: entityType, EntityID: entityID, RecipientContactID: requestedContactID,
		ActorUserID: actorUserID, SubjectTemplate: request.Subject, BodyTemplate: request.Body,
		TrackEngagement: trackEngagement, IdempotencyKey: idempotencyKey,
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
	if unresolved := moduleemailtemplates.UnresolvedTokens(subject, body); len(unresolved) > 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "EMAIL_MERGE_FIELDS_UNRESOLVED", "Preview and resolve every unknown merge field before sending")
		return
	}
	recipientUserID := int64(0)
	if key.Purpose == "test" {
		to = strings.TrimSpace(state.User.Email)
		recipientUserID = state.User.ID
		subject = "[TEST] " + subject
		body = "This is an Open CRM template test sent only to you. The CRM recipient did not receive it.\n\n" + body
		request.TrackEngagement = false
		if len(subject) > 998 || len(body) > 110000 {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Test subject or body is too long after adding the test notice")
			return
		}
	}
	if key.Purpose == "record" && suppressions != nil {
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
	unsubscribeURL := ""
	if key.Purpose == "record" {
		unsubscribeURL = emailUnsubscribeURL(r, suppressions, organizationID, to)
	}
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
		Request: key, ResolvedRecipientContactID: resolvedContactID, ResolvedRecipientUserID: recipientUserID, SenderEmail: account.FromEmail,
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
	if claimed.Purpose == "record" && suppressions != nil {
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
