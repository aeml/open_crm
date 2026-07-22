package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleaudit "github.com/aeml/open_crm/apps/api/internal/modules/audit"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
	moduledashboard "github.com/aeml/open_crm/apps/api/internal/modules/dashboard"
	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	moduleleadforms "github.com/aeml/open_crm/apps/api/internal/modules/leadforms"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	moduleorgprofile "github.com/aeml/open_crm/apps/api/internal/modules/orgprofile"
	moduletasks "github.com/aeml/open_crm/apps/api/internal/modules/tasks"
	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func decodeContactRequest(w http.ResponseWriter, r *http.Request) (modulecontacts.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request contactRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return modulecontacts.CreateInput{}, false
	}
	input := modulecontacts.CreateInput{FirstName: strings.TrimSpace(request.FirstName), LastName: strings.TrimSpace(request.LastName), Email: strings.TrimSpace(request.Email), Phone: strings.TrimSpace(request.Phone), AddressLine1: strings.TrimSpace(request.AddressLine1), AddressLine2: strings.TrimSpace(request.AddressLine2), City: strings.TrimSpace(request.City), State: strings.TrimSpace(request.State), PostalCode: strings.TrimSpace(request.PostalCode), Country: strings.TrimSpace(request.Country), JobTitle: strings.TrimSpace(request.JobTitle), Status: strings.TrimSpace(request.Status), IsClient: request.IsClient, CustomFields: request.CustomFields}
	if input.FirstName == "" || input.LastName == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "First name and last name are required")
		return modulecontacts.CreateInput{}, false
	}
	return input, true
}

func decodeCompanyRequest(w http.ResponseWriter, r *http.Request) (modulecompanies.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request companyRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return modulecompanies.CreateInput{}, false
	}
	input := modulecompanies.CreateInput{Name: strings.TrimSpace(request.Name), ClientType: normalizeCompanyClientType(request.ClientType), AddressLine1: strings.TrimSpace(request.AddressLine1), AddressLine2: strings.TrimSpace(request.AddressLine2), City: strings.TrimSpace(request.City), State: strings.TrimSpace(request.State), PostalCode: strings.TrimSpace(request.PostalCode), Country: strings.TrimSpace(request.Country), Industry: strings.TrimSpace(request.Industry), Phone: strings.TrimSpace(request.Phone), Website: strings.TrimSpace(request.Website), Status: strings.TrimSpace(request.Status), LinkedContactIDs: request.LinkedContactIDs, CustomFields: request.CustomFields}
	if input.Name == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Company name is required")
		return modulecompanies.CreateInput{}, false
	}
	if input.ClientType != "organization" && input.ClientType != "individual" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Client type must be organization or individual")
		return modulecompanies.CreateInput{}, false
	}
	if input.ClientType == "individual" && len(uniquePositiveInt64s(input.LinkedContactIDs)) != 1 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Individual clients must have exactly one linked contact")
		return modulecompanies.CreateInput{}, false
	}
	return input, true
}

func normalizeCompanyClientType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "organization"
	}
	return value
}

func uniquePositiveInt64s(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func decodeTaskCreateRequest(w http.ResponseWriter, r *http.Request) (moduletasks.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request taskCreateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return moduletasks.CreateInput{}, false
	}
	input := moduletasks.CreateInput{EntityType: strings.TrimSpace(request.EntityType), EntityID: request.EntityID, Title: strings.TrimSpace(request.Title), Description: strings.TrimSpace(request.Description), Status: strings.TrimSpace(request.Status), DueAt: strings.TrimSpace(request.DueAt), AssignedToUserID: request.AssignedToUserID}
	if input.EntityType == "" || input.EntityID <= 0 || input.Title == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type, entity id, and title are required")
		return moduletasks.CreateInput{}, false
	}
	return input, true
}

func decodeNoteRequest(w http.ResponseWriter, r *http.Request) (modulenotes.CreateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request noteRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return modulenotes.CreateInput{}, false
	}
	input := modulenotes.CreateInput{EntityType: strings.TrimSpace(request.EntityType), EntityID: request.EntityID, Body: strings.TrimSpace(request.Body)}
	if input.EntityType == "" || input.EntityID <= 0 || input.Body == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Entity type, entity id, and body are required")
		return modulenotes.CreateInput{}, false
	}
	return input, true
}

func decodeTaskUpdateRequest(w http.ResponseWriter, r *http.Request) (moduletasks.UpdateInput, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	var request taskUpdateRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return moduletasks.UpdateInput{}, false
	}
	return moduletasks.UpdateInput{Title: strings.TrimSpace(request.Title), Description: strings.TrimSpace(request.Description), Status: strings.TrimSpace(request.Status), DueAt: strings.TrimSpace(request.DueAt), CompletedAt: strings.TrimSpace(request.CompletedAt), AssignedToUserID: request.AssignedToUserID}, true
}

func respondContactDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail modulecontacts.Detail) {
	response := contactDetailResponse{}
	response.Data.Contact = detail.Summary
	response.Data.Notes = detail.Notes
	response.Data.Tasks = detail.Tasks
	response.Data.Activities = detail.Activities
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondCompanyDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail modulecompanies.Detail) {
	response := companyDetailResponse{}
	response.Data.Company = detail.Summary
	response.Data.LinkedContacts = detail.LinkedContacts
	response.Data.Activities = detail.Activities
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondDealDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail moduledeals.Detail) {
	response := dealDetailResponse{}
	response.Data.Deal = detail.Summary
	response.Data.Activities = detail.Activities
	response.Data.LineItems = detail.LineItems
	response.Data.Totals = detail.Totals
	response.Data.Quotes = detail.Quotes
	response.Data.SignatureRequests = detail.SignatureRequests
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondDealQuote(w http.ResponseWriter, r *http.Request, statusCode int, quote moduledeals.QuoteVersion) {
	response := dealQuoteResponse{}
	response.Data.Quote = quote
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondNotesList(w http.ResponseWriter, r *http.Request, statusCode int, notes []modulenotes.Entry) {
	response := notesListResponse{}
	response.Data.Notes = notes
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondNoteDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail modulenotes.CreateResult) {
	response := noteDetailResponse{}
	response.Data.Note = detail.Note
	response.Data.Activity = detail.Activity
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondTaskDetail(w http.ResponseWriter, r *http.Request, statusCode int, detail moduletasks.Detail) {
	response := taskDetailResponse{}
	response.Data.Task = detail.Task
	response.Data.Activities = detail.Activities
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondDashboardSummary(w http.ResponseWriter, r *http.Request, statusCode int, summary moduledashboard.Summary) {
	response := dashboardSummaryResponse{Data: summary}
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondOrganizationProfile(w http.ResponseWriter, r *http.Request, statusCode int, detail moduleorgprofile.Detail) {
	response := organizationProfileResponse{}
	response.Data.Profile = detail
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func parsePathInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	return platformweb.ParsePathInt64(w, r, requestID, name)
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseCoreListPagination(w http.ResponseWriter, r *http.Request) (platformpagination.Page, bool) {
	page, err := platformpagination.Parse(r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"), 20)
	if err != nil {
		platformweb.WriteError(
			w,
			http.StatusBadRequest,
			platformweb.RequestIDFromContext(r.Context()),
			"BAD_REQUEST",
			"Page size must be between 1 and 100, and page offset must be at most 50,000 records",
		)
		return platformpagination.Page{}, false
	}
	return page, true
}

func parseQueryInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func decodeJSONRequest(w http.ResponseWriter, r *http.Request, requestID string, dst any) bool {
	return platformweb.DecodeJSONRequest(w, r, requestID, dst, maxJSONBodyBytes)
}

func writeResourceNotFound(w http.ResponseWriter, requestID string, err error) bool {
	if !isResourceNotFound(err) {
		return false
	}
	platformweb.WriteNotFound(w, requestID)
	return true
}

func isResourceNotFound(err error) bool {
	return errors.Is(err, modulecontacts.ErrNotFound) || errors.Is(err, modulecompanies.ErrNotFound) || errors.Is(err, moduledashboard.ErrNotFound) || errors.Is(err, moduledeals.ErrNotFound) || errors.Is(err, moduleleadforms.ErrNotFound) || errors.Is(err, moduletasks.ErrNotFound)
}

func recordAuditEvent(r *http.Request, audit auditService, organizationID int64, input moduleaudit.RecordInput) {
	if audit == nil {
		return
	}
	_ = audit.Record(r.Context(), organizationID, input)
}

func respondStatus(w http.ResponseWriter, r *http.Request, statusCode int, status string) {
	response := statusResponse{}
	response.Data.Status = status
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func setSessionCookie(w http.ResponseWriter, env config.Env, token string) {
	// #nosec G124 -- production cookies are Secure; local HTTP development intentionally derives false from the environment.
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isProduction(env), MaxAge: int(sessionCookieTTL / time.Second), Expires: time.Now().Add(sessionCookieTTL)})
}

func clearSessionCookie(w http.ResponseWriter, env config.Env) {
	// #nosec G124 -- the deletion cookie must use the same environment-derived Secure attribute as the cookie being cleared.
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isProduction(env), MaxAge: -1, Expires: time.Unix(0, 0)})
}

func readSessionCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return cookie.Value, true
}
