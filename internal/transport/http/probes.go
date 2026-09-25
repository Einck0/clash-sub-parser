package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
)

type probeHandler struct {
	service      *probe.Service
	runs         domain.ProbeRunRepository
	observations domain.ProbeObservationRepository
	audit        domain.AuditRepository
}

type createProbeRunRequest struct {
	ConfigRevision string             `json:"config_revision"`
	Deadline       *time.Time         `json:"deadline"`
	NodeLogicalIDs []string           `json:"node_logical_ids"`
	Kinds          []domain.ProbeKind `json:"kinds"`
}

func registerProbeRoutes(r chi.Router, service *probe.Service, runs domain.ProbeRunRepository, observations domain.ProbeObservationRepository, audit domain.AuditRepository) {
	if service == nil || runs == nil || observations == nil {
		return
	}
	h := probeHandler{service: service, runs: runs, observations: observations, audit: audit}
	r.Post("/probes/runs", h.create)
	r.Get("/probes/runs", h.list)
	r.Get("/probes/runs/{run_id}", h.get)
	r.Post("/probes/runs/{run_id}/cancel", h.cancel)
	r.Get("/probes/runs/{run_id}/observations", h.runObservations)
	r.Get("/nodes/{logical_id}/observations", h.nodeObservations)
}

func (h probeHandler) create(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		WriteDomainError(w, r, domain.NewValidationError("missing_idempotency_key", "Idempotency-Key header is required"))
		return
	}
	var body createProbeRunRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	deadline := time.Time{}
	if body.Deadline != nil {
		deadline = body.Deadline.UTC()
	}
	actor := "default"
	if auth := GetAuthContext(r.Context()); auth != nil && strings.TrimSpace(auth.Subject) != "" {
		actor = auth.Subject
	}
	_, existingErr := h.runs.GetByIdempotencyKey(r.Context(), actor, key)
	alreadyExists := existingErr == nil
	run, err := h.service.Create(r.Context(), probe.CreateRunCommand{
		ActorScope: actor, IdempotencyKey: key, ConfigRevision: body.ConfigRevision,
		Deadline: deadline, NodeLogicalIDs: body.NodeLogicalIDs, Kinds: body.Kinds,
	})
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	if !alreadyExists {
		h.recordAudit(r, "probe_run.create", domain.AuditResultSuccess, "run_id="+run.ID)
		go func() {
			_ = h.service.TriggerRun(context.Background(), run.ID, body.NodeLogicalIDs, body.Kinds, nil)
		}()
	}
	WriteSuccess(w, r, http.StatusCreated, map[string]any{
		"run_id": run.ID, "state": run.State, "deadline_at": run.DeadlineAt,
	})
}

func (h probeHandler) list(w http.ResponseWriter, r *http.Request) {
	page, pageSize, err := ParsePagination(r)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	var state *domain.ProbeRunState
	if value := strings.TrimSpace(r.URL.Query().Get("state")); value != "" {
		parsed, parseErr := domain.ParseProbeRunState(value)
		if parseErr != nil {
			WriteDomainError(w, r, parseErr)
			return
		}
		state = &parsed
	}
	items, total, err := h.runs.List(r.Context(), state, page, pageSize)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WritePaginated(w, r, items, page, pageSize, total)
}

func (h probeHandler) get(w http.ResponseWriter, r *http.Request) {
	run, err := h.service.Get(r.Context(), chi.URLParam(r, "run_id"))
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WriteSuccess(w, r, http.StatusOK, run)
}

func (h probeHandler) cancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "run_id")
	if err := h.service.Cancel(r.Context(), id); err != nil {
		h.recordAudit(r, "probe_run.cancel", domain.AuditResultFailure, "run_id="+id)
		WriteDomainError(w, r, err)
		return
	}
	h.recordAudit(r, "probe_run.cancel", domain.AuditResultSuccess, "run_id="+id)
	WriteSuccess(w, r, http.StatusOK, map[string]any{"run_id": id, "state": domain.ProbeRunStateCancelled})
}

func (h probeHandler) recordAudit(r *http.Request, action string, result domain.AuditResult, summary string) {
	if h.audit == nil {
		return
	}
	actor := domain.ActorKindAnonymous
	if auth := GetAuthContext(r.Context()); auth != nil && auth.ActorKind == ActorKindAdmin {
		actor = domain.ActorKindAdmin
	}
	_ = h.audit.Record(r.Context(), &domain.AuditEvent{
		ID:              domain.MustNewUUIDv7(),
		ActorKind:       actor,
		RequestID:       GetRequestID(r.Context()),
		Action:          action,
		Result:          result,
		RedactedSummary: domain.RedactSensitiveInfo(summary),
		CreatedAt:       domain.NowUTC(),
	})
}

func (h probeHandler) runObservations(w http.ResponseWriter, r *http.Request) {
	page, pageSize, err := ParsePagination(r)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	items, err := h.observations.ListByRun(r.Context(), chi.URLParam(r, "run_id"))
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WritePaginated(w, r, paginateObservations(items, page, pageSize), page, pageSize, len(items))
}

func (h probeHandler) nodeObservations(w http.ResponseWriter, r *http.Request) {
	page, pageSize, err := ParsePagination(r)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	limit := page * pageSize
	items, err := h.observations.ListByNode(r.Context(), chi.URLParam(r, "logical_id"), limit)
	if err != nil {
		WriteDomainError(w, r, err)
		return
	}
	WritePaginated(w, r, paginateObservations(items, page, pageSize), page, pageSize, len(items))
}

func paginateObservations(items []domain.ProbeObservation, page, pageSize int) []domain.ProbeObservation {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []domain.ProbeObservation{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		WriteDomainError(w, r, domain.NewValidationError("invalid_json", "request body must be valid JSON"))
		return err
	}
	return nil
}
