package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
)

type policyHandler struct {
	service *policy.Service
	audit   domain.AuditRepository
}

type createGroupRequest struct {
	EmptyFallbackPass   bool                   `json:"empty_fallback_pass"`
	InheritGlobalFilter *bool                  `json:"inherit_global_filter,omitempty"`
	ID                  string                 `json:"id,omitempty"`
	Name                string                 `json:"name"`
	GroupType           domain.GroupType       `json:"group_type"`
	Position            *int                   `json:"position,omitempty"`
	Sources             []domain.MemberSource  `json:"sources,omitempty"`
	Edges               []policy.EdgeInput     `json:"edges,omitempty"`
	NodeFilter          *domain.NodeFilterSpec `json:"node_filter,omitempty"`
}

type updateGroupRequest struct {
	EmptyFallbackPass   *bool                  `json:"empty_fallback_pass,omitempty"`
	InheritGlobalFilter *bool                  `json:"inherit_global_filter,omitempty"`
	Name                *string                `json:"name,omitempty"`
	GroupType           *domain.GroupType      `json:"group_type,omitempty"`
	Position            *int                   `json:"position,omitempty"`
	ExpectedRevision    *int64                 `json:"expected_revision,omitempty"`
	Sources             *[]domain.MemberSource `json:"sources,omitempty"`
	Edges               *[]policy.EdgeInput    `json:"edges,omitempty"`
	NodeFilter          *domain.NodeFilterSpec `json:"node_filter,omitempty"`
}

type previewGroupRequest struct {
	GroupID             *string                `json:"group_id,omitempty"`
	ExpectedRevision    *int64                 `json:"expected_revision,omitempty"`
	Name                *string                `json:"name,omitempty"`
	GroupType           *domain.GroupType      `json:"group_type,omitempty"`
	EmptyFallbackPass   *bool                  `json:"empty_fallback_pass,omitempty"`
	InheritGlobalFilter *bool                  `json:"inherit_global_filter,omitempty"`
	Sources             []domain.MemberSource  `json:"sources"`
	Target              *domain.CompilerTarget `json:"target,omitempty"`
}

type setGlobalFilterRequest struct {
	Spec       *domain.NodeFilterSpec   `json:"spec,omitempty"`
	Conditions []domain.FilterCondition `json:"conditions,omitempty"`
}

type setGroupEdgesRequest struct {
	Edges []policy.EdgeInput `json:"edges"`
}

type createCategoryRequest struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

type updateCategoryRequest struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

type reorderCategoriesRequest struct {
	Order []string `json:"order"`
}

type createRuleRequest struct {
	Kind          string            `json:"kind,omitempty"` // "policy" or "admission"
	RevisionID    string            `json:"revision_id,omitempty"`
	CategoryID    string            `json:"category_id,omitempty"`
	Category      string            `json:"category,omitempty"`
	Name          string            `json:"name,omitempty"`
	TargetGroupID string            `json:"target_group_id,omitempty"`
	BuiltinTarget string            `json:"builtin_target,omitempty"`
	Expression    string            `json:"expression"`
	Action        domain.RuleAction `json:"action,omitempty"`
	Position      int               `json:"position"`
	Enabled       *bool             `json:"enabled,omitempty"`
}

type updateRuleRequest struct {
	CategoryID    *string `json:"category_id,omitempty"`
	Category      *string `json:"category,omitempty"`
	Name          *string `json:"name,omitempty"`
	TargetGroupID *string `json:"target_group_id,omitempty"`
	BuiltinTarget *string `json:"builtin_target,omitempty"`
	Expression    *string `json:"expression,omitempty"`
	Position      *int    `json:"position,omitempty"`
	Enabled       *bool   `json:"enabled,omitempty"`
}

type reorderRulesRequest struct {
	RevisionID string   `json:"revision_id,omitempty"`
	CategoryID string   `json:"category_id,omitempty"`
	Order      []string `json:"order"`
}

func registerPolicyRoutes(r chi.Router, service *policy.Service, audit domain.AuditRepository) {
	if service == nil {
		return
	}
	h := policyHandler{service: service, audit: audit}

	registerGroup := func(sub chi.Router) {
		sub.Get("/global-node-filter", h.getGlobalFilter)
		sub.Put("/global-node-filter", h.setGlobalFilter)

		sub.Get("/groups", h.listGroups)
		sub.Post("/groups", h.createGroup)
		sub.Post("/groups/preview", h.previewGroup)
		sub.Get("/groups/{id}", h.getGroup)
		sub.Patch("/groups/{id}", h.updateGroup)
		sub.Put("/groups/{id}", h.updateGroup)
		sub.Delete("/groups/{id}", h.deleteGroup)
		sub.Put("/groups/{id}/edges", h.setGroupEdges)
		sub.Post("/groups/{id}/edges", h.setGroupEdges)

		sub.Get("/categories", h.listCategories)
		sub.Post("/categories", h.createCategory)
		sub.Put("/categories/reorder", h.reorderCategories)
		sub.Put("/categories/{id}", h.updateCategory)
		sub.Delete("/categories/{id}", h.deleteCategory)

		sub.Get("/rules", h.listRules)
		sub.Post("/rules", h.createRule)
		sub.Post("/rules/backfill", h.backfillRules)
		sub.Put("/rules/reorder", h.reorderRules)
		sub.Put("/rules/{id}", h.updateRule)
		sub.Delete("/rules/{id}", h.deleteRule)

		sub.Post("/validate", h.validate)
		sub.Get("/validate", h.validate)
	}

	r.Route("/policies", registerGroup)
	r.Route("/policy", registerGroup)
	r.Route("/admission", func(sub chi.Router) {
		sub.Get("/rules", h.listRules)
		sub.Post("/rules", h.createAdmissionRule)
		sub.Delete("/rules/{id}", h.deleteAdmissionRule)
	})
}

// RegisterPolicyFilterRoutes explicitly registers global node filter endpoints on a router.
func RegisterPolicyFilterRoutes(r chi.Router, service *policy.Service, audit domain.AuditRepository) {
	if service == nil {
		return
	}
	h := policyHandler{service: service, audit: audit}
	r.Get("/policies/global-node-filter", h.getGlobalFilter)
	r.Put("/policies/global-node-filter", h.setGlobalFilter)
	r.Get("/policy/global-node-filter", h.getGlobalFilter)
	r.Put("/policy/global-node-filter", h.setGlobalFilter)
}

func (h policyHandler) listGroups(w http.ResponseWriter, r *http.Request) {
	page, pageSize, err := ParsePaginationWithLimit(r, 500)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	res, err := h.service.ListGroups(r.Context(), policy.ListGroupsQuery{
		Page:     page,
		PageSize: pageSize,
		Search:   r.URL.Query().Get("search"),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WritePaginated(w, r, res.Items, res.Page, res.PageSize, res.Total)
}

func (h policyHandler) getGlobalFilter(w http.ResponseWriter, r *http.Request) {
	filter, err := h.service.GetGlobalNodeFilter(r.Context())
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, filter)
}

func (h policyHandler) setGlobalFilter(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		WriteDomainError(w, r, domain.NewValidationError("invalid_json", "failed to read request body"))
		return
	}

	var req setGlobalFilterRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		WriteDomainError(w, r, domain.NewValidationError("invalid_json", err.Error()))
		return
	}

	var spec domain.NodeFilterSpec
	if req.Spec != nil {
		spec = *req.Spec
	} else if req.Conditions != nil {
		spec = domain.NodeFilterSpec{Conditions: req.Conditions}
	} else {
		var directSpec domain.NodeFilterSpec
		if err := json.Unmarshal(bodyBytes, &directSpec); err == nil && directSpec.Conditions != nil {
			spec = directSpec
		} else {
			spec = domain.NodeFilterSpec{Conditions: []domain.FilterCondition{}}
		}
	}

	filter, err := h.service.SetGlobalNodeFilter(r.Context(), policy.SetGlobalNodeFilterCommand{
		Spec:      spec,
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, filter)
}

func (h policyHandler) createGroup(w http.ResponseWriter, r *http.Request) {
	var body createGroupRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}

	view, err := h.service.CreateGroup(r.Context(), policy.CreateGroupCommand{
		EmptyFallbackPass:   body.EmptyFallbackPass,
		InheritGlobalFilter: body.InheritGlobalFilter,
		ID:                  body.ID,
		Name:                body.Name,
		GroupType:           body.GroupType,
		Position:            body.Position,
		Sources:             body.Sources,
		Edges:               body.Edges,
		NodeFilter:          body.NodeFilter,
		RequestID:           GetRequestID(r.Context()),
		ActorKind:           requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusCreated, view)
}

func (h policyHandler) getGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	view, err := h.service.GetGroup(r.Context(), id)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, view)
}

func (h policyHandler) updateGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		WriteDomainError(w, r, domain.NewValidationError("invalid_json", "failed to read body"))
		return
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		WriteDomainError(w, r, domain.NewValidationError("invalid_json", err.Error()))
		return
	}

	var body updateGroupRequest
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		WriteDomainError(w, r, domain.NewValidationError("invalid_json", err.Error()))
		return
	}

	cmd := policy.UpdateGroupCommand{
		EmptyFallbackPass:   body.EmptyFallbackPass,
		InheritGlobalFilter: body.InheritGlobalFilter,
		ID:                  id,
		Name:                body.Name,
		GroupType:           body.GroupType,
		Position:            body.Position,
		ExpectedRevision:    body.ExpectedRevision,
		Sources:             body.Sources,
		Edges:               body.Edges,
		RequestID:           GetRequestID(r.Context()),
		ActorKind:           requestActorKind(r),
	}

	if rawFilter, exists := raw["node_filter"]; exists {
		filterStr := strings.TrimSpace(string(rawFilter))
		if filterStr == "null" || body.NodeFilter == nil || body.NodeFilter.IsEmpty() {
			cmd.ClearNodeFilter = true
		} else {
			cmd.NodeFilter = body.NodeFilter
		}
	}

	view, err := h.service.UpdateGroup(r.Context(), cmd)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, view)
}

func (h policyHandler) previewGroup(w http.ResponseWriter, r *http.Request) {
	var body previewGroupRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}

	res, err := h.service.PreviewGroup(r.Context(), policy.PreviewGroupCommand{
		GroupID:             body.GroupID,
		ExpectedRevision:    body.ExpectedRevision,
		Name:                body.Name,
		GroupType:           body.GroupType,
		EmptyFallbackPass:   body.EmptyFallbackPass,
		InheritGlobalFilter: body.InheritGlobalFilter,
		Sources:             body.Sources,
		Target:              body.Target,
		RequestID:           GetRequestID(r.Context()),
		ActorKind:           requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, res)
}

func (h policyHandler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.service.DeleteGroup(r.Context(), policy.DeleteGroupCommand{
		ID:        id,
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h policyHandler) setGroupEdges(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body setGroupEdgesRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}

	err := h.service.SetGroupEdges(r.Context(), policy.SetGroupEdgesCommand{
		ParentGroupID: id,
		Edges:         body.Edges,
		RequestID:     GetRequestID(r.Context()),
		ActorKind:     requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, map[string]any{
		"parent_group_id": id,
		"status":          "updated",
	})
}

func (h policyHandler) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.service.ListCategories(r.Context())
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, cats)
}

func (h policyHandler) createCategory(w http.ResponseWriter, r *http.Request) {
	var body createCategoryRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	view, err := h.service.CreateCategory(r.Context(), policy.CreateCategoryCommand{
		Name:      body.Name,
		SortOrder: body.SortOrder,
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusCreated, view)
}

func (h policyHandler) updateCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body updateCategoryRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	view, err := h.service.UpdateCategory(r.Context(), policy.UpdateCategoryCommand{
		ID:        id,
		Name:      body.Name,
		SortOrder: body.SortOrder,
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, view)
}

func (h policyHandler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.service.DeleteCategory(r.Context(), policy.DeleteCategoryCommand{
		ID:        id,
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	}); err != nil {
		WriteDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h policyHandler) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var body reorderCategoriesRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if err := h.service.ReorderCategories(r.Context(), body.Order); err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, map[string]any{"status": "reordered"})
}

func (h policyHandler) listRules(w http.ResponseWriter, r *http.Request) {
	page, pageSize, err := ParsePaginationWithLimit(r, 1000)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	res, err := h.service.ListRules(r.Context(), policy.ListRulesQuery{
		RevisionID: strings.TrimSpace(r.URL.Query().Get("revision_id")),
		Kind:       strings.TrimSpace(r.URL.Query().Get("kind")),
		CategoryID: strings.TrimSpace(r.URL.Query().Get("category_id")),
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, res)
}

func (h policyHandler) createRule(w http.ResponseWriter, r *http.Request) {
	var body createRuleRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}

	kind := strings.ToLower(strings.TrimSpace(body.Kind))
	if kind == "admission" || (kind == "" && body.Action != "") {
		admView, err := h.service.CreateAdmissionRule(r.Context(), policy.CreateAdmissionRuleCommand{
			RevisionID: body.RevisionID,
			Name:       body.Name,
			Expression: body.Expression,
			Action:     body.Action,
			Position:   body.Position,
			RequestID:  GetRequestID(r.Context()),
			ActorKind:  requestActorKind(r),
		})
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusCreated, admView)
		return
	}

	if kind != "" && kind != "policy" {
		WriteDomainError(w, r, domain.NewValidationError("invalid_rule_kind", fmt.Sprintf("unsupported rule kind: %s", body.Kind)))
		return
	}

	polView, err := h.service.CreatePolicyRule(r.Context(), policy.CreatePolicyRuleCommand{
		RevisionID:    body.RevisionID,
		CategoryID:    body.CategoryID,
		Category:      body.Category,
		Name:          body.Name,
		TargetGroupID: body.TargetGroupID,
		BuiltinTarget: body.BuiltinTarget,
		Expression:    body.Expression,
		Position:      body.Position,
		Enabled:       body.Enabled,
		RequestID:     GetRequestID(r.Context()),
		ActorKind:     requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusCreated, polView)
}

func (h policyHandler) backfillRules(w http.ResponseWriter, r *http.Request) {
	revID := strings.TrimSpace(r.URL.Query().Get("revision_id"))
	count, err := h.service.BackfillRules(r.Context(), revID)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, map[string]any{
		"count":  count,
		"status": "backfilled",
	})
}

func (h policyHandler) updateRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body updateRuleRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}

	view, err := h.service.UpdatePolicyRule(r.Context(), policy.UpdatePolicyRuleCommand{
		ID:            id,
		CategoryID:    body.CategoryID,
		Category:      body.Category,
		Name:          body.Name,
		TargetGroupID: body.TargetGroupID,
		BuiltinTarget: body.BuiltinTarget,
		Expression:    body.Expression,
		Position:      body.Position,
		Enabled:       body.Enabled,
		RequestID:     GetRequestID(r.Context()),
		ActorKind:     requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, view)
}

func (h policyHandler) reorderRules(w http.ResponseWriter, r *http.Request) {
	var body reorderRulesRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}

	if err := h.service.ReorderPolicyRules(r.Context(), policy.ReorderPolicyRulesCommand{
		RevisionID: body.RevisionID,
		CategoryID: body.CategoryID,
		Order:      body.Order,
		RequestID:  GetRequestID(r.Context()),
		ActorKind:  requestActorKind(r),
	}); err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, map[string]any{"status": "reordered"})
}

func (h policyHandler) createAdmissionRule(w http.ResponseWriter, r *http.Request) {
	var body createRuleRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}

	admView, err := h.service.CreateAdmissionRule(r.Context(), policy.CreateAdmissionRuleCommand{
		RevisionID: body.RevisionID,
		Name:       body.Name,
		Expression: body.Expression,
		Action:     body.Action,
		Position:   body.Position,
		RequestID:  GetRequestID(r.Context()),
		ActorKind:  requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusCreated, admView)
}

func (h policyHandler) deleteRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	err := h.service.DeleteRule(r.Context(), policy.DeleteRuleCommand{
		ID:        id,
		Kind:      kind,
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h policyHandler) deleteAdmissionRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.service.DeleteAdmissionRule(r.Context(), policy.DeleteRuleCommand{
		ID:        id,
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h policyHandler) validate(w http.ResponseWriter, r *http.Request) {
	res, err := h.service.ValidateGraph(r.Context())
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, res)
}
