package app

import (
	"errors"
	"net/http"
	"strings"

	modulenurturecampaigns "github.com/aeml/open_crm/apps/api/internal/modules/nurturecampaigns"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type nurtureCampaignsListResponse struct {
	Data struct {
		Campaigns []modulenurturecampaigns.Campaign `json:"campaigns"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type nurtureCampaignResponse struct {
	Data struct {
		Campaign modulenurturecampaigns.Campaign `json:"campaign"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type nurtureCampaignRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	AudienceID  int64  `json:"audienceId"`
	SequenceID  int64  `json:"sequenceId"`
	Status      string `json:"status"`
}

func handleListNurtureCampaigns(auth authService, campaigns nurtureCampaignsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if campaigns == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Nurture campaigns service unavailable")
		return
	}

	result, err := campaigns.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load nurture campaigns")
		return
	}

	response := nurtureCampaignsListResponse{}
	response.Data.Campaigns = result
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateNurtureCampaign(auth authService, campaigns nurtureCampaignsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if campaigns == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Nurture campaigns service unavailable")
		return
	}

	var request nurtureCampaignRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	campaign, err := campaigns.Create(r.Context(), state.Organization.ID, state.User.ID, nurtureCampaignInput(request))
	if err != nil {
		writeNurtureCampaignError(w, requestID, err)
		return
	}
	respondNurtureCampaign(w, requestID, http.StatusCreated, campaign)
}

func handleUpdateNurtureCampaign(auth authService, campaigns nurtureCampaignsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if campaigns == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Nurture campaigns service unavailable")
		return
	}
	campaignID, ok := parsePathInt64(w, r, "campaignID")
	if !ok {
		return
	}

	var request nurtureCampaignRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	campaign, err := campaigns.Update(r.Context(), state.Organization.ID, campaignID, state.User.ID, nurtureCampaignInput(request))
	if err != nil {
		writeNurtureCampaignError(w, requestID, err)
		return
	}
	respondNurtureCampaign(w, requestID, http.StatusOK, campaign)
}

func nurtureCampaignInput(request nurtureCampaignRequest) modulenurturecampaigns.Input {
	return modulenurturecampaigns.Input{
		Name:        strings.TrimSpace(request.Name),
		Description: strings.TrimSpace(request.Description),
		AudienceID:  request.AudienceID,
		SequenceID:  request.SequenceID,
		Status:      strings.TrimSpace(request.Status),
	}
}

func respondNurtureCampaign(w http.ResponseWriter, requestID string, statusCode int, campaign modulenurturecampaigns.Campaign) {
	response := nurtureCampaignResponse{}
	response.Data.Campaign = campaign
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, statusCode, response)
}

func writeNurtureCampaignError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, modulenurturecampaigns.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid nurture campaign name, active audience, email sequence, and status")
	case errors.Is(err, modulenurturecampaigns.ErrInvalidAudience):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose an active lead audience for this nurture campaign")
	case errors.Is(err, modulenurturecampaigns.ErrInvalidSequence):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Choose an email sequence; active nurture campaigns require an active sequence")
	case errors.Is(err, modulenurturecampaigns.ErrDuplicateName):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A nurture campaign with that name already exists")
	case errors.Is(err, modulenurturecampaigns.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save nurture campaign")
	}
}
