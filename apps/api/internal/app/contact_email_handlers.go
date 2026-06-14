package app

import (
	"errors"
	"net/http"
	"strings"

	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type sendContactEmailRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
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

// handleSendContactEmail sends a one-to-one email to a contact through the
// sending user's own mailbox (their configured SMTP account), so the email
// comes from the user, not the platform. The subject and body may contain
// {{merge_field}} placeholders, rendered server-side from contact data. A note
// is logged on the contact so the send appears in its activity timeline.
func handleSendContactEmail(auth authService, contacts contactsService, accounts userEmailAccountService, notes notesService, messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
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

	var request sendContactEmailRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	fields := contactMergeFields(detail)
	subject := strings.TrimSpace(moduleemailtemplates.Render(request.Subject, fields))
	body := strings.TrimSpace(moduleemailtemplates.Render(request.Body, fields))
	if subject == "" || body == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Subject and body are required")
		return
	}

	if err := accounts.SendAs(r.Context(), state.Organization.ID, state.User.ID, to, subject, body); err != nil {
		if errors.Is(err, moduleuseremail.ErrNotFound) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "EMAIL_ACCOUNT_REQUIRED", "Connect your email account in Settings before sending email to contacts")
			return
		}
		recordContactEmail(r, messages, state.Organization.ID, state.User.ID, contactID, to, subject, body, "failed", err.Error())
		platformweb.WriteError(w, http.StatusBadGateway, requestID, "EMAIL_SEND_FAILED", "Unable to send email through your mail server")
		return
	}

	recordContactEmail(r, messages, state.Organization.ID, state.User.ID, contactID, to, subject, body, "sent", "")
	logContactEmailNote(r, notes, state.Organization.ID, state.User.ID, contactID, subject)

	response := sendEmailResponse{}
	response.Data.Sent = true
	response.Data.To = to
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
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

func logContactEmailNote(r *http.Request, notes notesService, organizationID, userID, contactID int64, subject string) {
	if notes == nil {
		return
	}
	_, _ = notes.Create(r.Context(), organizationID, userID, modulenotes.CreateInput{
		EntityType: "contact",
		EntityID:   contactID,
		Body:       "Sent email: " + subject,
	})
}

func recordContactEmail(r *http.Request, messages emailMessagesService, organizationID, userID, contactID int64, to, subject, body, status, errMsg string) {
	if messages == nil {
		return
	}
	_ = messages.Record(r.Context(), organizationID, moduleemailmessages.RecordInput{
		ToEmail:      to,
		Subject:      subject,
		Body:         body,
		Status:       status,
		Error:        errMsg,
		EntityType:   "contact",
		EntityID:     contactID,
		SentByUserID: userID,
	})
}
