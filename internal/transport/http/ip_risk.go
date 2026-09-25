package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/domain"
)

type ipRiskHandler struct {
	service     *iprisk.Service
	policyRepo  domain.RiskPolicyRevisionRepository
	bindingRepo domain.RiskPolicyGroupBindingRepository
	auditRepo   domain.AuditRepository
}

func registerIPRiskRoutes(r chi.Router, cfg RouterConfig) {
	h := ipRiskHandler{
		service:     cfg.IPRiskService,
		policyRepo:  cfg.RiskPolicyRepository,
		bindingRepo: cfg.RiskBindingRepository,
		auditRepo:   cfg.AuditRepository,
	}

	r.Route("/ip-risk", func(sub chi.Router) {
		// Risk policy revision endpoints
		sub.Get("/policies", h.listPolicies)
		sub.Post("/policies", h.createPolicy)
		sub.Get("/policies/active", h.getActivePolicy)
		sub.Get("/policies/{id}", h.getPolicy)
		sub.Post("/policies/{id}/review", h.reviewPolicy)
		sub.Post("/policies/{id}/activate", h.activatePolicy)
		sub.Post("/policies/{id}/deactivate", h.deactivatePolicy)

		// Group binding endpoints
		sub.Get("/bindings", h.listBindings)
		sub.Get("/bindings/{group_id}", h.getBinding)
		sub.Put("/bindings/{group_id}", h.bindGroup)
		sub.Post("/bindings", h.bindGroup)
		sub.Delete("/bindings/{group_id}", h.unbindGroup)

		// Group binding aliases under /ip-risk/groups
		sub.Get("/groups/{group_id}/binding", h.getBinding)
		sub.Put("/groups/{group_id}/binding", h.bindGroup)
		sub.Delete("/groups/{group_id}/binding", h.unbindGroup)

		// Group member evaluation under bound policy
		sub.Get("/groups/{group_id}/members", h.evaluateGroupMembers)
		sub.Post("/groups/{group_id}/evaluate", h.evaluateGroupMembers)
	})

	// Group binding aliases under /policies/groups/{id}/risk-policy
	r.Get("/policies/groups/{id}/risk-policy", h.getBindingForGroup)
	r.Put("/policies/groups/{id}/risk-policy", h.bindGroupForGroup)
	r.Delete("/policies/groups/{id}/risk-policy", h.unbindGroupForGroup)

	r.Get("/policy/groups/{id}/risk-policy", h.getBindingForGroup)
	r.Put("/policy/groups/{id}/risk-policy", h.bindGroupForGroup)
	r.Delete("/policy/groups/{id}/risk-policy", h.unbindGroupForGroup)
}

type createRiskPolicyRequest struct {
	RevisionID        string                       `json:"revision_id,omitempty"`
	ProviderSelection domain.RiskProviderSelection `json:"provider_selection"`
	MaxObservationAge int                          `json:"max_observation_age_seconds"`
	MinimumConfidence int                          `json:"minimum_confidence"`
	ScoreBands        []domain.ScoreBand           `json:"score_bands"`
	TraitRules        []domain.TraitRule           `json:"trait_rules,omitempty"`
	UnknownAction     domain.RiskAction            `json:"unknown_action,omitempty"`
	ConflictAction    domain.RiskAction            `json:"conflict_action,omitempty"`
	ReviewAction      domain.RiskAction            `json:"review_action,omitempty"`
}

type bindGroupRequest struct {
	GroupID          string `json:"group_id,omitempty"`
	PolicyRevisionID string `json:"policy_revision_id"`
}

func (h ipRiskHandler) requireIdempotencyKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		WriteDomainError(w, r, domain.NewValidationError("missing_idempotency_key", "Idempotency-Key header is required"))
		return "", false
	}
	return key, true
}

func (h ipRiskHandler) listPolicies(w http.ResponseWriter, r *http.Request) {
	page, pageSize, err := ParsePagination(r)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	filter := domain.RiskPolicyFilter{
		Page:     page,
		PageSize: pageSize,
	}

	if activeStr := strings.TrimSpace(r.URL.Query().Get("active")); activeStr != "" {
		switch strings.ToLower(activeStr) {
		case "true", "1":
			b := true
			filter.Active = &b
		case "false", "0":
			b := false
			filter.Active = &b
		default:
			WriteDomainError(w, r, domain.NewValidationError("invalid_active_filter", "active filter must be true or false"))
			return
		}
	}

	if h.service != nil {
		items, total, sErr := h.service.ListPolicies(r.Context(), filter)
		if sErr != nil {
			WriteDomainError(w, r, sErr)
			return
		}
		WritePaginated(w, r, items, page, pageSize, total)
		return
	}
	if h.policyRepo != nil {
		items, total, pErr := h.policyRepo.List(r.Context(), filter)
		if pErr != nil {
			WriteDomainError(w, r, pErr)
			return
		}
		WritePaginated(w, r, items, page, pageSize, total)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_policy_service", "risk policy service is not available"))
}

func (h ipRiskHandler) createPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdempotencyKey(w, r); !ok {
		return
	}

	var body createRiskPolicyRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}

	cmd := iprisk.CreateRiskPolicyCommand{
		RevisionID:        body.RevisionID,
		ProviderSelection: body.ProviderSelection,
		MaxObservationAge: time.Duration(body.MaxObservationAge) * time.Second,
		MinimumConfidence: body.MinimumConfidence,
		ScoreBands:        body.ScoreBands,
		TraitRules:        body.TraitRules,
		UnknownAction:     body.UnknownAction,
		ConflictAction:    body.ConflictAction,
		ReviewAction:      body.ReviewAction,
		RequestID:         GetRequestID(r.Context()),
		ActorKind:         requestActorKind(r),
	}

	if h.service != nil {
		rev, err := h.service.CreatePolicy(r.Context(), cmd)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusCreated, rev)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_policy_service", "risk policy service is not available"))
}

func (h ipRiskHandler) getPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !domain.IsValidUUIDv7(id) {
		WriteDomainError(w, r, domain.NewValidationError("invalid_risk_policy_revision_id", "risk policy revision ID must be UUIDv7"))
		return
	}

	if h.service != nil {
		rev, err := h.service.GetPolicy(r.Context(), id)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, rev)
		return
	}
	if h.policyRepo != nil {
		rev, err := h.policyRepo.GetByID(r.Context(), id)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, rev)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_policy_service", "risk policy service is not available"))
}

func (h ipRiskHandler) getActivePolicy(w http.ResponseWriter, r *http.Request) {
	if h.service != nil {
		rev, err := h.service.GetActivePolicy(r.Context())
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, rev)
		return
	}
	if h.policyRepo != nil {
		rev, err := h.policyRepo.GetActive(r.Context())
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, rev)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_policy_service", "risk policy service is not available"))
}

func (h ipRiskHandler) reviewPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdempotencyKey(w, r); !ok {
		return
	}
	id := chi.URLParam(r, "id")
	action := iprisk.Action{
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	}

	if h.service != nil {
		detail, err := h.service.ReviewPolicy(r.Context(), id, action)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, detail)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_policy_service", "risk policy service is not available"))
}

func (h ipRiskHandler) activatePolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdempotencyKey(w, r); !ok {
		return
	}
	id := chi.URLParam(r, "id")
	action := iprisk.Action{
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	}

	if h.service != nil {
		rev, err := h.service.ActivatePolicy(r.Context(), id, action)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, rev)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_policy_service", "risk policy service is not available"))
}

func (h ipRiskHandler) deactivatePolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdempotencyKey(w, r); !ok {
		return
	}
	id := chi.URLParam(r, "id")
	action := iprisk.Action{
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	}

	if h.service != nil {
		rev, err := h.service.DeactivatePolicy(r.Context(), id, action)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, rev)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_policy_service", "risk policy service is not available"))
}

func (h ipRiskHandler) listBindings(w http.ResponseWriter, r *http.Request) {
	if h.service != nil {
		bindings, err := h.service.ListGroupBindings(r.Context())
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, bindings)
		return
	}
	if h.bindingRepo != nil {
		bindings, err := h.bindingRepo.List(r.Context())
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, bindings)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_binding_service", "risk policy binding service is not available"))
}

func (h ipRiskHandler) getBinding(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "group_id")
	if groupID == "" {
		groupID = chi.URLParam(r, "id")
	}
	if !domain.IsValidUUIDv7(groupID) {
		WriteDomainError(w, r, domain.NewValidationError("invalid_group_id", "group ID must be a valid UUIDv7"))
		return
	}

	if h.service != nil {
		binding, err := h.service.GetGroupBinding(r.Context(), groupID)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, binding)
		return
	}
	if h.bindingRepo != nil {
		binding, err := h.bindingRepo.GetByGroupID(r.Context(), groupID)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, binding)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_binding_service", "risk policy binding service is not available"))
}

func (h ipRiskHandler) bindGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdempotencyKey(w, r); !ok {
		return
	}

	groupID := chi.URLParam(r, "group_id")
	if groupID == "" {
		groupID = chi.URLParam(r, "id")
	}

	var body bindGroupRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}

	if groupID == "" {
		groupID = body.GroupID
	}

	cmd := iprisk.BindGroupRiskPolicyCommand{
		GroupID:          groupID,
		PolicyRevisionID: body.PolicyRevisionID,
		RequestID:        GetRequestID(r.Context()),
		ActorKind:        requestActorKind(r),
	}

	if h.service != nil {
		binding, err := h.service.BindGroup(r.Context(), cmd)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, binding)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_binding_service", "risk policy binding service is not available"))
}

func (h ipRiskHandler) unbindGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdempotencyKey(w, r); !ok {
		return
	}
	groupID := chi.URLParam(r, "group_id")
	if groupID == "" {
		groupID = chi.URLParam(r, "id")
	}

	action := iprisk.Action{
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	}

	if h.service != nil {
		if err := h.service.UnbindGroup(r.Context(), groupID, action); err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, map[string]any{
			"unbound":  true,
			"group_id": groupID,
		})
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_binding_service", "risk policy binding service is not available"))
}

func (h ipRiskHandler) getBindingForGroup(w http.ResponseWriter, r *http.Request) {
	h.getBinding(w, r)
}

func (h ipRiskHandler) bindGroupForGroup(w http.ResponseWriter, r *http.Request) {
	h.bindGroup(w, r)
}

func (h ipRiskHandler) unbindGroupForGroup(w http.ResponseWriter, r *http.Request) {
	h.unbindGroup(w, r)
}

func (h ipRiskHandler) evaluateGroupMembers(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "group_id")
	if groupID == "" {
		groupID = chi.URLParam(r, "id")
	}

	if h.service != nil {
		res, err := h.service.EvaluateGroupMembers(r.Context(), groupID)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, res)
		return
	}

	WriteDomainError(w, r, domain.NewInternalError("missing_binding_service", "risk policy binding service is not available"))
}
