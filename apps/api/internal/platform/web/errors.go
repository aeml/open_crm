package web

import "net/http"

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func WriteError(w http.ResponseWriter, status int, requestID string, code, message string) {
	response := ErrorResponse{}
	response.Error.Code = code
	response.Error.Message = message
	response.Meta.RequestID = requestID
	WriteJSON(w, status, response)
}

func WriteNotFound(w http.ResponseWriter, requestID string) {
	WriteError(w, http.StatusNotFound, requestID, "NOT_FOUND", "Resource not found")
}
