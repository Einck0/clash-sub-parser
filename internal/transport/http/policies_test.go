package http_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func setupPolicyTestRouter(t *testing.T, db *sql.DB) (http.Handler, *policy.Service) {
	t.Helper()
	routerCfg := newTestRouterConfig(true)
	policyRepo := sqlite.NewPolicyRepository(db)
	revRepo := sqlite.NewRevisionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	filterRepo := sqlite.NewNodeFilterRepository(db)

	policySvc := policy.NewService(policyRepo, revRepo, nodeRepo, auditRepo, filterRepo)
	routerCfg.PolicyService = policySvc
	routerCfg.AuditRepository = auditRepo

	return transporthttp.NewRouter(routerCfg), policySvc
}

type groupListResponse struct {
	Data struct {
		Items    []policy.GroupView `json:"items"`
		Page     int                `json:"page"`
		PageSize int                `json:"page_size"`
		Total    int                `json:"total"`
	} `json:"data"`
}

type groupDetailResponse struct {
	Data policy.GroupView `json:"data"`
}

type ruleListResponse struct {
	Data struct {
		RevisionID     string                     `json:"revision_id"`
		AdmissionRules []policy.AdmissionRuleView `json:"admission_rules"`
		PolicyRules    []policy.PolicyRuleView    `json:"policy_rules"`
		Total          int                        `json:"total"`
	} `json:"data"`
}

func TestPolicyRoutesAuthAndCSRFGuardrails(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPolicyTestRouter(t, db)

	// 1. Unauthenticated request must return 401
	req := httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated GET /api/v1/policies/groups, got %d", rec.Code)
	}

	// 2. Publication token must be forbidden on admin API
	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups", nil)
	req.Header.Set("Authorization", "Bearer "+testPublicationToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Fatalf("expected 401 Unauthorized or 403 Forbidden for publication token on admin API, got %d", rec.Code)
	}

	// 3. State-changing request via cookie session without CSRF must return 403
	body := `{"name": "ProxyGroup", "group_type": "select"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policies/groups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: testValidSessionID})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cookie mutation without CSRF token, got %d", rec.Code)
	}

	// 4. Valid Bearer token authentication allows request
	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid admin bearer token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPolicyGroupsCRUDAndCycleRejection(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPolicyTestRouter(t, db)

	// Step 1: Create Group A
	reqA := httptest.NewRequest(http.MethodPost, "/api/v1/policies/groups", strings.NewReader(`{
		"name": "Group A",
		"group_type": "select"
	}`))
	reqA.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqA.Header.Set("Content-Type", "application/json")
	recA := httptest.NewRecorder()
	router.ServeHTTP(recA, reqA)

	if recA.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for Group A, got %d: %s", recA.Code, recA.Body.String())
	}

	var respA groupDetailResponse
	if err := json.Unmarshal(recA.Body.Bytes(), &respA); err != nil {
		t.Fatalf("failed to parse Group A response: %v", err)
	}
	grpAID := respA.Data.ID
	if grpAID == "" {
		t.Fatalf("expected valid ID for Group A")
	}

	// Step 2: Create Group B
	reqB := httptest.NewRequest(http.MethodPost, "/api/v1/policies/groups", strings.NewReader(`{
		"name": "Group B",
		"group_type": "urltest"
	}`))
	reqB.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqB.Header.Set("Content-Type", "application/json")
	recB := httptest.NewRecorder()
	router.ServeHTTP(recB, reqB)

	if recB.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for Group B, got %d: %s", recB.Code, recB.Body.String())
	}

	var respB groupDetailResponse
	if err := json.Unmarshal(recB.Body.Bytes(), &respB); err != nil {
		t.Fatalf("failed to parse Group B response: %v", err)
	}
	grpBID := respB.Data.ID

	// Step 3: Add Edge A -> B
	edgeBody := fmt.Sprintf(`{"edges": [{"child_group_id": "%s", "position": 0}]}`, grpBID)
	reqEdgeA := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/policies/groups/%s/edges", grpAID), strings.NewReader(edgeBody))
	reqEdgeA.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqEdgeA.Header.Set("Content-Type", "application/json")
	recEdgeA := httptest.NewRecorder()
	router.ServeHTTP(recEdgeA, reqEdgeA)

	if recEdgeA.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for setting edge A -> B, got %d: %s", recEdgeA.Code, recEdgeA.Body.String())
	}

	// Step 4: Attempt Edge B -> A (Creates Cycle: A -> B -> A). MUST FAIL with 422!
	edgeCycleBody := fmt.Sprintf(`{"edges": [{"child_group_id": "%s", "position": 0}]}`, grpAID)
	reqEdgeB := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/policies/groups/%s/edges", grpBID), strings.NewReader(edgeCycleBody))
	reqEdgeB.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqEdgeB.Header.Set("Content-Type", "application/json")
	recEdgeB := httptest.NewRecorder()
	router.ServeHTTP(recEdgeB, reqEdgeB)

	if recEdgeB.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for cyclic edge B -> A, got %d: %s", recEdgeB.Code, recEdgeB.Body.String())
	}

	var errResp testErrorResponse
	if err := json.Unmarshal(recEdgeB.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Code != "cycle_detected" {
		t.Errorf("expected error code 'cycle_detected', got: %s", errResp.Code)
	}
	if !strings.Contains(errResp.Message, grpAID) || !strings.Contains(errResp.Message, grpBID) {
		t.Errorf("expected error message to identify cycle group IDs, got: %s", errResp.Message)
	}

	// Step 5: Self-loop detection: setting A -> A must fail with self_loop_forbidden
	selfLoopBody := fmt.Sprintf(`{"edges": [{"child_group_id": "%s", "position": 0}]}`, grpAID)
	reqSelf := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/policies/groups/%s/edges", grpAID), strings.NewReader(selfLoopBody))
	reqSelf.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqSelf.Header.Set("Content-Type", "application/json")
	recSelf := httptest.NewRecorder()
	router.ServeHTTP(recSelf, reqSelf)

	if recSelf.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for self loop, got %d: %s", recSelf.Code, recSelf.Body.String())
	}

	var selfErr testErrorResponse
	if err := json.Unmarshal(recSelf.Body.Bytes(), &selfErr); err != nil {
		t.Fatalf("failed to parse self loop error response: %v", err)
	}
	if selfErr.Code != "self_loop_forbidden" {
		t.Errorf("expected error code 'self_loop_forbidden', got: %s", selfErr.Code)
	}

	// Step 6: List Groups
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups", nil)
	reqList.Header.Set("Authorization", "Bearer "+testAdminToken)
	recList := httptest.NewRecorder()
	router.ServeHTTP(recList, reqList)

	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for listing groups, got %d: %s", recList.Code, recList.Body.String())
	}

	var listResp groupListResponse
	if err := json.Unmarshal(recList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to parse group list response: %v", err)
	}
	if listResp.Data.Total != 2 {
		t.Errorf("expected 2 groups in total, got %d", listResp.Data.Total)
	}

	// Step 7: Get Group A details
	reqGet := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/policies/groups/%s", grpAID), nil)
	reqGet.Header.Set("Authorization", "Bearer "+testAdminToken)
	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for getting group details, got %d: %s", recGet.Code, recGet.Body.String())
	}
	var getResp groupDetailResponse
	if err := json.Unmarshal(recGet.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to parse get group response: %v", err)
	}
	if len(getResp.Data.Edges) != 1 || *getResp.Data.Edges[0].ChildGroupID != grpBID {
		t.Errorf("expected Group A to have 1 edge to Group B, got %#v", getResp.Data.Edges)
	}
}

func TestPolicyRulesEndpointsAndMATCHOrdering(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, policySvc := setupPolicyTestRouter(t, db)
	ctx := context.Background()

	// Create Group
	grp, err := policySvc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Proxy",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-grp",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create test group: %v", err)
	}

	// Create an active ConfigurationRevision
	rev, err := policySvc.CreateRevision(ctx, policy.CreateRevisionCommand{
		State:     domain.RevisionStateActive,
		RequestID: "req-rev",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create test revision: %v", err)
	}

	// 1. Create a non-MATCH policy rule at position 0
	rule0Body := fmt.Sprintf(`{
		"kind": "policy",
		"revision_id": "%s",
		"target_group_id": "%s",
		"expression": "DOMAIN-SUFFIX,github.com",
		"position": 0
	}`, rev.ID, grp.ID)

	req0 := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(rule0Body))
	req0.Header.Set("Authorization", "Bearer "+testAdminToken)
	req0.Header.Set("Content-Type", "application/json")
	rec0 := httptest.NewRecorder()
	router.ServeHTTP(rec0, req0)

	if rec0.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for policy rule 0, got %d: %s", rec0.Code, rec0.Body.String())
	}

	// 2. Create terminal MATCH rule at position 1
	ruleMatchBody := fmt.Sprintf(`{
		"kind": "policy",
		"revision_id": "%s",
		"target_group_id": "%s",
		"expression": "MATCH",
		"position": 1
	}`, rev.ID, grp.ID)

	reqMatch := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(ruleMatchBody))
	reqMatch.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqMatch.Header.Set("Content-Type", "application/json")
	recMatch := httptest.NewRecorder()
	router.ServeHTTP(recMatch, reqMatch)

	if recMatch.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for terminal MATCH rule, got %d: %s", recMatch.Code, recMatch.Body.String())
	}

	// 3. Attempt to add a rule at position 2 (after terminal MATCH rule) -> MUST BE REJECTED with 422!
	ruleInvalidBody := fmt.Sprintf(`{
		"kind": "policy",
		"revision_id": "%s",
		"target_group_id": "%s",
		"expression": "IP-CIDR,8.8.8.8/32",
		"position": 2
	}`, rev.ID, grp.ID)

	reqInvalid := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(ruleInvalidBody))
	reqInvalid.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqInvalid.Header.Set("Content-Type", "application/json")
	recInvalid := httptest.NewRecorder()
	router.ServeHTTP(recInvalid, reqInvalid)

	if recInvalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for rule after MATCH, got %d: %s", recInvalid.Code, recInvalid.Body.String())
	}

	var errResp testErrorResponse
	if err := json.Unmarshal(recInvalid.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Code != "invalid_match_rule_ordering" {
		t.Errorf("expected error code 'invalid_match_rule_ordering', got %s", errResp.Code)
	}

	// 4. Create an admission rule
	admBody := fmt.Sprintf(`{
		"kind": "admission",
		"revision_id": "%s",
		"name": "filter-us",
		"expression": "country == 'US'",
		"action": "allow",
		"position": 0
	}`, rev.ID)

	reqAdm := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(admBody))
	reqAdm.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqAdm.Header.Set("Content-Type", "application/json")
	recAdm := httptest.NewRecorder()
	router.ServeHTTP(recAdm, reqAdm)

	if recAdm.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for admission rule, got %d: %s", recAdm.Code, recAdm.Body.String())
	}

	// 5. Query rules via GET /api/v1/policies/rules
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/policies/rules?revision_id="+rev.ID, nil)
	reqList.Header.Set("Authorization", "Bearer "+testAdminToken)
	recList := httptest.NewRecorder()
	router.ServeHTTP(recList, reqList)

	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK listing rules, got %d: %s", recList.Code, recList.Body.String())
	}

	var rulesResp ruleListResponse
	if err := json.Unmarshal(recList.Body.Bytes(), &rulesResp); err != nil {
		t.Fatalf("failed to parse rules response: %v", err)
	}
	if len(rulesResp.Data.PolicyRules) != 2 {
		t.Errorf("expected 2 policy rules, got %d", len(rulesResp.Data.PolicyRules))
	}
	if len(rulesResp.Data.AdmissionRules) != 1 {
		t.Errorf("expected 1 admission rule, got %d", len(rulesResp.Data.AdmissionRules))
	}
}

type writeHeaderCounterRecorder struct {
	*httptest.ResponseRecorder
	writeHeaderCount int
}

func (w *writeHeaderCounterRecorder) WriteHeader(code int) {
	w.writeHeaderCount++
	w.ResponseRecorder.WriteHeader(code)
}

func assertSingleJSONBodyAndNoSuperfluousWriteHeader(t *testing.T, rec *writeHeaderCounterRecorder) {
	t.Helper()
	if rec.writeHeaderCount > 1 {
		t.Fatalf("expected WriteHeader to be called at most once, got %d", rec.writeHeaderCount)
	}
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	var first any
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("response body is not valid JSON (%v): %s", err, rec.Body.String())
	}
	if dec.More() {
		t.Fatalf("response body contains multiple concatenated JSON values: %s", rec.Body.String())
	}
}

func TestPolicyValidateEndpoint(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, policySvc := setupPolicyTestRouter(t, db)
	ctx := context.Background()

	// 1. No-body request validates current persisted DB graph state
	t.Run("no_body_validates_persisted_db_graph", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/validate", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := &writeHeaderCounterRecorder{ResponseRecorder: httptest.NewRecorder()}
		router.ServeHTTP(rec, req)

		assertSingleJSONBodyAndNoSuperfluousWriteHeader(t, rec)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for valid policy validation, got %d: %s", rec.Code, rec.Body.String())
		}

		var okResp testDataResponse[policy.ValidationResult]
		if err := json.Unmarshal(rec.Body.Bytes(), &okResp); err != nil {
			t.Fatalf("failed to parse validation response: %v", err)
		}
		if !okResp.Data.Valid {
			t.Errorf("expected validation to be valid: %#v", okResp.Data)
		}
	})

	// 2. Persisted groups, edges, and rules in DB validate cleanly without client payload
	t.Run("persisted_groups_edges_and_rules_validate", func(t *testing.T) {
		// Provide an active node belonging to an enabled subscription so groups resolve with members
		subID := domain.MustNewUUIDv7()
		fetchID := domain.MustNewUUIDv7()
		nodeID := "node_0123456789abcdef0123456789abcdef"
		if _, err := db.Exec(`INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at) VALUES (?, 'Sub 1', 'secret-ref', 1, 'rev-1', datetime('now'), datetime('now'))`, subID); err != nil {
			t.Fatalf("failed to insert subscription: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO subscription_fetches (id, subscription_id, started_at, finished_at, outcome) VALUES (?, ?, datetime('now'), datetime('now'), 'success')`, fetchID, subID); err != nil {
			t.Fatalf("failed to insert subscription_fetch: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at) VALUES (?, 'ss', 'Node 1', '1.1.1.1', 8388, '{}', 1, datetime('now'), datetime('now'))`, nodeID); err != nil {
			t.Fatalf("failed to insert node: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES (?, ?, ?)`, nodeID, subID, fetchID); err != nil {
			t.Fatalf("failed to insert node_source: %v", err)
		}

		g2, err := policySvc.CreateGroup(ctx, policy.CreateGroupCommand{
			Name:      "Auto",
			GroupType: domain.GroupTypeURLTest,
			Edges: []policy.EdgeInput{
				{NodeLogicalID: &nodeID, Position: 0},
			},
			RequestID: "req-g2",
			ActorKind: domain.ActorKindAdmin,
		})
		if err != nil {
			t.Fatalf("failed to create Auto group: %v", err)
		}
		g1, err := policySvc.CreateGroup(ctx, policy.CreateGroupCommand{
			Name:      "Proxy",
			GroupType: domain.GroupTypeSelect,
			Edges: []policy.EdgeInput{
				{ChildGroupID: &g2.ID, Position: 0},
			},
			RequestID: "req-g1",
			ActorKind: domain.ActorKindAdmin,
		})
		if err != nil {
			t.Fatalf("failed to create Proxy group: %v", err)
		}
		rev, err := policySvc.CreateRevision(ctx, policy.CreateRevisionCommand{
			State:     domain.RevisionStateActive,
			RequestID: "req-rev",
			ActorKind: domain.ActorKindAdmin,
		})
		if err != nil {
			t.Fatalf("failed to create active revision: %v", err)
		}
		if _, err := policySvc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
			RevisionID:    rev.ID,
			TargetGroupID: g1.ID,
			Expression:    "MATCH",
			Position:      0,
			RequestID:     "req-rule",
			ActorKind:     domain.ActorKindAdmin,
		}); err != nil {
			t.Fatalf("failed to create MATCH rule: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/validate", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := &writeHeaderCounterRecorder{ResponseRecorder: httptest.NewRecorder()}
		router.ServeHTTP(rec, req)

		assertSingleJSONBodyAndNoSuperfluousWriteHeader(t, rec)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for persisted graph validation, got %d: %s", rec.Code, rec.Body.String())
		}
		var okResp testDataResponse[policy.ValidationResult]
		if err := json.Unmarshal(rec.Body.Bytes(), &okResp); err != nil || !okResp.Data.Valid {
			t.Fatalf("expected valid=true response, got err=%v resp=%+v", err, okResp)
		}

		// Inject a cycle directly in DB to verify backend graph validation catches persisted cycles
		cycleEdgeID := domain.MustNewUUIDv7()
		if _, err := db.Exec(`INSERT INTO group_edges (id, parent_group_id, child_group_id, position) VALUES (?, ?, ?, 0)`, cycleEdgeID, g2.ID, g1.ID); err != nil {
			t.Fatalf("failed to insert cycle edge: %v", err)
		}
		defer func() {
			_, _ = db.Exec(`DELETE FROM group_edges WHERE id = ?`, cycleEdgeID)
		}()

		cycleReq := httptest.NewRequest(http.MethodPost, "/api/v1/policies/validate", nil)
		cycleReq.Header.Set("Authorization", "Bearer "+testAdminToken)
		cycleRec := &writeHeaderCounterRecorder{ResponseRecorder: httptest.NewRecorder()}
		router.ServeHTTP(cycleRec, cycleReq)

		assertSingleJSONBodyAndNoSuperfluousWriteHeader(t, cycleRec)
		if cycleRec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for cycle in persisted DB edges, got %d: %s", cycleRec.Code, cycleRec.Body.String())
		}
		var cycleErr testErrorResponse
		if err := json.Unmarshal(cycleRec.Body.Bytes(), &cycleErr); err != nil || cycleErr.Code != "cycle_detected" {
			t.Fatalf("expected cycle_detected error, got err=%v resp=%+v", err, cycleErr)
		}
	})
}

func TestPolicyGlobalNodeFilterEndpoint_AuthAndCRUD(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPolicyTestRouter(t, db)

	// 1. Unauthorized GET -> 401
	req := httptest.NewRequest(http.MethodGet, "/api/v1/policies/global-node-filter", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated GET, got %d", rec.Code)
	}

	// 2. Authorized GET -> 200 (returns default empty filter)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/global-node-filter", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET global-node-filter, got %d: %s", rec.Code, rec.Body.String())
	}
	var getResp testDataResponse[domain.GlobalNodeFilter]
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to unmarshal global filter response: %v", err)
	}
	if len(getResp.Data.Spec.Conditions) != 0 {
		t.Fatalf("expected 0 conditions initially, got %d", len(getResp.Data.Spec.Conditions))
	}

	// 3. Authorized PUT with valid spec -> 200
	validPayload := `{"spec":{"conditions":[{"field":"protocol","op":"equals","value":"ss"}]}}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/policies/global-node-filter", strings.NewReader(validPayload))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for PUT global-node-filter, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Verify updated filter via GET
	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/global-node-filter", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var updatedResp testDataResponse[domain.GlobalNodeFilter]
	if err := json.Unmarshal(rec.Body.Bytes(), &updatedResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(updatedResp.Data.Spec.Conditions) != 1 || updatedResp.Data.Spec.Conditions[0].Value != "ss" {
		t.Fatalf("unexpected updated filter: %+v", updatedResp.Data)
	}

	// 5. Authorized PUT with invalid spec (negative latency) -> 422 Unprocessable Entity
	invalidPayload := `{"spec":{"conditions":[{"field":"probe_latency_ms","op":"lte","value":"-50","probe_kind":"baseline"}]}}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/policies/global-node-filter", strings.NewReader(invalidPayload))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for invalid spec, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPolicyGroups_NodeFilter_CreateUpdatePatchClear(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPolicyTestRouter(t, db)

	// 1. Create group with node_filter
	createPayload := `{
		"name": "US-Nodes",
		"group_type": "select",
		"node_filter": {
			"conditions": [
				{"field": "display_name", "op": "contains", "value": "US"}
			]
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/groups", strings.NewReader(createPayload))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp testDataResponse[policy.GroupView]
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to parse create response: %v", err)
	}
	groupID := createResp.Data.ID
	if createResp.Data.NodeFilter == nil || len(createResp.Data.NodeFilter.Conditions) != 1 {
		t.Fatalf("expected created group to have filter, got %+v", createResp.Data.NodeFilter)
	}

	// 2. PATCH omitting node_filter must NOT clear filter
	patchOmitPayload := `{"name": "US-Nodes-Renamed"}`
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/policies/groups/"+groupID, strings.NewReader(patchOmitPayload))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for PATCH omit, got %d: %s", rec.Code, rec.Body.String())
	}
	var patchOmitResp testDataResponse[policy.GroupView]
	if err := json.Unmarshal(rec.Body.Bytes(), &patchOmitResp); err != nil {
		t.Fatalf("failed to parse patch response: %v", err)
	}
	if patchOmitResp.Data.NodeFilter == nil || len(patchOmitResp.Data.NodeFilter.Conditions) != 1 {
		t.Fatalf("omitting node_filter in PATCH must retain existing filter, got %+v", patchOmitResp.Data.NodeFilter)
	}

	// 3. PATCH with null node_filter must CLEAR filter
	patchClearPayload := `{"node_filter": null}`
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/policies/groups/"+groupID, strings.NewReader(patchClearPayload))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for PATCH clear, got %d: %s", rec.Code, rec.Body.String())
	}
	var patchClearResp testDataResponse[policy.GroupView]
	if err := json.Unmarshal(rec.Body.Bytes(), &patchClearResp); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if patchClearResp.Data.NodeFilter != nil {
		t.Fatalf("PATCH with null node_filter must clear filter, got %+v", patchClearResp.Data.NodeFilter)
	}

	// 4. Verify GET returns cleared filter
	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups/"+groupID, nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var getResp testDataResponse[policy.GroupView]
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if getResp.Data.NodeFilter != nil {
		t.Fatalf("persisted filter must be cleared, got %+v", getResp.Data.NodeFilter)
	}
}

func TestPolicyRulesDelete(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPolicyTestRouter(t, db)

	// 1. Create a policy group on a fresh DB (no revision created yet)
	grpPayload := `{"name": "ProxyGroup", "group_type": "select"}`
	reqGrp := httptest.NewRequest(http.MethodPost, "/api/v1/policies/groups", strings.NewReader(grpPayload))
	reqGrp.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqGrp.Header.Set("Content-Type", "application/json")
	recGrp := httptest.NewRecorder()
	router.ServeHTTP(recGrp, reqGrp)
	if recGrp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for group, got %d: %s", recGrp.Code, recGrp.Body.String())
	}
	var grpResp testDataResponse[policy.GroupView]
	if err := json.Unmarshal(recGrp.Body.Bytes(), &grpResp); err != nil {
		t.Fatalf("failed to parse group response: %v", err)
	}

	// 2. Create a policy rule WITHOUT revision_id on fresh DB -> must succeed with 201 Created (not 422)
	polRulePayload := fmt.Sprintf(`{
		"kind": "policy",
		"target_group_id": "%s",
		"expression": "DOMAIN-SUFFIX,google.com",
		"position": 0
	}`, grpResp.Data.ID)
	reqPol := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(polRulePayload))
	reqPol.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqPol.Header.Set("Content-Type", "application/json")
	recPol := httptest.NewRecorder()
	router.ServeHTTP(recPol, reqPol)
	if recPol.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for policy rule without revision_id, got %d: %s", recPol.Code, recPol.Body.String())
	}
	var polRuleResp testDataResponse[policy.PolicyRuleView]
	if err := json.Unmarshal(recPol.Body.Bytes(), &polRuleResp); err != nil {
		t.Fatalf("failed to parse policy rule response: %v", err)
	}
	if polRuleResp.Data.ID == "" || polRuleResp.Data.RevisionID == "" {
		t.Fatalf("expected valid rule ID and bootstrapped revision_id, got %+v", polRuleResp.Data)
	}

	// 3. Create an admission rule WITHOUT revision_id -> must succeed with 201 Created
	admRulePayload := `{
		"kind": "admission",
		"name": "BlockHighRisk",
		"expression": "country != 'CN'",
		"action": "allow",
		"position": 0
	}`
	reqAdm := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(admRulePayload))
	reqAdm.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqAdm.Header.Set("Content-Type", "application/json")
	recAdm := httptest.NewRecorder()
	router.ServeHTTP(recAdm, reqAdm)
	if recAdm.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for admission rule without revision_id, got %d: %s", recAdm.Code, recAdm.Body.String())
	}
	var admRuleResp testDataResponse[policy.AdmissionRuleView]
	if err := json.Unmarshal(recAdm.Body.Bytes(), &admRuleResp); err != nil {
		t.Fatalf("failed to parse admission rule response: %v", err)
	}

	// 4. Delete the policy rule -> 204 No Content
	reqDelPol := httptest.NewRequest(http.MethodDelete, "/api/v1/policies/rules/"+polRuleResp.Data.ID, nil)
	reqDelPol.Header.Set("Authorization", "Bearer "+testAdminToken)
	recDelPol := httptest.NewRecorder()
	router.ServeHTTP(recDelPol, reqDelPol)
	if recDelPol.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content deleting policy rule, got %d: %s", recDelPol.Code, recDelPol.Body.String())
	}

	// 5. Verify GET /api/v1/policies/rules no longer includes the deleted policy rule
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/policies/rules", nil)
	reqList.Header.Set("Authorization", "Bearer "+testAdminToken)
	recList := httptest.NewRecorder()
	router.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK listing rules, got %d", recList.Code)
	}
	var listResp testDataResponse[policy.ListRulesResult]
	if err := json.Unmarshal(recList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to parse list rules response: %v", err)
	}
	if len(listResp.Data.PolicyRules) != 0 {
		t.Fatalf("expected 0 policy rules after delete, got %d", len(listResp.Data.PolicyRules))
	}
	if len(listResp.Data.AdmissionRules) != 1 {
		t.Fatalf("expected 1 admission rule remaining, got %d", len(listResp.Data.AdmissionRules))
	}

	// 6. Delete the admission rule -> 204 No Content
	reqDelAdm := httptest.NewRequest(http.MethodDelete, "/api/v1/policies/rules/"+admRuleResp.Data.ID, nil)
	reqDelAdm.Header.Set("Authorization", "Bearer "+testAdminToken)
	recDelAdm := httptest.NewRecorder()
	router.ServeHTTP(recDelAdm, reqDelAdm)
	if recDelAdm.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content deleting admission rule, got %d: %s", recDelAdm.Code, recDelAdm.Body.String())
	}

	// Verify admission rule is also gone
	reqList2 := httptest.NewRequest(http.MethodGet, "/api/v1/policies/rules", nil)
	reqList2.Header.Set("Authorization", "Bearer "+testAdminToken)
	recList2 := httptest.NewRecorder()
	router.ServeHTTP(recList2, reqList2)
	if err := json.Unmarshal(recList2.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to parse list rules response: %v", err)
	}
	if len(listResp.Data.AdmissionRules) != 0 {
		t.Fatalf("expected 0 admission rules after delete, got %d", len(listResp.Data.AdmissionRules))
	}

	// 7. Deleting a non-existent rule -> 404 Not Found with code rule_not_found
	reqDelMissing := httptest.NewRequest(http.MethodDelete, "/api/v1/policies/rules/"+polRuleResp.Data.ID, nil)
	reqDelMissing.Header.Set("Authorization", "Bearer "+testAdminToken)
	recDelMissing := httptest.NewRecorder()
	router.ServeHTTP(recDelMissing, reqDelMissing)
	if recDelMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for non-existent rule delete, got %d: %s", recDelMissing.Code, recDelMissing.Body.String())
	}
	var errResp testErrorResponse
	if err := json.Unmarshal(recDelMissing.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Code != "rule_not_found" {
		t.Fatalf("expected error code 'rule_not_found', got %q", errResp.Code)
	}
}
