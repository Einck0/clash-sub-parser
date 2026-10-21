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
	"clash-sub-parser/internal/resolver"
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

	// 4. Create an admission rule -> rejected with 422 Unprocessable Entity (admission_rules_retired)
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

	if recAdm.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 UnprocessableEntity for admission rule, got %d: %s", recAdm.Code, recAdm.Body.String())
	}
	var admErrResp testErrorResponse
	if err := json.Unmarshal(recAdm.Body.Bytes(), &admErrResp); err != nil {
		t.Fatalf("failed to parse admission error response: %v", err)
	}
	if admErrResp.Code != "admission_rules_retired" {
		t.Errorf("expected error code 'admission_rules_retired', got %s", admErrResp.Code)
	}

	// Legacy admission rules can still be read if present in database
	_, err = db.Exec(`INSERT INTO admission_rules (id, revision_id, name, expression, action, position) VALUES ('legacy-adm-1', ?, 'filter-us', 'country == "US"', 'allow', 0)`, rev.ID)
	if err != nil {
		t.Fatalf("failed to insert legacy admission rule: %v", err)
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
		if _, err := db.Exec(`INSERT INTO policy_group_sources (id, group_id, target_group_id, position,operation,kind) VALUES (?, ?, ?, 0,'add','group_ref')`, cycleEdgeID, g2.ID, g1.ID); err != nil {
			t.Fatalf("failed to insert cycle edge: %v", err)
		}
		defer func() {
			_, _ = db.Exec(`DELETE FROM policy_group_sources WHERE id = ?`, cycleEdgeID)
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

	// 3. Create an admission rule WITHOUT revision_id -> must fail with 422 Unprocessable Entity
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
	if recAdm.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 UnprocessableEntity for admission rule, got %d: %s", recAdm.Code, recAdm.Body.String())
	}
	var admErrResp testErrorResponse
	if err := json.Unmarshal(recAdm.Body.Bytes(), &admErrResp); err != nil {
		t.Fatalf("failed to parse admission rule error response: %v", err)
	}
	if admErrResp.Code != "admission_rules_retired" {
		t.Fatalf("expected error code 'admission_rules_retired', got %s", admErrResp.Code)
	}

	// Insert legacy admission rule directly into SQLite DB
	legacyAdmID := "legacy-adm-bootstrap-1"
	_, err = db.Exec(`INSERT INTO admission_rules (id, revision_id, name, expression, action, position) VALUES (?, ?, 'BlockHighRisk', 'country != "CN"', 'allow', 0)`, legacyAdmID, polRuleResp.Data.RevisionID)
	if err != nil {
		t.Fatalf("failed to insert legacy admission rule: %v", err)
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

	// 6. Delete admission rule via DELETE /api/v1/policies/rules/{id}?kind=admission -> 422 Unprocessable Entity
	reqDelAdmKind := httptest.NewRequest(http.MethodDelete, "/api/v1/policies/rules/"+legacyAdmID+"?kind=admission", nil)
	reqDelAdmKind.Header.Set("Authorization", "Bearer "+testAdminToken)
	recDelAdmKind := httptest.NewRecorder()
	router.ServeHTTP(recDelAdmKind, reqDelAdmKind)
	if recDelAdmKind.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 UnprocessableEntity deleting admission rule, got %d: %s", recDelAdmKind.Code, recDelAdmKind.Body.String())
	}

	// Generic delete on admission rule ID -> 404 Not Found (no fallback from policy to admission)
	reqDelAdmGeneric := httptest.NewRequest(http.MethodDelete, "/api/v1/policies/rules/"+legacyAdmID, nil)
	reqDelAdmGeneric.Header.Set("Authorization", "Bearer "+testAdminToken)
	recDelAdmGeneric := httptest.NewRecorder()
	router.ServeHTTP(recDelAdmGeneric, reqDelAdmGeneric)
	if recDelAdmGeneric.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for generic delete on admission rule ID, got %d: %s", recDelAdmGeneric.Code, recDelAdmGeneric.Body.String())
	}

	// Verify admission rule is still present in database
	reqList2 := httptest.NewRequest(http.MethodGet, "/api/v1/policies/rules", nil)
	reqList2.Header.Set("Authorization", "Bearer "+testAdminToken)
	recList2 := httptest.NewRecorder()
	router.ServeHTTP(recList2, reqList2)
	if err := json.Unmarshal(recList2.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to parse list rules response: %v", err)
	}
	if len(listResp.Data.AdmissionRules) != 1 {
		t.Fatalf("expected 1 legacy admission rule preserved, got %d", len(listResp.Data.AdmissionRules))
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

func TestPolicyGroupsSearchAndPagination(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPolicyTestRouter(t, db)

	groups := []struct {
		name      string
		groupType string
		position  int
	}{
		{"HK-Auto", "urltest", 1},
		{"JP-Fallback", "fallback", 2},
		{"SG-Auto", "urltest", 3},
		{"US-Direct", "select", 4},
		{"US-Proxy", "select", 5},
	}

	for _, g := range groups {
		payload := fmt.Sprintf(`{"name": "%s", "group_type": "%s", "position": %d}`, g.name, g.groupType, g.position)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/groups", strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("failed to create group %s, got %d: %s", g.name, rec.Code, rec.Body.String())
		}
	}

	// 1. Pagination without search (page 1, pageSize 2)
	reqP1 := httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups?page=1&page_size=2", nil)
	reqP1.Header.Set("Authorization", "Bearer "+testAdminToken)
	recP1 := httptest.NewRecorder()
	router.ServeHTTP(recP1, reqP1)
	if recP1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", recP1.Code, recP1.Body.String())
	}
	var respP1 groupListResponse
	if err := json.Unmarshal(recP1.Body.Bytes(), &respP1); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if respP1.Data.Total != 5 || respP1.Data.Page != 1 || respP1.Data.PageSize != 2 || len(respP1.Data.Items) != 2 {
		t.Fatalf("expected total=5, page=1, page_size=2, items=2, got %+v", respP1.Data)
	}
	if respP1.Data.Items[0].Name != "HK-Auto" || respP1.Data.Items[1].Name != "JP-Fallback" {
		t.Fatalf("expected HK-Auto, JP-Fallback; got %s, %s", respP1.Data.Items[0].Name, respP1.Data.Items[1].Name)
	}

	// Page 2
	reqP2 := httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups?page=2&page_size=2", nil)
	reqP2.Header.Set("Authorization", "Bearer "+testAdminToken)
	recP2 := httptest.NewRecorder()
	router.ServeHTTP(recP2, reqP2)
	var respP2 groupListResponse
	if err := json.Unmarshal(recP2.Body.Bytes(), &respP2); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if respP2.Data.Total != 5 || len(respP2.Data.Items) != 2 || respP2.Data.Items[0].Name != "SG-Auto" || respP2.Data.Items[1].Name != "US-Direct" {
		t.Fatalf("expected SG-Auto, US-Direct; got %+v", respP2.Data)
	}

	// Page 3
	reqP3 := httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups?page=3&page_size=2", nil)
	reqP3.Header.Set("Authorization", "Bearer "+testAdminToken)
	recP3 := httptest.NewRecorder()
	router.ServeHTTP(recP3, reqP3)
	var respP3 groupListResponse
	if err := json.Unmarshal(recP3.Body.Bytes(), &respP3); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if respP3.Data.Total != 5 || len(respP3.Data.Items) != 1 || respP3.Data.Items[0].Name != "US-Proxy" {
		t.Fatalf("expected US-Proxy on p3; got %+v", respP3.Data)
	}

	// 2. Search by name (case-insensitive)
	reqSearchAuto := httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups?search=auto", nil)
	reqSearchAuto.Header.Set("Authorization", "Bearer "+testAdminToken)
	recSearchAuto := httptest.NewRecorder()
	router.ServeHTTP(recSearchAuto, reqSearchAuto)
	var respSearchAuto groupListResponse
	if err := json.Unmarshal(recSearchAuto.Body.Bytes(), &respSearchAuto); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if respSearchAuto.Data.Total != 2 || len(respSearchAuto.Data.Items) != 2 {
		t.Fatalf("expected 2 auto groups, got %+v", respSearchAuto.Data)
	}
	if respSearchAuto.Data.Items[0].Name != "HK-Auto" || respSearchAuto.Data.Items[1].Name != "SG-Auto" {
		t.Fatalf("expected HK-Auto and SG-Auto, got %v", respSearchAuto.Data.Items)
	}

	// 3. Search by group_type
	reqSearchType := httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups?search=SELECT", nil)
	reqSearchType.Header.Set("Authorization", "Bearer "+testAdminToken)
	recSearchType := httptest.NewRecorder()
	router.ServeHTTP(recSearchType, reqSearchType)
	var respSearchType groupListResponse
	if err := json.Unmarshal(recSearchType.Body.Bytes(), &respSearchType); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if respSearchType.Data.Total != 2 || len(respSearchType.Data.Items) != 2 {
		t.Fatalf("expected 2 select groups, got %+v", respSearchType.Data)
	}
	if respSearchType.Data.Items[0].Name != "US-Direct" || respSearchType.Data.Items[1].Name != "US-Proxy" {
		t.Fatalf("expected US-Direct and US-Proxy, got %v", respSearchType.Data.Items)
	}

	// 4. Combined search + pagination with accurate total
	reqSearchUSP1 := httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups?search=us&page=1&page_size=1", nil)
	reqSearchUSP1.Header.Set("Authorization", "Bearer "+testAdminToken)
	recSearchUSP1 := httptest.NewRecorder()
	router.ServeHTTP(recSearchUSP1, reqSearchUSP1)
	var respSearchUSP1 groupListResponse
	if err := json.Unmarshal(recSearchUSP1.Body.Bytes(), &respSearchUSP1); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if respSearchUSP1.Data.Total != 2 || len(respSearchUSP1.Data.Items) != 1 || respSearchUSP1.Data.Items[0].Name != "US-Direct" {
		t.Fatalf("expected US-Direct on p1, total=2; got %+v", respSearchUSP1.Data)
	}

	reqSearchUSP2 := httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups?search=us&page=2&page_size=1", nil)
	reqSearchUSP2.Header.Set("Authorization", "Bearer "+testAdminToken)
	recSearchUSP2 := httptest.NewRecorder()
	router.ServeHTTP(recSearchUSP2, reqSearchUSP2)
	var respSearchUSP2 groupListResponse
	if err := json.Unmarshal(recSearchUSP2.Body.Bytes(), &respSearchUSP2); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if respSearchUSP2.Data.Total != 2 || len(respSearchUSP2.Data.Items) != 1 || respSearchUSP2.Data.Items[0].Name != "US-Proxy" {
		t.Fatalf("expected US-Proxy on p2, total=2; got %+v", respSearchUSP2.Data)
	}

	// 5. Search with no matches
	reqSearchNone := httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups?search=nonexistent", nil)
	reqSearchNone.Header.Set("Authorization", "Bearer "+testAdminToken)
	recSearchNone := httptest.NewRecorder()
	router.ServeHTTP(recSearchNone, reqSearchNone)
	var respSearchNone groupListResponse
	if err := json.Unmarshal(recSearchNone.Body.Bytes(), &respSearchNone); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if respSearchNone.Data.Total != 0 || len(respSearchNone.Data.Items) != 0 {
		t.Fatalf("expected total=0, items=0 for search=nonexistent, got %+v", respSearchNone.Data)
	}
}

func TestPolicyHandler_PreviewGroup_Fields(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, policySvc := setupPolicyTestRouter(t, db)

	// Inject inputProvider so preview produces deterministic candidate counts and members
	n1 := domain.Node{LogicalID: "node-hk-01", DisplayName: "HK 01", Protocol: domain.ProtocolHysteria2, Active: true}
	n2 := domain.Node{LogicalID: "node-us-02", DisplayName: "US 02", Protocol: domain.ProtocolHysteria2, Active: true}
	patHK := "HK.*"

	policySvc.SetInputProvider(func(ctx context.Context, revID string) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{
			RevisionID:         "rev-preview-test",
			InventoryWatermark: "wm-1",
			CompilerVersion:    "1.0.0",
			Nodes:              []domain.Node{n1, n2},
			DNS: resolver.DNSConfig{
				Enabled:     true,
				Nameservers: []string{"1.1.1.1"},
			},
		}, nil
	})

	previewPayload := map[string]interface{}{
		"name": "HK Group Preview",
		"sources": []map[string]interface{}{
			{
				"operation": "add",
				"kind":      "regex",
				"position":  0,
				"pattern":   patHK,
			},
		},
	}
	body, _ := json.Marshal(previewPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/groups/preview", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/policies/groups/preview returned status %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			GroupID          string                      `json:"group_id"`
			SelectionDigest  string                      `json:"selection_digest"`
			MemberCount      int                         `json:"member_count"`
			NodeCount        int                         `json:"node_count"`
			CandidateCount   int                         `json:"candidate_count"`
			ExcludedCount    int                         `json:"excluded_count"`
			ExclusionSummary map[string]int              `json:"exclusion_summary"`
			FilterCounts     *resolver.GroupFilterCount  `json:"filter_counts"`
			Members          []policy.GroupPreviewMember `json:"members"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal preview response: %v\nBody: %s", err, rec.Body.String())
	}

	if resp.Data.MemberCount != 1 || len(resp.Data.Members) != 1 {
		t.Fatalf("expected member_count=1, got %d, members: %+v", resp.Data.MemberCount, resp.Data.Members)
	}
	if resp.Data.CandidateCount != 2 {
		t.Errorf("expected candidate_count=2, got %d", resp.Data.CandidateCount)
	}
	if resp.Data.ExcludedCount != 1 {
		t.Errorf("expected excluded_count=1, got %d", resp.Data.ExcludedCount)
	}
	if resp.Data.FilterCounts == nil {
		t.Fatalf("expected filter_counts to be non-nil")
	}
	if resp.Data.FilterCounts.Candidate != 2 || resp.Data.FilterCounts.Kept != 1 || resp.Data.FilterCounts.Excluded != 1 {
		t.Errorf("expected filter_counts {2, 1, 1}, got %+v", resp.Data.FilterCounts)
	}
}

func TestPolicyHandler_PreviewGroup_Pipeline_AddRemoveReentrantExclusion(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, policySvc := setupPolicyTestRouter(t, db)

	n1 := domain.Node{LogicalID: "0192e100-0000-7000-9100-000000000001", DisplayName: "Tokyo Synthetic Alpha", Protocol: domain.ProtocolVMess, Active: true}
	n2 := domain.Node{LogicalID: "0192e100-0000-7000-9100-000000000002", DisplayName: "London Synthetic Beta", Protocol: domain.ProtocolVLESS, Active: true}
	n3 := domain.Node{LogicalID: "0192e100-0000-7000-9100-000000000003", DisplayName: "Singapore Synthetic Gamma", Protocol: domain.ProtocolTrojan, Active: true}

	policySvc.SetInputProvider(func(ctx context.Context, revID string) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{
			RevisionID:         "rev-preview-pipeline",
			InventoryWatermark: "wm-1",
			CompilerVersion:    "1.0.0",
			Nodes:              []domain.Node{n1, n2, n3},
			DNS: resolver.DNSConfig{
				Enabled:     true,
				Nameservers: []string{"1.1.1.1"},
			},
		}, nil
	})

	patTokyoLondon := ".*(Tokyo|London).*"
	patLondon := ".*London.*"

	previewPayload := map[string]interface{}{
		"name": "Critic Pipeline Preview",
		"sources": []map[string]interface{}{
			// 1. Add Tokyo and London
			{
				"operation": "add",
				"kind":      "regex",
				"position":  0,
				"pattern":   patTokyoLondon,
			},
			// 2. Remove London
			{
				"operation": "remove",
				"kind":      "regex",
				"position":  1,
				"pattern":   patLondon,
			},
		},
	}
	body, _ := json.Marshal(previewPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/groups/preview", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/policies/groups/preview returned status %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			GroupID          string                      `json:"group_id"`
			MemberCount      int                         `json:"member_count"`
			NodeCount        int                         `json:"node_count"`
			CandidateCount   int                         `json:"candidate_count"`
			ExcludedCount    int                         `json:"excluded_count"`
			ExclusionSummary map[string]int              `json:"exclusion_summary"`
			Members          []policy.GroupPreviewMember `json:"members"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Active member should only be Tokyo (1 node)
	if resp.Data.MemberCount != 1 || resp.Data.NodeCount != 1 {
		t.Errorf("expected MemberCount=1, NodeCount=1, got MemberCount=%d, NodeCount=%d", resp.Data.MemberCount, resp.Data.NodeCount)
	}
	// Candidate nodes evaluated: Tokyo, London, Singapore (from the regex scan over candidate pool) = 3
	if resp.Data.CandidateCount != 3 {
		t.Errorf("expected CandidateCount=3, got %d", resp.Data.CandidateCount)
	}
	// Excluded count = Candidate (3) - NodeCount (1) = 2
	if resp.Data.ExcludedCount != 2 {
		t.Errorf("expected ExcludedCount=2, got %d", resp.Data.ExcludedCount)
	}
	// Candidate = NodeCount + ExcludedCount
	if resp.Data.CandidateCount != resp.Data.NodeCount+resp.Data.ExcludedCount {
		t.Errorf("expected CandidateCount == NodeCount + ExcludedCount, got %d != %d + %d",
			resp.Data.CandidateCount, resp.Data.NodeCount, resp.Data.ExcludedCount)
	}
	// Exclusion summary should have "regex removed": 1
	if resp.Data.ExclusionSummary["regex removed"] != 1 {
		t.Errorf("expected ExclusionSummary['regex removed']=1, got %d", resp.Data.ExclusionSummary["regex removed"])
	}
}

func TestPoliciesAdmissionRulesRetiredWorkflow(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, policySvc := setupPolicyTestRouter(t, db)
	ctx := context.Background()

	// 1. Create an active revision
	rev, err := policySvc.CreateRevision(ctx, policy.CreateRevisionCommand{
		State:     domain.RevisionStateActive,
		RequestID: "req-rev-retire-test",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create revision: %v", err)
	}

	// 2. Reject POST /api/v1/admission/rules -> 422 admission_rules_retired
	reqAdmAlias := httptest.NewRequest(http.MethodPost, "/api/v1/admission/rules", strings.NewReader(`{
		"name": "retired-alias",
		"expression": "country == 'US'",
		"action": "allow",
		"position": 0
	}`))
	reqAdmAlias.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqAdmAlias.Header.Set("Content-Type", "application/json")
	recAdmAlias := httptest.NewRecorder()
	router.ServeHTTP(recAdmAlias, reqAdmAlias)
	if recAdmAlias.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for POST /api/v1/admission/rules, got %d: %s", recAdmAlias.Code, recAdmAlias.Body.String())
	}
	var errResp testErrorResponse
	if err := json.Unmarshal(recAdmAlias.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Code != "admission_rules_retired" {
		t.Fatalf("expected code admission_rules_retired, got %s", errResp.Code)
	}

	// 3. Reject POST /api/v1/policies/rules with kind=admission -> 422 admission_rules_retired
	reqAdmKind := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(`{
		"kind": "admission",
		"name": "retired-kind",
		"expression": "country == 'HK'",
		"action": "allow",
		"position": 0
	}`))
	reqAdmKind.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqAdmKind.Header.Set("Content-Type", "application/json")
	recAdmKind := httptest.NewRecorder()
	router.ServeHTTP(recAdmKind, reqAdmKind)
	if recAdmKind.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for POST /api/v1/policies/rules with kind=admission, got %d", recAdmKind.Code)
	}
	if err := json.Unmarshal(recAdmKind.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Code != "admission_rules_retired" {
		t.Fatalf("expected code admission_rules_retired, got %s", errResp.Code)
	}

	// 4. Reject POST /api/v1/policies/rules with implicit action (action non-empty, kind empty) -> 422 admission_rules_retired
	reqAdmImplicit := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(`{
		"name": "retired-implicit",
		"expression": "country == 'JP'",
		"action": "reject",
		"position": 0
	}`))
	reqAdmImplicit.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqAdmImplicit.Header.Set("Content-Type", "application/json")
	recAdmImplicit := httptest.NewRecorder()
	router.ServeHTTP(recAdmImplicit, reqAdmImplicit)
	if recAdmImplicit.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for implicit admission action, got %d", recAdmImplicit.Code)
	}
	if err := json.Unmarshal(recAdmImplicit.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Code != "admission_rules_retired" {
		t.Fatalf("expected code admission_rules_retired, got %s", errResp.Code)
	}

	// 5. Reject POST /api/v1/policies/rules with unknown kind -> 422 invalid_rule_kind
	reqUnknownKind := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(`{
		"kind": "unsupported_kind",
		"expression": "DOMAIN,example.com",
		"builtin_target": "DIRECT"
	}`))
	reqUnknownKind.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqUnknownKind.Header.Set("Content-Type", "application/json")
	recUnknownKind := httptest.NewRecorder()
	router.ServeHTTP(recUnknownKind, reqUnknownKind)
	if recUnknownKind.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for unknown kind, got %d", recUnknownKind.Code)
	}
	if err := json.Unmarshal(recUnknownKind.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Code != "invalid_rule_kind" {
		t.Fatalf("expected code invalid_rule_kind, got %s", errResp.Code)
	}

	// 6. Legitimate policy rule creation still succeeds
	sharedID := "0192a000-0000-7000-8000-000000000001"
	reqCreatePol := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(fmt.Sprintf(`{
		"id": "%s",
		"kind": "policy",
		"expression": "DOMAIN-SUFFIX,shared-target.com",
		"builtin_target": "DIRECT",
		"position": 0
	}`, sharedID)))
	reqCreatePol.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqCreatePol.Header.Set("Content-Type", "application/json")
	recCreatePol := httptest.NewRecorder()
	router.ServeHTTP(recCreatePol, reqCreatePol)
	if recCreatePol.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for policy rule, got %d: %s", recCreatePol.Code, recCreatePol.Body.String())
	}

	// Directly insert a legacy admission rule with the SAME ID into SQLite
	_, err = db.Exec(`INSERT INTO admission_rules (id, revision_id, name, expression, action, position) VALUES (?, ?, 'SameIDLegacyAdm', 'protocol == "vless"', 'allow', 0)`, sharedID, rev.ID)
	if err != nil {
		t.Fatalf("failed to insert legacy admission rule with shared ID: %v", err)
	}

	// 7. Calling DELETE /api/v1/admission/rules/{id} must reject with 422 and NOT delete the policy rule with the same ID!
	reqDelAdmRoute := httptest.NewRequest(http.MethodDelete, "/api/v1/admission/rules/"+sharedID, nil)
	reqDelAdmRoute.Header.Set("Authorization", "Bearer "+testAdminToken)
	recDelAdmRoute := httptest.NewRecorder()
	router.ServeHTTP(recDelAdmRoute, reqDelAdmRoute)
	if recDelAdmRoute.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 UnprocessableEntity for DELETE /api/v1/admission/rules/{id}, got %d: %s", recDelAdmRoute.Code, recDelAdmRoute.Body.String())
	}
	if err := json.Unmarshal(recDelAdmRoute.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Code != "admission_rules_retired" {
		t.Fatalf("expected code admission_rules_retired, got %s", errResp.Code)
	}

	// Verify policy rule with sharedID is STILL ALIVE
	reqCheckPol := httptest.NewRequest(http.MethodGet, "/api/v1/policies/rules?kind=policy", nil)
	reqCheckPol.Header.Set("Authorization", "Bearer "+testAdminToken)
	recCheckPol := httptest.NewRecorder()
	router.ServeHTTP(recCheckPol, reqCheckPol)
	var polListResp testDataResponse[policy.ListRulesResult]
	if err := json.Unmarshal(recCheckPol.Body.Bytes(), &polListResp); err != nil {
		t.Fatalf("failed to parse list response: %v", err)
	}
	if len(polListResp.Data.PolicyRules) != 1 || polListResp.Data.PolicyRules[0].ID != sharedID {
		t.Fatalf("expected policy rule %s to be preserved after admission delete attempt, got %+v", sharedID, polListResp.Data.PolicyRules)
	}

	// 8. DELETE /api/v1/policies/rules/{id}?kind=admission also rejects with 422 and preserves both rules
	reqDelKindAdm := httptest.NewRequest(http.MethodDelete, "/api/v1/policies/rules/"+sharedID+"?kind=admission", nil)
	reqDelKindAdm.Header.Set("Authorization", "Bearer "+testAdminToken)
	recDelKindAdm := httptest.NewRecorder()
	router.ServeHTTP(recDelKindAdm, reqDelKindAdm)
	if recDelKindAdm.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for DELETE with kind=admission, got %d", recDelKindAdm.Code)
	}

	// 9. Generic DELETE /api/v1/policies/rules/{id} (kind empty) deletes the policy rule, and leaves admission rule untouched
	reqDelGeneric := httptest.NewRequest(http.MethodDelete, "/api/v1/policies/rules/"+sharedID, nil)
	reqDelGeneric.Header.Set("Authorization", "Bearer "+testAdminToken)
	recDelGeneric := httptest.NewRecorder()
	router.ServeHTTP(recDelGeneric, reqDelGeneric)
	if recDelGeneric.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for generic DELETE, got %d: %s", recDelGeneric.Code, recDelGeneric.Body.String())
	}

	// Verify policy rule is gone
	recCheckPol2 := httptest.NewRecorder()
	router.ServeHTTP(recCheckPol2, reqCheckPol)
	if err := json.Unmarshal(recCheckPol2.Body.Bytes(), &polListResp); err != nil {
		t.Fatalf("failed to parse list response: %v", err)
	}
	if len(polListResp.Data.PolicyRules) != 0 {
		t.Fatalf("expected 0 policy rules after generic delete, got %d", len(polListResp.Data.PolicyRules))
	}

	// Verify legacy admission rule in database is STILL intact
	var legacyAdmCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM admission_rules WHERE id = ?", sharedID).Scan(&legacyAdmCount); err != nil {
		t.Fatalf("failed to query admission_rules table: %v", err)
	}
	if legacyAdmCount != 1 {
		t.Fatalf("expected legacy admission rule to remain in table, got count %d", legacyAdmCount)
	}

	// 10. Legacy GET /api/v1/admission/rules returns 200 OK with legacy admission rules
	reqGetLegacyAdm := httptest.NewRequest(http.MethodGet, "/api/v1/admission/rules", nil)
	reqGetLegacyAdm.Header.Set("Authorization", "Bearer "+testAdminToken)
	recGetLegacyAdm := httptest.NewRecorder()
	router.ServeHTTP(recGetLegacyAdm, reqGetLegacyAdm)
	if recGetLegacyAdm.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET /api/v1/admission/rules, got %d", recGetLegacyAdm.Code)
	}
	var legacyListResp testDataResponse[policy.ListRulesResult]
	if err := json.Unmarshal(recGetLegacyAdm.Body.Bytes(), &legacyListResp); err != nil {
		t.Fatalf("failed to parse list response: %v", err)
	}
	if len(legacyListResp.Data.AdmissionRules) != 1 || legacyListResp.Data.AdmissionRules[0].ID != sharedID {
		t.Fatalf("expected 1 legacy admission rule returned, got %+v", legacyListResp.Data.AdmissionRules)
	}
}

func TestPoliciesCreateRule_CategorizedWithMATCH(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, policySvc := setupPolicyTestRouter(t, db)
	ctx := context.Background()

	rev, err := policySvc.CreateRevision(ctx, policy.CreateRevisionCommand{
		State:     domain.RevisionStateActive,
		RequestID: "req-rev-cat-test",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create revision: %v", err)
	}

	// 1. Create categories "Default" and "Final" via service
	catDefault, err := policySvc.CreateCategory(ctx, policy.CreateCategoryCommand{
		Name:      "Default",
		SortOrder: 1,
	})
	if err != nil {
		t.Fatalf("failed to create Default category: %v", err)
	}
	catFinal, err := policySvc.CreateCategory(ctx, policy.CreateCategoryCommand{
		Name:      "Final",
		SortOrder: 99,
	})
	if err != nil {
		t.Fatalf("failed to create Final category: %v", err)
	}

	// 2. Create MATCH rule in Final category via HTTP POST /api/v1/policies/rules
	matchPayload := fmt.Sprintf(`{
		"revision_id": "%s",
		"category_id": "%s",
		"builtin_target": "DIRECT",
		"expression": "MATCH",
		"position": 0
	}`, rev.ID, catFinal.ID)
	reqMatch := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(matchPayload))
	reqMatch.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqMatch.Header.Set("Content-Type", "application/json")
	recMatch := httptest.NewRecorder()
	router.ServeHTTP(recMatch, reqMatch)
	if recMatch.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for MATCH rule, got %d: %s", recMatch.Code, recMatch.Body.String())
	}

	// 3. Create a new routing rule in Default category via HTTP POST /api/v1/policies/rules
	// This exercises the RulesView workflow where an existing MATCH rule exists and a new categorized rule is created.
	newRulePayload := fmt.Sprintf(`{
		"revision_id": "%s",
		"category_id": "%s",
		"builtin_target": "DIRECT",
		"expression": "DOMAIN-KEYWORD,github",
		"position": 0
	}`, rev.ID, catDefault.ID)
	reqNewRule := httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", strings.NewReader(newRulePayload))
	reqNewRule.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqNewRule.Header.Set("Content-Type", "application/json")
	recNewRule := httptest.NewRecorder()
	router.ServeHTTP(recNewRule, reqNewRule)
	if recNewRule.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for new categorized rule when MATCH already exists, got %d: %s", recNewRule.Code, recNewRule.Body.String())
	}

	// 4. Verify GET /api/v1/policies/rules shows both rules
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/policies/rules?revision_id="+rev.ID, nil)
	reqList.Header.Set("Authorization", "Bearer "+testAdminToken)
	recList := httptest.NewRecorder()
	router.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK listing rules, got %d: %s", recList.Code, recList.Body.String())
	}
	var listResp testDataResponse[policy.ListRulesResult]
	if err := json.Unmarshal(recList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to decode list rules response: %v", err)
	}
	if len(listResp.Data.PolicyRules) != 2 {
		t.Fatalf("expected 2 policy rules, got %d", len(listResp.Data.PolicyRules))
	}

	// 5. Verify resolver.Resolve maintains MATCH as the terminal rule
	rulesForResolve := make([]domain.PolicyRule, len(listResp.Data.PolicyRules))
	for i, r := range listResp.Data.PolicyRules {
		rulesForResolve[i] = domain.PolicyRule{
			ID:            r.ID,
			RevisionID:    r.RevisionID,
			CategoryID:    r.CategoryID,
			Category:      r.Category,
			BuiltinTarget: r.BuiltinTarget,
			Expression:    r.Expression,
			Position:      r.Position,
			Enabled:       r.Enabled,
		}
	}
	snap, err := resolver.New().Resolve(ctx, resolver.ResolveInput{
		PolicyRules: rulesForResolve,
	})
	if err != nil {
		t.Fatalf("resolver.Resolve failed: %v", err)
	}
	if len(snap.Rules) != 2 {
		t.Fatalf("expected 2 resolved rules, got %d", len(snap.Rules))
	}
	lastRule := snap.Rules[len(snap.Rules)-1]
	if !lastRule.IsTerminal || lastRule.Expression != "MATCH" {
		t.Fatalf("expected terminal MATCH rule at end of resolved export, got: %+v", lastRule)
	}
}
