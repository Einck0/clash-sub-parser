package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
)

type publicationAdminHandler struct {
	service *publication.Service
	audit   domain.AuditRepository
}

type createPublicationRequest struct {
	Target     string `json:"target"`
	RevisionID string `json:"revision_id,omitempty"`
}

type previewPublicationRequest struct {
	Target     string `json:"target"`
	RevisionID string `json:"revision_id,omitempty"`
}

// publicationClientHandler serves client subscription export requests at /publish/v1/{publication_id}.
func publicationClientHandler(svc *publication.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubID := strings.TrimSpace(chi.URLParam(r, "publication_id"))
		if pubID == "" {
			WriteError(w, r, http.StatusNotFound, "publication_not_found", "Publication not found")
			return
		}

		token := strings.TrimSpace(r.URL.Query().Get("token"))
		if token == "" {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				token = strings.TrimSpace(authHeader[7:])
			}
		}

		if token == "" {
			WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Publication token required")
			return
		}

		artifact, err := svc.ResolveAndServe(r.Context(), pubID, token)
		if err != nil {
			switch err {
			case publication.ErrNotFound:
				WriteError(w, r, http.StatusNotFound, "publication_not_found", "Publication not found")
			case publication.ErrRevoked:
				WriteError(w, r, http.StatusForbidden, "publication_revoked", "Publication has been revoked")
			case publication.ErrUnauthorized:
				WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid publication token")
			default:
				WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve publication")
			}
			return
		}

		w.Header().Set("Content-Type", artifact.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifact.Filename))
		w.Header().Set("ETag", fmt.Sprintf("%q", artifact.ContentDigest))
		w.Header().Set("X-Content-Digest", artifact.ContentDigest)
		w.Header().Set("X-Snapshot-Digest", artifact.SnapshotDigest)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(artifact.Content)
	}
}

// registerPublicationRoutes mounts administrative publication endpoints on the /api/v1 router.
func registerPublicationRoutes(r chi.Router, service *publication.Service, audit domain.AuditRepository) {
	if service == nil {
		return
	}
	h := publicationAdminHandler{service: service, audit: audit}

	r.Route("/publications", func(sub chi.Router) {
		sub.Post("/", h.create)
		sub.Post("/preview", h.preview)
		sub.Post("/preflight", h.preflight)
		sub.Get("/preflight", h.preflight)
		sub.Get("/{id}", h.get)
		sub.Post("/{id}/revoke", h.revoke)
		sub.Delete("/{id}", h.revoke)
	})
}

// registerPublicationSkeleton mounts placeholder routes when publication service is not configured.
func registerPublicationSkeleton(r chi.Router) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		if r.Method == http.MethodPost {
			status = http.StatusCreated
		}
		WriteSuccess(w, r, status, map[string]any{
			"service": "publications",
			"status":  "skeleton_active",
		})
	}
	r.Get("/publications", handler)
	r.Post("/publications", handler)
	r.Get("/publications/*", handler)
	r.Post("/publications/*", handler)
	r.Patch("/publications/*", handler)
	r.Delete("/publications/*", handler)
}

func (h publicationAdminHandler) create(w http.ResponseWriter, r *http.Request) {
	var body createPublicationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, r, http.StatusUnprocessableEntity, "invalid_json", "Invalid JSON request body")
		return
	}

	target := domain.CompilerTarget(strings.TrimSpace(body.Target))
	if target == "" {
		WriteError(w, r, http.StatusUnprocessableEntity, "invalid_target", "Target compiler is required")
		return
	}

	cmd := publication.PublishCommand{
		Target:     target,
		RevisionID: strings.TrimSpace(body.RevisionID),
		ActorKind:  requestActorKind(r),
		RequestID:  GetRequestID(r.Context()),
	}

	res, err := h.service.Publish(r.Context(), cmd)
	if err != nil {
		var preflightErr *publication.PreflightError
		if errors.As(err, &preflightErr) {
			WriteJSON(w, http.StatusConflict, map[string]any{
				"code":            "publication_preflight_rejected",
				"message":         "Publication preflight rejected",
				"request_id":      GetRequestID(r.Context()),
				"policy_revision": preflightErr.Result.PolicyRevision,
				"diagnostics":     preflightErr.Result.Diagnostics,
			})
			return
		}
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusCreated, res)
}

func (h publicationAdminHandler) preview(w http.ResponseWriter, r *http.Request) {
	var body previewPublicationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, r, http.StatusUnprocessableEntity, "invalid_json", "Invalid JSON request body")
		return
	}

	target := domain.CompilerTarget(strings.TrimSpace(body.Target))
	if target == "" {
		WriteError(w, r, http.StatusUnprocessableEntity, "invalid_target", "Target compiler is required")
		return
	}

	query := publication.PreviewQuery{
		Target:     target,
		RevisionID: strings.TrimSpace(body.RevisionID),
	}

	res, err := h.service.Preview(r.Context(), query)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	if r.URL.Query().Get("format") == "raw" {
		w.Header().Set("Content-Type", res.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", res.Filename))
		w.Header().Set("ETag", fmt.Sprintf("%q", res.ContentDigest))
		w.Header().Set("X-Content-Digest", res.ContentDigest)
		w.Header().Set("X-Snapshot-Digest", res.SnapshotDigest)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(res.Content)
		return
	}

	WriteSuccess(w, r, http.StatusOK, map[string]any{
		"target":          res.Target,
		"snapshot_digest": res.SnapshotDigest,
		"content_digest":  res.ContentDigest,
		"content":         string(res.Content),
		"content_type":    res.ContentType,
		"filename":        res.Filename,
		"diagnostics":     res.Diagnostics,
	})
}

func (h publicationAdminHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		WriteError(w, r, http.StatusNotFound, "publication_not_found", "Publication ID is required")
		return
	}

	detail, err := h.service.Get(r.Context(), id)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, detail)
}

func (h publicationAdminHandler) revoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		WriteError(w, r, http.StatusNotFound, "publication_not_found", "Publication ID is required")
		return
	}

	cmd := publication.RevokeCommand{
		ID:        id,
		ActorKind: requestActorKind(r),
		RequestID: GetRequestID(r.Context()),
	}

	pub, err := h.service.Revoke(r.Context(), cmd)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, map[string]any{
		"id":         pub.ID,
		"state":      pub.State,
		"revoked_at": pub.RevokedAt,
	})
}
