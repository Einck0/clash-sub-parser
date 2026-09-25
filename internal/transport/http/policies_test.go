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

	policySvc := policy.NewService(policyRepo, revRepo, nodeRepo, auditRepo)
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

func TestPolicyValidateEndpoint(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPolicyTestRouter(t, db)

	// Valid graph (empty or valid)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/validate", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

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
}
