package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/domain"
)

// AdminTokenRequest defines the exact JSON payload for POST /api/v1/settings/admin-token.
type AdminTokenRequest struct {
	Token string `json:"token"`
}

// AdminTokenResponse defines the public response payload for admin token operations.
// SECURITY: Never contains token or verifier/hash.
type AdminTokenResponse struct {
	Mode       SecurityMode `json:"mode"`
	Configured bool         `json:"configured"`
	Message    string       `json:"message"`
}

// registerSettingsRoutes registers administrative settings endpoints on the Chi router.
func registerSettingsRoutes(r chi.Router, cfg RouterConfig) {
	r.Post("/settings/admin-token", settingsUpdateAdminTokenHandler(cfg))
	r.Get("/settings/admin-token", settingsGetAdminTokenStatusHandler(cfg))
}

// settingsUpdateAdminTokenHandler handles POST /api/v1/settings/admin-token.
func settingsUpdateAdminTokenHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.TokenHolder == nil {
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "Token holder not initialized")
			return
		}

		// Enforce bounded body read to protect against memory exhaustion
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)

		var req AdminTokenRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid request payload: must be JSON object with token string")
			return
		}

		// Ensure no unexpected trailing data
		var extra struct{}
		if err := dec.Decode(&extra); err != io.EOF {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "Unexpected extra data in request payload")
			return
		}

		trimmed := strings.TrimSpace(req.Token)
		if err := cfg.TokenHolder.UpdateToken(r.Context(), trimmed); err != nil {
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to update admin token")
			return
		}

		newMode := cfg.TokenHolder.Mode()
		msg := "Admin token updated successfully"
		action := "settings.admin_token.update"
		summary := "Admin token updated and active sessions invalidated"
		if newMode == SecurityModeOpen {
			msg = "Admin token cleared; server running in open mode"
			action = "settings.admin_token.clear"
			summary = "Admin token cleared and server returned to open mode"
		}

		// Record audit event without exposing secrets
		if cfg.AuditRepository != nil {
			actorKind := ActorKindAdmin
			if auth := GetAuthContext(r.Context()); auth != nil {
				actorKind = auth.ActorKind
			}
			_ = cfg.AuditRepository.Record(r.Context(), &domain.AuditEvent{
				ID:              domain.MustNewUUIDv7(),
				ActorKind:       domain.ActorKind(actorKind),
				RequestID:       GetRequestID(r.Context()),
				Action:          action,
				Result:          domain.AuditResultSuccess,
				RedactedSummary: summary,
				CreatedAt:       domain.NowUTC(),
			})
		}

		WriteSuccess(w, r, http.StatusOK, AdminTokenResponse{
			Mode:       newMode,
			Configured: newMode == SecurityModeProtected,
			Message:    msg,
		})
	}
}

// settingsGetAdminTokenStatusHandler handles GET /api/v1/settings/admin-token.
func settingsGetAdminTokenStatusHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := cfg.SecurityMode()
		if cfg.TokenHolder != nil {
			mode = cfg.TokenHolder.Mode()
		}
		isConfigured := mode == SecurityModeProtected
		msg := "Server running in protected mode"
		if mode == SecurityModeOpen {
			msg = "Server running in open mode"
		}
		WriteSuccess(w, r, http.StatusOK, AdminTokenResponse{
			Mode:       mode,
			Configured: isConfigured,
			Message:    msg,
		})
	}
}
