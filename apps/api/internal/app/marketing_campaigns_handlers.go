package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	modulemarketingcampaigns "github.com/aeml/open_crm/apps/api/internal/modules/marketingcampaigns"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type marketingCampaignsListResponse struct {
	Data struct {
		Campaigns []modulemarketingcampaigns.Campaign `json:"campaigns"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type marketingCampaignResponse struct {
	Data struct {
		Campaign modulemarketingcampaigns.Campaign `json:"campaign"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type marketingCampaignRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	AudienceID  int64      `json:"audienceId"`
	Subject     string     `json:"subject"`
	PreviewText string     `json:"previewText"`
	Body        string     `json:"body"`
	Status      string     `json:"status"`
	ScheduledAt *time.Time `json:"scheduledAt"`
}

func handleListMarketingCampaigns(auth authService, campaigns marketingCampaignsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if campaigns == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Marketing campaigns service unavailable")
		return
	}

	result, err := campaigns.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load marketing campaigns")
		return
	}

	response := marketingCampaignsListResponse{}
	response.Data.Campaigns = result
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateMarketingCampaign(auth authService, campaigns marketingCampaignsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if campaigns == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Marketing campaigns service unavailable")
		return
	}

	var request marketingCampaignRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	campaign, err := campaigns.Create(r.Context(), state.Organization.ID, state.User.ID, marketingCampaignInput(request))
	if err != nil {
		writeMarketingCampaignError(w, requestID, err)
		return
	}
	respondMarketingCampaign(w, requestID, http.StatusCreated, campaign)
}

func handleUpdateMarketingCampaign(auth authService, campaigns marketingCampaignsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if campaigns == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Marketing campaigns service unavailable")
		return
	}
	campaignID, ok := parsePathInt64(w, r, "campaignID")
	if !ok {
		return
	}

	var request marketingCampaignRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	campaign, err := campaigns.Update(r.Context(), state.Organization.ID, campaignID, state.User.ID, marketingCampaignInput(request))
	if err != nil {
		writeMarketingCampaignError(w, requestID, err)
		return
	}
	respondMarketingCampaign(w, requestID, http.StatusOK, campaign)
}

func marketingCampaignInput(request marketingCampaignRequest) modulemarketingcampaigns.Input {
	return modulemarketingcampaigns.Input{
		Name:        strings.TrimSpace(request.Name),
		Description: strings.TrimSpace(request.Description),
		AudienceID:  request.AudienceID,
		Subject:     strings.TrimSpace(request.Subject),
		PreviewText: strings.TrimSpace(request.PreviewText),
		Body:        strings.TrimSpace(request.Body),
		Status:      strings.TrimSpace(request.Status),
		ScheduledAt: request.ScheduledAt,
	}
}

func respondMarketingCampaign(w http.ResponseWriter, requestID string, statusCode int, campaign modulemarketingcampaigns.Campaign) {
	response := marketingCampaignResponse{}
	response.Data.Campaign = campaign
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, statusCode, response)
}

func writeMarketingCampaignError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, modulemarketingcampaigns.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid campaign name, active audience, subject, body, status, and schedule")
	case errors.Is(err, modulemarketingcampaigns.ErrInvalidAudience):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose an active lead audience for this campaign")
	case errors.Is(err, modulemarketingcampaigns.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A marketing campaign with that name already exists")
	case errors.Is(err, modulemarketingcampaigns.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save marketing campaign")
	}
}
