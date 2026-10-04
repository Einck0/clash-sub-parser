package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
)

func TestEnabledSubscriptionInventoryE2E(t *testing.T) {
	harness := setupTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	nowStr := now.Format(time.RFC3339)

	// 1. Setup Subscriptions:
	// - subA: enabled = 1
	// - subB: enabled = 0
	// - subC: enabled = 1 (never had a successful fetch)
	// - subD: enabled = 1 (had successful fetch F1, then newer failed fetch F2 -> keeps F1)
	subA := "01a10000-0000-7000-8000-00000000000a"
	subB := "01a10000-0000-7000-8000-00000000000b"
	subC := "01a10000-0000-7000-8000-00000000000c"
	subD := "01a10000-0000-7000-8000-00000000000d"

	for _, s := range []struct {
		id      string
		name    string
		enabled int
	}{
		{subA, "Subscription A", 1},
		{subB, "Subscription B", 0},
		{subC, "Subscription C", 1},
		{subD, "Subscription D", 1},
	} {
		_, err := harness.DB.ExecContext(ctx, `
			INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
			VALUES (?, ?, 'secret://url', ?, 'rev1', ?, ?);
		`, s.id, s.name, s.enabled, nowStr, nowStr)
		if err != nil {
			t.Fatalf("failed to insert subscription %s: %v", s.id, err)
		}
	}

	// 2. Setup Fetches:
	// - subA: fetchA1 (success)
	// - subB: fetchB1 (success)
	// - subC: fetchC1 (failed)
	// - subD: fetchD1 (success, started 2h ago), fetchD2 (failed, started 1h ago)
	fetchA1 := "01a10000-0000-7000-9000-0000000000a1"
	fetchB1 := "01a10000-0000-7000-9000-0000000000b1"
	fetchC1 := "01a10000-0000-7000-9000-0000000000c1"
	fetchD1 := "01a10000-0000-7000-9000-0000000000d1"
	fetchD2 := "01a10000-0000-7000-9000-0000000000d2"

	time2hAgo := now.Add(-2 * time.Hour).Format(time.RFC3339)
	time1hAgo := now.Add(-1 * time.Hour).Format(time.RFC3339)

	for _, f := range []struct {
		id      string
		subID   string
		started string
		outcome string
	}{
		{fetchA1, subA, nowStr, "success"},
		{fetchB1, subB, nowStr, "success"},
		{fetchC1, subC, nowStr, "failed"},
		{fetchD1, subD, time2hAgo, "success"},
		{fetchD2, subD, time1hAgo, "failed"},
	} {
		_, err := harness.DB.ExecContext(ctx, `
			INSERT INTO subscription_fetches (id, subscription_id, started_at, finished_at, outcome)
			VALUES (?, ?, ?, ?, ?);
		`, f.id, f.subID, f.started, f.started, f.outcome)
		if err != nil {
			t.Fatalf("failed to insert fetch %s: %v", f.id, err)
		}
	}

	// 3. Setup Nodes:
	// - node-a-only: only in subA, active=1
	// - node-shared-ab: in subA and subB, active=1
	// - node-inactive-a: in subA, active=0 (inactive proxy)
	// - node-orphan: no source at all, active=1
	// - node-b-only: only in subB (disabled), active=1
	// - node-notice-a: in subA, confirmed notice (entry_kind='notice')
	// - node-unknown-a: in subA, normal unknown proxy (not confirmed notice)
	// - node-d-member: in subD's fetchD1, active=1 (retained despite fetchD2 failing)
	// - node-removed-a: was in old fetch of subA, not in fetchA1 -> excluded
	nodes := []struct {
		id     string
		name   string
		proto  string
		active int
	}{
		{"node-a-only", "Node A Only", "ss", 1},
		{"node-shared-ab", "Shared Node AB", "vmess", 1},
		{"node-inactive-a", "Inactive Node A", "vless", 0},
		{"node-orphan", "Orphan Node No Source", "trojan", 1},
		{"node-b-only", "Node B Only", "ss", 1},
		{"node-notice-a", "Notice Node A 127.0.0.1", "vmess", 1},
		{"node-unknown-a", "Unknown Normal Proxy", "hysteria2", 1},
		{"node-d-member", "Node D Member Retained", "ss", 1},
		{"node-removed-a", "Old Removed Node A", "ss", 1},
	}

	for _, n := range nodes {
		_, err := harness.DB.ExecContext(ctx, `
			INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
			VALUES (?, ?, ?, '198.51.100.1', 8388, '{}', ?, ?, ?);
		`, n.id, n.proto, n.name, n.active, nowStr, nowStr)
		if err != nil {
			t.Fatalf("failed to insert node %s: %v", n.id, err)
		}
	}

	// 4. Setup node_sources:
	for _, ns := range []struct {
		nodeID  string
		subID   string
		fetchID string
	}{
		{"node-a-only", subA, fetchA1},
		{"node-shared-ab", subA, fetchA1},
		{"node-shared-ab", subB, fetchB1},
		{"node-inactive-a", subA, fetchA1},
		// node-orphan has NO source
		{"node-b-only", subB, fetchB1},
		{"node-notice-a", subA, fetchA1},
		{"node-unknown-a", subA, fetchA1},
		{"node-d-member", subD, fetchD1},
		{"node-removed-a", subA, "old-stale-fetch-id"},
	} {
		_, err := harness.DB.ExecContext(ctx, `
			INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
			VALUES (?, ?, ?);
		`, ns.nodeID, ns.subID, ns.fetchID)
		if err != nil {
			t.Fatalf("failed to insert node_source (%s, %s): %v", ns.nodeID, ns.subID, err)
		}
	}

	// 5. Setup subscription_entries for notice classification:
	payloadA1 := "01a10000-0000-7000-a000-0000000000a1"
	_, err := harness.DB.ExecContext(ctx, `
		INSERT INTO subscription_payloads (id, subscription_id, fetch_id, content_digest, body_blob, http_status, headers_json, pinned, created_at)
		VALUES (?, ?, ?, 'sha256:digest', X'00', 200, '{}', 0, ?);
	`, payloadA1, subA, fetchA1, nowStr)
	if err != nil {
		t.Fatalf("failed to insert payload: %v", err)
	}

	// entry for node-notice-a is confirmed notice
	_, err = harness.DB.ExecContext(ctx, `
		INSERT INTO subscription_entries (id, payload_id, subscription_id, ordinal, source_key, raw_name, protocol, server, port, entry_kind, classification_reason, node_logical_id, created_at)
		VALUES ('entry-notice-a', ?, ?, 0, 'k-notice', 'Notice Node A', 'vmess', '127.0.0.1', 1, 'notice', 'notice reason', 'node-notice-a', ?);
	`, payloadA1, subA, nowStr)
	if err != nil {
		t.Fatalf("failed to insert notice entry: %v", err)
	}

	// entry for node-unknown-a is unknown (not notice)
	_, err = harness.DB.ExecContext(ctx, `
		INSERT INTO subscription_entries (id, payload_id, subscription_id, ordinal, source_key, raw_name, protocol, server, port, entry_kind, classification_reason, node_logical_id, created_at)
		VALUES ('entry-unknown-a', ?, ?, 1, 'k-unknown', 'Unknown Proxy', 'hysteria2', '198.51.100.2', 443, 'unknown', 'unknown reason', 'node-unknown-a', ?);
	`, payloadA1, subA, nowStr)
	if err != nil {
		t.Fatalf("failed to insert unknown entry: %v", err)
	}

	authHeader := "Bearer " + harness.InitialAdminToken

	// ==========================================
	// Test Case 1: GET /api/v1/nodes default scope = enabled_subscriptions
	// ==========================================
	t.Run("GET /nodes default scope returns Scope E", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/nodes", nil)
		req.Header.Set("Authorization", authHeader)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var body struct {
			Data struct {
				Items []inventory.NodeView `json:"items"`
				Total int                  `json:"total"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}

		// Expected in Scope E:
		// - node-a-only (subA enabled)
		// - node-shared-ab (subA enabled)
		// - node-inactive-a (subA enabled, active=0 is NOT excluded from E!)
		// - node-unknown-a (subA enabled, not notice)
		// - node-d-member (subD enabled, last good fetchD1)
		// Total = 5!
		// Excluded:
		// - node-orphan (no source)
		// - node-b-only (subB disabled)
		// - node-notice-a (confirmed notice)
		// - node-removed-a (stale fetch)
		if body.Data.Total != 5 {
			var ids []string
			for _, it := range body.Data.Items {
				ids = append(ids, it.LogicalID)
			}
			t.Fatalf("expected total 5 in Scope E, got %d (ids: %v)", body.Data.Total, ids)
		}

		idsMap := make(map[string]bool)
		for _, it := range body.Data.Items {
			idsMap[it.LogicalID] = true
		}
		for _, expectedID := range []string{"node-a-only", "node-shared-ab", "node-inactive-a", "node-unknown-a", "node-d-member"} {
			if !idsMap[expectedID] {
				t.Errorf("expected node %s in Scope E", expectedID)
			}
		}
		for _, excludedID := range []string{"node-orphan", "node-b-only", "node-notice-a", "node-removed-a"} {
			if idsMap[excludedID] {
				t.Errorf("node %s must NOT be in Scope E", excludedID)
			}
		}
	})

	// ==========================================
	// Test Case 2: GET /api/v1/nodes?active_only=true on top of Scope E
	// ==========================================
	t.Run("GET /nodes active_only filters on top of Scope E", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/nodes?active_only=true", nil)
		req.Header.Set("Authorization", authHeader)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		var body struct {
			Data struct {
				Items []inventory.NodeView `json:"items"`
				Total int                  `json:"total"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)

		// node-inactive-a excluded, so 5 - 1 = 4
		if body.Data.Total != 4 {
			t.Fatalf("expected total 4, got %d", body.Data.Total)
		}
	})

	// ==========================================
	// Test Case 3: GET /api/v1/nodes?scope=all_assets returns all 9 assets
	// ==========================================
	t.Run("GET /nodes scope=all_assets returns full ledger", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/nodes?scope=all_assets", nil)
		req.Header.Set("Authorization", authHeader)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		var body struct {
			Data struct {
				Items []inventory.NodeView `json:"items"`
				Total int                  `json:"total"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)

		if body.Data.Total != 9 {
			t.Fatalf("expected total 9 in all_assets, got %d", body.Data.Total)
		}
	})

	// ==========================================
	// Test Case 4: Unknown scope returns 400 invalid_node_scope
	// ==========================================
	t.Run("GET /nodes unknown scope returns 400 invalid_node_scope", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/nodes?scope=unknown_scope", nil)
		req.Header.Set("Authorization", authHeader)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
		var errResp struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Code != "invalid_node_scope" {
			t.Fatalf("expected error code invalid_node_scope, got %s", errResp.Code)
		}
	})

	// ==========================================
	// Test Case 5: Single Node GET and PATCH works for historical/orphan assets
	// ==========================================
	t.Run("GET and PATCH node detail works for orphan/historical node", func(t *testing.T) {
		// GET node-orphan (which is not in Scope E)
		req, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/nodes/node-orphan", nil)
		req.Header.Set("Authorization", authHeader)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for orphan node detail, got %d", resp.StatusCode)
		}

		// PATCH node-orphan connection
		patchBody := `{"display_name": "Renamed Orphan Node"}`
		patchReq, _ := http.NewRequest(http.MethodPatch, harness.Server.URL+"/api/v1/nodes/node-orphan/connection", strings.NewReader(patchBody))
		patchReq.Header.Set("Authorization", authHeader)
		patchReq.Header.Set("Content-Type", "application/json")
		patchResp, err := http.DefaultClient.Do(patchReq)
		if err != nil {
			t.Fatal(err)
		}
		defer patchResp.Body.Close()
		if patchResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for orphan patch, got %d", patchResp.StatusCode)
		}
	})

	// ==========================================
	// Test Case 6: GET /api/v1/subscriptions returns node_count, source_node_count, counts_scope
	// ==========================================
	t.Run("GET /subscriptions reports per-sub counts and overlap", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/subscriptions", nil)
		req.Header.Set("Authorization", authHeader)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		var body struct {
			Data struct {
				Items []struct {
					ID              string `json:"id"`
					Name            string `json:"name"`
					Enabled         bool   `json:"enabled"`
					NodeCount       int    `json:"node_count"`
					SourceNodeCount int    `json:"source_node_count"`
					CountsScope     string `json:"counts_scope"`
				} `json:"items"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)

		subMap := make(map[string]struct {
			Enabled         bool
			NodeCount       int
			SourceNodeCount int
			CountsScope     string
		})
		for _, it := range body.Data.Items {
			subMap[it.ID] = struct {
				Enabled         bool
				NodeCount       int
				SourceNodeCount int
				CountsScope     string
			}{it.Enabled, it.NodeCount, it.SourceNodeCount, it.CountsScope}
		}

		// subA (enabled): members in fetchA1 excluding notice (node-notice-a excluded):
		// - node-a-only
		// - node-shared-ab
		// - node-inactive-a
		// - node-unknown-a
		// Count = 4!
		subAInfo := subMap[subA]
		if subAInfo.CountsScope != "enabled_subscriptions" {
			t.Errorf("expected counts_scope enabled_subscriptions, got %s", subAInfo.CountsScope)
		}
		if subAInfo.NodeCount != 4 || subAInfo.SourceNodeCount != 4 {
			t.Errorf("subA: expected node_count 4 and source_node_count 4, got %d / %d", subAInfo.NodeCount, subAInfo.SourceNodeCount)
		}

		// subB (disabled):
		// - node-shared-ab
		// - node-b-only
		// source_node_count = 2, node_count = 0 (disabled)!
		subBInfo := subMap[subB]
		if subBInfo.NodeCount != 0 {
			t.Errorf("disabled subB node_count must be 0, got %d", subBInfo.NodeCount)
		}
		if subBInfo.SourceNodeCount != 2 {
			t.Errorf("disabled subB source_node_count must be 2, got %d", subBInfo.SourceNodeCount)
		}

		// subC (enabled, but all fetches failed):
		// node_count = 0, source_node_count = 0
		subCInfo := subMap[subC]
		if subCInfo.NodeCount != 0 || subCInfo.SourceNodeCount != 0 {
			t.Errorf("subC without good fetch: expected 0/0, got %d / %d", subCInfo.NodeCount, subCInfo.SourceNodeCount)
		}

		// subD (enabled, latest good fetchD1):
		// - node-d-member -> 1
		subDInfo := subMap[subD]
		if subDInfo.NodeCount != 1 || subDInfo.SourceNodeCount != 1 {
			t.Errorf("subD: expected 1/1, got %d / %d", subDInfo.NodeCount, subDInfo.SourceNodeCount)
		}
	})

	// ==========================================
	// Test Case 7: Toggle subA disabled -> Scope E changes instantly, node active untouched
	// ==========================================
	t.Run("Toggle subA disabled and re-enabled", func(t *testing.T) {
		// Disable subA: PATCH /api/v1/subscriptions/subA
		toggleReq, _ := http.NewRequest(http.MethodPatch, harness.Server.URL+"/api/v1/subscriptions/"+subA, strings.NewReader(`{"enabled": false}`))
		toggleReq.Header.Set("Authorization", authHeader)
		toggleReq.Header.Set("If-Match", "rev1")
		toggleReq.Header.Set("Content-Type", "application/json")
		toggleResp, err := http.DefaultClient.Do(toggleReq)
		if err != nil {
			t.Fatal(err)
		}
		defer toggleResp.Body.Close()
		if toggleResp.StatusCode != http.StatusOK {
			t.Fatalf("failed to disable subA: %d", toggleResp.StatusCode)
		}

		var toggleView struct {
			Data struct {
				Revision string `json:"revision"`
			} `json:"data"`
		}
		_ = json.NewDecoder(toggleResp.Body).Decode(&toggleView)

		// Verify Scope E now: subA is disabled, subB is disabled, subC has 0.
		// Only subD is enabled with node-d-member!
		req, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/nodes", nil)
		req.Header.Set("Authorization", authHeader)
		resp, _ := http.DefaultClient.Do(req)
		var bodyAfterDisable struct {
			Data struct {
				Items []inventory.NodeView `json:"items"`
				Total int                  `json:"total"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&bodyAfterDisable)
		resp.Body.Close()

		if bodyAfterDisable.Data.Total != 1 || len(bodyAfterDisable.Data.Items) != 1 || bodyAfterDisable.Data.Items[0].LogicalID != "node-d-member" {
			t.Fatalf("expected Scope E to only contain node-d-member after disabling subA, got total %d", bodyAfterDisable.Data.Total)
		}

		// Re-enable subA:
		enableReq, _ := http.NewRequest(http.MethodPatch, harness.Server.URL+"/api/v1/subscriptions/"+subA, strings.NewReader(`{"enabled": true}`))
		enableReq.Header.Set("Authorization", authHeader)
		enableReq.Header.Set("If-Match", toggleView.Data.Revision)
		enableReq.Header.Set("Content-Type", "application/json")
		enableResp, _ := http.DefaultClient.Do(enableReq)
		enableResp.Body.Close()

		// Verify Scope E restored back to 5!
		req2, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/nodes", nil)
		req2.Header.Set("Authorization", authHeader)
		resp2, _ := http.DefaultClient.Do(req2)
		var bodyAfterReenable struct {
			Data struct {
				Total int `json:"total"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp2.Body).Decode(&bodyAfterReenable)
		resp2.Body.Close()

		if bodyAfterReenable.Data.Total != 5 {
			t.Fatalf("expected Scope E restored to 5, got %d", bodyAfterReenable.Data.Total)
		}
	})

	// ==========================================
	// Test Case 8: GET /api/v1/probes/pool inventory_total vs candidate_total reconciliation
	// ==========================================
	t.Run("GET /probes/pool inventory vs candidate reconciliation", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/probes/pool", nil)
		req.Header.Set("Authorization", authHeader)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		var poolResp struct {
			Data domain.ProbePoolStatus `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&poolResp)

		// Scope E total inventory = 5
		// Scope E candidate (active=1) = 4 (node-inactive-a has active=0)
		if poolResp.Data.Scope != "enabled_subscriptions" {
			t.Errorf("expected pool scope enabled_subscriptions, got %s", poolResp.Data.Scope)
		}
		if poolResp.Data.InventoryTotal != 5 {
			t.Errorf("expected inventory_total 5, got %d", poolResp.Data.InventoryTotal)
		}
		if poolResp.Data.CandidateTotal != 4 {
			t.Errorf("expected candidate_total 4, got %d", poolResp.Data.CandidateTotal)
		}
		if poolResp.Data.TotalCount != 4 {
			t.Errorf("expected legacy total_count 4 (matching candidate_total), got %d", poolResp.Data.TotalCount)
		}
	})

	// ==========================================
	// Test Case 9: Zero enabled / all disabled:
	// E=0, pool=0, publication preview in compatible mode returns empty_group_not_allowed error,
	// and prior published bytes remain immutable.
	// ==========================================
	t.Run("Zero enabled and orphan inventory: E=0, pool=0, publication preview error, immutable old published bytes", func(t *testing.T) {
		// 1. Setup policy group and revision while subA is enabled
		revID, _ := domain.NewUUIDv7()
		_ = harness.RevisionRepo.Create(ctx, &domain.ConfigurationRevision{
			ID:            revID,
			ContentDigest: "sha256:e2e-rev-digest",
			State:         domain.RevisionStateDraft,
			CreatedAt:     domain.NowUTC(),
		})
		_ = harness.RevisionRepo.SetActive(ctx, revID)

		groupID, _ := domain.NewUUIDv7()
		_ = harness.PolicyRepo.CreateGroup(ctx, &domain.NodeGroup{
			ID:        groupID,
			Name:      "TEST-GROUP",
			GroupType: domain.GroupTypeSelect,
		})
		nodeAID := "node-a-only"
		_ = harness.PolicyRepo.SetEdgesForGroup(ctx, groupID, []domain.GroupEdge{
			{ID: domain.MustNewUUIDv7(), ParentGroupID: groupID, NodeLogicalID: &nodeAID, Position: 0},
		})
		ruleID, _ := domain.NewUUIDv7()
		_ = harness.PolicyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
			ID:            ruleID,
			RevisionID:    revID,
			TargetGroupID: groupID,
			Expression:    "MATCH",
			Position:      0,
		})

		// 2. Preview and Publish while subA is enabled
		prevReq, _ := http.NewRequest(http.MethodPost, harness.Server.URL+"/api/v1/publications/preview", strings.NewReader(`{"target": "mihomo"}`))
		prevReq.Header.Set("Authorization", authHeader)
		prevReq.Header.Set("Content-Type", "application/json")
		prevResp, err := http.DefaultClient.Do(prevReq)
		if err != nil {
			t.Fatal(err)
		}
		var prevData struct {
			Data struct {
				SnapshotID string `json:"snapshot_id"`
			} `json:"data"`
		}
		_ = json.NewDecoder(prevResp.Body).Decode(&prevData)
		prevResp.Body.Close()

		pubReq, _ := http.NewRequest(http.MethodPost, harness.Server.URL+"/api/v1/publications", strings.NewReader(fmt.Sprintf(`{"target": "mihomo", "snapshot_id": %q}`, prevData.Data.SnapshotID)))
		pubReq.Header.Set("Authorization", authHeader)
		pubReq.Header.Set("Content-Type", "application/json")
		pubResp, err := http.DefaultClient.Do(pubReq)
		if err != nil {
			t.Fatal(err)
		}
		var pubData struct {
			Data struct {
				Publication struct {
					ID string `json:"id"`
				} `json:"publication"`
				RawToken string `json:"raw_token"`
			} `json:"data"`
		}
		_ = json.NewDecoder(pubResp.Body).Decode(&pubData)
		pubResp.Body.Close()
		pubID := pubData.Data.Publication.ID
		rawToken := pubData.Data.RawToken

		// Fetch original published bytes
		exportReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/publish/v1/%s?token=%s", harness.Server.URL, pubID, rawToken), nil)
		exportResp, err := http.DefaultClient.Do(exportReq)
		if err != nil {
			t.Fatal(err)
		}
		origBytes, _ := io.ReadAll(exportResp.Body)
		exportResp.Body.Close()
		if len(origBytes) == 0 {
			t.Fatalf("expected non-empty original published bytes")
		}

		// Helper to toggle sub enabled state with proper If-Match
		toggleSub := func(sID string, enabled bool) {
			getReq, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/subscriptions/"+sID, nil)
			getReq.Header.Set("Authorization", authHeader)
			getResp, err := http.DefaultClient.Do(getReq)
			if err != nil {
				t.Fatal(err)
			}
			var getView struct {
				Data struct {
					Revision string `json:"revision"`
				} `json:"data"`
			}
			_ = json.NewDecoder(getResp.Body).Decode(&getView)
			getResp.Body.Close()

			patchBody := fmt.Sprintf(`{"enabled": %t}`, enabled)
			r, _ := http.NewRequest(http.MethodPatch, harness.Server.URL+"/api/v1/subscriptions/"+sID, strings.NewReader(patchBody))
			r.Header.Set("Authorization", authHeader)
			r.Header.Set("If-Match", getView.Data.Revision)
			r.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != http.StatusOK {
				t.Fatalf("failed to toggle sub %s to %t, got status %d", sID, enabled, res.StatusCode)
			}
			res.Body.Close()
		}

		// 3. Disable all subscriptions (subA, subD)
		toggleSub(subA, false)
		toggleSub(subD, false)

		// 4. Verify Scope E = 0 (even though all 9 nodes exist in ledger)
		nodesReq, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/nodes", nil)
		nodesReq.Header.Set("Authorization", authHeader)
		nodesResp, _ := http.DefaultClient.Do(nodesReq)
		var nodesBody struct {
			Data struct {
				Total int                  `json:"total"`
				Items []inventory.NodeView `json:"items"`
			} `json:"data"`
		}
		_ = json.NewDecoder(nodesResp.Body).Decode(&nodesBody)
		nodesResp.Body.Close()
		if nodesBody.Data.Total != 0 || len(nodesBody.Data.Items) != 0 {
			t.Fatalf("expected 0 nodes in Scope E when all subs disabled, got %d", nodesBody.Data.Total)
		}

		// 5. Verify probes pool = 0
		poolReq, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/api/v1/probes/pool", nil)
		poolReq.Header.Set("Authorization", authHeader)
		poolResp, _ := http.DefaultClient.Do(poolReq)
		var poolBody struct {
			Data domain.ProbePoolStatus `json:"data"`
		}
		_ = json.NewDecoder(poolResp.Body).Decode(&poolBody)
		poolResp.Body.Close()
		if poolBody.Data.InventoryTotal != 0 || poolBody.Data.CandidateTotal != 0 || poolBody.Data.TotalCount != 0 {
			t.Fatalf("expected 0 pool counts when all subs disabled, got inv=%d cand=%d total=%d",
				poolBody.Data.InventoryTotal, poolBody.Data.CandidateTotal, poolBody.Data.TotalCount)
		}

		// 6. Verify new preview in compatible mode fails with business error empty_group_not_allowed
		errPrevReq, _ := http.NewRequest(http.MethodPost, harness.Server.URL+"/api/v1/publications/preview", strings.NewReader(`{"target": "mihomo", "compat_mode": "compatible"}`))
		errPrevReq.Header.Set("Authorization", authHeader)
		errPrevReq.Header.Set("Content-Type", "application/json")
		errPrevResp, err := http.DefaultClient.Do(errPrevReq)
		if err != nil {
			t.Fatal(err)
		}
		errBodyBytes, _ := io.ReadAll(errPrevResp.Body)
		errPrevResp.Body.Close()
		if errPrevResp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for compatible preview with 0 candidates, got %d. body=%s", errPrevResp.StatusCode, string(errBodyBytes))
		}
		var errPrevBody struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(errBodyBytes, &errPrevBody)
		if errPrevBody.Code != "empty_group_not_allowed" {
			t.Fatalf("expected error code empty_group_not_allowed, got %s", errPrevBody.Code)
		}

		// 7. Verify previously published bytes remain strictly immutable
		exportReq2, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/publish/v1/%s?token=%s", harness.Server.URL, pubID, rawToken), nil)
		exportResp2, err := http.DefaultClient.Do(exportReq2)
		if err != nil {
			t.Fatal(err)
		}
		afterBytes, _ := io.ReadAll(exportResp2.Body)
		exportResp2.Body.Close()
		if string(afterBytes) != string(origBytes) {
			t.Fatalf("published bytes changed after disabling subscriptions: expected %s, got %s", string(origBytes), string(afterBytes))
		}

		// Clean up: re-enable subA and subD
		toggleSub(subA, true)
		toggleSub(subD, true)
	})
}
