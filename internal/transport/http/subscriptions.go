package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
)

type subscriptionHandler struct {
	service   *subscription.Service
	inventory *inventory.Service
}

// SubscriptionEntryDTO represents a safe, non-secret parsed entry view.
type SubscriptionEntryDTO struct {
	EntryID              string            `json:"entry_id"`
	SubscriptionID       string            `json:"subscription_id"`
	PayloadID            string            `json:"payload_id"`
	Ordinal              int               `json:"ordinal"`
	Name                 string            `json:"name"`
	EntryKind            domain.EntryKind  `json:"entry_kind"`
	UserKindOverride     *domain.EntryKind `json:"user_kind_override,omitempty"`
	EffectiveKind        domain.EntryKind  `json:"effective_kind"`
	NodeLogicalID        *string           `json:"node_logical_id,omitempty"`
	ClassificationReason string            `json:"classification_reason,omitempty"`
	RuleVersion          string            `json:"rule_version,omitempty"`
	OverrideReason       string            `json:"override_reason,omitempty"`
	OverrideAt           *string           `json:"override_at,omitempty"`
	Conflict             string            `json:"conflict,omitempty"`
}

type overrideEntryKindRequest struct {
	UserKindOverride *domain.EntryKind `json:"user_kind_override"`
	Reason           *string           `json:"reason,omitempty"`
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

func registerSubscriptionRoutes(r chi.Router, service *subscription.Service, invService *inventory.Service) {
	if service == nil {
		return
	}
	h := subscriptionHandler{service: service, inventory: invService}
	r.Get("/subscriptions", h.list)
	r.Post("/subscriptions", h.create)
	r.Get("/subscriptions/{id}", h.get)
	r.Patch("/subscriptions/{id}", h.update)
	r.Delete("/subscriptions/{id}", h.delete)
	r.Post("/subscriptions/{id}/refresh", h.refresh)
	r.Get("/subscriptions/{id}/entries", h.listEntries)
	r.Post("/subscriptions/{id}/entries/{entry_id}/override", h.overrideEntry)
}

func (h subscriptionHandler) listEntries(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "id")
	if strings.TrimSpace(subID) == "" {
		WriteDomainError(w, r, domain.NewValidationError("missing_subscription_id", "subscription ID is required"))
		return
	}

	if h.inventory == nil {
		WriteSuccess(w, r, http.StatusOK, map[string]any{"items": []any{}, "total": 0})
		return
	}

	entries, err := h.inventory.ListLatestEntries(r.Context(), subID)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	dtos := make([]SubscriptionEntryDTO, len(entries))
	for i, e := range entries {
		var oAt *string
		if e.OverrideAt != nil {
			str := e.OverrideAt.Format(time.RFC3339)
			oAt = &str
		}
		dtos[i] = SubscriptionEntryDTO{
			EntryID:              e.ID,
			SubscriptionID:       e.SubscriptionID,
			PayloadID:            e.PayloadID,
			Ordinal:              e.Ordinal,
			Name:                 e.RawName,
			EntryKind:            e.EntryKind,
			UserKindOverride:     e.UserKindOverride,
			EffectiveKind:        e.EffectiveKind(),
			NodeLogicalID:        e.NodeLogicalID,
			ClassificationReason: e.ClassificationReason,
			RuleVersion:          e.ClassificationVersion,
			OverrideReason:       e.OverrideReason,
			OverrideAt:           oAt,
		}
	}

	WriteSuccess(w, r, http.StatusOK, map[string]any{
		"items": dtos,
		"total": len(dtos),
	})
}

func (h subscriptionHandler) overrideEntry(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "entry_id")
	if strings.TrimSpace(entryID) == "" {
		WriteDomainError(w, r, domain.NewValidationError("missing_entry_id", "entry ID is required"))
		return
	}

	var body overrideEntryKindRequest
	if err := decodeSubscriptionJSON(w, r, &body); err != nil {
		return
	}

	if h.inventory == nil {
		WriteDomainError(w, r, domain.NewInternalError("inventory_unavailable", "inventory service not available"))
		return
	}

	reason := ""
	if body.Reason != nil {
		reason = strings.TrimSpace(*body.Reason)
	}

	actor := "admin"
	if auth := GetAuthContext(r.Context()); auth != nil && auth.Subject != "" {
		actor = auth.Subject
	}

	updated, err := h.inventory.OverrideEntryKind(r.Context(), entryID, body.UserKindOverride, reason, actor)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	var oAt *string
	if updated.OverrideAt != nil {
		str := updated.OverrideAt.Format(time.RFC3339)
		oAt = &str
	}
	dto := SubscriptionEntryDTO{
		EntryID:              updated.ID,
		SubscriptionID:       updated.SubscriptionID,
		PayloadID:            updated.PayloadID,
		Ordinal:              updated.Ordinal,
		Name:                 updated.RawName,
		EntryKind:            updated.EntryKind,
		UserKindOverride:     updated.UserKindOverride,
		EffectiveKind:        updated.EffectiveKind(),
		NodeLogicalID:        updated.NodeLogicalID,
		ClassificationReason: updated.ClassificationReason,
		RuleVersion:          updated.ClassificationVersion,
		OverrideReason:       updated.OverrideReason,
		OverrideAt:           oAt,
	}

	WriteSuccess(w, r, http.StatusOK, dto)
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
		Config:    body.Config,
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

	view, err := h.service.Update(r.Context(), subscription.UpdateSubscriptionCommand{
		ID: chi.URLParam(r, "id"), Revision: strings.TrimSpace(r.Header.Get("If-Match")), Name: body.Name,
		SourceURLSecretRef: body.SourceURLSecretRef, Enabled: body.Enabled, RefreshPolicy: body.RefreshPolicy,
		Config:    body.Config,
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
