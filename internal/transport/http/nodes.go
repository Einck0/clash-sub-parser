package http

import (
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

	filter := domain.NodeFilter{
		Pagination: domain.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
		SortBy:    strings.TrimSpace(q.Get("sort_by")),
		SortOrder: strings.TrimSpace(q.Get("sort_order")),
	}

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

	nodeView := inventory.ToNodeView(detail.Node)
	nodeView.IPRiskSummary = detail.IPRiskSummary
	resp := NodeDetailResponse{
		Node:    nodeView,
		Sources: sources,
	}

	WriteSuccess(w, r, http.StatusOK, resp)
}
