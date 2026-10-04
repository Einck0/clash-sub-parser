package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
)

type nodeHandler struct {
	service *inventory.Service
}

func registerNodeRoutes(r chi.Router, service *inventory.Service) {
	if service == nil {
		return
	}
	h := nodeHandler{service: service}
	r.Get("/nodes", h.list)
	r.Get("/nodes/{logical_id}", h.get)
	r.Get("/nodes/{logical_id}/source-history", h.getSourceHistory)
	r.Patch("/nodes/{logical_id}", h.patchConnection)
	r.Patch("/nodes/{logical_id}/connection", h.patchConnection)
}

// NodeDetailResponse represents the response envelope for single node details with provenance sources.
type NodeDetailResponse struct {
	Node          inventory.NodeView    `json:"node"`
	Sources       []domain.NodeSource   `json:"sources"`
	IPRiskSummary *domain.IPRiskSummary `json:"ip_risk_summary,omitempty"`
}

func (h nodeHandler) list(w http.ResponseWriter, r *http.Request) {
	page := 1
	pageSize := DefaultPageSize

	q := r.URL.Query()
	if pageStr := q.Get("page"); pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil || p < 1 {
			WriteDomainError(w, r, domain.NewValidationError("invalid_page", "page must be a positive integer greater than or equal to 1"))
			return
		}
		page = p
	}

	if pageSizeStr := q.Get("page_size"); pageSizeStr != "" {
		ps, err := strconv.Atoi(pageSizeStr)
		if err != nil || ps < 1 {
			WriteDomainError(w, r, domain.NewValidationError("invalid_page_size", "page_size must be a positive integer"))
			return
		}
		// Enforce upper boundary: page_size > 100 is truncated to 100
		if ps > MaxPageSize {
			ps = MaxPageSize
		}
		pageSize = ps
	}

	scopeStr := strings.TrimSpace(q.Get("scope"))
	var scope domain.NodeScope
	if scopeStr == "" {
		scope = domain.NodeScopeEnabledSubscriptions
	} else {
		parsed, err := domain.ParseNodeScope(scopeStr)
		if err != nil {
			WriteError(w, r, http.StatusBadRequest, "invalid_node_scope", err.Error())
			return
		}
		scope = parsed
	}

	filter := domain.NodeFilter{
		Pagination: domain.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
		SortBy:    strings.TrimSpace(q.Get("sort_by")),
		SortOrder: strings.TrimSpace(q.Get("sort_order")),
		Scope:     scope,
	}

	subID := strings.TrimSpace(q.Get("subscription_id"))
	if subID == "" {
		subID = strings.TrimSpace(q.Get("sub"))
	}
	filter.SubscriptionID = subID

	if activeOnlyStr := q.Get("active_only"); activeOnlyStr == "true" || activeOnlyStr == "1" {
		filter.ActiveOnly = true
	}

	search := strings.TrimSpace(q.Get("search"))
	if search == "" {
		search = strings.TrimSpace(q.Get("search_text"))
	}
	filter.SearchText = search

	if protoParams, ok := q["protocol"]; ok {
		for _, p := range protoParams {
			for _, part := range strings.Split(p, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					filter.Protocols = append(filter.Protocols, domain.Protocol(part))
				}
			}
		}
	}

	for _, value := range q["risk_decision"] {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				filter.RiskDecisions = append(filter.RiskDecisions, domain.RiskAction(part))
			}
		}
	}
	for _, value := range q["risk_band"] {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				filter.RiskBands = append(filter.RiskBands, domain.RiskBand(part))
			}
		}
	}
	for _, value := range q["risk_provider"] {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				filter.RiskProviders = append(filter.RiskProviders, part)
			}
		}
	}
	for _, value := range q["risk_status"] {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				filter.RiskStatuses = append(filter.RiskStatuses, domain.IPRiskStatus(part))
			}
		}
	}
	for _, value := range q["policy_revision"] {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				filter.RiskPolicyRevisions = append(filter.RiskPolicyRevisions, part)
			}
		}
	}

	items, total, err := h.service.ListNodesReadModel(r.Context(), filter)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WritePaginated(w, r, items, page, pageSize, total)
}

func (h nodeHandler) get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	logicalID := chi.URLParam(r, "logical_id")
	if strings.TrimSpace(logicalID) == "" {
		WriteDomainError(w, r, domain.NewValidationError("missing_logical_id", "logical_id is required"))
		return
	}

	detail, err := h.service.GetNodeDetailWithRisk(r.Context(), logicalID, "")
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	sources := detail.Sources
	if sources == nil {
		sources = make([]domain.NodeSource, 0)
	}

	nodeView := detail.ToNodeView()
	resp := NodeDetailResponse{
		Node:          nodeView,
		Sources:       sources,
		IPRiskSummary: detail.IPRiskSummary,
	}

	WriteSuccess(w, r, http.StatusOK, resp)
}

func (h nodeHandler) getSourceHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	logicalID := chi.URLParam(r, "logical_id")
	if logicalID == "" {
		logicalID = chi.URLParam(r, "id")
	}
	logicalID = strings.TrimSpace(logicalID)
	if logicalID == "" {
		WriteDomainError(w, r, domain.NewValidationError("missing_logical_id", "node logical_id is required"))
		return
	}

	data, err := h.service.GetNodeSourceHistory(r.Context(), logicalID)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, data)
}

func (h nodeHandler) patchConnection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	logicalID := strings.TrimSpace(chi.URLParam(r, "logical_id"))
	if logicalID == "" {
		WriteDomainError(w, r, domain.NewValidationError("missing_logical_id", "logical_id is required"))
		return
	}

	var body inventory.NodePatchRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		WriteDomainError(w, r, domain.NewValidationError("invalid_json", "request body must be valid JSON"))
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		WriteDomainError(w, r, domain.NewValidationError("invalid_json", "request body must contain one JSON object"))
		return
	}

	detail, err := h.service.UpdateNodeConnection(r.Context(), inventory.UpdateNodeConnectionCommand{
		LogicalID: logicalID,
		Patch:     body,
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	sources := detail.Sources
	if sources == nil {
		sources = make([]domain.NodeSource, 0)
	}
	nodeView := detail.ToNodeView()
	resp := NodeDetailResponse{
		Node:          nodeView,
		Sources:       sources,
		IPRiskSummary: detail.IPRiskSummary,
	}

	WriteSuccess(w, r, http.StatusOK, resp)
}
