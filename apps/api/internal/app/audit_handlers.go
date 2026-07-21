package app

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleListAuditEvents(auth authService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if audit == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Audit service unavailable")
		return
	}

	events, err := audit.ListByOrganization(r.Context(), state.Organization.ID, moduleaudit.ListQuery{
		EventType: strings.TrimSpace(r.URL.Query().Get("eventType")),
		Limit:     parsePositiveInt(r.URL.Query().Get("limit"), 50),
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load audit events")
		return
	}

	response := auditEventsResponse{}
	response.Data.Events = events
	response.Data.Policy = moduleaudit.CurrentRetentionPolicy()
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleExportAuditEvents(auth authService, audit auditService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if audit == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Audit service unavailable")
		return
	}

	eventType := strings.TrimSpace(r.URL.Query().Get("eventType"))
	file, err := audit.ExportCSV(r.Context(), state.Organization.ID, moduleaudit.ListQuery{EventType: eventType})
	if errors.Is(err, moduleaudit.ErrTooManyRows) {
		platformweb.WriteError(w, http.StatusUnprocessableEntity, requestID, "EXPORT_TOO_LARGE", err.Error())
		return
	}
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to export audit events")
		return
	}
	filterDescription := eventType
	if filterDescription == "" {
		filterDescription = "all"
	}
	if err := audit.Record(r.Context(), state.Organization.ID, moduleaudit.RecordInput{
		ActorUserID: state.User.ID,
		EventType:   "audit.export_downloaded",
		EntityType:  "organization",
		EntityID:    state.Organization.ID,
		Summary:     "Downloaded audit event CSV",
		Metadata: map[string]string{
			"eventTypeFilter": filterDescription,
			"maximumRows":     strconv.Itoa(moduleaudit.MaxExportRows),
		},
	}); err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to record audit export")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file.Filename))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Content)
}
