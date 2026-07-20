package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	moduleimports "github.com/aeml/open_crm/apps/api/internal/modules/imports"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type importPreviewResponse struct {
	Data moduleimports.PreviewResult `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type importBatchResponse struct {
	Data struct {
		Batch moduleimports.Batch `json:"batch"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type importBatchesResponse struct {
	Data struct {
		Batches []moduleimports.Batch `json:"batches"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type importUpload struct {
	EntityType     string
	IdempotencyKey string
	OriginalName   string
	Mapping        map[string]string
	File           multipart.File
}

func handlePreviewImport(auth authService, imports importsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if imports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Import service unavailable")
		return
	}
	upload, ok := parseImportUpload(w, r, requestID, false)
	if !ok {
		return
	}
	defer closeImportUpload(r, upload)
	result, err := imports.Preview(r.Context(), moduleimports.PreviewInput{OrganizationID: state.Organization.ID, EntityType: upload.EntityType, Reader: upload.File, Mapping: upload.Mapping})
	if err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
		return
	}
	response := importPreviewResponse{Data: result}
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleExecuteImport(auth authService, imports importsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if imports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Import service unavailable")
		return
	}
	upload, ok := parseImportUpload(w, r, requestID, true)
	if !ok {
		return
	}
	defer closeImportUpload(r, upload)
	batch, err := imports.Execute(r.Context(), moduleimports.ExecuteInput{
		OrganizationID: state.Organization.ID,
		ActorUserID:    state.User.ID,
		EntityType:     upload.EntityType,
		OriginalName:   upload.OriginalName,
		IdempotencyKey: upload.IdempotencyKey,
		Reader:         upload.File,
		Mapping:        upload.Mapping,
	})
	if writeImportServiceError(w, requestID, err) {
		return
	}
	response := importBatchResponse{}
	response.Data.Batch = batch
	response.Meta.RequestID = requestID
	status := http.StatusCreated
	if batch.Replayed {
		status = http.StatusOK
	}
	platformweb.WriteJSON(w, status, response)
}

func handleListImports(auth authService, imports importsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if imports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Import service unavailable")
		return
	}
	batches, err := imports.List(r.Context(), state.Organization.ID, parsePositiveInt(r.URL.Query().Get("limit"), 50))
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load import history")
		return
	}
	response := importBatchesResponse{}
	response.Data.Batches = batches
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleRollbackImport(auth authService, imports importsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if imports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Import service unavailable")
		return
	}
	batchID, ok := parseImportBatchID(w, r, requestID)
	if !ok {
		return
	}
	batch, err := imports.Rollback(r.Context(), state.Organization.ID, state.User.ID, batchID)
	if writeImportServiceError(w, requestID, err) {
		return
	}
	response := importBatchResponse{}
	response.Data.Batch = batch
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleImportErrorsCSV(auth authService, imports importsService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgAdmin(auth, w, r)
	if !ok {
		return
	}
	if imports == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Import service unavailable")
		return
	}
	batchID, ok := parseImportBatchID(w, r, requestID)
	if !ok {
		return
	}
	file, err := imports.ErrorCSV(r.Context(), state.Organization.ID, batchID)
	if writeImportServiceError(w, requestID, err) {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file.Filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Content)
}

func parseImportUpload(w http.ResponseWriter, r *http.Request, requestID string, requireIdempotency bool) (importUpload, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBodyBytes)
	if err := r.ParseMultipartForm(maxImportBodyBytes); err != nil {
		removeImportForm(r)
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			platformweb.WriteError(w, http.StatusRequestEntityTooLarge, requestID, "REQUEST_BODY_TOO_LARGE", "Import file is too large")
			return importUpload{}, false
		}
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Import requires multipart form data")
		return importUpload{}, false
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		removeImportForm(r)
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "CSV file is required")
		return importUpload{}, false
	}
	mapping := map[string]string{}
	if rawMapping := strings.TrimSpace(r.FormValue("mapping")); rawMapping != "" {
		if err := json.Unmarshal([]byte(rawMapping), &mapping); err != nil {
			file.Close()
			removeImportForm(r)
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Mapping must be a JSON object of CRM fields to CSV columns")
			return importUpload{}, false
		}
	}
	upload := importUpload{
		EntityType: strings.TrimSpace(r.FormValue("entityType")), IdempotencyKey: strings.TrimSpace(r.FormValue("idempotencyKey")),
		OriginalName: header.Filename, Mapping: mapping, File: file,
	}
	if requireIdempotency && (len(upload.IdempotencyKey) < 8 || len(upload.IdempotencyKey) > 200) {
		file.Close()
		removeImportForm(r)
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Idempotency key must be 8-200 characters")
		return importUpload{}, false
	}
	return upload, true
}

func closeImportUpload(r *http.Request, upload importUpload) {
	_ = upload.File.Close()
	removeImportForm(r)
}

func removeImportForm(r *http.Request) {
	if r != nil && r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}

func parseImportBatchID(w http.ResponseWriter, r *http.Request, requestID string) (int64, bool) {
	batchID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("batchID")), 10, 64)
	if err != nil || batchID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a valid import batch id")
		return 0, false
	}
	return batchID, true
}

func writeImportServiceError(w http.ResponseWriter, requestID string, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case writeCapacityError(w, requestID, "contacts", err):
	case errors.Is(err, moduleimports.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", err.Error())
	case errors.Is(err, moduleimports.ErrNotFound):
		platformweb.WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Import batch not found")
	case errors.Is(err, moduleimports.ErrConflict), errors.Is(err, moduleimports.ErrIdempotencyConflict):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", err.Error())
	case errors.Is(err, moduleimports.ErrInactiveActor):
		platformweb.WriteError(w, http.StatusForbidden, requestID, "FORBIDDEN", "Your organization access is no longer active")
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to complete import operation")
	}
	return true
}
