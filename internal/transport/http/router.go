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

	webHandler := cfg.WebHandler
	if webHandler == nil {
		webHandler, _ = webassets.Handler()
	}
	if webHandler != nil {
		r.Get("/", webHandler.ServeHTTP)
		r.Get("/*", webHandler.ServeHTTP)
	}

	// Hard 410 Gone legacy interceptors
	r.HandleFunc("/yaml", Legacy410Handler)
	r.HandleFunc("/yaml/*", Legacy410Handler)
	r.HandleFunc("/script", Legacy410Handler)
	r.HandleFunc("/script/*", Legacy410Handler)

	// Publication endpoint: /publish/v1/{publication_id}
	r.Get("/publish/v1/{publication_id}", publicationHandler(cfg))

	// Public auth endpoints
	r.Get("/api/v1/auth/status", authStatusHandler(cfg))
	r.Post("/api/v1/auth/login", authLoginHandler(cfg))
	r.Post("/api/v1/auth/logout", authLogoutHandler(cfg))

	// Admin API v1 Control Plane
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(AdminAuthMiddleware(cfg))
		api.Use(CSRFMiddleware(cfg))

		if cfg.SubscriptionService != nil {
			registerSubscriptionRoutes(api, cfg.SubscriptionService)
		} else {
			registerSubscriptionSkeleton(api)
		}
		if cfg.InventoryService != nil {
			registerNodeRoutes(api, cfg.InventoryService)
		} else {
			registerNodeSkeleton(api)
		}
		if cfg.ProbeService != nil {
			registerProbeRoutes(api, cfg.ProbeService, cfg.ProbeRunRepository, cfg.ProbeObservationRepository, cfg.AuditRepository)
		}
		if cfg.PolicyService != nil {
			registerPolicyRoutes(api, cfg.PolicyService, cfg.AuditRepository)
		}
		if cfg.RevisionService != nil {
			registerRevisionRoutes(api, cfg.RevisionService)
		} else {
			registerRevisionSkeleton(api)
		}
		if cfg.PublicationService != nil {
			registerPublicationRoutes(api, cfg.PublicationService, cfg.AuditRepository)
		} else {
			registerPublicationSkeleton(api)
		}
		registerIPRiskRoutes(api, cfg)
		registerSettingsRoutes(api, cfg)
		registerSkeletonRoutes(api)

		api.NotFound(func(w http.ResponseWriter, r *http.Request) {
			WriteError(w, r, http.StatusNotFound, "not_found", "API endpoint not found")
		})
	})

	// Legacy unversioned /api/* routes interceptor
	r.HandleFunc("/api", Legacy410Handler)
	r.HandleFunc("/api/*", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1") {
			WriteError(w, r, http.StatusNotFound, "not_found", "API endpoint not found")
			return
		}
		Legacy410Handler(w, r)
	})

	// Global 404 handler with unified error envelope
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, "not_found", "Endpoint not found")
	})

	return r
}

// publicationHandler serves publication requests, requiring valid publication export tokens.
func publicationHandler(cfg RouterConfig) http.HandlerFunc {
	if cfg.PublicationService != nil {
		return publicationClientHandler(cfg.PublicationService)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		pubID := chi.URLParam(r, "publication_id")
		token := strings.TrimSpace(r.URL.Query().Get("token"))
		if token == "" {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				token = strings.TrimSpace(authHeader[7:])
			}
		}

		if token == "" {
			WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Publication token required")
			return
		}

		if cfg.PublicationTokenValidator != nil {
			valid, err := cfg.PublicationTokenValidator(r.Context(), pubID, token)
			if err != nil || !valid {
				WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid publication token")
				return
			}
		}

		WriteSuccess(w, r, http.StatusOK, map[string]any{
			"publication_id": pubID,
			"status":         "active",
		})
	}
}

// authStatusHandler serves GET /api/v1/auth/status
func authStatusHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := cfg.SecurityMode()
		if mode == SecurityModeOpen {
			WriteSuccess(w, r, http.StatusOK, AuthStatusData{
				Mode:          SecurityModeOpen,
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
					Authenticated: true,
					Subject:       session.Subject,
				})
				return
			}
		}

		WriteSuccess(w, r, http.StatusOK, AuthStatusData{
			Mode:          SecurityModeProtected,
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
			Token:         trimmedToken,
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

// registerSubscriptionSkeleton preserves the Phase 1 route contract until a service is injected.
func registerSubscriptionSkeleton(r chi.Router) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		if r.Method == http.MethodPost {
			status = http.StatusCreated
		}
		WriteSuccess(w, r, status, map[string]any{"service": "subscriptions", "status": "skeleton_active"})
	}
	r.Get("/subscriptions", handler)
	r.Post("/subscriptions", handler)
	r.Get("/subscriptions/*", handler)
	r.Post("/subscriptions/*", handler)
	r.Patch("/subscriptions/*", handler)
	r.Delete("/subscriptions/*", handler)
}

// registerNodeSkeleton preserves the Phase 1 route contract until a service is injected.
func registerNodeSkeleton(r chi.Router) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		if r.Method == http.MethodPost {
			status = http.StatusCreated
		}
		WriteSuccess(w, r, status, map[string]any{"service": "nodes", "status": "skeleton_active"})
	}
	r.Get("/nodes", handler)
	r.Post("/nodes", handler)
	r.Get("/nodes/*", handler)
	r.Post("/nodes/*", handler)
	r.Patch("/nodes/*", handler)
	r.Delete("/nodes/*", handler)
}

// registerRevisionSkeleton preserves the Phase 1 route contract until a service is injected.
func registerRevisionSkeleton(r chi.Router) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		if r.Method == http.MethodPost {
			status = http.StatusCreated
		}
		WriteSuccess(w, r, status, map[string]any{"service": "revisions", "status": "skeleton_active"})
	}
	r.Get("/revisions", handler)
	r.Post("/revisions", handler)
	r.Get("/revisions/*", handler)
	r.Post("/revisions/*", handler)
	r.Patch("/revisions/*", handler)
	r.Delete("/revisions/*", handler)
}

// registerSkeletonRoutes sets up route placeholders for CSP 1.0 control plane services.
func registerSkeletonRoutes(r chi.Router) {
	services := []string{
		"probes",
		"policy",
		"audit",
	}

	for _, svc := range services {
		name := svc
		handler := func(w http.ResponseWriter, r *http.Request) {
			status := http.StatusOK
			if r.Method == http.MethodPost {
				status = http.StatusCreated
			}
			WriteSuccess(w, r, status, map[string]any{
				"service": name,
				"status":  "skeleton_active",
			})
		}

		r.Get("/"+name, handler)
		r.Post("/"+name, handler)
		r.Get("/"+name+"/*", handler)
		r.Post("/"+name+"/*", handler)
		r.Patch("/"+name+"/*", handler)
		r.Delete("/"+name+"/*", handler)
	}
}
