package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/webassets"
)

// RouterConfig holds configuration and dependency providers for the HTTP router.
type RouterConfig struct {
	AdminToken                 string // Plaintext token fallback for unit tests; TokenHolder is preferred
	TokenHolder                AdminTokenHolder
	SettingsRepository         domain.SettingsRepository
	DNSSettingsRepository      domain.DNSSettingsRepository
	EnvironmentFetchProxy      string
	ReadinessChecker           ReadinessCheckerFunc
	PublicationTokenValidator  PublicationTokenValidatorFunc
	SessionValidator           SessionValidatorFunc
	SessionStore               SessionStore
	IsPublicationToken         func(ctx context.Context, token string) bool
	SubscriptionService        *subscription.Service
	InventoryService           *inventory.Service
	ProbeService               *probe.Service
	PolicyService              *policy.Service
	RevisionService            *revision.Service
	PublicationService         *publication.Service
	IPRiskService              *iprisk.Service
	RiskPolicyRepository       domain.RiskPolicyRevisionRepository
	RiskBindingRepository      domain.RiskPolicyGroupBindingRepository
	ProbeRunRepository         domain.ProbeRunRepository
	ProbeObservationRepository domain.ProbeObservationRepository
	AuditRepository            domain.AuditRepository
	WebHandler                 http.Handler
}

// NewRouter constructs and configures the top-level Chi HTTP router.
func NewRouter(cfg RouterConfig) http.Handler {
	if cfg.PolicyService != nil && cfg.PublicationService != nil {
		cfg.PolicyService.SetSnapshotProvider(cfg.PublicationService.ResolvePolicySnapshot)
		cfg.PolicyService.SetInputProvider(cfg.PublicationService.AssembleResolveInput)
	}
	if cfg.TokenHolder == nil {
		cost := MinHashCost
		if cfg.SettingsRepository != nil {
			cost = DefaultHashCost
		}
		cfg.TokenHolder = NewDynamicTokenHolder(TokenHolderConfig{
			InitialToken: cfg.AdminToken,
			SettingsRepo: cfg.SettingsRepository,
			SessionStore: cfg.SessionStore,
			HashCost:     cost,
		})
	}

	r := chi.NewRouter()

	// Top-level middleware
	r.Use(Recoverer)
	r.Use(RequestID)

	// Liveness and Readiness Probes
	r.Get("/healthz", HealthzHandler())
	r.Get("/readyz", ReadyzHandler(cfg.ReadinessChecker))

	// Hard 410 Gone legacy interceptors
	r.HandleFunc("/yaml", Legacy410Handler)
	r.HandleFunc("/yaml/*", Legacy410Handler)
	r.HandleFunc("/script", Legacy410Handler)
	r.HandleFunc("/script/*", Legacy410Handler)

	// Direct human-readable client subscription export endpoints & historical endpoints
	if cfg.PublicationService != nil {
		directMihomo := directSubscriptionHandler(cfg.PublicationService, cfg.TokenHolder, domain.TargetMihomo)
		directSingBox := directSubscriptionHandler(cfg.PublicationService, cfg.TokenHolder, domain.TargetSingBox)
		directSurge := directSubscriptionHandler(cfg.PublicationService, cfg.TokenHolder, domain.TargetSurge)
		directQX := directSubscriptionHandler(cfg.PublicationService, cfg.TokenHolder, domain.TargetQuantumultX)
		directGeneric := directSubscriptionHandler(cfg.PublicationService, cfg.TokenHolder, "")

		r.Get("/clash", directMihomo)
		r.Head("/clash", directMihomo)
		r.Get("/mihomo", directMihomo)
		r.Head("/mihomo", directMihomo)
		r.Get("/singbox", directSingBox)
		r.Head("/singbox", directSingBox)
		r.Get("/surge", directSurge)
		r.Head("/surge", directSurge)
		r.Get("/qx", directQX)
		r.Head("/qx", directQX)
		r.Get("/quantumult-x", directQX)
		r.Head("/quantumult-x", directQX)
		r.Get("/sub/{target}", directGeneric)
		r.Head("/sub/{target}", directGeneric)

		// Historical endpoints
		client := publicationClientHandler(cfg.PublicationService, cfg.TokenHolder)
		r.Get("/publish/v1/{publication_id}", client)
		r.Head("/publish/v1/{publication_id}", client)
		r.Get("/p/{publication_id}", client)
		r.Head("/p/{publication_id}", client)
		r.Get("/subscribe/v1/{id}", liveSubscriptionClientHandler(cfg.PublicationService))
		r.Head("/subscribe/v1/{id}", liveSubscriptionClientHandler(cfg.PublicationService))
	}

	webHandler := cfg.WebHandler
	if webHandler == nil {
		webHandler, _ = webassets.Handler()
	}
	if webHandler != nil {
		r.Get("/", webHandler.ServeHTTP)
		r.Get("/*", webHandler.ServeHTTP)
		r.Head("/", webHandler.ServeHTTP)
		r.Head("/*", webHandler.ServeHTTP)
	}

	// Public auth endpoints
	r.Get("/api/v1/auth/status", authStatusHandler(cfg))
	r.Post("/api/v1/auth/login", authLoginHandler(cfg))
	r.Post("/api/v1/auth/logout", authLogoutHandler(cfg))

	// Admin API v1 Control Plane
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(AdminAuthMiddleware(cfg))
		api.Use(CSRFMiddleware(cfg))

		if cfg.SubscriptionService != nil {
			registerSubscriptionRoutes(api, cfg.SubscriptionService, cfg.InventoryService)
		}
		if cfg.InventoryService != nil {
			registerNodeRoutes(api, cfg.InventoryService)
		}
		if cfg.ProbeService != nil {
			registerProbeRoutes(api, cfg.ProbeService, cfg.ProbeRunRepository, cfg.ProbeObservationRepository, cfg.AuditRepository)
		}
		if cfg.PolicyService != nil {
			registerPolicyRoutes(api, cfg.PolicyService, cfg.AuditRepository)
		}
		if cfg.RevisionService != nil {
			registerRevisionRoutes(api, cfg.RevisionService)
		}
		if cfg.PublicationService != nil {
			registerPublicationRoutes(api, cfg.PublicationService, cfg.AuditRepository)
			registerSubscriptionLinkRoutes(api, cfg.PublicationService, cfg.SettingsRepository, cfg.AuditRepository)
		}
		registerIPRiskRoutes(api, cfg)
		registerSettingsRoutes(api, cfg)

		api.NotFound(func(w http.ResponseWriter, r *http.Request) {
			WriteError(w, r, http.StatusNotFound, "not_found", "API endpoint not found")
		})
	})

	// Global 404 handler with unified error envelope
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, "not_found", "Endpoint not found")
	})

	return r
}

const (
	// LegacyEndpointRemovedCode is the error code returned for deprecated Python-era routes.
	LegacyEndpointRemovedCode = "legacy_endpoint_removed"
	// LegacyEndpointRemovedMessage provides migration guidance for removed legacy routes.
	LegacyEndpointRemovedMessage = "This endpoint has been permanently removed in CSP 1.0. Please use the versioned API at /api/v1/."
)

// Legacy410Handler returns a hard HTTP 410 Gone for legacy endpoints with unified error payload.
func Legacy410Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Del("Location")
	WriteError(w, r, http.StatusGone, LegacyEndpointRemovedCode, LegacyEndpointRemovedMessage)
}

// authStatusHandler serves GET /api/v1/auth/status
func authStatusHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := cfg.SecurityMode()
		exportMode := SecurityModeProtected
		if h, ok := cfg.TokenHolder.(interface{ IsExportAuthRequired() bool }); ok && !h.IsExportAuthRequired() {
			exportMode = SecurityModeOpen
		}
		if mode == SecurityModeOpen {
			WriteSuccess(w, r, http.StatusOK, AuthStatusData{
				Mode:          SecurityModeOpen,
				AdminMode:     mode,
				ExportMode:    exportMode,
				Authenticated: true,
				Subject:       "admin",
			})
			return
		}

		// Protected mode: evaluate caller credentials
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		var token string
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
				token = strings.TrimSpace(parts[1])
			}
		}

		if token != "" {
			valid := false
			if cfg.TokenHolder != nil {
				valid = cfg.TokenHolder.Verify(token)
			} else if cfg.AdminToken != "" {
				valid = subtle.ConstantTimeCompare([]byte(token), []byte(cfg.AdminToken)) == 1
			}
			if valid {
				WriteSuccess(w, r, http.StatusOK, AuthStatusData{
					Mode:          SecurityModeProtected,
					AdminMode:     mode,
					ExportMode:    exportMode,
					Authenticated: true,
					Subject:       "admin",
				})
				return
			}
		}

		if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
			var session *SessionInfo
			var ok bool
			if cfg.SessionValidator != nil {
				session, ok = cfg.SessionValidator(cookie.Value)
			}
			if !ok && cfg.SessionStore != nil {
				session, ok = cfg.SessionStore.Get(cookie.Value)
			}
			if ok && session != nil {
				WriteSuccess(w, r, http.StatusOK, AuthStatusData{
					Mode:          SecurityModeProtected,
					AdminMode:     mode,
					ExportMode:    exportMode,
					Authenticated: true,
					Subject:       session.Subject,
				})
				return
			}
		}

		WriteSuccess(w, r, http.StatusOK, AuthStatusData{
			Mode:          SecurityModeProtected,
			AdminMode:     mode,
			ExportMode:    exportMode,
			Authenticated: false,
			Subject:       "",
		})
	}
}

// authLoginHandler serves POST /api/v1/auth/login
func authLoginHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := cfg.SecurityMode()
		if mode == SecurityModeOpen {
			WriteSuccess(w, r, http.StatusOK, LoginResponseData{
				Mode:          SecurityModeOpen,
				Authenticated: true,
				Subject:       "admin",
				Message:       "Server is running in open mode (no authentication required)",
			})
			return
		}

		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid login request payload")
			return
		}

		trimmedToken := strings.TrimSpace(req.Token)
		valid := false
		if cfg.TokenHolder != nil {
			valid = cfg.TokenHolder.Verify(trimmedToken)
		} else if cfg.AdminToken != "" {
			valid = subtle.ConstantTimeCompare([]byte(trimmedToken), []byte(cfg.AdminToken)) == 1
		}
		if trimmedToken == "" || !valid {
			WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid authentication credentials")
			return
		}

		var sessionID, csrfToken string
		if cfg.SessionStore != nil {
			if session, err := cfg.SessionStore.Create("admin"); err == nil && session != nil {
				sessionID = session.SessionID
				csrfToken = session.CSRFToken
				http.SetCookie(w, &http.Cookie{
					Name:     SessionCookieName,
					Value:    sessionID,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					MaxAge:   86400,
				})
			}
		}

		WriteSuccess(w, r, http.StatusOK, LoginResponseData{
			Mode:          SecurityModeProtected,
			Authenticated: true,
			Subject:       "admin",
			CSRFToken:     csrfToken,
			Message:       "Authentication successful",
		})
	}
}

// authLogoutHandler serves POST /api/v1/auth/logout
func authLogoutHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
			if cfg.SessionStore != nil {
				cfg.SessionStore.Revoke(cookie.Value)
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		WriteSuccess(w, r, http.StatusOK, map[string]any{
			"message": "Logged out successfully",
		})
	}
}
