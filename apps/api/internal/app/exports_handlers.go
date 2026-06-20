package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	moduleexports "github.com/aeml/open_crm/apps/api/internal/modules/exports"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleExportContacts(auth authService, exports dataExportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if exports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Export service unavailable")
		return
	}

	file, err := exports.ContactsCSV(r.Context(), state.Organization.ID, moduleexports.ContactsQuery{Search: strings.TrimSpace(r.URL.Query().Get("q"))})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to export contacts")
		return
	}
	writeCSVFile(w, http.StatusOK, file)
}

func handleExportCompanies(auth authService, exports dataExportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if exports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Export service unavailable")
		return
	}

	file, err := exports.CompaniesCSV(r.Context(), state.Organization.ID, moduleexports.CompaniesQuery{Search: strings.TrimSpace(r.URL.Query().Get("q"))})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to export clients")
		return
	}
	writeCSVFile(w, http.StatusOK, file)
}

func handleExportDeals(auth authService, exports dataExportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if exports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Export service unavailable")
		return
	}

	file, err := exports.DealsCSV(r.Context(), state.Organization.ID, moduleexports.DealsQuery{
		Search:           strings.TrimSpace(r.URL.Query().Get("q")),
		PipelineID:       parseExportInt64(r.URL.Query().Get("pipelineId")),
		StageID:          parseExportInt64(r.URL.Query().Get("stageId")),
		OwnerUserID:      parseExportInt64(r.URL.Query().Get("ownerUserId")),
		CompanyID:        parseExportInt64(r.URL.Query().Get("companyId")),
		PrimaryContactID: parseExportInt64(r.URL.Query().Get("primaryContactId")),
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to export deals")
		return
	}
	writeCSVFile(w, http.StatusOK, file)
}

func handleExportTasks(auth authService, exports dataExportsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if exports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Export service unavailable")
		return
	}

	file, err := exports.TasksCSV(r.Context(), state.Organization.ID, moduleexports.TasksQuery{
		Search:         strings.TrimSpace(r.URL.Query().Get("q")),
		Status:         strings.TrimSpace(r.URL.Query().Get("status")),
		EntityType:     strings.TrimSpace(r.URL.Query().Get("entityType")),
		EntityID:       parseExportInt64(r.URL.Query().Get("entityId")),
		DueView:        strings.TrimSpace(r.URL.Query().Get("due")),
		AssigneeFilter: strings.TrimSpace(r.URL.Query().Get("assignee")),
	})
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to export tasks")
		return
	}
	writeCSVFile(w, http.StatusOK, file)
}

func writeCSVFile(w http.ResponseWriter, status int, file moduleexports.File) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file.Filename))
	w.WriteHeader(status)
	_, _ = w.Write(file.Content)
}

func parseExportInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}
