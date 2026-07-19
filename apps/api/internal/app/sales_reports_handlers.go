package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	modulesalesreports "github.com/aeml/open_crm/apps/api/internal/modules/salesreports"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type salesActivityReportResponse struct {
	Data modulesalesreports.Report `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func handleSalesActivityReport(auth authService, service salesReportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Sales activity reporting service unavailable")
		return
	}

	ownerUserID, ok := parseOptionalPositiveQueryID(r.URL.Query().Get("ownerUserId"))
	if !ok {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", modulesalesreports.ErrInvalidInput.Error())
		return
	}
	report, err := service.Activity(r.Context(), state.Organization.ID, modulesalesreports.Query{
		FromDate:    r.URL.Query().Get("from"),
		ToDate:      r.URL.Query().Get("to"),
		OwnerUserID: ownerUserID,
	})
	if err != nil {
		if errors.Is(err, modulesalesreports.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
		} else {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load sales activity report")
		}
		return
	}
	response := salesActivityReportResponse{Data: report}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func parseOptionalPositiveQueryID(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed > 0
}
