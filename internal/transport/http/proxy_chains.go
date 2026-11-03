package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"clash-sub-parser/internal/application/proxychain"
	"clash-sub-parser/internal/domain"
	"github.com/go-chi/chi/v5"
)

type proxyChainListResponse struct {
	Bindings []domain.ProxyChainBinding `json:"bindings"`
	Version  string                     `json:"version"`
}

type previewProxyChainRequest struct {
	Binding       domain.ProxyChainBinding `json:"binding"`
	ConfigVersion string                   `json:"config_version"`
}

type createProxyChainRequest struct {
	Binding       domain.ProxyChainBinding `json:"binding"`
	ConfigVersion string                   `json:"config_version"`
}

type updateProxyChainRequest struct {
	Binding          domain.ProxyChainBinding `json:"binding"`
	ConfigVersion    string                   `json:"config_version"`
	ExpectedRevision *int64                   `json:"expected_revision"`
}

type deleteProxyChainRequest struct {
	ConfigVersion    string `json:"config_version"`
	ExpectedRevision *int64 `json:"expected_revision"`
}

func registerProxyChainRoutes(r chi.Router, svc *proxychain.Service) {
	r.Route("/proxy-chains", func(cr chi.Router) {
		cr.Get("/", proxyChainsListHandler(svc))
		cr.Get("/options", proxyChainsOptionsHandler(svc))
		cr.Get("/effects", proxyChainsEffectsHandler(svc))
		cr.Post("/preview", proxyChainsPreviewHandler(svc))
		cr.Post("/", proxyChainsCreateHandler(svc))
		cr.Put("/{id}", proxyChainsUpdateHandler(svc))
		cr.Delete("/{id}", proxyChainsDeleteHandler(svc))
	})
}

func proxyChainsListHandler(svc *proxychain.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "proxy chain service not configured")
			return
		}
		cfg, err := svc.Get(r.Context())
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		bindings := cfg.Bindings
		if bindings == nil {
			bindings = make([]domain.ProxyChainBinding, 0)
		}
		WriteSuccess(w, r, http.StatusOK, proxyChainListResponse{
			Bindings: bindings,
			Version:  cfg.Version,
		})
	}
}

func proxyChainsOptionsHandler(svc *proxychain.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "proxy chain service not configured")
			return
		}
		opts, err := svc.Options(r.Context())
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, opts)
	}
}

func proxyChainsEffectsHandler(svc *proxychain.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "proxy chain service not configured")
			return
		}
		effects, err := svc.Effects(r.Context())
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, effects)
	}
}

func proxyChainsPreviewHandler(svc *proxychain.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "proxy chain service not configured")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		var req previewProxyChainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
			return
		}
		if strings.TrimSpace(req.ConfigVersion) == "" {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_chain_binding", "config_version is required")
			return
		}

		res, err := svc.Preview(r.Context(), proxychain.PreviewCommand{
			Binding:       req.Binding,
			ConfigVersion: req.ConfigVersion,
		})
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, res)
	}
}

func proxyChainsCreateHandler(svc *proxychain.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "proxy chain service not configured")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		var req createProxyChainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
			return
		}
		if strings.TrimSpace(req.ConfigVersion) == "" {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_chain_binding", "config_version is required")
			return
		}

		res, err := svc.Create(r.Context(), proxychain.CreateCommand{
			Binding:       req.Binding,
			ConfigVersion: req.ConfigVersion,
			ActorKind:     requestActorKind(r),
			RequestID:     GetRequestID(r.Context()),
		})
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusCreated, res)
	}
}

func proxyChainsUpdateHandler(svc *proxychain.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "proxy chain service not configured")
			return
		}
		id := chi.URLParam(r, "id")
		if strings.TrimSpace(id) == "" {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "missing path id parameter")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		var req updateProxyChainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
			return
		}

		if strings.TrimSpace(req.ConfigVersion) == "" {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_chain_binding", "config_version is required")
			return
		}
		if req.ExpectedRevision == nil {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_chain_binding", "expected_revision is required")
			return
		}
		if *req.ExpectedRevision <= 0 {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_chain_binding", "expected_revision must be greater than zero")
			return
		}

		res, err := svc.Update(r.Context(), id, proxychain.UpdateCommand{
			Binding:          req.Binding,
			ConfigVersion:    req.ConfigVersion,
			ExpectedRevision: *req.ExpectedRevision,
			ActorKind:        requestActorKind(r),
			RequestID:        GetRequestID(r.Context()),
		})
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, res)
	}
}

func proxyChainsDeleteHandler(svc *proxychain.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "proxy chain service not configured")
			return
		}
		id := chi.URLParam(r, "id")
		if strings.TrimSpace(id) == "" {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "missing path id parameter")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		var req deleteProxyChainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "invalid request body")
			return
		}

		if strings.TrimSpace(req.ConfigVersion) == "" {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_chain_binding", "config_version is required")
			return
		}
		if req.ExpectedRevision == nil {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_chain_binding", "expected_revision is required")
			return
		}
		if *req.ExpectedRevision <= 0 {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_chain_binding", "expected_revision must be greater than zero")
			return
		}

		res, err := svc.Delete(r.Context(), id, proxychain.DeleteCommand{
			ConfigVersion:    req.ConfigVersion,
			ExpectedRevision: *req.ExpectedRevision,
			ActorKind:        requestActorKind(r),
			RequestID:        GetRequestID(r.Context()),
		})
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		WriteSuccess(w, r, http.StatusOK, res)
	}
}
