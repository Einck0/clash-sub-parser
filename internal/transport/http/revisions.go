package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/domain"
)

type revisionHandler struct {
	service *revision.Service
}

type createRevisionRequest struct {
	ParentID *string `json:"parent_id,omitempty"`
	State    string  `json:"state,omitempty"`
}

func registerRevisionRoutes(r chi.Router, svc *revision.Service) {
	if svc == nil {
		return
	}
	h := revisionHandler{service: svc}
	r.Get("/revisions", h.list)
	r.Post("/revisions", h.create)
	r.Get("/revisions/active", h.getActive)
	r.Get("/revisions/{id}", h.get)
	r.Post("/revisions/{id}/review", h.review)
	r.Post("/revisions/{id}/activate", h.activate)
}

func (h revisionHandler) create(w http.ResponseWriter, r *http.Request) {
	var body createRevisionRequest
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			WriteDomainError(w, r, domain.NewValidationError("invalid_json", "request body must be valid JSON"))
			return
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&body); err != nil {
				WriteDomainError(w, r, domain.NewValidationError("invalid_json", "request body must be valid JSON"))
				return
			}
		}
	}

	var state domain.ConfigurationRevisionState
	if s := strings.TrimSpace(body.State); s != "" {
		parsed, err := domain.ParseRevisionState(s)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}
		state = parsed
	}

	rev, err := h.service.Create(r.Context(), revision.CreateCommand{
		ParentID:  body.ParentID,
		State:     state,
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusCreated, rev)
}

func (h revisionHandler) list(w http.ResponseWriter, r *http.Request) {
	page, pageSize, err := ParsePagination(r)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}

	filter := domain.RevisionFilter{
		Pagination: domain.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
	}

	if s := strings.TrimSpace(r.URL.Query().Get("state")); s != "" {
		st, pErr := domain.ParseRevisionState(s)
		if pErr != nil {
			WriteDomainError(w, r, pErr)
			return
		}
		filter.State = &st
	}

	items, total, err := h.service.List(r.Context(), filter)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WritePaginated(w, r, items, page, pageSize, total)
}

func (h revisionHandler) getActive(w http.ResponseWriter, r *http.Request) {
	rev, err := h.service.GetActive(r.Context())
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, rev)
}

func (h revisionHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	detail, err := h.service.Get(r.Context(), id)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, detail)
}

func (h revisionHandler) review(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	detail, err := h.service.Review(r.Context(), id, revision.Action{
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, detail)
}

func (h revisionHandler) activate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rev, err := h.service.Activate(r.Context(), id, revision.Action{
		RequestID: GetRequestID(r.Context()),
		ActorKind: requestActorKind(r),
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, rev)
}
