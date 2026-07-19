package app

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type backgroundJobsResponse struct {
	Data struct {
		Jobs  []modulejobs.Job      `json:"jobs"`
		Stats modulejobs.QueueStats `json:"stats"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type backgroundJobResponse struct {
	Data struct {
		Job modulejobs.Job `json:"job"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type sequenceDeliveryResolutionRequest struct {
	Resolution string `json:"resolution"`
}

type sequenceDeliveryResolutionResponse struct {
	Data struct {
		Resolution moduleemailsequences.DeliveryResolution `json:"resolution"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleListBackgroundJobs(auth authService, jobs backgroundJobsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if jobs == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Background job service unavailable")
		return
	}
	query := modulejobs.ListQuery{
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Type:   strings.TrimSpace(r.URL.Query().Get("type")),
		Limit:  parsePositiveInt(r.URL.Query().Get("limit"), 50),
	}
	list, err := jobs.List(r.Context(), state.Organization.ID, query)
	if errors.Is(err, modulejobs.ErrInvalidInput) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid job status, type, and limit")
		return
	}
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load background jobs")
		return
	}
	stats, err := jobs.Stats(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load background job health")
		return
	}
	response := backgroundJobsResponse{}
	response.Data.Jobs = list
	response.Data.Stats = stats
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleReplayBackgroundJob(auth authService, jobs backgroundJobsService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if jobs == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Background job service unavailable")
		return
	}
	jobID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("jobID")), 10, 64)
	if err != nil || jobID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid background job id")
		return
	}
	job, err := jobs.Replay(r.Context(), state.Organization.ID, jobID)
	if errors.Is(err, modulejobs.ErrNotFound) {
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Dead background job not found")
		return
	}
	if errors.Is(err, modulejobs.ErrInvalidInput) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Background job cannot be replayed")
		return
	}
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to replay background job")
		return
	}
	recordAuditEvent(r, audit, state.Organization.ID, moduleaudit.RecordInput{
		ActorUserID: state.User.ID,
		EventType:   "background_job.replayed",
		EntityType:  "background_job",
		EntityID:    job.ID,
		Summary:     fmt.Sprintf("Replayed failed background job %s", job.Type),
		Metadata:    map[string]string{"jobType": job.Type, "idempotencyKey": job.IdempotencyKey},
	})
	response := backgroundJobResponse{}
	response.Data.Job = job
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleResolveSequenceDelivery(auth authService, deliveries sequenceDeliveryOperationsService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if deliveries == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Sequence delivery operations unavailable")
		return
	}
	jobID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("jobID")), 10, 64)
	if err != nil || jobID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid background job id")
		return
	}
	var request sequenceDeliveryResolutionRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	request.Resolution = strings.TrimSpace(strings.ToLower(request.Resolution))
	resolution, err := deliveries.ResolveUncertainDeliveryJob(r.Context(), state.Organization.ID, jobID, request.Resolution)
	if errors.Is(err, moduleemailsequences.ErrInvalidInput) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Resolution must be confirmed_sent or retry")
		return
	}
	if errors.Is(err, moduleemailsequences.ErrNotFound) {
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Sequence delivery job not found")
		return
	}
	if errors.Is(err, moduleemailsequences.ErrDeliveryState) {
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "Sequence delivery is not awaiting an operator decision")
		return
	}
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to resolve sequence delivery")
		return
	}
	recordAuditEvent(r, audit, state.Organization.ID, moduleaudit.RecordInput{
		ActorUserID: state.User.ID,
		EventType:   "email_sequence.delivery_resolved",
		EntityType:  "background_job",
		EntityID:    jobID,
		Summary:     fmt.Sprintf("Resolved uncertain sequence delivery as %s", request.Resolution),
		Metadata:    map[string]string{"resolution": request.Resolution, "deliveryId": strconv.FormatInt(resolution.DeliveryID, 10)},
	})
	response := sequenceDeliveryResolutionResponse{}
	response.Data.Resolution = resolution
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}
