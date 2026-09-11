package server

import (
	"fmt"
	"net/http"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository"
)

type groupHandler struct {
	repos *repository.Repositories
}

func newGroupHandler(repos *repository.Repositories) *groupHandler {
	return &groupHandler{repos: repos}
}

func (h *groupHandler) list(w http.ResponseWriter, r *http.Request) {
	groups, err := h.repos.NodeGroups.List(r.Context())
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list node groups: %v", err))
		return
	}
	if groups == nil {
		groups = []*domain.NodeGroup{}
	}
	renderJSON(w, http.StatusOK, groups)
}

func (h *groupHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	group, err := h.repos.NodeGroups.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Node group not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get node group: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, group)
}

func (h *groupHandler) create(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name                 string                     `json:"name"`
		Kind                 string                     `json:"kind"`
		GroupType            string                     `json:"group_type"`
		SortOrder            int                        `json:"sort_order"`
		RegexRules           []string                   `json:"regex_rules"`
		FilterMinSpeedMbps   *float64                   `json:"filter_min_speed_mbps"`
		FilterMediaUnlock    []string                   `json:"filter_media_unlock"`
		IncludeNodes         []string                   `json:"include_nodes"`
		IncludeGroupIDs      []int64                    `json:"include_group_ids"`
		IncludeGroupNodesIDs []int64                    `json:"include_group_nodes_ids"`
		IncludeEntries       []domain.GroupIncludeEntry `json:"include_entries"`
		AddFallback          bool                       `json:"add_fallback"`
		ExcludeNodes         []string                   `json:"exclude_nodes"`
		ExcludeGroupIDs      []int64                    `json:"exclude_group_ids"`
		URLTestConfig        domain.URLTestConfig       `json:"url_test_config"`
		LoadBalanceConfig    domain.LoadBalanceConfig   `json:"load_balance_config"`
		FallbackConfig       domain.FallbackConfig      `json:"fallback_config"`
	}

	if err := decodeJSON(r, &payload); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Sprintf("invalid json body: %v", err))
		return
	}

	if strings.TrimSpace(payload.Name) == "" {
		renderError(w, http.StatusBadRequest, "Node group name is required")
		return
	}

	gType := domain.GroupType(strings.TrimSpace(payload.GroupType))
	if gType == "" {
		gType = domain.GroupTypeSelect
	}

	kind := domain.GroupKind(strings.TrimSpace(payload.Kind))
	if kind == "" {
		kind = domain.GroupKindManual
	}

	group := &domain.NodeGroup{
		Name:                 strings.TrimSpace(payload.Name),
		Kind:                 kind,
		GroupType:            gType,
		SortOrder:            payload.SortOrder,
		RegexRules:           payload.RegexRules,
		FilterMinSpeedMbps:   payload.FilterMinSpeedMbps,
		FilterMediaUnlock:    payload.FilterMediaUnlock,
		IncludeNodes:         payload.IncludeNodes,
		IncludeGroupIDs:      payload.IncludeGroupIDs,
		IncludeGroupNodesIDs: payload.IncludeGroupNodesIDs,
		IncludeEntries:       payload.IncludeEntries,
		AddFallback:          payload.AddFallback,
		ExcludeNodes:         payload.ExcludeNodes,
		ExcludeGroupIDs:      payload.ExcludeGroupIDs,
		URLTestConfig:        payload.URLTestConfig,
		LoadBalanceConfig:    payload.LoadBalanceConfig,
		FallbackConfig:       payload.FallbackConfig,
	}

	if err := h.repos.NodeGroups.Create(r.Context(), group); err != nil {
		if err == repository.ErrDuplicate {
			renderError(w, http.StatusConflict, "Node group with this name already exists")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create node group: %v", err))
		return
	}

	renderJSON(w, http.StatusCreated, group)
}

func (h *groupHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	group, err := h.repos.NodeGroups.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Node group not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get node group: %v", err))
		return
	}

	var payload map[string]any
	if err := decodeJSON(r, &payload); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Sprintf("invalid json body: %v", err))
		return
	}

	if name, ok := payload["name"].(string); ok && strings.TrimSpace(name) != "" {
		group.Name = strings.TrimSpace(name)
	}
	if kind, ok := payload["kind"].(string); ok && strings.TrimSpace(kind) != "" {
		group.Kind = domain.GroupKind(strings.TrimSpace(kind))
	}
	if gt, ok := payload["group_type"].(string); ok && strings.TrimSpace(gt) != "" {
		group.GroupType = domain.GroupType(strings.TrimSpace(gt))
	}
	if sortOrder, ok := payload["sort_order"].(float64); ok {
		group.SortOrder = int(sortOrder)
	}
	if addFallback, ok := payload["add_fallback"].(bool); ok {
		group.AddFallback = addFallback
	}
	if incGroupIDs, ok := payload["include_group_ids"].([]any); ok {
		var ids []int64
		for _, v := range incGroupIDs {
			if num, ok := v.(float64); ok {
				ids = append(ids, int64(num))
			}
		}
		group.IncludeGroupIDs = ids
	}
	if excGroupIDs, ok := payload["exclude_group_ids"].([]any); ok {
		var ids []int64
		for _, v := range excGroupIDs {
			if num, ok := v.(float64); ok {
				ids = append(ids, int64(num))
			}
		}
		group.ExcludeGroupIDs = ids
	}

	if err := h.repos.NodeGroups.Update(r.Context(), group); err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update node group: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, group)
}

func (h *groupHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.repos.NodeGroups.Delete(r.Context(), id); err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Node group not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete node group: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *groupHandler) reorder(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Items []struct {
			ID        int64 `json:"id"`
			SortOrder int   `json:"sort_order"`
		} `json:"items"`
	}

	if err := decodeJSON(r, &payload); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Sprintf("invalid json body: %v", err))
		return
	}

	ctx := r.Context()
	for _, item := range payload.Items {
		grp, err := h.repos.NodeGroups.GetByID(ctx, item.ID)
		if err == nil && grp != nil {
			grp.SortOrder = item.SortOrder
			_ = h.repos.NodeGroups.Update(ctx, grp)
		}
	}

	groups, _ := h.repos.NodeGroups.List(ctx)
	if groups == nil {
		groups = []*domain.NodeGroup{}
	}
	renderJSON(w, http.StatusOK, groups)
}

func (h *groupHandler) validate(w http.ResponseWriter, r *http.Request) {
	groups, err := h.repos.NodeGroups.List(r.Context())
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list groups: %v", err))
		return
	}

	if err := validateGroupGraph(groups); err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	renderJSON(w, http.StatusOK, map[string]any{
		"valid":   true,
		"message": "All node groups are valid",
	})
}

// validateGroupGraph checks self-inclusion, existence of referenced groups, and circular references.
func validateGroupGraph(groups []*domain.NodeGroup) error {
	groupMap := make(map[int64]*domain.NodeGroup)
	for _, g := range groups {
		groupMap[g.ID] = g
	}

	adj := make(map[int64][]int64)

	for _, g := range groups {
		var referenced []int64
		referenced = append(referenced, g.IncludeGroupIDs...)
		referenced = append(referenced, g.ExcludeGroupIDs...)
		referenced = append(referenced, g.IncludeGroupNodesIDs...)

		for _, refID := range referenced {
			if refID == g.ID {
				return fmt.Errorf("Node group cannot include itself")
			}
			if _, exists := groupMap[refID]; !exists {
				return fmt.Errorf("Referenced node groups do not exist")
			}
			adj[g.ID] = append(adj[g.ID], refID)
		}
	}

	// Detect cycles using DFS (0: unvisited, 1: visiting, 2: visited)
	visited := make(map[int64]int)

	var hasCycle func(curr int64) bool
	hasCycle = func(curr int64) bool {
		visited[curr] = 1
		for _, neighbor := range adj[curr] {
			if visited[neighbor] == 1 {
				return true
			}
			if visited[neighbor] == 0 {
				if hasCycle(neighbor) {
					return true
				}
			}
		}
		visited[curr] = 2
		return false
	}

	for _, g := range groups {
		if visited[g.ID] == 0 {
			if hasCycle(g.ID) {
				return fmt.Errorf("Circular node group reference detected")
			}
		}
	}

	return nil
}
