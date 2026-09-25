package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	transporthttp "clash-sub-parser/internal/transport/http"
)

type probeRunMemory struct {
	mu   sync.Mutex
	data map[string]domain.ProbeRun
}

func (m *probeRunMemory) GetByID(_ context.Context, id string) (*domain.ProbeRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.data[id]
	if !ok {
		return nil, domain.NewNotFoundError("probe_run_not_found", fmt.Sprintf("probe run %s not found", id))
	}
	copy := run
	return &copy, nil
}

func (m *probeRunMemory) GetByIdempotencyKey(_ context.Context, actor, key string) (*domain.ProbeRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.data {
		if run.ActorScope == actor && run.IdempotencyKey == key {
			copy := run
			return &copy, nil
		}
	}
	return nil, domain.NewNotFoundError("probe_run_not_found", "not found")
}

func (m *probeRunMemory) Create(_ context.Context, run *domain.ProbeRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[run.ID] = *run
	return nil
}

func (m *probeRunMemory) UpdateState(_ context.Context, id string, state domain.ProbeRunState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.data[id]
	if !ok {
		return domain.NewNotFoundError("probe_run_not_found", fmt.Sprintf("probe run %s not found", id))
	}
	run.State = state
	run.UpdatedAt = time.Now().UTC()
	m.data[id] = run
	return nil
}

func (m *probeRunMemory) ListActive(context.Context) ([]domain.ProbeRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var active []domain.ProbeRun
	for _, run := range m.data {
		if run.State == domain.ProbeRunStateQueued || run.State == domain.ProbeRunStateRunning {
			active = append(active, run)
		}
	}
	return active, nil
}

func (m *probeRunMemory) List(_ context.Context, state *domain.ProbeRunState, page, pageSize int) ([]domain.ProbeRun, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]domain.ProbeRun, 0)
	for _, run := range m.data {
		if state == nil || run.State == *state {
			items = append(items, run)
		}
	}
	if pageSize <= 0 {
		return items, len(items), nil
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []domain.ProbeRun{}, len(items), nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], len(items), nil
}

type observationMemory struct {
	mu    sync.Mutex
	items []domain.ProbeObservation
}

func (m *observationMemory) GetByID(_ context.Context, id string) (*domain.ProbeObservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.items {
		if item.ID == id {
			copy := item
			return &copy, nil
		}
	}
	return nil, domain.NewNotFoundError("observation_not_found", "not found")
}

func (m *observationMemory) ListByRun(_ context.Context, runID string) ([]domain.ProbeObservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]domain.ProbeObservation, 0)
	for _, item := range m.items {
		if item.ProbeRunID == runID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (m *observationMemory) ListByNode(_ context.Context, nodeID string, limit int) ([]domain.ProbeObservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]domain.ProbeObservation, 0)
	for _, item := range m.items {
		if item.NodeLogicalID == nodeID {
			items = append(items, item)
			if limit > 0 && len(items) >= limit {
				break
			}
		}
	}
	return items, nil
}

func (m *observationMemory) ListLatestByNodes(_ context.Context, nodeIDs []string, kinds []domain.ProbeKind) (map[string]map[domain.ProbeKind]domain.ProbeObservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make(map[string]map[domain.ProbeKind]domain.ProbeObservation)
	for _, id := range nodeIDs {
		res[id] = make(map[domain.ProbeKind]domain.ProbeObservation)
	}
	return res, nil
}

func (m *observationMemory) Create(_ context.Context, item *domain.ProbeObservation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, *item)
	return nil
}

type auditMemory struct {
	mu     sync.Mutex
	events []domain.AuditEvent
}

func (m *auditMemory) Record(_ context.Context, event *domain.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, *event)
	return nil
}

func (m *auditMemory) List(context.Context, domain.AuditFilter) ([]domain.AuditEvent, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events, len(m.events), nil
}

const (
	probeTestAdminToken       = "probe-admin-token-secret"
	probeTestPublicationToken = "probe-pub-token-export"
	probeTestSessionID        = "probe-session-id-123"
	probeTestCSRFToken        = "probe-csrf-token-abc"
)

func newProbeRouter() (http.Handler, *probeRunMemory, *observationMemory, *auditMemory) {
	runs := &probeRunMemory{data: make(map[string]domain.ProbeRun)}
	audit := &auditMemory{}
	observations := &observationMemory{
		items: []domain.ProbeObservation{
			{
				ID:              "obs-1",
				ProbeRunID:      "run-100",
				NodeLogicalID:   "node-hk-01",
				Kind:            domain.ProbeKindAI,
				Verdict:         domain.VerdictRestricted,
				EvidenceDigest:  "sha256:abcd1234ef",
				LatencyMS:       120,
				RedactedSummary: "profile=ai version=v1 verdict=restricted reason=ip_blocked status=403 latency_ms=120",
				ObservedAt:      time.Now().UTC(),
			},
			{
				ID:              "obs-2",
				ProbeRunID:      "run-100",
				NodeLogicalID:   "node-hk-01",
				Kind:            domain.ProbeKindStreaming,
				Verdict:         domain.VerdictAvailable,
				EvidenceDigest:  "sha256:5678fedcba",
				LatencyMS:       85,
				RedactedSummary: "profile=streaming version=v1 verdict=available reason=unlocked status=200 latency_ms=85",
				ObservedAt:      time.Now().UTC(),
			},
		},
	}
	service := probe.NewService(runs, probe.WithAudit(audit))
	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		AdminToken: probeTestAdminToken,
		PublicationTokenValidator: func(ctx context.Context, publicationID, token string) (bool, error) {
			return token == probeTestPublicationToken, nil
		},
		SessionValidator: func(sessionID string) (*transporthttp.SessionInfo, bool) {
			if sessionID == probeTestSessionID {
				return &transporthttp.SessionInfo{
					SessionID: probeTestSessionID,
					Subject:   "admin",
					CSRFToken: probeTestCSRFToken,
				}, true
			}
			return nil, false
		},
		ProbeService:               service,
		ProbeRunRepository:         runs,
		ProbeObservationRepository: observations,
		AuditRepository:            audit,
	})
	return router, runs, observations, audit
}

// 1. Authentication and Authorization Guardrails
func TestProbeAPIAuthAndCSRFGuardrails(t *testing.T) {
	router, _, _, _ := newProbeRouter()

	// A: Missing credentials on runs
	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for missing auth, got %d", rec.Code)
	}

	// B: Publication export token cannot access Admin probe API
	req = httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestPublicationToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for publication token on probe API, got %d", rec.Code)
	}

	// C: Session cookie without CSRF header fails on POST
	req = httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs", strings.NewReader(`{"kinds":["ai"]}`))
	req.Header.Set("Idempotency-Key", "csrf-test-key")
	req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: probeTestSessionID})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for missing CSRF on POST, got %d", rec.Code)
	}

	// D: Session cookie with valid CSRF header succeeds
	req = httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs", strings.NewReader(`{"kinds":["ai"]}`))
	req.Header.Set("Idempotency-Key", "csrf-valid-key")
	req.Header.Set(transporthttp.CSRFHeader, probeTestCSRFToken)
	req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: probeTestSessionID})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created with valid CSRF and session cookie, got %d", rec.Code)
	}
}

// 2. Idempotency Key Validation and Reuse
func TestProbeAPIIdempotencyKeyEnforcement(t *testing.T) {
	router, runs, _, audit := newProbeRouter()

	// Missing Idempotency-Key header returns 422
	req := httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs", strings.NewReader(`{"kinds":["ai"]}`))
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for missing Idempotency-Key, got %d", rec.Code)
	}

	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatal(err)
	}
	if errResp.Code != "missing_idempotency_key" {
		t.Fatalf("expected error code missing_idempotency_key, got %s", errResp.Code)
	}

	// First submission creates new run and records audit
	createReq := func(key string) map[string]any {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs", strings.NewReader(`{"node_logical_ids":["node-1"],"kinds":["baseline","ai"]}`))
		r.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d (body: %s)", w.Code, w.Body.String())
		}
		var resp struct {
			Data map[string]any `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		return resp.Data
	}

	data1 := createReq("idemp-unique-key-1")
	runID1, ok1 := data1["run_id"].(string)
	if !ok1 || runID1 == "" {
		t.Fatalf("expected valid run_id, got %v", data1)
	}

	// Second submission with exact same key returns the existing run without duplicate audit/creation
	data2 := createReq("idemp-unique-key-1")
	runID2, ok2 := data2["run_id"].(string)
	if !ok2 || runID2 != runID1 {
		t.Fatalf("expected duplicate submission to return same run_id %s, got %s", runID1, runID2)
	}

	if len(runs.data) != 1 {
		t.Fatalf("expected exactly 1 probe run in repo, got %d", len(runs.data))
	}
	if len(audit.events) != 1 {
		t.Fatalf("expected exactly 1 audit event for idempotent creations, got %d", len(audit.events))
	}
}

// 3. Pagination Limits and Validation
func TestProbeAPIPaginationBoundaries(t *testing.T) {
	router, runs, _, _ := newProbeRouter()

	// Populate 10 runs
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("run-%03d", i)
		runs.data[id] = domain.ProbeRun{
			ID:             id,
			IdempotencyKey: fmt.Sprintf("key-%d", i),
			ActorScope:     "admin",
			State:          domain.ProbeRunStateSucceeded,
			CreatedAt:      time.Now().Add(time.Duration(i) * time.Minute),
			DeadlineAt:     time.Now().Add(time.Hour),
		}
	}

	// A: Valid pagination: page=1, page_size=5
	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs?page=1&page_size=5", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var resp struct {
		Data struct {
			Items    []domain.ProbeRun `json:"items"`
			Page     int               `json:"page"`
			PageSize int               `json:"page_size"`
			Total    int               `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Page != 1 || resp.Data.PageSize != 5 || resp.Data.Total != 10 || len(resp.Data.Items) != 5 {
		t.Fatalf("unexpected pagination data: %+v", resp.Data)
	}

	// B: Page 2
	req = httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs?page=2&page_size=5", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var resp2 struct {
		Data struct {
			Items    []domain.ProbeRun `json:"items"`
			Page     int               `json:"page"`
			PageSize int               `json:"page_size"`
			Total    int               `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp2); err != nil {
		t.Fatal(err)
	}
	if resp2.Data.Page != 2 || len(resp2.Data.Items) != 5 || resp2.Data.Total != 10 {
		t.Fatalf("unexpected page 2 data: %+v", resp2.Data)
	}

	// C: page_size > 100 rejected with 422
	req = httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs?page=1&page_size=101", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for page_size > 100, got %d", rec.Code)
	}

	// D: invalid page (< 1) rejected with 422
	req = httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs?page=0&page_size=10", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for page < 1, got %d", rec.Code)
	}

	// E: invalid state filter rejected with 422
	req = httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs?state=invalid_status", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for invalid state query, got %d", rec.Code)
	}
}

// 4. Detail, Cancellation and Audit Trail
func TestProbeAPIDetailAndCancellation(t *testing.T) {
	router, runs, _, audit := newProbeRouter()

	// Seed a queued run
	runID := "run-cancel-target"
	runs.data[runID] = domain.ProbeRun{
		ID:             runID,
		IdempotencyKey: "key-cancel-1",
		ActorScope:     "admin",
		State:          domain.ProbeRunStateQueued,
		CreatedAt:      time.Now().UTC(),
		DeadlineAt:     time.Now().Add(time.Hour),
	}

	// A: Get details of existing run
	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs/"+runID, nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for run detail, got %d", rec.Code)
	}
	var detailResp struct {
		Data domain.ProbeRun `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&detailResp); err != nil {
		t.Fatal(err)
	}
	if detailResp.Data.ID != runID || detailResp.Data.State != domain.ProbeRunStateQueued {
		t.Fatalf("unexpected detail payload: %+v", detailResp.Data)
	}

	// B: Get details of non-existent run -> 404
	req = httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs/non-existent-id", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", rec.Code)
	}

	// C: Cancel queued run -> succeeds and transitions to cancelled
	req = httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs/"+runID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for cancel, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if runs.data[runID].State != domain.ProbeRunStateCancelled {
		t.Fatalf("expected run state cancelled, got %s", runs.data[runID].State)
	}

	// Verify audit event recorded for cancel
	foundCancelAudit := false
	for _, event := range audit.events {
		if event.Action == "probe_run.cancel" && event.Result == domain.AuditResultSuccess {
			foundCancelAudit = true
			break
		}
	}
	if !foundCancelAudit {
		t.Fatalf("expected probe_run.cancel audit event to be logged")
	}

	// D: Idempotent second cancel succeeds without error
	req = httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs/"+runID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for idempotent cancel, got %d", rec.Code)
	}

	// E: Cancel terminal succeeded run -> 409 Conflict
	succID := "run-already-succeeded"
	runs.data[succID] = domain.ProbeRun{
		ID:             succID,
		IdempotencyKey: "key-succ",
		ActorScope:     "admin",
		State:          domain.ProbeRunStateSucceeded,
		CreatedAt:      time.Now().UTC(),
		DeadlineAt:     time.Now().Add(time.Hour),
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs/"+succID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for cancelling succeeded run, got %d", rec.Code)
	}
}

// 5. Evidence Observations and Token Separation
func TestProbeAPIObservationsAndTokenSeparation(t *testing.T) {
	router, _, _, _ := newProbeRouter()

	// A: Read run observations
	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs/run-100/observations?page=1&page_size=10", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for run observations, got %d", rec.Code)
	}
	bodyStr := rec.Body.String()
	// Strict token separation assertion: response must never contain passwords or raw secrets
	if strings.Contains(bodyStr, "password") || strings.Contains(bodyStr, "private_key") {
		t.Fatalf("sensitive credentials leaked in run observations response: %s", bodyStr)
	}

	var obsResp struct {
		Data struct {
			Items []domain.ProbeObservation `json:"items"`
			Total int                       `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&obsResp); err != nil {
		t.Fatal(err)
	}
	if obsResp.Data.Total != 2 || len(obsResp.Data.Items) != 2 {
		t.Fatalf("expected 2 observations for run-100, got total=%d items=%d", obsResp.Data.Total, len(obsResp.Data.Items))
	}

	// B: Read node observations: /api/v1/nodes/{logical_id}/observations
	nodeReq := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-hk-01/observations?page=1&page_size=10", nil)
	nodeReq.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	nodeRec := httptest.NewRecorder()
	router.ServeHTTP(nodeRec, nodeReq)
	if nodeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for node observations, got %d", nodeRec.Code)
	}
	nodeBodyStr := nodeRec.Body.String()
	if strings.Contains(nodeBodyStr, "password") || strings.Contains(nodeBodyStr, "private_key") {
		t.Fatalf("sensitive credentials leaked in node observations response: %s", nodeBodyStr)
	}
	var nodeObsResp struct {
		Data struct {
			Items []domain.ProbeObservation `json:"items"`
			Total int                       `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(nodeRec.Body).Decode(&nodeObsResp); err != nil {
		t.Fatal(err)
	}
	if nodeObsResp.Data.Total != 2 || len(nodeObsResp.Data.Items) != 2 {
		t.Fatalf("expected 2 observations for node-hk-01, got total=%d items=%d", nodeObsResp.Data.Total, len(nodeObsResp.Data.Items))
	}
}

// 6. Concurrent Read/Write Race Safety
func TestProbeAPIConcurrentRequestsRaceSafety(t *testing.T) {
	router, runs, _, _ := newProbeRouter()

	const workers = 8
	const iterations = 25
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		workerID := i
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Concurrent creation with shared key for every pair
				key := fmt.Sprintf("concurrent-key-%d", j%5)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs", strings.NewReader(`{"kinds":["baseline"]}`))
				req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
				req.Header.Set("Idempotency-Key", key)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code != http.StatusCreated {
					t.Errorf("worker %d creation failed: %d", workerID, rec.Code)
				}

				// Concurrent list
				listReq := httptest.NewRequest(http.MethodGet, "/api/v1/probes/runs?page=1&page_size=10", nil)
				listReq.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
				listRec := httptest.NewRecorder()
				router.ServeHTTP(listRec, listReq)
				if listRec.Code != http.StatusOK {
					t.Errorf("worker %d list failed: %d", workerID, listRec.Code)
				}
			}
		}()
	}

	wg.Wait()
	if len(runs.data) > 5 {
		t.Fatalf("expected at most 5 distinct idempotency runs, got %d", len(runs.data))
	}
}

type probeScheduleMemory struct {
	mu       sync.Mutex
	schedule domain.ProbeSchedule
	batches  map[string]domain.ProbeBatch
	runs     map[string][]string
}

func newProbeScheduleMemory() *probeScheduleMemory {
	return &probeScheduleMemory{
		schedule: domain.DefaultProbeSchedule(),
		batches:  make(map[string]domain.ProbeBatch),
		runs:     make(map[string][]string),
	}
}

func (m *probeScheduleMemory) Get(_ context.Context) (*domain.ProbeSchedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := m.schedule
	return &copy, nil
}

func (m *probeScheduleMemory) Update(_ context.Context, s *domain.ProbeSchedule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schedule = *s
	return nil
}

func (m *probeScheduleMemory) GetBatchByID(_ context.Context, id string) (*domain.ProbeBatch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.batches[id]
	if !ok {
		return nil, domain.NewNotFoundError("probe_batch_not_found", fmt.Sprintf("probe batch %s not found", id))
	}
	copy := b
	copy.RunIDs = append([]string{}, m.runs[id]...)
	return &copy, nil
}

func (m *probeScheduleMemory) ListBatches(_ context.Context, page, pageSize int) ([]domain.ProbeBatch, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]domain.ProbeBatch, 0, len(m.batches))
	for id, b := range m.batches {
		copy := b
		copy.RunIDs = append([]string{}, m.runs[id]...)
		res = append(res, copy)
	}
	return res, len(res), nil
}

func (m *probeScheduleMemory) GetBatchByWindow(_ context.Context, generation int64, windowAt time.Time) (*domain.ProbeBatch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, b := range m.batches {
		if b.Generation == generation && b.WindowAt.Equal(windowAt) {
			copy := b
			copy.RunIDs = append([]string{}, m.runs[id]...)
			return &copy, nil
		}
	}
	return nil, domain.NewNotFoundError("probe_batch_not_found", "not found")
}

func (m *probeScheduleMemory) CreateBatch(_ context.Context, b *domain.ProbeBatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batches[b.ID] = *b
	m.runs[b.ID] = append([]string{}, b.RunIDs...)
	return nil
}

func (m *probeScheduleMemory) UpdateBatch(_ context.Context, b *domain.ProbeBatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.batches[b.ID]; !ok {
		return domain.NewNotFoundError("probe_batch_not_found", "not found")
	}
	m.batches[b.ID] = *b
	m.runs[b.ID] = append([]string{}, b.RunIDs...)
	return nil
}

func (m *probeScheduleMemory) AcquireLease(_ context.Context, batchID string, owner string, leaseDuration time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.batches[batchID]
	if !ok {
		return false, domain.NewNotFoundError("probe_batch_not_found", "not found")
	}
	if b.State.IsTerminal() {
		return false, nil
	}
	now := time.Now().UTC()
	if b.Owner != "" && b.Owner != owner && b.LeaseUntil != nil && b.LeaseUntil.After(now) {
		return false, nil
	}
	until := now.Add(leaseDuration)
	b.Owner = owner
	b.LeaseUntil = &until
	m.batches[batchID] = b
	return true, nil
}

func (m *probeScheduleMemory) HeartbeatLease(_ context.Context, batchID string, owner string, leaseDuration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.batches[batchID]
	if !ok {
		return domain.NewNotFoundError("probe_batch_not_found", "not found")
	}
	if b.Owner != owner || b.State.IsTerminal() {
		return domain.NewConflictError("lost_lease", "lost lease")
	}
	until := time.Now().UTC().Add(leaseDuration)
	b.LeaseUntil = &until
	m.batches[batchID] = b
	return nil
}

func (m *probeScheduleMemory) ReleaseLease(_ context.Context, batchID string, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.batches[batchID]
	if !ok || b.Owner != owner {
		return nil
	}
	b.Owner = ""
	b.LeaseUntil = nil
	m.batches[batchID] = b
	return nil
}

func newProbeRouterWithSchedule() (http.Handler, *probeScheduleMemory, *auditMemory) {
	runs := &probeRunMemory{data: make(map[string]domain.ProbeRun)}
	audit := &auditMemory{}
	observations := &observationMemory{items: []domain.ProbeObservation{}}
	schedRepo := newProbeScheduleMemory()

	service := probe.NewService(
		runs,
		probe.WithAudit(audit),
		probe.WithScheduleRepository(schedRepo),
	)

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		AdminToken:                 probeTestAdminToken,
		AuditRepository:            audit,
		ProbeRunRepository:         runs,
		ProbeObservationRepository: observations,
		ProbeService:               service,
	})

	return router, schedRepo, audit
}

func TestProbeScheduleAndBatchEndpoints_Auth(t *testing.T) {
	router, _, _ := newProbeRouterWithSchedule()

	endpoints := []struct {
		method string
		url    string
		body   string
	}{
		{http.MethodGet, "/api/v1/probes/schedule", ""},
		{http.MethodPut, "/api/v1/probes/schedule", `{"enabled": true}`},
		{http.MethodGet, "/api/v1/probes/batches", ""},
		{http.MethodGet, "/api/v1/probes/batches/batch-1", ""},
		{http.MethodPost, "/api/v1/probes/batches/batch-1/cancel", ""},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.url, func(t *testing.T) {
			var bodyReader *strings.Reader
			if ep.body != "" {
				bodyReader = strings.NewReader(ep.body)
			} else {
				bodyReader = strings.NewReader("")
			}
			req := httptest.NewRequest(ep.method, ep.url, bodyReader)
			// No Authorization header
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
			}
		})
	}
}

func TestProbeScheduleEndpoints_GetAndUpdate(t *testing.T) {
	router, _, audit := newProbeRouterWithSchedule()

	// 1. GET schedule initially returns default disabled schedule
	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes/schedule", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET /probes/schedule, got %d", rec.Code)
	}

	var getResp struct {
		Data domain.ProbeSchedule `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&getResp); err != nil {
		t.Fatal(err)
	}
	if getResp.Data.Enabled {
		t.Fatalf("expected initial schedule to be disabled")
	}
	if getResp.Data.IntervalSeconds != 3600 {
		t.Fatalf("expected default interval 3600, got %d", getResp.Data.IntervalSeconds)
	}

	// 2. PUT schedule with invalid interval (< 60) -> 400
	badReq := httptest.NewRequest(http.MethodPut, "/api/v1/probes/schedule", strings.NewReader(`{"interval_seconds": 10}`))
	badReq.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	badRec := httptest.NewRecorder()
	router.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for interval < 60, got %d", badRec.Code)
	}

	// 3. PUT schedule with valid configuration
	updatePayload := `{"enabled": true, "interval_seconds": 600, "kinds": ["baseline", "streaming"]}`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/probes/schedule", strings.NewReader(updatePayload))
	putReq.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for PUT /probes/schedule, got %d: %s", putRec.Code, putRec.Body.String())
	}

	var putResp struct {
		Data domain.ProbeSchedule `json:"data"`
	}
	if err := json.NewDecoder(putRec.Body).Decode(&putResp); err != nil {
		t.Fatal(err)
	}
	if !putResp.Data.Enabled {
		t.Fatalf("expected updated schedule to be enabled")
	}
	if putResp.Data.IntervalSeconds != 600 {
		t.Fatalf("expected interval 600, got %d", putResp.Data.IntervalSeconds)
	}
	if len(putResp.Data.Kinds) != 2 || putResp.Data.Kinds[1] != domain.ProbeKindStreaming {
		t.Fatalf("expected kinds [baseline, streaming], got %+v", putResp.Data.Kinds)
	}
	if putResp.Data.Generation != 1 {
		t.Fatalf("expected generation 1, got %d", putResp.Data.Generation)
	}
	if putResp.Data.NextDueAt == nil {
		t.Fatalf("expected next_due_at to be populated for enabled schedule")
	}

	// 4. Verify audit event
	audit.mu.Lock()
	var foundAudit bool
	for _, ev := range audit.events {
		if ev.Action == "probe_schedule.update" && ev.Result == domain.AuditResultSuccess {
			foundAudit = true
			break
		}
	}
	audit.mu.Unlock()
	if !foundAudit {
		t.Fatalf("expected audit event for probe_schedule.update, got none")
	}
}

func TestProbeBatchEndpoints_ListGetCancel(t *testing.T) {
	router, schedRepo, audit := newProbeRouterWithSchedule()
	ctx := context.Background()
	now := time.Now().UTC()

	batch1 := domain.ProbeBatch{
		ID:         "batch-http-1",
		WindowAt:   now,
		Generation: 1,
		State:      domain.ProbeBatchStateRunning,
		RunIDs:     []string{"run-1"},
		Counts: domain.ProbeBatchCounts{
			TotalNodes:     5,
			DispatchedRuns: 1,
			CompletedRuns:  0,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = schedRepo.CreateBatch(ctx, &batch1)

	// 1. GET /probes/batches
	req := httptest.NewRequest(http.MethodGet, "/api/v1/probes/batches?page=1&page_size=10", nil)
	req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET /probes/batches, got %d", rec.Code)
	}
	var listResp struct {
		Data struct {
			Items []domain.ProbeBatch `json:"items"`
			Total int                 `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatal(err)
	}
	if listResp.Data.Total != 1 || len(listResp.Data.Items) != 1 {
		t.Fatalf("expected 1 batch in list, got total=%d len=%d", listResp.Data.Total, len(listResp.Data.Items))
	}

	// 2. GET /probes/batches/{batch_id}
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/probes/batches/"+batch1.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET /probes/batches/{id}, got %d", getRec.Code)
	}

	// 3. GET /probes/batches/non-existent -> 404
	notFoundReq := httptest.NewRequest(http.MethodGet, "/api/v1/probes/batches/no-such-batch", nil)
	notFoundReq.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	notFoundRec := httptest.NewRecorder()
	router.ServeHTTP(notFoundRec, notFoundReq)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", notFoundRec.Code)
	}

	// 4. POST /probes/batches/{batch_id}/cancel -> 200 OK
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/probes/batches/"+batch1.ID+"/cancel", nil)
	cancelReq.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
	cancelRec := httptest.NewRecorder()
	router.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for cancel batch, got %d", cancelRec.Code)
	}

	var cancelResp struct {
		Data domain.ProbeBatch `json:"data"`
	}
	if err := json.NewDecoder(cancelRec.Body).Decode(&cancelResp); err != nil {
		t.Fatal(err)
	}
	if cancelResp.Data.ID != batch1.ID || cancelResp.Data.State != domain.ProbeBatchStateCancelled {
		t.Fatalf("unexpected cancel response: %+v", cancelResp.Data)
	}

	// 5. Verify batch is now cancelled
	cancelled, _ := schedRepo.GetBatchByID(ctx, batch1.ID)
	if cancelled.State != domain.ProbeBatchStateCancelled {
		t.Fatalf("expected batch state cancelled, got %s", cancelled.State)
	}

	// 6. Verify audit event
	audit.mu.Lock()
	var foundAudit bool
	for _, ev := range audit.events {
		if ev.Action == "probe_batch.cancel" && ev.Result == domain.AuditResultSuccess {
			foundAudit = true
			break
		}
	}
	audit.mu.Unlock()
	if !foundAudit {
		t.Fatalf("expected audit event for probe_batch.cancel, got none")
	}
}
