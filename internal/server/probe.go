package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe"
	"clash-sub-parser/internal/repository"
)

type probeHandler struct {
	repos    *repository.Repositories
	probeMgr *ProbeManager
}

func newProbeHandler(repos *repository.Repositories, probeMgr *ProbeManager) *probeHandler {
	return &probeHandler{
		repos:    repos,
		probeMgr: probeMgr,
	}
}

func (h *probeHandler) status(w http.ResponseWriter, r *http.Request) {
	status := h.probeMgr.GetStatus()
	renderJSON(w, http.StatusOK, status)
}

func (h *probeHandler) start(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Concurrency    int      `json:"concurrency"`
		TimeoutSeconds int      `json:"timeout_seconds"`
		SubscriptionID int64    `json:"subscription_id"`
		NodeKeys       []string `json:"node_keys"`
	}
	_ = decodeJSON(r, &payload)

	var nodes []*domain.Node
	var err error
	if payload.SubscriptionID > 0 {
		nodes, _, err = h.repos.Nodes.ListFiltered(r.Context(), repository.NodeFilter{
			SubscriptionID: payload.SubscriptionID,
			State:          domain.LifecycleActive,
			Limit:          10000,
		})
	} else {
		nodes, err = h.repos.Nodes.List(r.Context(), domain.LifecycleActive)
	}
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get nodes: %v", err))
		return
	}

	opts := probe.DefaultOptions()
	if payload.Concurrency > 0 {
		opts.Concurrency = payload.Concurrency
	}
	if payload.TimeoutSeconds > 0 {
		opts.Timeout = time.Duration(payload.TimeoutSeconds) * time.Second
	}

	if err := h.probeMgr.Start(nodes, opts); err != nil {
		renderError(w, http.StatusConflict, err.Error())
		return
	}

	renderJSON(w, http.StatusOK, map[string]any{
		"status":  "started",
		"message": "Probe job started",
		"count":   len(nodes),
	})
}

func (h *probeHandler) results(w http.ResponseWriter, r *http.Request) {
	limit := 100
	offset := 0
	probeList, err := h.repos.Probes.List(r.Context(), limit, offset)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list probe results: %v", err))
		return
	}

	resultsMap := make(map[string]any)
	for _, p := range probeList {
		resultsMap[p.NodeKey] = map[string]any{
			"node_key":   p.NodeKey,
			"name":       p.Name,
			"status":     string(p.Status),
			"latency_ms": p.LatencyMs,
			"speed_mbps": p.SpeedMbps,
			"country":    p.Country,
			"ip":         p.IP,
			"media":      p.Media,
		}
	}

	renderJSON(w, http.StatusOK, map[string]any{
		"results": resultsMap,
		"total":   len(probeList),
	})
}

func (h *probeHandler) detail(w http.ResponseWriter, r *http.Request) {
	nodeKey := strings.TrimSpace(r.URL.Query().Get("node_key"))
	if nodeKey == "" {
		renderError(w, http.StatusBadRequest, "node_key parameter is required")
		return
	}

	res, err := h.repos.Probes.GetByNodeKey(r.Context(), nodeKey)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Probe result not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get probe result: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, res)
}

func (h *probeHandler) clearResults(w http.ResponseWriter, r *http.Request) {
	if err := h.repos.Probes.DeleteAll(r.Context()); err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete probe results: %v", err))
		return
	}
	renderJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "All probe results cleared",
	})
}
