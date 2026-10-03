package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	transporthttp "clash-sub-parser/internal/transport/http"
)

// Task 5.4: TestAPISemantic_ProbesAndPublications
// Comprehensive, isolated semantic integration test covering Probes and Publications modules.
// Verifies:
// 1. Probes: Missing Idempotency-Key validation, Idempotent Creation & Replay, List, Get, Cancellation state transition, Pool status, Schedule CRUD
// 2. Publications: Preflight GET & POST, Target Preview (Mihomo/Sing-box), Publication Creation, Admin Detail GET,
//    Immutable Client Export Retrieval (/publish/v1/{id} and /p/{id}), Export Auth Token Enforcement, and Revocation Blocking
func TestAPISemantic_ProbesAndPublications(t *testing.T) {
	harness := setupTestHarness(t)
	ctx := context.Background()

	// 0. Enable Admin Auth & Export Auth
	harness.TokenHolder.SetAuthSwitches(true, true)

	// Seed an active test node so that policy snapshot and probes have real entities
	now := domain.NowUTC()
	testNodeID := "node-probe-e2e-01"
	err := harness.NodeRepo.UpsertBatch(ctx, []domain.Node{
		{
			LogicalID:          testNodeID,
			Protocol:           domain.ProtocolSS,
			DisplayName:        "E2E Probe Edge Node",
			Server:             "198.51.100.222",
			Port:               8388,
			Active:             true,
			ConnectionRevision: 1,
			Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "test-password-1"},
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	})
	if err != nil {
		t.Fatalf("failed to seed test node: %v", err)
	}

	// Ensure an active policy revision exists with default group/rule covering this node
	activeRev, err := harness.RevisionService.GetActive(ctx)
	if err != nil || activeRev == nil {
		t.Fatalf("failed to get active revision: %v", err)
	}

	// =========================================================================
	// Part 1: Probes Lifecycle, Idempotency & State Machine
	// =========================================================================

	// 1.1 Unauthenticated POST /api/v1/probes/runs -> 401 Unauthorized
	probeBody := map[string]any{
		"config_revision":  activeRev.ID,
		"node_logical_ids": []string{testNodeID},
		"kinds":            []string{"connectivity"},
	}
	resp, err := harness.Request(http.MethodPost, "/api/v1/probes/runs", probeBody, nil, nil)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated probe run creation, got %d", resp.StatusCode)
	}

	// 1.2 Missing Idempotency-Key Header -> 422 Unprocessable Entity
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/probes/runs", probeBody, nil)
	if err != nil || resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing Idempotency-Key header, got %d", resp.StatusCode)
	}

	// 1.3 Valid Probe Run Creation with Idempotency-Key -> 201 Created
	idempKey := "e2e-idemp-probe-key-001"
	headers := map[string]string{"Idempotency-Key": idempKey}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/probes/runs", probeBody, headers)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for valid probe run creation, got %d body=%s", resp.StatusCode, resp.Body)
	}
	var createdRun struct {
		Data struct {
			RunID string `json:"run_id"`
			State string `json:"state"`
		} `json:"data"`
	}
	if err := resp.JSON(&createdRun); err != nil || createdRun.Data.RunID == "" {
		t.Fatalf("unmarshal created probe run: %v", err)
	}
	runID1 := createdRun.Data.RunID

	// DB verification: probe run row exists in `probe_runs` table!
	var dbRunCount int
	err = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM probe_runs WHERE id = ?", runID1).Scan(&dbRunCount)
	if err != nil || dbRunCount != 1 {
		t.Fatalf("DB verification failed: probe_runs record not found, count=%d", dbRunCount)
	}

	// 1.4 Idempotency Replay: Re-sending request with the SAME Idempotency-Key
	// MUST return the SAME runID without inserting a second row into `probe_runs`!
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/probes/runs", probeBody, headers)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("idempotency replay failed: status=%d", resp.StatusCode)
	}
	var replayRun struct {
		Data struct {
			RunID string `json:"run_id"`
		} `json:"data"`
	}
	_ = resp.JSON(&replayRun)
	if replayRun.Data.RunID != runID1 {
		t.Fatalf("IDEMPOTENCY VIOLATION: expected identical runID %s on replay, got %s", runID1, replayRun.Data.RunID)
	}
	var totalRuns int
	_ = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM probe_runs WHERE idempotency_key = ?", idempKey).Scan(&totalRuns)
	if totalRuns != 1 {
		t.Fatalf("IDEMPOTENCY VIOLATION: multiple rows created for same idempotency key: %d", totalRuns)
	}

	// 1.5 List Probe Runs: GET /api/v1/probes/runs -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/probes/runs?page=1&page_size=10", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/probes/runs failed: %d", resp.StatusCode)
	}

	// 1.6 Get Probe Run Detail: GET /api/v1/probes/runs/{run_id} -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/probes/runs/"+runID1, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/probes/runs/%s failed: %d", runID1, resp.StatusCode)
	}

	// 1.7 Probe Run Cancellation: POST /api/v1/probes/runs/{run_id}/cancel
	// Seed a queued probe run directly to test active cancellation
	queuedRunID := "run-e2e-queued-for-cancel"
	err = harness.ProbeRunRepo.Create(ctx, &domain.ProbeRun{
		ID:             queuedRunID,
		IdempotencyKey: "idemp-cancel-target-001",
		ActorScope:     "default",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     domain.NowUTC().Add(time.Hour),
		CreatedAt:      domain.NowUTC(),
		UpdatedAt:      domain.NowUTC(),
	})
	if err != nil {
		t.Fatalf("failed to seed queued probe run: %v", err)
	}

	// Active cancellation on queued run -> 200 OK
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/probes/runs/%s/cancel", queuedRunID), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/probes/runs/%s/cancel failed: %d body=%s", queuedRunID, resp.StatusCode, resp.Body)
	}

	// DB verification: state in `probe_runs` has transitioned to 'cancelled'
	var runState string
	_ = harness.DB.QueryRowContext(ctx, "SELECT state FROM probe_runs WHERE id = ?", queuedRunID).Scan(&runState)
	if runState != string(domain.ProbeRunStateCancelled) {
		t.Fatalf("expected state 'cancelled' after cancellation, got %q", runState)
	}

	// Idempotent cancellation: re-cancelling returns 200 OK with state 'cancelled'
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/probes/runs/%s/cancel", queuedRunID), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on idempotent probe cancel, got %d", resp.StatusCode)
	}

	// 1.8 Probe Run Observations: GET /api/v1/probes/runs/{run_id}/observations -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, fmt.Sprintf("/api/v1/probes/runs/%s/observations", runID1), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/probes/runs/%s/observations failed: %d", runID1, resp.StatusCode)
	}

	// 1.9 Probe Pool Status: GET /api/v1/probes/pool -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/probes/pool", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/probes/pool failed: %d", resp.StatusCode)
	}

	// 1.10 Probe Schedule CRUD: GET and PUT /api/v1/probes/schedule -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/probes/schedule", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/probes/schedule failed: %d", resp.StatusCode)
	}
	scheduleUpdate := map[string]any{
		"enabled":          true,
		"interval_seconds": 600,
	}
	resp, err = harness.AuthRequest(http.MethodPut, "/api/v1/probes/schedule", scheduleUpdate, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/probes/schedule failed: %d", resp.StatusCode)
	}

	// =========================================================================
	// Part 2: Publications Lifecycle, Preview, Export & Revocation
	// =========================================================================

	// 2.1 Publications Preflight: GET & POST /api/v1/publications/preflight
	// Missing target query parameter -> 422
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/publications/preflight", nil, nil)
	if err != nil || resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for preflight without target, got %d", resp.StatusCode)
	}

	// Valid target -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/publications/preflight?target=mihomo", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/publications/preflight failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/publications/preflight", map[string]any{"target": "mihomo"}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/publications/preflight failed: %d body=%s", resp.StatusCode, resp.Body)
	}

	// 2.2 Publications Preview: POST /api/v1/publications/preview for Mihomo and Sing-box
	previewMihomo := map[string]any{
		"target":      "mihomo",
		"revision_id": activeRev.ID,
	}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/publications/preview", previewMihomo, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/publications/preview (mihomo) failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	var previewData struct {
		Data struct {
			Target     string `json:"target"`
			Content    string `json:"content"`
			SnapshotID string `json:"snapshot_id"`
		} `json:"data"`
	}
	if err := resp.JSON(&previewData); err != nil || previewData.Data.Content == "" {
		t.Fatalf("expected non-empty preview content, got %+v", previewData)
	}

	// 2.3 Create Publication: POST /api/v1/publications -> 201 Created
	createPubBody := map[string]any{
		"target":      "mihomo",
		"snapshot_id": previewData.Data.SnapshotID,
		"revision_id": activeRev.ID,
	}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/publications", createPubBody, nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/publications failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	var createdPub struct {
		Data struct {
			Publication struct {
				ID             string `json:"id"`
				Target         string `json:"target"`
				State          string `json:"state"`
				SnapshotDigest string `json:"snapshot_digest"`
			} `json:"publication"`
			RawToken string `json:"raw_token"`
		} `json:"data"`
	}
	if err := resp.JSON(&createdPub); err != nil || createdPub.Data.Publication.ID == "" || createdPub.Data.RawToken == "" {
		t.Fatalf("unmarshal created publication: %v", err)
	}
	pubID := createdPub.Data.Publication.ID
	rawToken := createdPub.Data.RawToken

	// DB verification: publication exists in `publications` table!
	var dbPubState string
	err = harness.DB.QueryRowContext(ctx, "SELECT state FROM publications WHERE id = ?", pubID).Scan(&dbPubState)
	if err != nil || dbPubState != "active" {
		t.Fatalf("DB verification failed: publication state=%q err=%v", dbPubState, err)
	}

	// 2.4 Administrative Detail GET: /api/v1/publications/{id} -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/publications/"+pubID, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/publications/%s failed: %d", pubID, resp.StatusCode)
	}

	// 2.5 Client Export Endpoint: /publish/v1/{id}
	// a. Without token when export auth is enabled -> 401 Unauthorized
	resp, err = harness.Request(http.MethodGet, "/publish/v1/"+pubID, nil, nil, nil)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for client export without token, got %d", resp.StatusCode)
	}

	// b. With invalid token -> 401 Unauthorized
	resp, err = harness.Request(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=invalid-secret-token", pubID), nil, nil, nil)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for client export with invalid token, got %d", resp.StatusCode)
	}

	// c. With correct token via query parameter -> 200 OK with raw config content!
	resp, err = harness.Request(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", pubID, rawToken), nil, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /publish/v1/%s?token=%s failed: %d body=%s", pubID, rawToken, resp.StatusCode, resp.Body)
	}
	if len(resp.Body) == 0 {
		t.Fatalf("expected non-empty published config payload")
	}
	exportedYAML := string(resp.Body)
	if !strings.Contains(exportedYAML, "E2E Probe Edge Node") && !strings.Contains(exportedYAML, "198.51.100.222") {
		t.Fatalf("expected exported config to contain node data, got: %s", exportedYAML)
	}

	// 2.6 Short URL Endpoint: /p/{id}?token={token} -> 200 OK with identical content!
	respShort, err := harness.Request(http.MethodGet, fmt.Sprintf("/p/%s?token=%s", pubID, rawToken), nil, nil, nil)
	if err != nil || respShort.StatusCode != http.StatusOK {
		t.Fatalf("GET /p/%s failed: %d body=%s", pubID, respShort.StatusCode, respShort.Body)
	}
	if string(respShort.Body) != exportedYAML {
		t.Fatalf("content mismatch between /publish/v1/{id} and /p/{id}")
	}

	// 2.7 Publication Revocation: POST /api/v1/publications/{id}/revoke -> 200 OK
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/publications/%s/revoke", pubID), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/publications/%s/revoke failed: %d body=%s", pubID, resp.StatusCode, resp.Body)
	}

	// DB verification: state in `publications` has transitioned to 'revoked'
	_ = harness.DB.QueryRowContext(ctx, "SELECT state FROM publications WHERE id = ?", pubID).Scan(&dbPubState)
	if dbPubState != "revoked" {
		t.Fatalf("DB verification failed: state is not revoked, got %q", dbPubState)
	}

	// 2.8 Subsequent Client Export Request on Revoked Publication MUST BE BLOCKED (403 Forbidden)
	resp, err = harness.Request(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", pubID, rawToken), nil, nil, nil)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden after publication revocation, got %d body=%s", resp.StatusCode, resp.Body)
	}
	var errResp transporthttp.ErrorResponse
	_ = resp.JSON(&errResp)
	if errResp.Code != "publication_revoked" {
		t.Fatalf("expected error code 'publication_revoked', got %q", errResp.Code)
	}

	// 2.9 Direct DELETE /api/v1/publications/{id} (Alias to revoke via HTTP DELETE)
	prevResp2, err := harness.AuthRequest(http.MethodPost, "/api/v1/publications/preview", map[string]any{
		"target":      "singbox",
		"revision_id": activeRev.ID,
	}, nil)
	if err != nil || prevResp2.StatusCode != http.StatusOK {
		t.Fatalf("preview for second publication failed: %v", err)
	}
	var prev2Data struct {
		Data struct {
			SnapshotID string `json:"snapshot_id"`
		} `json:"data"`
	}
	_ = prevResp2.JSON(&prev2Data)

	createPubBody2 := map[string]any{
		"target":      "singbox",
		"snapshot_id": prev2Data.Data.SnapshotID,
		"revision_id": activeRev.ID,
	}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/publications", createPubBody2, nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST second publication failed: %d", resp.StatusCode)
	}
	var pub2Created struct {
		Data struct {
			Publication struct {
				ID string `json:"id"`
			} `json:"publication"`
		} `json:"data"`
	}
	_ = resp.JSON(&pub2Created)
	pubID2 := pub2Created.Data.Publication.ID

	// Call DELETE /api/v1/publications/{id}
	resp, err = harness.AuthRequest(http.MethodDelete, "/api/v1/publications/"+pubID2, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/v1/publications/%s failed: %d body=%s", pubID2, resp.StatusCode, resp.Body)
	}
	var dbPub2State string
	_ = harness.DB.QueryRowContext(ctx, "SELECT state FROM publications WHERE id = ?", pubID2).Scan(&dbPub2State)
	if dbPub2State != "revoked" {
		t.Fatalf("DB verification failed: DELETE /publications/%s did not revoke publication, state=%q", pubID2, dbPub2State)
	}

	// 1.8 Probes Schedule Trigger & Batches
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/probes/schedule/trigger", map[string]any{}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/probes/schedule/trigger failed: %d body=%s", resp.StatusCode, resp.Body)
	}

	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/probes/batches", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/probes/batches failed: %d", resp.StatusCode)
	}
}
