package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository"
	"clash-sub-parser/internal/service"
)

type generateHandler struct {
	repos    *repository.Repositories
	compiler compiler.Compiler
	genSvc   interface {
		service.GenerateService
		GenerateResult(ctx context.Context, req *domain.GenerateRequest, subID *int64, filterKeyword string) (*compiler.Result, error)
		GetPrimarySubscriptionHeaders(ctx context.Context) (map[string]string, error)
		BuildQuickExport(ctx context.Context, baseURL string, token string, subID *int64, targetFilter string) (map[string]any, error)
	}
}

func newGenerateHandler(repos *repository.Repositories, comp compiler.Compiler) *generateHandler {
	genSvc := service.NewGenerateService(repos, comp)
	return &generateHandler{
		repos:    repos,
		compiler: comp,
		genSvc: genSvc.(interface {
			service.GenerateService
			GenerateResult(ctx context.Context, req *domain.GenerateRequest, subID *int64, filterKeyword string) (*compiler.Result, error)
			GetPrimarySubscriptionHeaders(ctx context.Context) (map[string]string, error)
			BuildQuickExport(ctx context.Context, baseURL string, token string, subID *int64, targetFilter string) (map[string]any, error)
		}),
	}
}

func (h *generateHandler) exportRoot(target domain.ExportTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.handleExport(w, r, target, nil, "inline")
	}
}

func (h *generateHandler) deprecatedScript(w http.ResponseWriter, r *http.Request) {
	renderError(w, http.StatusNotFound, "SCRIPT format has been deprecated and removed")
}

func (h *generateHandler) exportAPI(w http.ResponseWriter, r *http.Request) {
	targetParam := r.URL.Query().Get("target")
	if strings.TrimSpace(targetParam) == "" {
		targetParam = "clash"
	}
	if strings.ToLower(strings.TrimSpace(targetParam)) == "script" {
		h.deprecatedScript(w, r)
		return
	}
	h.handleExport(w, r, domain.ExportTarget(targetParam), nil, "inline")
}

func (h *generateHandler) exportTargetInline(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "target")
	if strings.ToLower(strings.TrimSpace(target)) == "script" {
		h.deprecatedScript(w, r)
		return
	}
	h.handleExport(w, r, domain.ExportTarget(target), nil, "inline")
}

func (h *generateHandler) exportTargetCurrent(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "target")
	if strings.ToLower(strings.TrimSpace(target)) == "script" {
		h.deprecatedScript(w, r)
		return
	}
	h.handleExport(w, r, domain.ExportTarget(target), nil, "attachment")
}

func (h *generateHandler) exportTargetDownload(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "target")
	if strings.ToLower(strings.TrimSpace(target)) == "script" {
		h.deprecatedScript(w, r)
		return
	}
	h.handleExport(w, r, domain.ExportTarget(target), nil, "attachment")
}

func (h *generateHandler) generateTargetPost(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "target")
	if strings.ToLower(strings.TrimSpace(target)) == "script" {
		h.deprecatedScript(w, r)
		return
	}

	var switches map[string]bool
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&switches)
	}

	req := domain.DefaultGenerateRequest(domain.ExportTarget(target))
	if switches != nil {
		req.Switches = switches
	}

	res, err := h.genSvc.GenerateResult(r.Context(), req, nil, "")
	if err != nil {
		if strings.Contains(err.Error(), "unsupported export target") {
			renderError(w, http.StatusBadRequest, fmt.Sprintf("Unsupported target: %s", target))
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("generate failed: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, map[string]any{
		"target":  target,
		"content": string(res.Content),
	})
}

func (h *generateHandler) exportSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	target := r.URL.Query().Get("target")
	if strings.TrimSpace(target) == "" {
		target = "clash"
	}
	if strings.ToLower(strings.TrimSpace(target)) == "script" {
		h.deprecatedScript(w, r)
		return
	}

	h.handleExport(w, r, domain.ExportTarget(target), &id, "inline")
}

func (h *generateHandler) generateSubscriptionPost(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}

	target := r.URL.Query().Get("target")
	if strings.TrimSpace(target) == "" {
		target = "clash"
	}
	if strings.ToLower(strings.TrimSpace(target)) == "script" {
		h.deprecatedScript(w, r)
		return
	}

	req := domain.DefaultGenerateRequest(domain.ExportTarget(target))
	res, err := h.genSvc.GenerateResult(r.Context(), req, &id, "")
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Subscription not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("generate subscription failed: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, map[string]any{
		"target":          target,
		"subscription_id": id,
		"content":         string(res.Content),
	})
}

func (h *generateHandler) handleExport(
	w http.ResponseWriter,
	r *http.Request,
	target domain.ExportTarget,
	subID *int64,
	disposition string,
) {
	norm := strings.ToLower(strings.TrimSpace(string(target)))
	if norm == "yaml" {
		norm = "clash"
	}

	req := domain.DefaultGenerateRequest(domain.ExportTarget(norm))
	if inc := r.URL.Query().Get("include_unchecked"); inc != "" {
		req.IncludeUnchecked = (inc == "true" || inc == "1")
	}
	if grp := r.URL.Query().Get("group"); grp != "" {
		req.FilterGroups = strings.Split(grp, ",")
	}

	filterKeyword := r.URL.Query().Get("filter")

	res, err := h.genSvc.GenerateResult(r.Context(), req, subID, filterKeyword)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported export target") {
			renderError(w, http.StatusBadRequest, fmt.Sprintf("Unsupported target: %s", target))
			return
		}
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Subscription not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("export error: %v", err))
		return
	}

	if norm == "clash" || norm == "yaml" {
		res.Filename = "config.yaml"
	}

	h.serveConfig(w, r, res, disposition)
}

func (h *generateHandler) serveConfig(
	w http.ResponseWriter,
	r *http.Request,
	res *compiler.Result,
	disposition string,
) {
	// 1. Calculate ETag
	sum := sha256.Sum256(res.Content)
	etag := fmt.Sprintf("\"%x\"", sum)

	// 2. Check If-None-Match
	inm := r.Header.Get("If-None-Match")
	if inm != "" {
		cleanINM := strings.Trim(inm, "\"")
		cleanETag := strings.Trim(etag, "\"")
		if cleanINM == cleanETag || inm == etag || inm == "*" {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	// 3. Set standard response headers
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", res.ContentType)
	w.Header().Set("Cache-Control", "private, no-cache")

	filename := res.Filename
	if filename == "" {
		filename = "config.yaml"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=\"%s\"", disposition, filename))

	// 4. Primary subscription headers
	if subHeaders, err := h.genSvc.GetPrimarySubscriptionHeaders(r.Context()); err == nil {
		for k, v := range subHeaders {
			if v != "" {
				w.Header().Set(k, v)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.Content)
}

func (h *generateHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.genSvc.GetConfig(r.Context())
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get config: %v", err))
		return
	}
	renderJSON(w, http.StatusOK, cfg)
}

func (h *generateHandler) patchSettings(w http.ResponseWriter, r *http.Request) {
	current, err := h.genSvc.GetConfig(r.Context())
	if err != nil {
		current = domain.DefaultGenerateConfig()
	}

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		renderError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if v, ok := updates["enabled"].(bool); ok {
		current.Enabled = v
	}
	if v, ok := updates["subscriptions"].(bool); ok {
		current.Subscriptions = v
	}
	if v, ok := updates["node_groups"].(bool); ok {
		current.NodeGroups = v
	}
	if v, ok := updates["rules"].(bool); ok {
		current.Rules = v
	}
	if v, ok := updates["dns"].(bool); ok {
		current.DNS = v
	}
	if v, ok := updates["exclude_node_proxies"].(bool); ok {
		current.ExcludeNodeProxies = v
	}

	if err := h.genSvc.UpdateConfig(r.Context(), current); err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update config: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, current)
}

func (h *generateHandler) quickExport(w http.ResponseWriter, r *http.Request) {
	var subID *int64
	if sIDStr := r.URL.Query().Get("subscription_id"); sIDStr != "" {
		if id, err := strconv.ParseInt(sIDStr, 10, 64); err == nil {
			subID = &id
		}
	}

	target := r.URL.Query().Get("target")
	token := r.URL.Query().Get("token")

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, host)

	resp, err := h.genSvc.BuildQuickExport(r.Context(), baseURL, token, subID, target)
	if err != nil {
		if err == repository.ErrNotFound {
			renderError(w, http.StatusNotFound, "Subscription not found")
			return
		}
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to build quick-export: %v", err))
		return
	}

	renderJSON(w, http.StatusOK, resp)
}

func (h *generateHandler) qrcode(w http.ResponseWriter, r *http.Request) {
	content := r.URL.Query().Get("url")
	if strings.TrimSpace(content) == "" {
		content = r.URL.Query().Get("text")
	}
	if strings.TrimSpace(content) == "" {
		renderError(w, http.StatusBadRequest, "missing 'url' or 'text' query parameter")
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	size := 256
	if sizeStr := r.URL.Query().Get("size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
			size = s
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")

	if format == "svg" {
		svgStr, err := GenerateQRCodeSVG(content)
		if err != nil {
			renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to generate SVG QR code: %v", err))
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(svgStr))
		return
	}

	pngBytes, err := GenerateQRCodePNG(content, size)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Sprintf("failed to generate PNG QR code: %v", err))
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pngBytes)
}
