package http

import (
	"encoding/json"
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
	ID         string                 `json:"id,omitempty"`
	Name       string                 `json:"name"`
	GroupType  domain.GroupType       `json:"group_type"`
	Edges      []policy.EdgeInput     `json:"edges,omitempty"`
	NodeFilter *domain.NodeFilterSpec `json:"node_filter,omitempty"`
}

type updateGroupRequest struct {
	Name       *string                `json:"name,omitempty"`
	GroupType  *domain.GroupType      `json:"group_type,omitempty"`
	Edges      *[]policy.EdgeInput    `json:"edges,omitempty"`
	NodeFilter *domain.NodeFilterSpec `json:"node_filter,omitempty"`
}

type setGlobalFilterRequest struct {
	Spec       *domain.NodeFilterSpec   `json:"spec,omitempty"`
	Conditions []domain.FilterCondition `json:"conditions,omitempty"`
}

type setGroupEdgesRequest struct {
	Edges []policy.EdgeInput `json:"edges"`
}

type createRuleRequest struct {
	Kind          string            `json:"kind,omitempty"` // "policy" or "admission"
	RevisionID    string            `json:"revision_id,omitempty"`
	TargetGroupID string            `json:"target_group_id,omitempty"`
	Name          string            `json:"name,omitempty"`
	Expression    string            `json:"expression"`
	Action        domain.RuleAction `json:"action,omitempty"`
	Position      int               `json:"position"`
}

type validateGraphRequest struct {
	Groups         []domain.NodeGroup            `json:"groups,omitempty"`
	Edges          map[string][]domain.GroupEdge `json:"edges,omitempty"`
	PolicyRules    []domain.PolicyRule           `json:"policy_rules,omitempty"`
	AdmissionRules []domain.AdmissionRule        `json:"admission_rules,omitempty"`
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
		sub.Get("/groups/{id}", h.getGroup)
		sub.Patch("/groups/{id}", h.updateGroup)
		sub.Put("/groups/{id}", h.updateGroup)
		sub.Delete("/groups/{id}", h.deleteGroup)
		sub.Put("/groups/{id}/edges", h.setGroupEdges)
		sub.Post("/groups/{id}/edges", h.setGroupEdges)

		sub.Get("/rules", h.listRules)
		sub.Post("/rules", h.createRule)

		sub.Post("/validate", h.validate)
	}

	r.Route("/policies", registerGroup)
	r.Route("/policy", registerGroup)
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
	page, pageSize, err := ParsePagination(r)
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
		ID:         body.ID,
		Name:       body.Name,
		GroupType:  body.GroupType,
		Edges:      body.Edges,
		NodeFilter: body.NodeFilter,
		RequestID:  GetRequestID(r.Context()),
		ActorKind:  requestActorKind(r),
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
		ID:        id,
		Name:      body.Name,
		GroupType: body.GroupType,
		Edges:     body.Edges,
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
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

func (h policyHandler) listRules(w http.ResponseWriter, r *http.Request) {
	page, pageSize, err := ParsePagination(r)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	res, err := h.service.ListRules(r.Context(), policy.ListRulesQuery{
		RevisionID: strings.TrimSpace(r.URL.Query().Get("revision_id")),
		Kind:       strings.TrimSpace(r.URL.Query().Get("kind")),
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

	polView, err := h.service.CreatePolicyRule(r.Context(), policy.CreatePolicyRuleCommand{
		RevisionID:    body.RevisionID,
		TargetGroupID: body.TargetGroupID,
		Expression:    body.Expression,
		Position:      body.Position,
		RequestID:     GetRequestID(r.Context()),
		ActorKind:     requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusCreated, polView)
}

func (h policyHandler) validate(w http.ResponseWriter, r *http.Request) {
	var body validateGraphRequest
	// Try to decode optional request body, ignore if empty or invalid
	_ = decodeJSON(w, r, &body)

	if len(body.Groups) > 0 {
		if err := policy.ValidatePolicyGraph(body.Groups, body.Edges, body.PolicyRules, body.AdmissionRules); err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, policy.ValidationResult{
			Valid:  true,
			Errors: nil,
		})
		return
	}

	res, err := h.service.ValidateGraph(r.Context())
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, res)
}
