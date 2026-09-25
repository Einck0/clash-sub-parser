package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/domain"
)

type revisionHandler struct {
	service *revision.Service
}

func registerRevisionRoutes(r chi.Router, svc *revision.Service) {
	if svc == nil {
		return
	}
	h := revisionHandler{service: svc}
	r.Get("/revisions", h.list)
	r.Get("/revisions/active", h.getActive)
	r.Get("/revisions/{id}", h.get)
	r.Post("/revisions/{id}/review", h.review)
	r.Post("/revisions/{id}/activate", h.activate)
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
