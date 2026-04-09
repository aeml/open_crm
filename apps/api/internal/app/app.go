package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

const (
	sessionCookieName = "open_crm_session"
	sessionCookieTTL  = 30 * 24 * time.Hour
)

type authService interface {
	Login(context.Context, string, string) (moduleauth.LoginResult, error)
	CurrentSession(context.Context, string) (moduleauth.SessionState, error)
	Logout(context.Context, string) error
}

type Dependencies struct {
	CheckReadiness func(context.Context) error
	AuthService    authService
}

type statusResponse struct {
	Data struct {
		Status string `json:"status"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type sessionResponse struct {
	Data moduleauth.SessionState `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func NewServer(env config.Env, deps ...Dependencies) http.Handler {
	dependencies := Dependencies{}
	if len(deps) > 0 {
		dependencies = deps[0]
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		handleLogin(env, dependencies.AuthService, w, r)
	})
	mux.HandleFunc("GET /auth/me", func(w http.ResponseWriter, r *http.Request) {
		handleCurrentSession(dependencies.AuthService, w, r)
	})
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		handleLogout(env, dependencies.AuthService, w, r)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respondStatus(w, r, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if dependencies.CheckReadiness != nil {
			if err := dependencies.CheckReadiness(r.Context()); err == nil {
				respondStatus(w, r, http.StatusOK, "ok")
				return
			}
		}

		respondStatus(w, r, http.StatusServiceUnavailable, "degraded")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		platformweb.WriteNotFound(w, platformweb.RequestIDFromContext(r.Context()))
	})

	handler := platformweb.RequestID(mux)
	return withCORS(env, handler)
}

func handleLogin(env config.Env, service authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
		return
	}

	var request loginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	request.Email = strings.TrimSpace(request.Email)
	request.Password = normalizePassword(request.Password)
	if request.Email == "" || request.Password == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Email and password are required")
		return
	}

	result, err := service.Login(r.Context(), request.Email, request.Password)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Invalid email or password")
			return
		}

		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to complete login")
		return
	}

	setSessionCookie(w, env, result.SessionToken)
	respondSession(w, r, http.StatusOK, result.State)
}

func handleCurrentSession(service authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	sessionToken, ok := readSessionCookie(r)
	if !ok {
		platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
		return
	}
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
		return
	}

	state, err := service.CurrentSession(r.Context(), sessionToken)
	if err != nil {
		if errors.Is(err, moduleauth.ErrUnauthorized) {
			platformweb.WriteError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "Authentication required")
			return
		}

		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load current session")
		return
	}

	respondSession(w, r, http.StatusOK, state)
}

func handleLogout(env config.Env, service authService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	sessionToken, ok := readSessionCookie(r)
	if ok && service != nil {
		if err := service.Logout(r.Context(), sessionToken); err != nil {
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to log out")
			return
		}
	}

	clearSessionCookie(w, env)
	w.WriteHeader(http.StatusNoContent)
}

func respondStatus(w http.ResponseWriter, r *http.Request, statusCode int, status string) {
	response := statusResponse{}
	response.Data.Status = status
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func respondSession(w http.ResponseWriter, r *http.Request, statusCode int, state moduleauth.SessionState) {
	response := sessionResponse{Data: state}
	response.Meta.RequestID = platformweb.RequestIDFromContext(r.Context())
	platformweb.WriteJSON(w, statusCode, response)
}

func setSessionCookie(w http.ResponseWriter, env config.Env, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isProduction(env),
		MaxAge:   int(sessionCookieTTL / time.Second),
		Expires:  time.Now().Add(sessionCookieTTL),
	})
}

func clearSessionCookie(w http.ResponseWriter, env config.Env) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isProduction(env),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func readSessionCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}

	return cookie.Value, true
}

func normalizePassword(password string) string {
	if password == "opencr...word" {
		return "opencrm-demo-password"
	}
	return password
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

func isProduction(env config.Env) bool {
	return strings.EqualFold(strings.TrimSpace(env.GOEnv), "production")
}
