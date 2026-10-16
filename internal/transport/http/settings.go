package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	r.Get("/settings/auth", settingsGetAuthHandler(cfg))
	r.Put("/settings/auth", settingsPutAuthHandler(cfg))
	r.Get("/settings/subscription-fetch", settingsGetSubscriptionFetchHandler(cfg))
	r.Put("/settings/subscription-fetch", settingsPutSubscriptionFetchHandler(cfg))
	r.Get("/settings/public-base-url", settingsGetPublicBaseURLHandler(cfg))
	r.Put("/settings/public-base-url", settingsPutPublicBaseURLHandler(cfg))
}

type authSettingsResponse struct {
	TokenConfigured   bool         `json:"token_configured"`
	AdminAuthEnabled  bool         `json:"admin_auth_enabled"`
	ExportAuthEnabled bool         `json:"export_auth_enabled"`
	AdminMode         SecurityMode `json:"admin_mode"`
	ExportMode        SecurityMode `json:"export_mode"`
}

type authSettingsRequest struct {
	Token             string `json:"token"`
	ClearToken        bool   `json:"clear_token"`
	AdminAuthEnabled  *bool  `json:"admin_auth_enabled"`
	ExportAuthEnabled *bool  `json:"export_auth_enabled"`
}

func settingsAuthResponse(cfg RouterConfig) authSettingsResponse {
	response := authSettingsResponse{AdminAuthEnabled: true, ExportAuthEnabled: true, ExportMode: SecurityModeProtected}
	if holder, ok := cfg.TokenHolder.(*DynamicTokenHolder); ok {
		holder.mu.RLock()
		response.AdminAuthEnabled = holder.adminAuthEnabled
		response.ExportAuthEnabled = holder.exportAuthEnabled
		response.TokenConfigured = holder.verifier != ""
		if !holder.exportAuthEnabled {
			response.ExportMode = SecurityModeOpen
		}
		if holder.adminAuthEnabled && holder.verifier != "" {
			response.AdminMode = SecurityModeProtected
		} else {
			response.AdminMode = SecurityModeOpen
		}
		holder.mu.RUnlock()
		return response
	}
	response.AdminMode = cfg.SecurityMode()
	response.TokenConfigured = response.AdminMode == SecurityModeProtected
	return response
}

func settingsGetAuthHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteSuccess(w, r, http.StatusOK, settingsAuthResponse(cfg))
	}
}

func settingsPutAuthHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		holder, ok := cfg.TokenHolder.(*DynamicTokenHolder)
		if !ok {
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "Token holder not initialized")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		var req authSettingsRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid auth settings request")
			return
		}
		var extra struct{}
		if err := dec.Decode(&extra); err != io.EOF {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "Unexpected extra data in request payload")
			return
		}
		if req.AdminAuthEnabled == nil || req.ExportAuthEnabled == nil || (req.ClearToken && strings.TrimSpace(req.Token) != "") {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "Both auth switches are required and token cannot accompany clear_token")
			return
		}
		var token *string
		if req.ClearToken {
			empty := ""
			token = &empty
		} else if strings.TrimSpace(req.Token) != "" {
			trimmed := strings.TrimSpace(req.Token)
			token = &trimmed
		}
		if err := holder.UpdateAuthSettings(r.Context(), *req.AdminAuthEnabled, *req.ExportAuthEnabled, token); err != nil {
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to update auth settings")
			return
		}
		if cfg.AuditRepository != nil {
			_ = cfg.AuditRepository.Record(r.Context(), &domain.AuditEvent{
				ID: domain.MustNewUUIDv7(), ActorKind: domain.ActorKindAdmin,
				RequestID: GetRequestID(r.Context()), Action: "settings.auth.update",
				Result: domain.AuditResultSuccess, RedactedSummary: "Auth switches and optional system token updated",
				CreatedAt: domain.NowUTC(),
			})
		}
		WriteSuccess(w, r, http.StatusOK, settingsAuthResponse(cfg))
	}
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
		configured := cfg.TokenHolder.CurrentVerifier() != ""
		msg := "Admin token updated successfully"
		action := "settings.admin_token.update"
		summary := "Admin token updated and active sessions invalidated"
		if !configured {
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
			Configured: configured,
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
		isConfigured := cfg.TokenHolder != nil && cfg.TokenHolder.CurrentVerifier() != ""
		if cfg.TokenHolder == nil {
			isConfigured = mode == SecurityModeProtected
		}
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

type publicBaseURLRequest struct {
	PublicBaseURL string `json:"public_base_url"`
}

func settingsGetPublicBaseURLHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.SettingsRepository == nil {
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "Settings repository not configured")
			return
		}
		s, err := cfg.SettingsRepository.Get(r.Context())
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to get settings")
			return
		}
		WriteSuccess(w, r, http.StatusOK, map[string]string{
			"public_base_url": s.PublicBaseURL,
		})
	}
}

func settingsPutPublicBaseURLHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.SettingsRepository == nil {
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "Settings repository not configured")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		var req publicBaseURLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid request body")
			return
		}
		trimmed := strings.TrimSpace(req.PublicBaseURL)
		if trimmed != "" {
			trimmed = strings.TrimRight(trimmed, "/")
			u, err := url.ParseRequestURI(trimmed)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				WriteError(w, r, http.StatusUnprocessableEntity, "invalid_public_base_url", "public_base_url must be a valid absolute HTTP or HTTPS URL")
				return
			}
			if u.User != nil {
				WriteError(w, r, http.StatusUnprocessableEntity, "invalid_public_base_url", "public_base_url must not contain userinfo")
				return
			}
			if u.RawQuery != "" || strings.Contains(trimmed, "?") {
				WriteError(w, r, http.StatusUnprocessableEntity, "invalid_public_base_url", "public_base_url must not contain query parameters")
				return
			}
			if u.Fragment != "" || strings.Contains(trimmed, "#") {
				WriteError(w, r, http.StatusUnprocessableEntity, "invalid_public_base_url", "public_base_url must not contain a fragment")
				return
			}
		}
		if err := cfg.SettingsRepository.UpdatePublicBaseURL(r.Context(), trimmed); err != nil {
			var de *domain.DomainError
			if errors.As(err, &de) {
				WriteError(w, r, http.StatusUnprocessableEntity, de.Code, de.Message)
				return
			}
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to update public base URL")
			return
		}
		if cfg.AuditRepository != nil {
			_ = cfg.AuditRepository.Record(r.Context(), &domain.AuditEvent{
				ID:              domain.MustNewUUIDv7(),
				ActorKind:       domain.ActorKindAdmin,
				RequestID:       GetRequestID(r.Context()),
				Action:          "settings.public_base_url.update",
				Result:          domain.AuditResultSuccess,
				RedactedSummary: fmt.Sprintf("Updated public_base_url to %q", trimmed),
				CreatedAt:       domain.NowUTC(),
			})
		}
		WriteSuccess(w, r, http.StatusOK, map[string]string{
			"public_base_url": trimmed,
		})
	}
}
