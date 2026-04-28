package web

import (
	"net/http"
	"strconv"
	"strings"
)

func ParsePathInt64(w http.ResponseWriter, r *http.Request, requestID string, name string) (int64, bool) {
	value := strings.TrimSpace(r.PathValue(name))
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid resource id")
		return 0, false
	}
	return parsed, true
}

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details,omitempty"`
	} `json:"error"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func WriteError(w http.ResponseWriter, status int, requestID string, code, message string) {
	WriteErrorWithDetails(w, status, requestID, code, message, nil)
}

func WriteErrorWithDetails(w http.ResponseWriter, status int, requestID string, code, message string, details any) {
	response := ErrorResponse{}
	response.Error.Code = code
	response.Error.Message = message
	response.Error.Details = details
	response.Meta.RequestID = requestID
	WriteJSON(w, status, response)
}

func WriteNotFound(w http.ResponseWriter, requestID string) {
	WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Resource not found")
}
