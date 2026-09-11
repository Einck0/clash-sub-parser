package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository"
)

type subscriptionHandler struct {
	repos *repository.Repositories
}

func newSubscriptionHandler(repos *repository.Repositories) *subscriptionHandler {
	return &subscriptionHandler{repos: repos}
}

func (h *subscriptionHandler) list(w http.ResponseWriter, r *http.Request) {
	subs, err := h.repos.Subscriptions.List(r.Context(), false)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list subscriptions: %v", err))
		return
	}
	if subs == nil {
		subs = []*domain.Subscription{}
	}
	renderJSON(w, http.StatusOK, subs)
}

func (h *subscriptionHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	sub, err := h.repos.Subscriptions.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Subscription not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get subscription: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, sub)
}

func (h *subscriptionHandler) create(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name                  string            `json:"name"`
		URL                   string            `json:"url"`
		UpdateInterval        int               `json:"update_interval"`
		IsPrimary             bool              `json:"is_primary"`
		Enabled               *bool             `json:"enabled"`
		NodePrefix            string            `json:"node_prefix"`
		FilterRegex           []string          `json:"filter_regex"`
		FilterMinSpeedMbps    *float64          `json:"filter_min_speed_mbps"`
		FilterMediaUnlock     []string          `json:"filter_media_unlock"`
		IncludeNodeNames      []string          `json:"include_node_names"`
		ExcludeNodeNames      []string          `json:"exclude_node_names"`
		NodeRenames           map[string]string `json:"node_renames"`
		ManualNodes           []*domain.Node    `json:"manual_nodes"`
		SubscriptionUserinfo  string            `json:"subscription_userinfo"`
		ProfileUpdateInterval string            `json:"profile_update_interval"`
		ProfileWebPageURL     string            `json:"profile_web_page_url"`
		ProxyChain            string            `json:"proxy_chain"`
		NodeProxyChains       map[string]string `json:"node_proxy_chains"`
	}

	if err := decodeJSON(r, &payload); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Sprintf("invalid json body: %v", err))
		return
	}

	if strings.TrimSpace(payload.Name) == "" {
		renderError(w, http.StatusBadRequest, "Subscription name is required")
		return
	}
	if strings.TrimSpace(payload.URL) == "" {
		renderError(w, http.StatusBadRequest, "Subscription url is required")
		return
	}

	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}

	sub := &domain.Subscription{
		Name:                  strings.TrimSpace(payload.Name),
		URL:                   strings.TrimSpace(payload.URL),
		UpdateInterval:        payload.UpdateInterval,
		IsPrimary:             payload.IsPrimary,
		Enabled:               enabled,
		NodePrefix:            payload.NodePrefix,
		FilterRegex:           payload.FilterRegex,
		FilterMinSpeedMbps:    payload.FilterMinSpeedMbps,
		FilterMediaUnlock:     payload.FilterMediaUnlock,
		IncludeNodeNames:      payload.IncludeNodeNames,
		ExcludeNodeNames:      payload.ExcludeNodeNames,
		NodeRenames:           payload.NodeRenames,
		ManualNodes:           payload.ManualNodes,
		SubscriptionUserinfo:  payload.SubscriptionUserinfo,
		ProfileUpdateInterval: payload.ProfileUpdateInterval,
		ProfileWebPageURL:     payload.ProfileWebPageURL,
		ProxyChain:            payload.ProxyChain,
		NodeProxyChains:       payload.NodeProxyChains,
	}

	if err := h.repos.Subscriptions.Create(r.Context(), sub); err != nil {
		if err == repository.ErrDuplicate {
			renderError(w, http.StatusConflict, "Subscription with this name already exists")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create subscription: %v", err))
		return
	}

	renderJSON(w, http.StatusCreated, sub)
}

func (h *subscriptionHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	sub, err := h.repos.Subscriptions.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Subscription not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get subscription: %v", err))
		return
	}

	var payload map[string]any
	if err := decodeJSON(r, &payload); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Sprintf("invalid json body: %v", err))
		return
	}

	if name, ok := payload["name"].(string); ok && strings.TrimSpace(name) != "" {
		sub.Name = strings.TrimSpace(name)
	}
	if url, ok := payload["url"].(string); ok && strings.TrimSpace(url) != "" {
		sub.URL = strings.TrimSpace(url)
	}
	if interval, ok := payload["update_interval"].(float64); ok {
		sub.UpdateInterval = int(interval)
	}
	if isPrimary, ok := payload["is_primary"].(bool); ok {
		sub.IsPrimary = isPrimary
	}
	if enabled, ok := payload["enabled"].(bool); ok {
		sub.Enabled = enabled
	}
	if nodePrefix, ok := payload["node_prefix"].(string); ok {
		sub.NodePrefix = nodePrefix
	}
	if renames, ok := payload["node_renames"].(map[string]any); ok {
		m := make(map[string]string)
		for k, v := range renames {
			if vs, ok := v.(string); ok {
				m[k] = vs
			}
		}
		sub.NodeRenames = m
	}
	if filterMinSpeed, ok := payload["filter_min_speed_mbps"].(float64); ok {
		sub.FilterMinSpeedMbps = &filterMinSpeed
	}

	if err := h.repos.Subscriptions.Update(r.Context(), sub); err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update subscription: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, sub)
}

func (h *subscriptionHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.repos.Subscriptions.Delete(r.Context(), id); err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Subscription not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete subscription: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *subscriptionHandler) refresh(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	sub, err := h.repos.Subscriptions.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Subscription not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get subscription: %v", err))
		return
	}

	now := time.Now().UTC()
	sub.LastFetchedAt = &now

	// If HTTP(S) URL, attempt quick live fetch if reachable
	if strings.HasPrefix(sub.URL, "http://") || strings.HasPrefix(sub.URL, "https://") {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		req, reqErr := http.NewRequestWithContext(ctx, "GET", sub.URL, nil)
		if reqErr == nil {
			req.Header.Set("User-Agent", "Clash-Sub-Parser/1.0.0-Go")
			resp, httpErr := http.DefaultClient.Do(req)
			if httpErr != nil {
				sub.LastFetchError = httpErr.Error()
				sub.FetchFailedCount++
			} else {
				defer resp.Body.Close()
				if userinfo := resp.Header.Get("subscription-userinfo"); userinfo != "" {
					sub.SubscriptionUserinfo = userinfo
				}
				if interval := resp.Header.Get("profile-update-interval"); interval != "" {
					sub.ProfileUpdateInterval = interval
				}
				sub.LastFetchError = ""
			}
		}
	}

	_ = h.repos.Subscriptions.Update(r.Context(), sub)
	renderJSON(w, http.StatusOK, sub)
}

func (h *subscriptionHandler) getNodes(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	sub, err := h.repos.Subscriptions.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Subscription not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get subscription: %v", err))
		return
	}

	nodes := sub.RawNodes
	if nodes == nil {
		nodes = sub.SourceNodes
	}
	if nodes == nil {
		nodes = []*domain.Node{}
	}
	renderJSON(w, http.StatusOK, nodes)
}

func (h *subscriptionHandler) getAllNodes(w http.ResponseWriter, r *http.Request) {
	subs, err := h.repos.Subscriptions.List(r.Context(), true)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list subscriptions: %v", err))
		return
	}

	var allNodes []*domain.Node
	for _, s := range subs {
		if len(s.RawNodes) > 0 {
			allNodes = append(allNodes, s.RawNodes...)
		} else if len(s.SourceNodes) > 0 {
			allNodes = append(allNodes, s.SourceNodes...)
		}
	}
	if allNodes == nil {
		allNodes = []*domain.Node{}
	}
	renderJSON(w, http.StatusOK, allNodes)
}
