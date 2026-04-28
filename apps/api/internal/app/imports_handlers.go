package app

import (
	"errors"
	"net/http"
	"strings"

	moduleimports "github.com/aeml/open_crm/apps/api/internal/modules/imports"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handlePreviewImport(auth authService, imports importsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	_, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if imports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Import service unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImportBodyBytes)
	if err := r.ParseMultipartForm(maxImportBodyBytes); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			platformweb.WriteError(w, http.StatusRequestEntityTooLarge, requestID, "REQUEST_BODY_TOO_LARGE", "Import preview file is too large")
			return
		}
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Import preview requires multipart form data")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "CSV file is required")
		return
	}
	defer file.Close()

	result, err := imports.Preview(r.Context(), moduleimports.PreviewInput{EntityType: strings.TrimSpace(r.FormValue("entityType")), Reader: file})
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
		return
	}

	response := importPreviewResponse{Data: result}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}
