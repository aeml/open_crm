package app

import (
	"errors"
	"net/http"
	"strings"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type emailReplyRequest struct {
	Body string `json:"body"`
}

type emailReplyResolutionRequest struct {
	Resolution string `json:"resolution"`
}

type emailReplyView struct {
	ID                  int64  `json:"id"`
	SourceMessageID     int64  `json:"sourceMessageId"`
	ThreadRootMessageID int64  `json:"threadRootMessageId"`
	ActorUserID         int64  `json:"actorUserId"`
	SenderEmail         string `json:"senderEmail"`
	RecipientEmail      string `json:"recipientEmail"`
	Subject             string `json:"subject"`
	Body                string `json:"body"`
	Status              string `json:"status"`
	LastError           string `json:"lastError,omitempty"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

type emailThreadResponse struct {
	Data struct {
		Messages []emailMessageDetailView `json:"messages"`
		Replies  []emailReplyView         `json:"replies"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type emailReplyResponse struct {
	Data struct {
		Reply emailReplyView `json:"reply"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleGetEmailThread(auth authService, messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if messages == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email thread service unavailable")
		return
	}
	messageID, ok := parsePathInt64(w, r, "messageID")
	if !ok {
		return
	}
	source, err := messages.GetByID(r.Context(), state.Organization.ID, messageID)
	if err != nil {
		writeEmailReplyServiceError(w, requestID, err)
		return
	}
	if !canViewEmailMessageDetail(state, source) {
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "You cannot view this email thread")
		return
	}
	threadMessages, replies, err := messages.ListThread(r.Context(), state.Organization.ID, source.ThreadRootMessageID, state.User.ID, isOrgAdminRole(state.Membership.Role))
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load email thread")
		return
	}
	response := emailThreadResponse{}
	response.Data.Messages = make([]emailMessageDetailView, 0, len(threadMessages))
	for _, message := range threadMessages {
		response.Data.Messages = append(response.Data.Messages, toEmailMessageDetailView(message))
	}
	response.Data.Replies = toEmailReplyViews(replies)
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleSendEmailReply(auth authService, accounts userEmailAccountService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil || messages == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Mailbox reply service unavailable")
		return
	}
	messageID, ok := parsePathInt64(w, r, "messageID")
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide an Idempotency-Key header of 16-200 characters")
		return
	}
	var input emailReplyRequest
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	replayInput := moduleemailmessages.PrepareReplyInput{
		SourceMessageID: messageID, ActorUserID: state.User.ID, Body: input.Body, IdempotencyKey: idempotencyKey,
	}
	replayed, found, err := messages.ReplayReply(r.Context(), state.Organization.ID, replayInput)
	if err != nil {
		writeEmailReplyServiceError(w, requestID, err)
		return
	}
	if found {
		if replayed.Status != "prepared" && replayed.Status != "sending" {
			writeEmailReplyResponse(w, http.StatusOK, requestID, replayed)
			return
		}
		sendEmailReply(accounts, messages, suppressions, state.Organization.ID, state.User.ID, replayed, w, r)
		return
	}
	account, err := accounts.GetForUser(r.Context(), state.Organization.ID, state.User.ID)
	if err != nil {
		writeMailboxReplyProviderError(w, requestID, err)
		return
	}
	source, err := messages.GetByID(r.Context(), state.Organization.ID, messageID)
	if err != nil {
		writeEmailReplyServiceError(w, requestID, err)
		return
	}
	if !canViewEmailMessageDetail(state, source) || source.Direction != "inbound" {
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "You cannot reply to this email message")
		return
	}
	if suppressions != nil {
		suppressed, suppressionErr := suppressions.IsSuppressed(r.Context(), state.Organization.ID, source.FromEmail)
		if suppressionErr != nil {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to check email suppression status")
			return
		}
		if suppressed {
			platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_SUPPRESSED", "This recipient is suppressed from email")
			return
		}
	}
	reply, err := messages.PrepareReply(r.Context(), state.Organization.ID, moduleemailmessages.PrepareReplyInput{
		SourceMessageID: messageID,
		ActorUserID:     state.User.ID,
		SenderEmail:     account.FromEmail,
		Body:            input.Body,
		IdempotencyKey:  idempotencyKey,
	})
	if err != nil {
		writeEmailReplyServiceError(w, requestID, err)
		return
	}
	sendEmailReply(accounts, messages, suppressions, state.Organization.ID, state.User.ID, reply, w, r)
}

func handleResolveEmailReply(auth authService, accounts userEmailAccountService, messages emailMessagesService, suppressions emailSuppressionsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if accounts == nil || messages == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Mailbox reply service unavailable")
		return
	}
	replyID, ok := parsePathInt64(w, r, "replyID")
	if !ok {
		return
	}
	var input emailReplyResolutionRequest
	if !decodeJSONRequest(w, r, requestID, &input) {
		return
	}
	resolution, err := messages.ResolveReply(r.Context(), state.Organization.ID, replyID, state.User.ID, input.Resolution)
	if err != nil {
		writeEmailReplyServiceError(w, requestID, err)
		return
	}
	if !resolution.ShouldSend {
		writeEmailReplyResponse(w, http.StatusOK, requestID, resolution.Reply)
		return
	}
	sendEmailReply(accounts, messages, suppressions, state.Organization.ID, resolution.Reply.ActorUserID, resolution.Reply, w, r)
}

func sendEmailReply(accounts userEmailAccountService, messages emailMessagesService, suppressions emailSuppressionsService, organizationID, actorUserID int64, reply moduleemailmessages.ReplyRequest, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	claimed, shouldSend, err := messages.ClaimReply(r.Context(), organizationID, reply.ID, actorUserID)
	if err != nil {
		writeEmailReplyServiceError(w, requestID, err)
		return
	}
	if !shouldSend {
		writeEmailReplyResponse(w, http.StatusAccepted, requestID, claimed)
		return
	}
	account, err := accounts.GetForUser(r.Context(), organizationID, actorUserID)
	if err != nil {
		_, recordErr := messages.FailReply(r.Context(), organizationID, claimed.ID, err, false)
		if recordErr != nil {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to record mailbox reply outcome")
			return
		}
		writeMailboxReplyProviderError(w, requestID, err)
		return
	}
	if suppressions != nil {
		suppressed, suppressionErr := suppressions.IsSuppressed(r.Context(), organizationID, claimed.RecipientEmail)
		if suppressionErr != nil {
			_, _ = messages.FailReply(r.Context(), organizationID, claimed.ID, suppressionErr, false)
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to check email suppression status")
			return
		}
		if suppressed {
			_, _ = messages.FailReply(r.Context(), organizationID, claimed.ID, errors.New("recipient is suppressed"), false)
			platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_SUPPRESSED", "This recipient is suppressed from email")
			return
		}
	}
	if !strings.EqualFold(strings.TrimSpace(account.FromEmail), claimed.SenderEmail) {
		failure := errors.New("connected mailbox sender changed; create a new reply")
		failed, _ := messages.FailReply(r.Context(), organizationID, claimed.ID, failure, false)
		writeEmailReplyResponse(w, http.StatusConflict, requestID, failed)
		return
	}
	receipt, sendErr := accounts.SendMessageAs(r.Context(), organizationID, actorUserID, moduleemail.Message{
		To:         claimed.RecipientEmail,
		Subject:    claimed.Subject,
		TextBody:   claimed.Body,
		MessageID:  claimed.RFCMessageID,
		InReplyTo:  claimed.InReplyTo,
		References: claimed.ReferenceMessageIDs,
	})
	if sendErr != nil {
		uncertain := errors.Is(sendErr, moduleuseremail.ErrOAuthDeliveryUncertain) || errors.Is(sendErr, moduleemail.ErrDeliveryUncertain)
		failed, recordErr := messages.FailReply(r.Context(), organizationID, claimed.ID, sendErr, uncertain)
		if recordErr != nil {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to record mailbox reply outcome")
			return
		}
		if uncertain {
			writeEmailReplyResponse(w, http.StatusAccepted, requestID, failed)
			return
		}
		writeMailboxReplyProviderError(w, requestID, sendErr)
		return
	}
	completed, err := messages.CompleteReply(r.Context(), organizationID, claimed.ID, receipt)
	if err != nil {
		_, _ = messages.FailReply(r.Context(), organizationID, claimed.ID, err, true)
		platformweb.WriteError(w, http.StatusBadGateway, requestID, "EMAIL_DELIVERY_UNCERTAIN", "The provider accepted the reply but its CRM record could not be finalized; check the thread before resolving it")
		return
	}
	writeEmailReplyResponse(w, http.StatusOK, requestID, completed)
}

func writeEmailReplyResponse(w http.ResponseWriter, status int, requestID string, reply moduleemailmessages.ReplyRequest) {
	response := emailReplyResponse{}
	response.Data.Reply = toEmailReplyView(reply)
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, status, response)
}

func toEmailReplyViews(replies []moduleemailmessages.ReplyRequest) []emailReplyView {
	views := make([]emailReplyView, 0, len(replies))
	for _, reply := range replies {
		views = append(views, toEmailReplyView(reply))
	}
	return views
}

func toEmailReplyView(reply moduleemailmessages.ReplyRequest) emailReplyView {
	return emailReplyView{
		ID: reply.ID, SourceMessageID: reply.SourceMessageID, ThreadRootMessageID: reply.ThreadRootMessageID,
		ActorUserID: reply.ActorUserID, SenderEmail: reply.SenderEmail, RecipientEmail: reply.RecipientEmail,
		Subject: reply.Subject, Body: reply.Body, Status: reply.Status, LastError: reply.LastError,
		CreatedAt: reply.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), UpdatedAt: reply.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func writeEmailReplyServiceError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleemailmessages.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	case errors.Is(err, moduleemailmessages.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid email reply")
	case errors.Is(err, moduleemailmessages.ErrForbidden):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "You cannot reply to this email thread")
	case errors.Is(err, moduleemailmessages.ErrReplyIdempotencyConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "This reply key was already used for different content")
	case errors.Is(err, moduleemailmessages.ErrReplyThreadUnavailable):
		platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_THREAD_UNAVAILABLE", "This message has no safe provider thread reference")
	case errors.Is(err, moduleemailmessages.ErrReplyState):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "This reply changed; reload the thread")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to process email reply")
	}
}

func writeMailboxReplyProviderError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleuseremail.ErrNotFound):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "EMAIL_ACCOUNT_REQUIRED", "Connect your own mailbox in Settings before replying")
	case errors.Is(err, moduleuseremail.ErrOAuthReconnectRequired):
		platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_OAUTH_RECONNECT_REQUIRED", "Reconnect your mailbox in Settings to approve sending email")
	case errors.Is(err, moduleuseremail.ErrOAuthDeliveryUnavailable):
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "OAuth mailbox delivery is not configured on this server")
	default:
		platformweb.WriteError(w, http.StatusBadGateway, requestID, "EMAIL_SEND_FAILED", "Unable to send the reply through your connected mailbox")
	}
}
