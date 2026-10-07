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
	Target                         string `json:"target"`
	SnapshotID                     string `json:"snapshot_id"`
	RevisionID                     string `json:"revision_id,omitempty"`
	PruneUnavailableOptionalGroups bool   `json:"prune_unavailable_optional_groups,omitempty"`
	OmitUnavailableOptionalGroups  bool   `json:"omit_unavailable_optional_groups,omitempty"`
}

type previewPublicationRequest struct {
	Target                         string `json:"target"`
	RevisionID                     string `json:"revision_id,omitempty"`
	CompatMode                     string `json:"compat_mode,omitempty"`
	PruneUnavailableOptionalGroups bool   `json:"prune_unavailable_optional_groups,omitempty"`
	OmitUnavailableOptionalGroups  bool   `json:"omit_unavailable_optional_groups,omitempty"`
}

// publicationClientHandler serves client subscription export requests at /publish/v1/{publication_id}.
func publicationClientHandler(svc *publication.Service, holder AdminTokenHolder) http.HandlerFunc {
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

		artifact, err := svc.ResolveAndServeAuthorized(r.Context(), pubID, func(pubTokenHash string) bool {
			if h, ok := holder.(interface{ VerifyExportToken(string, string) bool }); ok {
				return h.VerifyExportToken(token, pubTokenHash)
			}
			return false
		})
		if err != nil {
			switch {
			case errors.Is(err, publication.ErrNotFound):
				WriteError(w, r, http.StatusNotFound, "publication_not_found", "Publication not found")
			case errors.Is(err, publication.ErrRevoked):
				WriteError(w, r, http.StatusForbidden, "publication_revoked", "Publication has been revoked")
			case errors.Is(err, publication.ErrUnauthorized):
				WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid publication token")
			case errors.Is(err, publication.ErrUnsupportedTarget):
				WriteError(w, r, http.StatusUnprocessableEntity, "unsupported_target", "Unsupported compiler target")
			default:
				WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to resolve publication")
			}
			return
		}

		w.Header().Set("Cache-Control", "no-store")
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
	if !target.IsValid() {
		WriteError(w, r, http.StatusUnprocessableEntity, "unsupported_target", fmt.Sprintf("target %q is not supported", target))
		return
	}

	snapshotID := strings.TrimSpace(body.SnapshotID)
	if snapshotID == "" {
		preflightRes, err := h.service.Preflight(r.Context(), publication.PreflightCommand{
			Target:                         target,
			RevisionID:                     strings.TrimSpace(body.RevisionID),
			PruneUnavailableOptionalGroups: body.PruneUnavailableOptionalGroups,
			OmitUnavailableOptionalGroups:  body.OmitUnavailableOptionalGroups,
		})
		if err != nil {
			if writePublicationPreflightError(w, r, err) {
				return
			}
			var capErr *domain.DomainError
			if errors.As(err, &capErr) && capErr.Code == "unsupported_target_capability" {
				WriteError(w, r, http.StatusUnprocessableEntity, "unsupported_target_capability", err.Error())
				return
			}
			WriteDomainError(w, r, err)
			return
		}
		if preflightRes != nil && !preflightRes.Allowed {
			writePublicationPreflightError(w, r, publication.NewPreflightError(*preflightRes))
			return
		}

		WriteError(w, r, http.StatusBadRequest, "snapshot_required", "snapshot_id is required")
		return
	}

	cmd := publication.PublishCommand{
		Target:                         target,
		SnapshotID:                     snapshotID,
		RevisionID:                     strings.TrimSpace(body.RevisionID),
		PruneUnavailableOptionalGroups: body.PruneUnavailableOptionalGroups,
		OmitUnavailableOptionalGroups:  body.OmitUnavailableOptionalGroups,
		ActorKind:                      requestActorKind(r),
		RequestID:                      GetRequestID(r.Context()),
	}

	res, err := h.service.Publish(r.Context(), cmd)
	if err != nil {
		if writePublicationPreflightError(w, r, err) {
			return
		}
		WriteDomainError(w, r, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	statusCode := http.StatusCreated
	if res.Publication.State == domain.PublicationStateActive && res.RawToken == "active" {
		statusCode = http.StatusOK
	}
	WriteSuccess(w, r, statusCode, res)
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
		Target:                         target,
		RevisionID:                     strings.TrimSpace(body.RevisionID),
		CompatMode:                     strings.TrimSpace(body.CompatMode),
		PruneUnavailableOptionalGroups: body.PruneUnavailableOptionalGroups,
		OmitUnavailableOptionalGroups:  body.OmitUnavailableOptionalGroups,
	}

	res, err := h.service.Preview(r.Context(), query)
	if err != nil {
		var strictErr *publication.StrictCapabilityError
		if errors.As(err, &strictErr) {
			w.Header().Set("Cache-Control", "no-store")
			resp := map[string]any{
				"code":       strictErr.Code,
				"message":    strictErr.Message,
				"request_id": GetRequestID(r.Context()),
				"details": map[string]any{
					"diagnostics": strictErr.Diagnostics,
				},
			}
			if strictErr.SnapshotID != "" {
				resp["details"].(map[string]any)["snapshot_id"] = strictErr.SnapshotID
			}
			WriteJSON(w, http.StatusUnprocessableEntity, resp)
			return
		}
		if writePublicationPreflightError(w, r, err) {
			return
		}
		WriteDomainError(w, r, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
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

	data := map[string]any{
		"snapshot_id":     res.SnapshotID,
		"target":          res.Target,
		"snapshot_digest": res.SnapshotDigest,
		"content_digest":  res.ContentDigest,
		"content":         string(res.Content),
		"content_type":    res.ContentType,
		"filename":        res.Filename,
		"manifest":        res.Manifest,
		"diagnostics":     res.Diagnostics,
	}
	if res.FilterCounts != nil {
		data["filter_counts"] = res.FilterCounts
	}
	WriteSuccess(w, r, http.StatusOK, data)
}

// writePublicationPreflightError keeps preview (including raw format) and publish
// rejections in the same JSON envelope. Only the known filter diagnostic needs
// a fixed public message; never serialize an underlying repository error.
func writePublicationPreflightError(w http.ResponseWriter, r *http.Request, err error) bool {
	var preflightErr *publication.PreflightError
	if !errors.As(err, &preflightErr) {
		return false
	}
	diagnostics := make([]publication.PreflightDiagnostic, len(preflightErr.Result.Diagnostics))
	for i, d := range preflightErr.Result.Diagnostics {
		if d.Code == "filtered_nodes_empty" {
			d.Message = "Configured node filters left no exportable nodes; check filter conditions and probe observations"
			d.Target = ""
		} else {
			d.Message = sanitizeErrorMessage(d.Message)
		}
		diagnostics[i] = d
	}
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusConflict, map[string]any{
		"code":            "publication_preflight_rejected",
		"message":         "Publication preflight rejected",
		"request_id":      GetRequestID(r.Context()),
		"policy_revision": preflightErr.Result.PolicyRevision,
		"diagnostics":     diagnostics,
	})
	return true
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
