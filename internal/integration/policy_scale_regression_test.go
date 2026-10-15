package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ipriskApp "clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
	"clash-sub-parser/migrations"
	"gopkg.in/yaml.v3"
)

// TestPolicyScale_IsolatedAPI_250Nodes150Groups validates the unified member sources rebuild
// against a real isolated temporary SQLite database, with >250 active nodes and 150 groups,
// testing end-to-end HTTP API behavior.
func TestPolicyScale_IsolatedAPI_250Nodes150Groups(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "policy_scale_isolated.db")

	db, err := sqlite.Open(sqlite.Config{
		Path:        dbPath,
		BusyTimeout: 10 * time.Second,
		ForeignKeys: true,
	})
	if err != nil {
		t.Fatalf("failed to open isolated sqlite db: %v", err)
	}
	defer db.Close()

	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Repositories
	settingsRepo := sqlite.NewSettingsRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	nodeSourceRepo := sqlite.NewNodeSourceRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	revisionRepo := sqlite.NewRevisionRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	nodeFilterRepo := sqlite.NewNodeFilterRepository(db)
	riskObsRepo := sqlite.NewIPRiskObservationRepository(db)
	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	riskBindingRepo := sqlite.NewRiskPolicyGroupBindingRepository(db)
	probeObsRepo := sqlite.NewProbeObservationRepository(db)

	policyService := policy.NewService(policyRepo, revisionRepo, nodeRepo, auditRepo, nodeFilterRepo)
	revisionService := revision.NewService(revisionRepo, auditRepo, revision.WithPolicyRepository(policyRepo))

	ipriskService := ipriskApp.NewService(
		riskObsRepo,
		riskPolicyRepo,
		ipriskApp.WithBindingRepository(riskBindingRepo),
		ipriskApp.WithGroupRepository(policyRepo),
		ipriskApp.WithNodeRepository(nodeRepo),
		ipriskApp.WithAuditRepository(auditRepo),
	)

	pubService := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revisionRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeFilterRepository(nodeFilterRepo),
		publication.WithNodeSourceRepository(nodeSourceRepo),
		publication.WithProbeObservationRepository(probeObsRepo),
		publication.WithIPRiskService(ipriskService),
		publication.WithRiskPolicyRepository(riskPolicyRepo),
		publication.WithRiskBindingRepository(riskBindingRepo),
		publication.WithRiskObservationRepository(riskObsRepo),
	)

	const adminToken = "secret-scale-test-token-002"
	hashedToken, err := transporthttp.HashToken(adminToken)
	if err != nil {
		t.Fatalf("hash admin token failed: %v", err)
	}
	if err := settingsRepo.UpdateAdminToken(ctx, hashedToken); err != nil {
		t.Fatalf("persist admin token failed: %v", err)
	}

	sessionStore := transporthttp.NewMemorySessionStore(1 * time.Hour)
	tokenHolder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		InitialToken:    adminToken,
		InitialVerifier: hashedToken,
		SettingsRepo:    settingsRepo,
		SessionStore:    sessionStore,
		HashCost:        transporthttp.MinHashCost,
	})

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		TokenHolder:        tokenHolder,
		SettingsRepository: settingsRepo,
		SessionStore:       sessionStore,
		PolicyService:      policyService,
		RevisionService:    revisionService,
		PublicationService: pubService,
		AuditRepository:    auditRepo,
	})

	ts := httptest.NewServer(router)
	defer ts.Close()
	client := ts.Client()

	doReq := func(method, path string, body any) (*http.Response, []byte) {
		var rdr io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal request body failed: %v", err)
			}
			rdr = bytes.NewReader(b)
		}
		req, err := http.NewRequest(method, ts.URL+path, rdr)
		if err != nil {
			t.Fatalf("create request failed: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+adminToken)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("do request %s %s failed: %v", method, path, err)
		}
		defer resp.Body.Close()
		out, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read response body failed: %v", err)
		}
		return resp, out
	}

	// 1. Seed Subscription and 260 Real Nodes into SQLite
	subID := domain.MustNewUUIDv7()
	fetchID := domain.MustNewUUIDv7()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	_, err = db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES (?, 'Scale Test Subscription', 'secret-scale', 1, 'rev-1', ?, ?);
	`, subID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert subscription failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, finished_at, outcome)
		VALUES (?, ?, ?, ?, 'success');
	`, fetchID, subID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert subscription fetch failed: %v", err)
	}

	const totalNodes = 260
	seededNodes := make([]domain.Node, totalNodes)
	nodeLogicalIDs := make([]string, totalNodes)
	for i := 0; i < totalNodes; i++ {
		var proto domain.Protocol
		switch i % 3 {
		case 0:
			proto = domain.ProtocolTrojan
		case 1:
			proto = domain.ProtocolSS
		case 2:
			proto = domain.ProtocolVMess
		}

		dispName := fmt.Sprintf("Node-%03d-HK-Fast", i+1)
		if i%5 == 0 {
			dispName = fmt.Sprintf("Node-%03d-US-BGP", i+1)
		} else if i%7 == 0 {
			dispName = fmt.Sprintf("Node-%03d-JP-Direct", i+1)
		}

		server := fmt.Sprintf("198.51.100.%d", (i%250)+1)
		port := 10000 + i
		transport := map[string]string{}
		if proto == domain.ProtocolTrojan || proto == domain.ProtocolHysteria2 {
			transport["sni"] = "scale.example.com"
			transport["tls"] = "true"
		}
		lid := domain.ComputeNodeLogicalID(proto, server, port, transport)
		nodeLogicalIDs[i] = lid

		creds := domain.InboundProtocolCredential{
			Transport: transport,
		}
		if proto == domain.ProtocolSS {
			creds.Method = "chacha20-ietf-poly1305"
			creds.Password = "ss-secret-12345"
		} else if proto == domain.ProtocolTrojan || proto == domain.ProtocolHysteria2 {
			creds.Password = "trojan-pass-12345"
			creds.SNI = "scale.example.com"
		} else {
			creds.UUID = "b6a0b9af-c116-7806-9740-66b8b070ba2f"
		}

		node := domain.Node{
			LogicalID:   lid,
			Protocol:    proto,
			DisplayName: dispName,
			Server:      server,
			Port:        port,
			Credentials: creds,
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		seededNodes[i] = node
	}

	if err := nodeRepo.UpsertBatch(ctx, seededNodes); err != nil {
		t.Fatalf("upsert batch nodes failed: %v", err)
	}

	for i := 0; i < totalNodes; i++ {
		_, err := db.ExecContext(ctx, `
			INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
			VALUES (?, ?, ?);
		`, nodeLogicalIDs[i], subID, fetchID)
		if err != nil {
			t.Fatalf("insert node source %d failed: %v", i, err)
		}
	}

	// Active configuration revision
	activeRev, err := policyService.EnsureActiveRevision(ctx)
	if err != nil {
		t.Fatalf("ensure active revision failed: %v", err)
	}

	// 2. Seed 150 Policy Groups into SQLite
	const totalGroups = 150
	groupIDs := make([]string, totalGroups)
	for i := 0; i < totalGroups; i++ {
		gid := domain.MustNewUUIDv7()
		groupIDs[i] = gid
		gName := fmt.Sprintf("PolicyGroup-%03d", i+1)
		gType := domain.GroupTypeSelect
		if i%3 == 1 {
			gType = domain.GroupTypeURLTest
		} else if i%3 == 2 {
			gType = domain.GroupTypeFallback
		}

		g := domain.NodeGroup{
			ID:                gid,
			Name:              gName,
			GroupType:         gType,
			Revision:          1,
			EmptyFallbackPass: true,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := policyRepo.CreateGroup(ctx, &g); err != nil {
			t.Fatalf("seed group %d failed: %v", i+1, err)
		}
	}

	// Route the primary group to satisfy default rules
	rootGroupID := groupIDs[0]
	childGroupID := groupIDs[1]

	// Populate rootGroupID with HK nodes regex so it has active members
	rev1 := int64(1)
	srcs := []domain.MemberSource{
		{
			Operation: domain.OperationAdd,
			Kind:      domain.SourceKindRegex,
			Position:  0,
			Pattern:   func() *string { s := ".*"; return &s }(),
		},
	}
	_, err = policyService.UpdateGroup(ctx, policy.UpdateGroupCommand{
		ID:               rootGroupID,
		ExpectedRevision: &rev1,
		Sources:          &srcs,
	})
	if err != nil {
		t.Fatalf("populate root group failed: %v", err)
	}

	_, err = policyService.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		RevisionID:    activeRev.ID,
		TargetGroupID: rootGroupID,
		Expression:    "MATCH",
		Position:      0,
	})
	if err != nil {
		t.Fatalf("create MATCH policy rule failed: %v", err)
	}

	// -------------------------------------------------------------
	// SCENARIO A: source exact orderedPUTGET
	// -------------------------------------------------------------
	t.Run("source_exact_ordered_PUT_GET", func(t *testing.T) {
		testGID := groupIDs[10]
		node1 := nodeLogicalIDs[0]
		node2 := nodeLogicalIDs[1]
		pat := ".*HK.*"
		direct := "DIRECT"

		putPayload := map[string]any{
			"name":                "Ordered Group 10",
			"group_type":          "select",
			"expected_revision":   1,
			"empty_fallback_pass": false,
			"sources": []map[string]any{
				{
					"operation":       "add",
					"kind":            "manual_node",
					"position":        0,
					"node_logical_id": node1,
				},
				{
					"operation": "add",
					"kind":      "regex",
					"position":  1,
					"pattern":   pat,
				},
				{
					"operation":    "add",
					"kind":         "typed_builtin",
					"position":     2,
					"builtin_name": direct,
				},
				{
					"operation":       "add",
					"kind":            "group_nodes",
					"position":        3,
					"target_group_id": childGroupID,
				},
				{
					"operation":       "add",
					"kind":            "manual_node",
					"position":        4,
					"node_logical_id": node2,
				},
			},
		}

		resp, body := doReq("PUT", "/api/v1/policies/groups/"+testGID, putPayload)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT /groups/%s failed with status %d: %s", testGID, resp.StatusCode, string(body))
		}

		// GET back and verify byte-for-byte exact order
		getResp, getBody := doReq("GET", "/api/v1/policies/groups/"+testGID, nil)
		if getResp.StatusCode != http.StatusOK {
			t.Fatalf("GET /groups/%s failed with status %d: %s", testGID, getResp.StatusCode, string(getBody))
		}

		var getEnvelope struct {
			Data struct {
				ID       string                `json:"id"`
				Revision int64                 `json:"revision"`
				Sources  []domain.MemberSource `json:"sources"`
			} `json:"data"`
		}
		if err := json.Unmarshal(getBody, &getEnvelope); err != nil {
			t.Fatalf("unmarshal GET response failed: %v", err)
		}

		if getEnvelope.Data.Revision != 2 {
			t.Fatalf("expected revision to increment to 2, got %d", getEnvelope.Data.Revision)
		}
		if len(getEnvelope.Data.Sources) != 5 {
			t.Fatalf("expected 5 sources, got %d", len(getEnvelope.Data.Sources))
		}

		expectedKinds := []domain.MemberSourceKind{
			domain.SourceKindManualNode,
			domain.SourceKindRegex,
			domain.SourceKindTypedBuiltin,
			domain.SourceKindGroupNodes,
			domain.SourceKindManualNode,
		}
		for idx, src := range getEnvelope.Data.Sources {
			if src.Position != idx {
				t.Errorf("source [%d] position mismatch: expected %d, got %d", idx, idx, src.Position)
			}
			if src.Kind != expectedKinds[idx] {
				t.Errorf("source [%d] kind mismatch: expected %s, got %s", idx, expectedKinds[idx], src.Kind)
			}
			if src.Operation != domain.OperationAdd {
				t.Errorf("source [%d] operation mismatch: expected add, got %s", idx, src.Operation)
			}
		}
	})

	// -------------------------------------------------------------
	// SCENARIO B: readdatend (sequential add-remove-add)
	// -------------------------------------------------------------
	t.Run("readdatend_sequential_tombstone_cleared", func(t *testing.T) {
		testGID := groupIDs[20]
		targetNode := nodeLogicalIDs[5]
		otherNode := nodeLogicalIDs[6]

		putPayload := map[string]any{
			"name":                "ReAdd Group 20",
			"group_type":          "select",
			"expected_revision":   1,
			"empty_fallback_pass": false,
			"sources": []map[string]any{
				{
					"operation":       "add",
					"kind":            "manual_node",
					"position":        0,
					"node_logical_id": targetNode,
				},
				{
					"operation":       "add",
					"kind":            "manual_node",
					"position":        1,
					"node_logical_id": otherNode,
				},
				{
					"operation":       "remove",
					"kind":            "manual_node",
					"position":        2,
					"node_logical_id": targetNode,
				},
				{
					"operation":       "add",
					"kind":            "manual_node",
					"position":        3,
					"node_logical_id": targetNode, // re-added at the end!
				},
			},
		}

		resp, body := doReq("PUT", "/api/v1/policies/groups/"+testGID, putPayload)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT failed: %d %s", resp.StatusCode, string(body))
		}

		// Preview group resolution
		prevPayload := map[string]any{
			"group_id":          testGID,
			"expected_revision": 2,
			"sources":           putPayload["sources"],
		}
		pResp, pBody := doReq("POST", "/api/v1/policies/groups/preview", prevPayload)
		if pResp.StatusCode != http.StatusOK {
			t.Fatalf("preview failed: %d %s", pResp.StatusCode, string(pBody))
		}

		var prevResult struct {
			Data struct {
				Members []struct {
					Kind          string  `json:"kind"`
					NodeLogicalID *string `json:"node_logical_id"`
					Position      int     `json:"position"`
				} `json:"members"`
			} `json:"data"`
		}
		if err := json.Unmarshal(pBody, &prevResult); err != nil {
			t.Fatalf("unmarshal preview failed: %v", err)
		}

		members := prevResult.Data.Members
		if len(members) != 2 {
			t.Fatalf("expected 2 members in final set (otherNode and targetNode at end), got %d", len(members))
		}
		if *members[0].NodeLogicalID != otherNode {
			t.Errorf("expected first member to be otherNode %s, got %s", otherNode, *members[0].NodeLogicalID)
		}
		if *members[1].NodeLogicalID != targetNode {
			t.Errorf("expected second member to be re-added targetNode %s at end, got %s", targetNode, *members[1].NodeLogicalID)
		}
	})

	// -------------------------------------------------------------
	// SCENARIO C: samechildbothref+expand
	// -------------------------------------------------------------
	t.Run("samechildbothref_and_expand", func(t *testing.T) {
		// Populate childGroupID with 2 manual nodes first
		cNode1 := nodeLogicalIDs[10]
		cNode2 := nodeLogicalIDs[11]
		putChild := map[string]any{
			"name":                "Child Group",
			"group_type":          "select",
			"expected_revision":   1,
			"empty_fallback_pass": false,
			"sources": []map[string]any{
				{"operation": "add", "kind": "manual_node", "position": 0, "node_logical_id": cNode1},
				{"operation": "add", "kind": "manual_node", "position": 1, "node_logical_id": cNode2},
			},
		}
		resp, body := doReq("PUT", "/api/v1/policies/groups/"+childGroupID, putChild)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT child group failed: %d %s", resp.StatusCode, string(body))
		}

		// Parent includes child as group_ref AND group_nodes
		parentGID := groupIDs[30]
		putParent := map[string]any{
			"name":                "Parent Ref and Expand",
			"group_type":          "select",
			"expected_revision":   1,
			"empty_fallback_pass": false,
			"sources": []map[string]any{
				{
					"operation":       "add",
					"kind":            "group_ref",
					"position":        0,
					"target_group_id": childGroupID,
				},
				{
					"operation":       "add",
					"kind":            "group_nodes",
					"position":        1,
					"target_group_id": childGroupID,
				},
			},
		}
		resp, body = doReq("PUT", "/api/v1/policies/groups/"+parentGID, putParent)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT parent group failed: %d %s", resp.StatusCode, string(body))
		}

		// Preview parent
		prevPayload := map[string]any{
			"group_id":          parentGID,
			"expected_revision": 2,
			"sources":           putParent["sources"],
		}
		pResp, pBody := doReq("POST", "/api/v1/policies/groups/preview", prevPayload)
		if pResp.StatusCode != http.StatusOK {
			t.Fatalf("preview parent failed: %d %s", pResp.StatusCode, string(pBody))
		}

		var prevResult struct {
			Data struct {
				Members []struct {
					Kind          string  `json:"kind"`
					GroupID       *string `json:"group_id"`
					NodeLogicalID *string `json:"node_logical_id"`
				} `json:"members"`
			} `json:"data"`
		}
		if err := json.Unmarshal(pBody, &prevResult); err != nil {
			t.Fatalf("unmarshal preview failed: %v", err)
		}

		members := prevResult.Data.Members
		// Should have 1 group member and 2 expanded node members = 3 members total
		if len(members) != 3 {
			t.Fatalf("expected 3 members (1 group_ref + 2 expanded nodes), got %d", len(members))
		}
		if members[0].Kind != "group" || *members[0].GroupID != childGroupID {
			t.Errorf("expected member 0 to be group %s, got %+v", childGroupID, members[0])
		}
		if members[1].Kind != "node" || *members[1].NodeLogicalID != cNode1 {
			t.Errorf("expected member 1 to be node %s, got %+v", cNode1, members[1])
		}
		if members[2].Kind != "node" || *members[2].NodeLogicalID != cNode2 {
			t.Errorf("expected member 2 to be node %s, got %+v", cNode2, members[2])
		}
	})

	// -------------------------------------------------------------
	// SCENARIO D: typeidentityremove
	// -------------------------------------------------------------
	t.Run("typeidentityremove_groupref_vs_groupnodes", func(t *testing.T) {
		parentGID := groupIDs[40]
		putPayload := map[string]any{
			"name":                "Type Identity Remove",
			"group_type":          "select",
			"expected_revision":   1,
			"empty_fallback_pass": false,
			"sources": []map[string]any{
				{"operation": "add", "kind": "group_ref", "position": 0, "target_group_id": childGroupID},
				{"operation": "add", "kind": "group_nodes", "position": 1, "target_group_id": childGroupID},
				// Remove ONLY group_ref, NOT group_nodes
				{"operation": "remove", "kind": "group_ref", "position": 2, "target_group_id": childGroupID},
			},
		}

		pResp, pBody := doReq("POST", "/api/v1/policies/groups/preview", map[string]any{
			"group_id": parentGID,
			"sources":  putPayload["sources"],
		})
		if pResp.StatusCode != http.StatusOK {
			t.Fatalf("preview failed: %d %s", pResp.StatusCode, string(pBody))
		}

		var prevResult struct {
			Data struct {
				Members []struct {
					Kind string `json:"kind"`
				} `json:"members"`
			} `json:"data"`
		}
		json.Unmarshal(pBody, &prevResult)

		// Group ref was removed; only the 2 nodes from group_nodes remain!
		if len(prevResult.Data.Members) != 2 {
			t.Fatalf("expected 2 node members remaining after removing group_ref, got %d", len(prevResult.Data.Members))
		}
		for idx, m := range prevResult.Data.Members {
			if m.Kind != "node" {
				t.Errorf("member %d expected kind 'node', got %s", idx, m.Kind)
			}
		}
	})

	// -------------------------------------------------------------
	// SCENARIO E: revision conflict rollback (409)
	// -------------------------------------------------------------
	t.Run("revision_conflict_409_rollback", func(t *testing.T) {
		testGID := groupIDs[50]

		// Initial update to revision 2
		_, _ = doReq("PUT", "/api/v1/policies/groups/"+testGID, map[string]any{
			"name":                "Initial Conflicted",
			"group_type":          "select",
			"expected_revision":   1,
			"empty_fallback_pass": true,
			"sources":             []any{},
		})

		// Now send stale revision 1 when current is 2
		stalePayload := map[string]any{
			"name":              "Stale Update Attempt",
			"group_type":        "select",
			"expected_revision": 1,
			"sources": []map[string]any{
				{"operation": "add", "kind": "manual_node", "position": 0, "node_logical_id": nodeLogicalIDs[0]},
			},
		}

		resp, body := doReq("PUT", "/api/v1/policies/groups/"+testGID, stalePayload)
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d: %s", resp.StatusCode, string(body))
		}

		var errResp struct {
			Code            string         `json:"code"`
			CurrentRevision *int64         `json:"current_revision"`
			Details         map[string]any `json:"details"`
		}
		if err := json.Unmarshal(body, &errResp); err != nil {
			t.Fatalf("unmarshal error response failed: %v", err)
		}

		if errResp.Code != "revision_conflict" {
			t.Errorf("expected code 'revision_conflict', got %s", errResp.Code)
		}
		if errResp.CurrentRevision == nil || *errResp.CurrentRevision != 2 {
			t.Errorf("expected current_revision 2, got %v", errResp.CurrentRevision)
		}
		if errResp.Details == nil || errResp.Details["current_revision"] != float64(2) {
			t.Errorf("expected details.current_revision 2, got %+v", errResp.Details)
		}

		// Verify database rollback: group still has revision 2 and 0 sources
		g, err := policyRepo.GetGroupByID(ctx, testGID)
		if err != nil {
			t.Fatalf("get group failed: %v", err)
		}
		if g.Revision != 2 {
			t.Errorf("expected DB revision to remain 2, got %d", g.Revision)
		}
		if len(g.Sources) != 0 {
			t.Errorf("expected DB sources to remain 0 after rollback, got %d", len(g.Sources))
		}
	})

	// -------------------------------------------------------------
	// SCENARIO F: badregex / cycle (422)
	// -------------------------------------------------------------
	t.Run("badregex_and_cycle_422", func(t *testing.T) {
		testGID := groupIDs[60]

		// 1. Bad regex
		badRegexPayload := map[string]any{
			"name":              "Bad Regex Group",
			"group_type":        "select",
			"expected_revision": 1,
			"sources": []map[string]any{
				{"operation": "add", "kind": "regex", "position": 0, "pattern": "(?["},
			},
		}
		resp, body := doReq("PUT", "/api/v1/policies/groups/"+testGID, badRegexPayload)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for invalid regex, got %d: %s", resp.StatusCode, string(body))
		}
		if !strings.Contains(string(body), "invalid_regex") {
			t.Errorf("expected error message to contain invalid_regex, got: %s", string(body))
		}

		// 2. Cycle detection: A -> B -> A
		gidA := groupIDs[61]
		gidB := groupIDs[62]

		// Set A -> B
		_, _ = doReq("PUT", "/api/v1/policies/groups/"+gidA, map[string]any{
			"name":              "Group A",
			"group_type":        "select",
			"expected_revision": 1,
			"sources": []map[string]any{
				{"operation": "add", "kind": "group_ref", "position": 0, "target_group_id": gidB},
			},
		})

		// Now try to set B -> A (creates cycle!)
		resp, body = doReq("PUT", "/api/v1/policies/groups/"+gidB, map[string]any{
			"name":              "Group B",
			"group_type":        "select",
			"expected_revision": 1,
			"sources": []map[string]any{
				{"operation": "add", "kind": "group_ref", "position": 0, "target_group_id": gidA},
			},
		})

		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for cycle detection, got %d: %s", resp.StatusCode, string(body))
		}

		var cycleErr struct {
			Code      string         `json:"code"`
			CyclePath []string       `json:"cycle_path"`
			Details   map[string]any `json:"details"`
		}
		json.Unmarshal(body, &cycleErr)
		if cycleErr.Code != "cycle_detected" {
			t.Errorf("expected code 'cycle_detected', got %s", cycleErr.Code)
		}
		if len(cycleErr.CyclePath) == 0 {
			t.Errorf("expected non-empty cycle_path, got %+v", cycleErr.CyclePath)
		}
		if cycleErr.Details == nil || cycleErr.Details["cycle_path"] == nil {
			t.Errorf("expected details.cycle_path to be present, got %+v", cycleErr.Details)
		}
	})

	// -------------------------------------------------------------
	// SCENARIO G: draft noDBwrite & selection digest fidelity
	// -------------------------------------------------------------
	t.Run("draft_noDBwrite_and_digest_fidelity", func(t *testing.T) {
		testGID := groupIDs[70]
		initialGroup, err := policyRepo.GetGroupByID(ctx, testGID)
		if err != nil {
			t.Fatalf("failed to get group: %v", err)
		}
		initRev := initialGroup.Revision

		// Draft payload with 3 manual nodes
		draftPayload := map[string]any{
			"group_id":          testGID,
			"expected_revision": initRev,
			"sources": []map[string]any{
				{"operation": "add", "kind": "manual_node", "position": 0, "node_logical_id": nodeLogicalIDs[0]},
				{"operation": "add", "kind": "manual_node", "position": 1, "node_logical_id": nodeLogicalIDs[1]},
				{"operation": "add", "kind": "manual_node", "position": 2, "node_logical_id": nodeLogicalIDs[2]},
			},
		}

		pResp, pBody := doReq("POST", "/api/v1/policies/groups/preview", draftPayload)
		if pResp.StatusCode != http.StatusOK {
			t.Fatalf("preview draft failed: %d %s", pResp.StatusCode, string(pBody))
		}

		var prevResp struct {
			Data struct {
				SelectionDigest string `json:"selection_digest"`
				NodeCount       int    `json:"node_count"`
			} `json:"data"`
		}
		json.Unmarshal(pBody, &prevResp)

		if prevResp.Data.NodeCount != 3 {
			t.Errorf("expected draft preview to resolve 3 nodes, got %d", prevResp.Data.NodeCount)
		}
		if prevResp.Data.SelectionDigest == "" {
			t.Error("expected non-empty selection digest in preview")
		}

		// Assert Database was NOT modified (no DB write)
		postGroup, err := policyRepo.GetGroupByID(ctx, testGID)
		if err != nil {
			t.Fatalf("failed to re-read group: %v", err)
		}
		if postGroup.Revision != initRev {
			t.Errorf("expected DB revision to remain %d, got %d (preview must NOT persist)", initRev, postGroup.Revision)
		}
		if len(postGroup.Sources) != 0 {
			t.Errorf("expected DB sources to remain 0, got %d", len(postGroup.Sources))
		}

		// When group IS saved, calling preview with exact saved state matches publication selection_digest
		_, _ = doReq("PUT", "/api/v1/policies/groups/"+testGID, map[string]any{
			"name":              "Saved Group 70",
			"group_type":        "select",
			"expected_revision": initRev,
			"sources":           draftPayload["sources"],
		})

		savedPreviewResp, savedPreviewBody := doReq("POST", "/api/v1/policies/groups/preview", map[string]any{
			"group_id":          testGID,
			"expected_revision": initRev + 1,
			"sources":           draftPayload["sources"],
		})
		if savedPreviewResp.StatusCode != http.StatusOK {
			t.Fatalf("saved preview failed: %d %s", savedPreviewResp.StatusCode, string(savedPreviewBody))
		}

		var savedPrevResult struct {
			Data struct {
				SelectionDigest string `json:"selection_digest"`
			} `json:"data"`
		}
		json.Unmarshal(savedPreviewBody, &savedPrevResult)

		pubSnap, err := pubService.ResolvePolicySnapshot(ctx, activeRev.ID)
		if err != nil {
			t.Fatalf("resolve publication snapshot failed: %v", err)
		}

		if savedPrevResult.Data.SelectionDigest != pubSnap.SnapshotDigest {
			t.Errorf("saved preview selection digest mismatch with publication snapshot:\npreview: %s\npubSnap:  %s",
				savedPrevResult.Data.SelectionDigest, pubSnap.SnapshotDigest)
		}
	})

	// -------------------------------------------------------------
	// SCENARIO H: Mihomo parsed order + Pure 3 Targets Node Content
	// -------------------------------------------------------------
	t.Run("mihomo_order_and_pure3_targets_export", func(t *testing.T) {
		// Verify Mihomo compilation
		mihomoPrev, err := pubService.Preview(ctx, publication.PreviewQuery{
			Target: domain.TargetMihomo,
		})
		if err != nil {
			t.Fatalf("mihomo preview export failed: %v", err)
		}

		var parsedMihomo struct {
			ProxyGroups []struct {
				Name    string   `yaml:"name"`
				Type    string   `yaml:"type"`
				Proxies []string `yaml:"proxies"`
			} `yaml:"proxy-groups"`
			Proxies []map[string]any `yaml:"proxies"`
		}
		if err := yaml.Unmarshal([]byte(mihomoPrev.Content), &parsedMihomo); err != nil {
			t.Fatalf("unmarshal mihomo YAML failed: %v", err)
		}

		if len(parsedMihomo.Proxies) != totalNodes {
			t.Errorf("expected Mihomo export to contain %d proxies, got %d", totalNodes, len(parsedMihomo.Proxies))
		}

		// Verify Pure 3 Node Targets export node content without empty-group blockage
		pureTargets := []domain.CompilerTarget{
			domain.TargetSingBox,
			domain.TargetSurge,
			domain.TargetQuantumultX,
		}

		var pureDigest string
		for _, target := range pureTargets {
			t.Run(string(target)+"_pure_export", func(t *testing.T) {
				res, err := pubService.Preview(ctx, publication.PreviewQuery{
					Target: target,
				})
				if err != nil {
					t.Fatalf("pure target %s preview export failed: %v", target, err)
				}
				if len(res.Content) == 0 {
					t.Fatalf("pure target %s export content is empty", target)
				}
				if pureDigest == "" {
					pureDigest = res.SnapshotDigest
				}
				if res.SnapshotDigest != pureDigest {
					t.Errorf("pure-node selection digest mismatch: %s != %s", res.SnapshotDigest, pureDigest)
				}
			})
		}
	})

	// -------------------------------------------------------------
	// SCENARIO I: Fail-Closed on filtered_nodes_empty
	// -------------------------------------------------------------
	t.Run("fail_closed_on_filtered_nodes_empty", func(t *testing.T) {
		// Set a global filter that matches NOTHING
		emptyCond := domain.FilterCondition{
			Field: domain.FilterFieldDisplayName,
			Op:    domain.FilterOpContains,
			Value: "NON_EXISTENT_MAGIC_VALUE_99999",
		}
		err := nodeFilterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
			Spec: domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{emptyCond},
			},
			UpdatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("set global filter failed: %v", err)
		}

		_, err = pubService.Preview(ctx, publication.PreviewQuery{
			Target: domain.TargetMihomo,
		})
		if err == nil {
			t.Fatal("expected Preview to fail closed when 0 nodes match filter, but it succeeded")
		}

		var pErr *publication.PreflightError
		var domErr *domain.DomainError
		hasFilteredNodesEmpty := false
		if errors.As(err, &pErr) {
			for _, d := range pErr.Result.Diagnostics {
				if d.Code == "filtered_nodes_empty" {
					hasFilteredNodesEmpty = true
					break
				}
			}
		} else if errors.As(err, &domErr) {
			if domErr.Code == "filtered_nodes_empty" {
				hasFilteredNodesEmpty = true
			}
		}
		if !hasFilteredNodesEmpty {
			t.Fatalf("expected filtered_nodes_empty diagnostic in error, got: %v", err)
		}
	})
}
