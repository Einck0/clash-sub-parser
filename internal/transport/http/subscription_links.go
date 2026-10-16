package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
)

type subscriptionLinkAdminHandler struct {
	service  *publication.Service
	settings domain.SettingsRepository
	audit    domain.AuditRepository
}

type createSubscriptionLinkRequest struct {
	Target    string `json:"target"`
	Label     string `json:"label,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
}

type patchSubscriptionLinkRequest struct {
	Label string `json:"label"`
}

type subscriptionLinkResponse struct {
	ID          string                       `json:"id"`
	Label       string                       `json:"label"`
	Target      domain.CompilerTarget        `json:"target"`
	Token       string                       `json:"token,omitempty"`
	TokenMasked string                       `json:"token_masked"`
	State       domain.SubscriptionLinkState `json:"state"`
	IsDefault   bool                         `json:"is_default"`
	CreatedAt   time.Time                    `json:"created_at"`
	UpdatedAt   time.Time                    `json:"updated_at"`
	RevokedAt   *time.Time                   `json:"revoked_at,omitempty"`
}

func toSubscriptionLinkResponse(link *domain.SubscriptionLink, includeToken bool) subscriptionLinkResponse {
	resp := subscriptionLinkResponse{
		ID:          link.ID,
		Label:       link.Label,
		Target:      link.Target,
		TokenMasked: link.MaskedToken(),
		State:       link.State,
		IsDefault:   link.IsDefault,
		CreatedAt:   link.CreatedAt,
		UpdatedAt:   link.UpdatedAt,
		RevokedAt:   link.RevokedAt,
	}
	if includeToken {
		resp.Token = link.Token
	}
	return resp
}

// registerSubscriptionLinkRoutes mounts /api/v1/subscription-links endpoints.
func registerSubscriptionLinkRoutes(r chi.Router, service *publication.Service, settings domain.SettingsRepository, audit domain.AuditRepository) {
	if service == nil {
		return
	}
	h := subscriptionLinkAdminHandler{service: service, settings: settings, audit: audit}

	r.Route("/subscription-links", func(sub chi.Router) {
		sub.Get("/", h.list)
		sub.Post("/", h.create)
		sub.Post("/{id}/copy", h.copy)
		sub.Post("/{id}/rotate-token", h.rotateToken)
		sub.Post("/{id}/revoke", h.revoke)
		sub.Delete("/{id}", h.revoke)
		sub.Patch("/{id}", h.patch)
		sub.Post("/{id}/check", h.check)
	})
}

func (h subscriptionLinkAdminHandler) list(w http.ResponseWriter, r *http.Request) {
	links, err := h.service.ListSubscriptionLinks(r.Context())
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list subscription links")
		return
	}

	res := make([]subscriptionLinkResponse, len(links))
	for i, l := range links {
		res[i] = toSubscriptionLinkResponse(&l, false)
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Referrer-Policy", "no-referrer")
	WriteSuccess(w, r, http.StatusOK, res)
}

func (h subscriptionLinkAdminHandler) create(w http.ResponseWriter, r *http.Request) {
	var body createSubscriptionLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, r, http.StatusUnprocessableEntity, "invalid_json", "Invalid JSON request body")
		return
	}

	target := domain.CompilerTarget(strings.TrimSpace(body.Target))
	if target == "" {
		WriteError(w, r, http.StatusUnprocessableEntity, "invalid_target", "Target compiler is required")
		return
	}
	if !target.IsValid() {
		WriteError(w, r, http.StatusUnprocessableEntity, "unsupported_target", fmt.Sprintf("target %q is not supported", target))
		return
	}

	cmd := publication.CreateSubscriptionLinkCommand{
		Target:    target,
		Label:     strings.TrimSpace(body.Label),
		IsDefault: body.IsDefault,
		ActorKind: domain.ActorKindAdmin,
		RequestID: GetRequestID(r.Context()),
	}

	link, err := h.service.CreateSubscriptionLink(r.Context(), cmd)
	if err != nil {
		var de *domain.DomainError
		if errors.As(err, &de) {
			status := http.StatusBadRequest
			if de.Category == domain.CategoryValidation {
				status = http.StatusUnprocessableEntity
			}
			WriteError(w, r, status, de.Code, de.Message)
			return
		}
		WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create subscription link")
		return
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Referrer-Policy", "no-referrer")
	WriteSuccess(w, r, http.StatusCreated, toSubscriptionLinkResponse(link, true))
}

func (h subscriptionLinkAdminHandler) copy(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
		return
	}

	link, err := h.service.GetSubscriptionLink(r.Context(), id)
	if err != nil {
		var de *domain.DomainError
		if errors.As(err, &de) && de.Category == domain.CategoryNotFound {
			WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
			return
		}
		WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to get subscription link")
		return
	}

	if !link.IsActive() {
		WriteError(w, r, http.StatusForbidden, "subscription_link_revoked", "Subscription link has been revoked")
		return
	}

	var publicBaseURL string
	if h.settings != nil {
		s, err := h.settings.Get(r.Context())
		if err == nil && s != nil {
			publicBaseURL = strings.TrimRight(strings.TrimSpace(s.PublicBaseURL), "/")
		}
	}

	if publicBaseURL == "" {
		WriteError(w, r, http.StatusUnprocessableEntity, "public_base_url_not_configured",
			"Public base URL is not configured. Please configure a trusted public base URL in Settings before copying client subscription links.")
		return
	}

	fullURL := fmt.Sprintf("%s/subscribe/v1/%s?token=%s", publicBaseURL, link.ID, link.Token)

	if h.audit != nil {
		_ = h.audit.Record(r.Context(), &domain.AuditEvent{
			ID:              domain.MustNewUUIDv7(),
			ActorKind:       domain.ActorKindAdmin,
			RequestID:       GetRequestID(r.Context()),
			Action:          "subscription_link.copy",
			Result:          domain.AuditResultSuccess,
			RedactedSummary: fmt.Sprintf("Revealed subscription link URL for link %s (target: %s)", link.ID, link.Target),
			CreatedAt:       domain.NowUTC(),
		})
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Referrer-Policy", "no-referrer")
	WriteSuccess(w, r, http.StatusOK, map[string]string{
		"id":              link.ID,
		"target":          string(link.Target),
		"url":             fullURL,
		"token":           link.Token,
		"public_base_url": publicBaseURL,
	})
}

func (h subscriptionLinkAdminHandler) rotateToken(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
		return
	}

	link, err := h.service.RotateSubscriptionLinkToken(r.Context(), id, domain.ActorKindAdmin, GetRequestID(r.Context()))
	if err != nil {
		if errors.Is(err, publication.ErrLinkRevoked) {
			WriteError(w, r, http.StatusForbidden, "subscription_link_revoked", "Cannot rotate token for revoked subscription link")
			return
		}
		var de *domain.DomainError
		if errors.As(err, &de) {
			status := http.StatusBadRequest
			if de.Category == domain.CategoryNotFound {
				status = http.StatusNotFound
			} else if de.Category == domain.CategoryConflict {
				status = http.StatusConflict
			}
			WriteError(w, r, status, de.Code, de.Message)
			return
		}
		WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to rotate subscription link token")
		return
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Referrer-Policy", "no-referrer")
	WriteSuccess(w, r, http.StatusOK, toSubscriptionLinkResponse(link, true))
}

func (h subscriptionLinkAdminHandler) revoke(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
		return
	}

	if err := h.service.RevokeSubscriptionLink(r.Context(), id, domain.ActorKindAdmin, GetRequestID(r.Context())); err != nil {
		var de *domain.DomainError
		if errors.As(err, &de) && de.Category == domain.CategoryNotFound {
			WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
			return
		}
		WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to revoke subscription link")
		return
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Referrer-Policy", "no-referrer")
	WriteSuccess(w, r, http.StatusOK, map[string]string{
		"id":    id,
		"state": string(domain.SubscriptionLinkStateRevoked),
	})
}

func (h subscriptionLinkAdminHandler) patch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
		return
	}

	var body patchSubscriptionLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, r, http.StatusUnprocessableEntity, "invalid_json", "Invalid JSON request body")
		return
	}

	if err := h.service.UpdateSubscriptionLinkLabel(r.Context(), id, body.Label); err != nil {
		var de *domain.DomainError
		if errors.As(err, &de) && de.Category == domain.CategoryNotFound {
			WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
			return
		}
		WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to update subscription link label")
		return
	}

	link, err := h.service.GetSubscriptionLink(r.Context(), id)
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to get updated subscription link")
		return
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Referrer-Policy", "no-referrer")
	WriteSuccess(w, r, http.StatusOK, toSubscriptionLinkResponse(link, false))
}

func (h subscriptionLinkAdminHandler) check(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Referrer-Policy", "no-referrer")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
		return
	}

	res, err := h.service.CheckSubscriptionLink(r.Context(), id)
	if err != nil {
		if errors.Is(err, publication.ErrLinkNotFound) {
			WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
			return
		}
		if errors.Is(err, publication.ErrLinkRevoked) {
			WriteError(w, r, http.StatusForbidden, "subscription_link_revoked", "Subscription link has been revoked")
			return
		}
		if errors.Is(err, publication.ErrRateLimitExceeded) {
			w.Header().Set("Retry-After", "5")
			WriteError(w, r, http.StatusServiceUnavailable, "concurrency_limit_exceeded", "Generation concurrency limit exceeded; retry shortly")
			return
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			w.Header().Set("Retry-After", "5")
			WriteError(w, r, http.StatusServiceUnavailable, "request_timeout", "Check timed out or was cancelled")
			return
		}
		if isTemporaryStorageError(err) {
			w.Header().Set("Retry-After", "5")
			WriteError(w, r, http.StatusServiceUnavailable, "database_busy", "Database busy or temporarily unavailable; retry shortly")
			return
		}
		var de *domain.DomainError
		if errors.As(err, &de) {
			status := http.StatusBadRequest
			if de.Category == domain.CategoryNotFound {
				status = http.StatusNotFound
			} else if de.Category == domain.CategoryConflict {
				status = http.StatusConflict
			}
			WriteError(w, r, status, de.Code, de.Message)
			return
		}
		WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to check subscription link")
		return
	}

	WriteSuccess(w, r, http.StatusOK, res)
}
