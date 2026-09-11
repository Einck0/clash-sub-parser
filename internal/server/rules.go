package server

import (
	"fmt"
	"net/http"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository"
)

type ruleHandler struct {
	repos *repository.Repositories
}

func newRuleHandler(repos *repository.Repositories) *ruleHandler {
	return &ruleHandler{repos: repos}
}

func (h *ruleHandler) list(w http.ResponseWriter, r *http.Request) {
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	var rules []*domain.Rule
	var err error

	if category != "" {
		rules, err = h.repos.Rules.ListByCategory(r.Context(), category)
	} else {
		rules, err = h.repos.Rules.List(r.Context(), false)
	}

	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list rules: %v", err))
		return
	}
	if rules == nil {
		rules = []*domain.Rule{}
	}
	renderJSON(w, http.StatusOK, rules)
}

func (h *ruleHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	rule, err := h.repos.Rules.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Rule not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get rule: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, rule)
}

func (h *ruleHandler) create(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name      string   `json:"name"`
		Category  string   `json:"category"`
		Type      string   `json:"type"`
		Value     string   `json:"value"`
		Proxy     string   `json:"proxy"`
		Options   []string `json:"options"`
		SortOrder int      `json:"sort_order"`
		Enabled   *bool    `json:"enabled"`
	}

	if err := decodeJSON(r, &payload); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Sprintf("invalid json body: %v", err))
		return
	}

	ruleType := domain.RuleType(strings.ToUpper(strings.TrimSpace(payload.Type)))
	if ruleType == "" {
		renderError(w, http.StatusBadRequest, "Rule type is required")
		return
	}
	if strings.TrimSpace(payload.Proxy) == "" {
		renderError(w, http.StatusBadRequest, "Rule proxy target is required")
		return
	}

	category := strings.TrimSpace(payload.Category)
	if category == "" {
		category = "default"
	}

	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}

	rule := &domain.Rule{
		Name:      strings.TrimSpace(payload.Name),
		Category:  category,
		Type:      ruleType,
		Value:     strings.TrimSpace(payload.Value),
		Proxy:     strings.TrimSpace(payload.Proxy),
		Options:   payload.Options,
		SortOrder: payload.SortOrder,
		Enabled:   enabled,
	}

	if err := h.repos.Rules.Create(r.Context(), rule); err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create rule: %v", err))
		return
	}

	renderJSON(w, http.StatusCreated, rule)
}

func (h *ruleHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	rule, err := h.repos.Rules.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Rule not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get rule: %v", err))
		return
	}

	var payload map[string]any
	if err := decodeJSON(r, &payload); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Sprintf("invalid json body: %v", err))
		return
	}

	if name, ok := payload["name"].(string); ok {
		rule.Name = strings.TrimSpace(name)
	}
	if category, ok := payload["category"].(string); ok && strings.TrimSpace(category) != "" {
		rule.Category = strings.TrimSpace(category)
	}
	if rType, ok := payload["type"].(string); ok && strings.TrimSpace(rType) != "" {
		rule.Type = domain.RuleType(strings.ToUpper(strings.TrimSpace(rType)))
	}
	if val, ok := payload["value"].(string); ok {
		rule.Value = strings.TrimSpace(val)
	}
	if proxy, ok := payload["proxy"].(string); ok && strings.TrimSpace(proxy) != "" {
		rule.Proxy = strings.TrimSpace(proxy)
	}
	if sortOrder, ok := payload["sort_order"].(float64); ok {
		rule.SortOrder = int(sortOrder)
	}
	if enabled, ok := payload["enabled"].(bool); ok {
		rule.Enabled = enabled
	}

	if err := h.repos.Rules.Update(r.Context(), rule); err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update rule: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, rule)
}

func (h *ruleHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.repos.Rules.Delete(r.Context(), id); err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Rule not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete rule: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ruleHandler) reorder(w http.ResponseWriter, r *http.Request) {
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
		rule, err := h.repos.Rules.GetByID(ctx, item.ID)
		if err == nil && rule != nil {
			rule.SortOrder = item.SortOrder
			_ = h.repos.Rules.Update(ctx, rule)
		}
	}

	rules, _ := h.repos.Rules.List(ctx, false)
	if rules == nil {
		rules = []*domain.Rule{}
	}
	renderJSON(w, http.StatusOK, rules)
}

func (h *ruleHandler) batch(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Delete []int64 `json:"delete"`
		Create []struct {
			Name      string   `json:"name"`
			Category  string   `json:"category"`
			Type      string   `json:"type"`
			Value     string   `json:"value"`
			Proxy     string   `json:"proxy"`
			Options   []string `json:"options"`
			SortOrder int      `json:"sort_order"`
			Enabled   *bool    `json:"enabled"`
		} `json:"create"`
		Update []struct {
			ID        int64    `json:"id"`
			Name      *string  `json:"name"`
			Category  *string  `json:"category"`
			Type      *string  `json:"type"`
			Value     *string  `json:"value"`
			Proxy     *string  `json:"proxy"`
			Options   []string `json:"options"`
			SortOrder *int     `json:"sort_order"`
			Enabled   *bool    `json:"enabled"`
		} `json:"update"`
	}

	if err := decodeJSON(r, &payload); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Sprintf("invalid json body: %v", err))
		return
	}

	ctx := r.Context()

	// 1. Deletions
	for _, id := range payload.Delete {
		_ = h.repos.Rules.Delete(ctx, id)
	}

	// 2. Creates
	for _, item := range payload.Create {
		category := strings.TrimSpace(item.Category)
		if category == "" {
			category = "default"
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		newRule := &domain.Rule{
			Name:      strings.TrimSpace(item.Name),
			Category:  category,
			Type:      domain.RuleType(strings.ToUpper(strings.TrimSpace(item.Type))),
			Value:     strings.TrimSpace(item.Value),
			Proxy:     strings.TrimSpace(item.Proxy),
			Options:   item.Options,
			SortOrder: item.SortOrder,
			Enabled:   enabled,
		}
		_ = h.repos.Rules.Create(ctx, newRule)
	}

	// 3. Updates
	for _, item := range payload.Update {
		existing, err := h.repos.Rules.GetByID(ctx, item.ID)
		if err == nil && existing != nil {
			if item.Name != nil {
				existing.Name = strings.TrimSpace(*item.Name)
			}
			if item.Category != nil && strings.TrimSpace(*item.Category) != "" {
				existing.Category = strings.TrimSpace(*item.Category)
			}
			if item.Type != nil && strings.TrimSpace(*item.Type) != "" {
				existing.Type = domain.RuleType(strings.ToUpper(strings.TrimSpace(*item.Type)))
			}
			if item.Value != nil {
				existing.Value = strings.TrimSpace(*item.Value)
			}
			if item.Proxy != nil && strings.TrimSpace(*item.Proxy) != "" {
				existing.Proxy = strings.TrimSpace(*item.Proxy)
			}
			if item.SortOrder != nil {
				existing.SortOrder = *item.SortOrder
			}
			if item.Enabled != nil {
				existing.Enabled = *item.Enabled
			}
			_ = h.repos.Rules.Update(ctx, existing)
		}
	}

	rules, _ := h.repos.Rules.List(ctx, false)
	if rules == nil {
		rules = []*domain.Rule{}
	}
	renderJSON(w, http.StatusOK, rules)
}
