package app

import (
	"net/http"
	"slices"
	"strings"

	"github.com/aeml/open_crm/apps/api/internal/config"
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

func NewServer(env config.Env) http.Handler {
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

	handler := platformweb.RequestID(mux)
	return withCORS(env, handler)
}

func withCORS(env config.Env, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if isAllowedOrigin(origin, env.AllowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Add("Vary", "Origin")
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
		}

		if r.Method == http.MethodOptions {
			if isAllowedOrigin(origin, env.AllowedOrigins) {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			w.WriteHeader(http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isAllowedOrigin(origin string, allowedOrigins []string) bool {
	if origin == "" || len(allowedOrigins) == 0 {
		return false
	}

	return slices.Contains(allowedOrigins, origin)
}
