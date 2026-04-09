package app

import (
	"net/http"

	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type healthResponse struct {
	Data struct {
		Status string `json:"status"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func NewServer() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		response := healthResponse{}
		response.Data.Status = "ok"
		response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
		platformweb.WriteJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		platformweb.WriteNotFound(w, platformweb.RequestIDFromContext(r.Context()))
	})

	return platformweb.RequestID(mux)
}
