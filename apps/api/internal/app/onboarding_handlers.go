package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleonboarding "github.com/aeml/open_crm/apps/api/internal/modules/onboarding"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func handleBootstrap(service onboardingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Onboarding service unavailable")
		return
	}

	var request bootstrapRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}

	result, err := service.BootstrapOrganization(r.Context(), moduleonboarding.BootstrapInput{
		OrganizationName: strings.TrimSpace(request.OrganizationName),
		BusinessType:     strings.TrimSpace(request.BusinessType),
		FirstName:        strings.TrimSpace(request.FirstName),
		LastName:         strings.TrimSpace(request.LastName),
		Email:            strings.TrimSpace(request.Email),
		Password:         normalizePassword(request.Password),
		IdempotencyKey:   strings.TrimSpace(request.IdempotencyKey),
	})
	if err != nil {
		switch {
		case errors.Is(err, moduleonboarding.ErrInvalidInput):
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Complete every field, use a password of at least 12 characters, and retry")
		case errors.Is(err, moduleonboarding.ErrIdempotencyConflict):
			platformweb.WriteError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "This signup retry key was already used with different details")
		case errors.Is(err, moduleonboarding.ErrAccountExists):
			platformweb.WriteError(w, http.StatusConflict, requestID, "ACCOUNT_EXISTS", "An account with this email already exists; sign in or request another verification email")
		case errors.Is(err, moduleonboarding.ErrAlreadyVerified):
			platformweb.WriteError(w, http.StatusConflict, requestID, "EMAIL_ALREADY_VERIFIED", "This workspace is already verified; sign in to continue")
		case errors.Is(err, moduleonboarding.ErrVerificationDelivery):
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "VERIFICATION_DELIVERY_FAILED", "The workspace was created, but the verification email could not be sent; retry this form safely")
		default:
			platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to create workspace")
		}
		return
	}

	response := struct {
		Data moduleonboarding.BootstrapResult `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
		} `json:"meta"`
	}{Data: result}
	response.Meta.RequestID = requestID
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	platformweb.WriteJSON(w, status, response)
}

func handleVerifyEmail(env config.Env, service onboardingService, billing billingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workspace verification unavailable")
		return
	}
	var request verifyEmailRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := service.VerifyEmail(r.Context(), strings.TrimSpace(request.Token))
	if err != nil {
		if errors.Is(err, moduleonboarding.ErrInvalidVerificationToken) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "INVALID_VERIFICATION_TOKEN", "This verification link is invalid or expired; request a new email")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to verify workspace email")
		return
	}
	setSessionCookie(w, env, result.SessionToken)
	respondSession(w, r, http.StatusOK, result.State, billing)
}

func handleResendVerification(service onboardingService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	if service == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Workspace verification unavailable")
		return
	}
	var request resendVerificationRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	result, err := service.ResendVerification(r.Context(), strings.TrimSpace(request.Email))
	if err != nil {
		if errors.Is(err, moduleonboarding.ErrInvalidInput) {
			platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Email is required")
			return
		}
		if errors.Is(err, moduleonboarding.ErrVerificationDelivery) {
			platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "VERIFICATION_DELIVERY_FAILED", "Unable to send a verification email; retry in a moment")
			return
		}
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to request verification email")
		return
	}
	response := struct {
		Data struct {
			Accepted         bool   `json:"accepted"`
			VerificationLink string `json:"verificationLink,omitempty"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
		} `json:"meta"`
	}{}
	response.Data.Accepted = true
	response.Data.VerificationLink = result.VerificationLink
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusAccepted, response)
}
