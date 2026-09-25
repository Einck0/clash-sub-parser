package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
)

type preflightPublicationRequest struct {
	Target     string `json:"target"`
	RevisionID string `json:"revision_id,omitempty"`
}

// preflight evaluates publication preflight diagnostics for a target compiler without creating a publication.
func (h publicationAdminHandler) preflight(w http.ResponseWriter, r *http.Request) {
	var target string
	var revID string

	if r.Method == http.MethodPost {
		var body preflightPublicationRequest
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				WriteError(w, r, http.StatusUnprocessableEntity, "invalid_json", "Invalid JSON request body")
				return
			}
		}
		target = strings.TrimSpace(body.Target)
		revID = strings.TrimSpace(body.RevisionID)
	} else {
		target = strings.TrimSpace(r.URL.Query().Get("target"))
		revID = strings.TrimSpace(r.URL.Query().Get("revision_id"))
	}

	if target == "" {
		WriteError(w, r, http.StatusUnprocessableEntity, "invalid_target", "Target compiler is required")
		return
	}

	cmd := publication.PreflightCommand{
		Target:     domain.CompilerTarget(target),
		RevisionID: revID,
		ActorKind:  requestActorKind(r),
		RequestID:  GetRequestID(r.Context()),
	}

	res, err := h.service.Preflight(r.Context(), cmd)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	WriteSuccess(w, r, http.StatusOK, res)
}
