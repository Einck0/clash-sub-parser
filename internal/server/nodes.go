package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository"
)

type nodeHandler struct {
	repos *repository.Repositories
}

func newNodeHandler(repos *repository.Repositories) *nodeHandler {
	return &nodeHandler{repos: repos}
}

func (h *nodeHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := 50
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	} else if ps := q.Get("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := q.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	} else if p := q.Get("page"); p != "" {
		if page, err := strconv.Atoi(p); err == nil && page > 1 {
			offset = (page - 1) * limit
		}
	}

	protocol := strings.TrimSpace(q.Get("protocol"))
	keyword := strings.TrimSpace(q.Get("keyword"))
	if keyword == "" {
		keyword = strings.TrimSpace(q.Get("search"))
	}

	var subID int64
	if sid := q.Get("subscription_id"); sid != "" {
		subID, _ = strconv.ParseInt(sid, 10, 64)
	} else if sid := q.Get("sub_id"); sid != "" {
		subID, _ = strconv.ParseInt(sid, 10, 64)
	}

	state := domain.LifecycleState(strings.TrimSpace(q.Get("state")))
	if state == "" {
		state = domain.LifecycleActive
	}

	filter := repository.NodeFilter{
		State:          state,
		Protocol:       protocol,
		Keyword:        keyword,
		SubscriptionID: subID,
		Limit:          limit,
		Offset:         offset,
	}

	nodes, total, err := h.repos.Nodes.ListFiltered(r.Context(), filter)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list nodes: %v", err))
		return
	}
	if nodes == nil {
		nodes = []*domain.Node{}
	}

	resp := map[string]any{
		"items":  nodes,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}
	renderJSON(w, http.StatusOK, resp)
}

func (h *nodeHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	node, err := h.repos.Nodes.GetByID(r.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Node not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get node: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, node)
}

func (h *nodeHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.repos.Nodes.Delete(r.Context(), id); err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Node not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete node: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *nodeHandler) batchDelete(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		IDs []int64 `json:"ids"`
	}

	if err := decodeJSON(r, &payload); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Sprintf("invalid json body: %v", err))
		return
	}

	if len(payload.IDs) == 0 {
		renderJSON(w, http.StatusOK, map[string]any{"deleted": 0})
		return
	}

	deleted, err := h.repos.Nodes.BatchDelete(r.Context(), payload.IDs)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to batch delete nodes: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}
