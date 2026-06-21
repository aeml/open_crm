package app

import (
	"errors"
	"net/http"
	"strings"

	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type leadChatWidgetsListResponse struct {
	Data struct {
		Widgets []moduleleadforms.ChatWidget `json:"widgets"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadChatWidgetResponse struct {
	Data struct {
		Widget moduleleadforms.ChatWidget `json:"widget"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type publicLeadChatWidgetResponse struct {
	Data moduleleadforms.PublicChatWidget `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type leadChatWidgetRequest struct {
	Name              string `json:"name"`
	Title             string `json:"title"`
	WelcomeMessage    string `json:"welcomeMessage"`
	PromptLabel       string `json:"promptLabel"`
	CTALabel          string `json:"ctaLabel"`
	Theme             string `json:"theme"`
	Position          string `json:"position"`
	LeadCaptureFormID int64  `json:"leadCaptureFormId"`
	IsActive          *bool  `json:"isActive"`
}

func handleListLeadChatWidgets(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead chat widgets service unavailable")
		return
	}

	widgets, err := forms.ListChatWidgetsByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load lead chat widgets")
		return
	}

	response := leadChatWidgetsListResponse{}
	response.Data.Widgets = widgets
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateLeadChatWidget(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead chat widgets service unavailable")
		return
	}

	var request leadChatWidgetRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	widget, err := forms.CreateChatWidget(r.Context(), state.Organization.ID, state.User.ID, leadChatWidgetInput(request))
	if err != nil {
		writeLeadChatWidgetError(w, requestID, err)
		return
	}

	respondLeadChatWidget(w, requestID, http.StatusCreated, widget)
}

func handleUpdateLeadChatWidget(auth authService, forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead chat widgets service unavailable")
		return
	}
	widgetID, ok := parsePathInt64(w, r, "widgetID")
	if !ok {
		return
	}

	var request leadChatWidgetRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	widget, err := forms.UpdateChatWidget(r.Context(), state.Organization.ID, widgetID, state.User.ID, leadChatWidgetInput(request))
	if err != nil {
		writeLeadChatWidgetError(w, requestID, err)
		return
	}

	respondLeadChatWidget(w, requestID, http.StatusOK, widget)
}

func handleGetPublicLeadChatWidget(forms leadFormsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if forms == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Lead chat widgets service unavailable")
		return
	}
	publicID := strings.TrimSpace(r.PathValue("publicID"))
	if publicID == "" {
		platformweb.WriteNotFound(w, requestID)
		return
	}

	result, err := forms.GetPublicChatWidget(r.Context(), publicID)
	if err != nil {
		if writeResourceNotFound(w, requestID, err) {
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load lead chat widget")
		return
	}

	response := publicLeadChatWidgetResponse{Data: result}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func respondLeadChatWidget(w http.ResponseWriter, requestID string, statusCode int, widget moduleleadforms.ChatWidget) {
	response := leadChatWidgetResponse{}
	response.Data.Widget = widget
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, statusCode, response)
}

func leadChatWidgetInput(request leadChatWidgetRequest) moduleleadforms.ChatWidgetInput {
	return moduleleadforms.ChatWidgetInput{
		Name:              request.Name,
		Title:             request.Title,
		WelcomeMessage:    request.WelcomeMessage,
		PromptLabel:       request.PromptLabel,
		CTALabel:          request.CTALabel,
		Theme:             request.Theme,
		Position:          request.Position,
		LeadCaptureFormID: request.LeadCaptureFormID,
		IsActive:          request.IsActive,
	}
}

func writeLeadChatWidgetError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleleadforms.ErrInvalidWidget):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid widget name, title, labels, theme, position, and lead form")
	case errors.Is(err, moduleleadforms.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save lead chat widget")
	}
}
