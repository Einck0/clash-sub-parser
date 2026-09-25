package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
)

type subscriptionHandler struct {
	service *subscription.Service
}

type createSubscriptionRequest struct {
	Name               string                    `json:"name"`
	SourceURLSecretRef string                    `json:"source_url_secret_ref"`
	Enabled            bool                      `json:"enabled"`
	RefreshPolicy      domain.RefreshPolicy      `json:"refresh_policy"`
	Config             domain.SubscriptionConfig `json:"config"`
}

type updateSubscriptionRequest struct {
	Name               *string                    `json:"name"`
	SourceURLSecretRef *string                    `json:"source_url_secret_ref"`
	Enabled            *bool                      `json:"enabled"`
	RefreshPolicy      *domain.RefreshPolicy      `json:"refresh_policy"`
	Config             *domain.SubscriptionConfig `json:"config"`
}

func registerSubscriptionRoutes(r chi.Router, service *subscription.Service) {
	if service == nil {
		return
	}
	h := subscriptionHandler{service: service}
	r.Get("/subscriptions", h.list)
	r.Post("/subscriptions", h.create)
	r.Get("/subscriptions/{id}", h.get)
	r.Patch("/subscriptions/{id}", h.update)
	r.Delete("/subscriptions/{id}", h.delete)
	r.Post("/subscriptions/{id}/refresh", h.refresh)
}

func (h subscriptionHandler) list(w http.ResponseWriter, r *http.Request) {
	page, pageSize, err := ParsePagination(r)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	result, err := h.service.List(r.Context(), subscription.ListSubscriptionsQuery{
		Page: page, PageSize: pageSize, EnabledOnly: r.URL.Query().Get("enabled_only") == "true", SearchText: r.URL.Query().Get("search_text"),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WritePaginated(w, r, result.Items, result.Page, result.PageSize, result.Total)
}

func (h subscriptionHandler) create(w http.ResponseWriter, r *http.Request) {
	var body createSubscriptionRequest
	if err := decodeSubscriptionJSON(w, r, &body); err != nil {
		return
	}
	view, err := h.service.Create(r.Context(), subscription.CreateSubscriptionCommand{
		Name: body.Name, SourceURLSecretRef: body.SourceURLSecretRef, Enabled: body.Enabled, RefreshPolicy: body.RefreshPolicy,
		Config: body.Config,
		RequestID: GetRequestID(r.Context()), ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusCreated, view)
}

func (h subscriptionHandler) get(w http.ResponseWriter, r *http.Request) {
	view, err := h.service.Get(r.Context(), subscription.GetSubscriptionQuery{ID: chi.URLParam(r, "id")})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, view)
}

func (h subscriptionHandler) update(w http.ResponseWriter, r *http.Request) {
	var body updateSubscriptionRequest
	if err := decodeSubscriptionJSON(w, r, &body); err != nil {
		return
	}
	// Desensitize check: if source_url_secret_ref is empty or "***", preserve the original secret ref
	var sourceURLRef *string
	if body.SourceURLSecretRef != nil {
		trimmed := strings.TrimSpace(*body.SourceURLSecretRef)
		if trimmed != "" && trimmed != "***" {
			sourceURLRef = &trimmed
		}
	}

	view, err := h.service.Update(r.Context(), subscription.UpdateSubscriptionCommand{
		ID: chi.URLParam(r, "id"), Revision: strings.TrimSpace(r.Header.Get("If-Match")), Name: body.Name,
		SourceURLSecretRef: sourceURLRef, Enabled: body.Enabled, RefreshPolicy: body.RefreshPolicy,
		Config: body.Config,
		RequestID: GetRequestID(r.Context()), ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, view)
}

func (h subscriptionHandler) delete(w http.ResponseWriter, r *http.Request) {
	err := h.service.Delete(r.Context(), subscription.DeleteSubscriptionCommand{
		ID: chi.URLParam(r, "id"), RequestID: GetRequestID(r.Context()), ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h subscriptionHandler) refresh(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		WriteDomainError(w, r, domain.NewValidationError("missing_idempotency_key", "Idempotency-Key header is required"))
		return
	}
	summary, err := h.service.Refresh(r.Context(), chi.URLParam(r, "id"), GetRequestID(r.Context()), requestActorKind(r))
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, summary)
}

func decodeSubscriptionJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		WriteDomainError(w, r, domain.NewValidationError("invalid_json", "request body must be valid JSON"))
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		WriteDomainError(w, r, domain.NewValidationError("invalid_json", "request body must contain one JSON object"))
		return domain.NewValidationError("invalid_json", "request body must contain one JSON object")
	}
	return nil
}

func requestActorKind(r *http.Request) domain.ActorKind {
	if auth := GetAuthContext(r.Context()); auth != nil && auth.ActorKind == ActorKindAdmin {
		return domain.ActorKindAdmin
	}
	return domain.ActorKindAnonymous
}
