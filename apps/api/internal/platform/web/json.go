package web

import (
	"encoding/json"
	"errors"
	"net/http"
)

var ErrRequestBodyTooLarge = errors.New("request body too large")

func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	if maxBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	}

	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return ErrRequestBodyTooLarge
		}
		return err
	}

	return nil
}

func DecodeJSONRequest(w http.ResponseWriter, r *http.Request, requestID string, dst any, maxBytes int64) bool {
	if err := DecodeJSONBody(w, r, dst, maxBytes); err != nil {
		if errors.Is(err, ErrRequestBodyTooLarge) {
			WriteError(w, http.StatusRequestEntityTooLarge, requestID, "REQUEST_BODY_TOO_LARGE", "Request body is too large")
			return false
		}
		WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return false
	}
	return true
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
