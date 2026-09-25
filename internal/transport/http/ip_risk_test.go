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
	"time"

	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func setupIPRiskTestRouter(t *testing.T, db *sql.DB) (http.Handler, *iprisk.Service, *policy.Service, domain.AuditRepository) {
	t.Helper()
	routerCfg := newTestRouterConfig(true)
	routerCfg.IsPublicationToken = func(_ context.Context, token string) bool {
		return token == testPublicationToken
	}

	// Ensure provider settings exist for FK constraints
	_, _ = db.ExecContext(context.Background(), `
		INSERT OR IGNORE INTO ip_risk_provider_settings (
			provider, schema_version, secret_reference, max_concurrency,
			requests_per_minute, daily_request_budget, per_request_timeout_ms,
			max_response_bytes, created_at, updated_at
		) VALUES ('fixture', 'v1', 'secret://fixture', 1, 10, 100, 5, 1024,
			'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');
	`)

	policyRepo := sqlite.NewPolicyRepository(db)
	revRepo := sqlite.NewRevisionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	obsRepo := sqlite.NewIPRiskObservationRepository(db)
	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	bindingRepo := sqlite.NewRiskPolicyGroupBindingRepository(db)

	policySvc := policy.NewService(policyRepo, revRepo, nodeRepo, auditRepo)
	ipRiskSvc := iprisk.NewService(
		obsRepo,
		riskPolicyRepo,
		iprisk.WithBindingRepository(bindingRepo),
		iprisk.WithGroupRepository(policyRepo),
		iprisk.WithNodeRepository(nodeRepo),
		iprisk.WithAuditRepository(auditRepo),
	)

	routerCfg.PolicyService = policySvc
	routerCfg.AuditRepository = auditRepo
	routerCfg.IPRiskService = ipRiskSvc
	routerCfg.RiskPolicyRepository = riskPolicyRepo
	routerCfg.RiskBindingRepository = bindingRepo

	return transporthttp.NewRouter(routerCfg), ipRiskSvc, policySvc, auditRepo
}

func validRiskPolicyPayload() map[string]any {
	return map[string]any{
		"provider_selection": map[string]any{
			"mode": "single_provider",
			"providers": []map[string]any{
				{"provider": "fixture", "schema_version": "v1"},
			},
		},
		"max_observation_age_seconds": 3600,
		"minimum_confidence":          50,
		"score_bands": []map[string]any{
			{"min": 0, "max": 49, "band": "low", "action": "allow"},
			{"min": 50, "max": 79, "band": "high", "action": "review"},
			{"min": 80, "max": 100, "band": "critical", "action": "block"},
		},
		"trait_rules": []map[string]any{
			{"trait": "tor", "action": "block"},
		},
		"unknown_action":  "review",
		"conflict_action": "review",
		"review_action":   "review",
	}
}

func TestIPRiskPolicyRoutesAuthAndCSRFGuardrails(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, _, _ := setupIPRiskTestRouter(t, db)

	// 1. Unauthenticated requests strictly return 401 Unauthorized
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/ip-risk/policies"},
		{http.MethodPost, "/api/v1/ip-risk/policies"},
		{http.MethodGet, "/api/v1/ip-risk/policies/active"},
		{http.MethodGet, "/api/v1/ip-risk/bindings"},
	} {
		t.Run("unauthenticated "+tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
			}
		})
	}

	// 2. Publication export tokens attempting to access risk management routes return 403 Forbidden
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ip-risk/policies", nil)
	req.Header.Set("Authorization", "Bearer "+testPublicationToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for publication token, got %d", rec.Code)
	}

	// 3. Cookie session without CSRF token on POST returns 403 Forbidden
	body, _ := json.Marshal(validRiskPolicyPayload())
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/ip-risk/policies", bytes.NewReader(body))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Idempotency-Key", "csrf-missing-key")
	postReq.AddCookie(&http.Cookie{Name: "csp_session", Value: testValidSessionID})
	recPost := httptest.NewRecorder()
	router.ServeHTTP(recPost, postReq)
	if recPost.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for missing CSRF on cookie session, got %d", recPost.Code)
	}

	// 4. Cookie session with valid CSRF token succeeds
	postReqOk := httptest.NewRequest(http.MethodPost, "/api/v1/ip-risk/policies", bytes.NewReader(body))
	postReqOk.Header.Set("Content-Type", "application/json")
	postReqOk.Header.Set("Idempotency-Key", "csrf-valid-key")
	postReqOk.Header.Set("X-CSRF-Token", testValidCSRFToken)
	postReqOk.AddCookie(&http.Cookie{Name: "csp_session", Value: testValidSessionID})
	recPostOk := httptest.NewRecorder()
	router.ServeHTTP(recPostOk, postReqOk)
	if recPostOk.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created with valid CSRF, got %d: %s", recPostOk.Code, recPostOk.Body.String())
	}
}

func TestIPRiskPolicyIdempotencyKeyEnforcement(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, _, _ := setupIPRiskTestRouter(t, db)

	body, _ := json.Marshal(validRiskPolicyPayload())

	// 1. Missing Idempotency-Key on policy creation returns 422 Unprocessable Entity
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ip-risk/policies", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for missing Idempotency-Key, got %d", rec.Code)
	}

	var errResp testErrorResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Code != "missing_idempotency_key" {
		t.Fatalf("expected code missing_idempotency_key, got %s", errResp.Code)
	}

	// 2. With Idempotency-Key, policy creation succeeds
	reqOk := httptest.NewRequest(http.MethodPost, "/api/v1/ip-risk/policies", bytes.NewReader(body))
	reqOk.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqOk.Header.Set("Content-Type", "application/json")
	reqOk.Header.Set("Idempotency-Key", "key-policy-create-1")
	recOk := httptest.NewRecorder()
	router.ServeHTTP(recOk, reqOk)
	if recOk.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created with Idempotency-Key, got %d", recOk.Code)
	}

	var created domain.RiskPolicyRevision
	var wrapper testDataResponse[domain.RiskPolicyRevision]
	if err := json.Unmarshal(recOk.Body.Bytes(), &wrapper); err != nil {
		t.Fatalf("decode created revision: %v", err)
	}
	created = wrapper.Data

	// 3. Missing Idempotency-Key on activate returns 422
	actReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/activate", created.RevisionID), nil)
	actReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	recAct := httptest.NewRecorder()
	router.ServeHTTP(recAct, actReq)
	if recAct.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing Idempotency-Key on activate, got %d", recAct.Code)
	}

	// 4. Missing Idempotency-Key on deactivate returns 422
	deactReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/deactivate", created.RevisionID), nil)
	deactReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	recDeact := httptest.NewRecorder()
	router.ServeHTTP(recDeact, deactReq)
	if recDeact.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing Idempotency-Key on deactivate, got %d", recDeact.Code)
	}
}

func TestIPRiskPolicyRejectsInvalidPayloadAndFilters(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, _, _ := setupIPRiskTestRouter(t, db)

	// 1. Invalid query parameters
	for _, tc := range []struct {
		name string
		url  string
	}{
		{"negative page", "/api/v1/ip-risk/policies?page=-1"},
		{"zero page_size", "/api/v1/ip-risk/policies?page_size=0"},
		{"oversized page_size", "/api/v1/ip-risk/policies?page_size=200"},
		{"invalid active boolean", "/api/v1/ip-risk/policies?active=maybe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			req.Header.Set("Authorization", "Bearer "+testAdminToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422 for %s, got %d: %s", tc.name, rec.Code, rec.Body.String())
			}
		})
	}

	// 2. Invalid policy payloads
	for _, tc := range []struct {
		name    string
		payload map[string]any
	}{
		{
			name: "incomplete score bands starting at 10",
			payload: func() map[string]any {
				p := validRiskPolicyPayload()
				p["score_bands"] = []map[string]any{
					{"min": 10, "max": 100, "band": "high", "action": "block"},
				}
				return p
			}(),
		},
		{
			name: "overlapping score bands",
			payload: func() map[string]any {
				p := validRiskPolicyPayload()
				p["score_bands"] = []map[string]any{
					{"min": 0, "max": 50, "band": "low", "action": "allow"},
					{"min": 50, "max": 100, "band": "high", "action": "block"},
				}
				return p
			}(),
		},
		{
			name: "incomplete score bands ending at 80",
			payload: func() map[string]any {
				p := validRiskPolicyPayload()
				p["score_bands"] = []map[string]any{
					{"min": 0, "max": 80, "band": "low", "action": "allow"},
				}
				return p
			}(),
		},
		{
			name: "invalid fusion mode",
			payload: func() map[string]any {
				p := validRiskPolicyPayload()
				p["provider_selection"] = map[string]any{
					"mode": "invalid_mode",
					"providers": []map[string]any{
						{"provider": "fixture", "schema_version": "v1"},
					},
				}
				return p
			}(),
		},
		{
			name: "invalid trait rule action",
			payload: func() map[string]any {
				p := validRiskPolicyPayload()
				p["trait_rules"] = []map[string]any{
					{"trait": "tor", "action": "invalid_action"},
				}
				return p
			}(),
		},
		{
			name: "invalid confidence over 100",
			payload: func() map[string]any {
				p := validRiskPolicyPayload()
				p["minimum_confidence"] = 150
				return p
			}(),
		},
		{
			name: "non-positive observation age",
			payload: func() map[string]any {
				p := validRiskPolicyPayload()
				p["max_observation_age_seconds"] = 0
				return p
			}(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/ip-risk/policies", bytes.NewReader(raw))
			req.Header.Set("Authorization", "Bearer "+testAdminToken)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "key-"+tc.name)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422 for %s, got %d: %s", tc.name, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestIPRiskPolicyRevisionLifecycleAndAudit(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, _, auditRepo := setupIPRiskTestRouter(t, db)

	// 1. Create a risk policy revision
	body, _ := json.Marshal(validRiskPolicyPayload())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ip-risk/policies", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "create-key-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var wrapper testDataResponse[domain.RiskPolicyRevision]
	if err := json.Unmarshal(rec.Body.Bytes(), &wrapper); err != nil {
		t.Fatalf("unmarshal created policy: %v", err)
	}
	rev1 := wrapper.Data
	if rev1.RevisionID == "" || rev1.Active {
		t.Fatalf("newly created revision must have UUID and active=false: %+v", rev1)
	}

	// 2. Query policy by ID
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/ip-risk/policies/"+rev1.RevisionID, nil)
	getReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET policy, got %d", getRec.Code)
	}

	// 3. List policies
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/ip-risk/policies?page=1&page_size=10", nil)
	listReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for list policies, got %d", listRec.Code)
	}
	var listWrapper testDataResponse[testPaginationData[domain.RiskPolicyRevision]]
	if err := json.Unmarshal(listRec.Body.Bytes(), &listWrapper); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listWrapper.Data.Total != 1 || len(listWrapper.Data.Items) != 1 {
		t.Fatalf("expected 1 policy in list, got total=%d items=%d", listWrapper.Data.Total, len(listWrapper.Data.Items))
	}

	// 4. Review policy
	revReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/review", rev1.RevisionID), nil)
	revReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	revReq.Header.Set("Idempotency-Key", "review-key-1")
	revRec := httptest.NewRecorder()
	router.ServeHTTP(revRec, revReq)
	if revRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for review policy, got %d: %s", revRec.Code, revRec.Body.String())
	}

	// 5. Activate policy
	actReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/activate", rev1.RevisionID), nil)
	actReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	actReq.Header.Set("Idempotency-Key", "act-key-1")
	actRec := httptest.NewRecorder()
	router.ServeHTTP(actRec, actReq)
	if actRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for activate policy, got %d: %s", actRec.Code, actRec.Body.String())
	}
	var actWrapper testDataResponse[domain.RiskPolicyRevision]
	_ = json.Unmarshal(actRec.Body.Bytes(), &actWrapper)
	if !actWrapper.Data.Active {
		t.Fatalf("activated policy must have active=true")
	}

	// 6. Get active policy
	actGetReq := httptest.NewRequest(http.MethodGet, "/api/v1/ip-risk/policies/active", nil)
	actGetReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	actGetRec := httptest.NewRecorder()
	router.ServeHTTP(actGetRec, actGetReq)
	if actGetRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for active policy, got %d", actGetRec.Code)
	}

	// 7. Deactivate policy
	deactReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/deactivate", rev1.RevisionID), nil)
	deactReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	deactReq.Header.Set("Idempotency-Key", "deact-key-1")
	deactRec := httptest.NewRecorder()
	router.ServeHTTP(deactRec, deactReq)
	if deactRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for deactivate policy, got %d", deactRec.Code)
	}
	var deactWrapper testDataResponse[domain.RiskPolicyRevision]
	_ = json.Unmarshal(deactRec.Body.Bytes(), &deactWrapper)
	if deactWrapper.Data.Active {
		t.Fatalf("deactivated policy must have active=false")
	}

	// 8. Verify audit logs in database
	events, total, err := auditRepo.List(context.Background(), domain.AuditFilter{})
	if err != nil || total < 4 {
		t.Fatalf("expected at least 4 audit events, got %d (err: %v)", total, err)
	}
	actionsSeen := make(map[string]bool)
	for _, ev := range events {
		actionsSeen[ev.Action] = true
	}
	for _, expectedAction := range []string{
		"risk_policy.create",
		"risk_policy.review",
		"risk_policy.activate",
		"risk_policy.deactivate",
	} {
		if !actionsSeen[expectedAction] {
			t.Fatalf("expected audit action %s to be recorded, actions seen: %+v", expectedAction, actionsSeen)
		}
	}
}

func TestIPRiskPolicyGroupBindingLifecycleAndAudit(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, policySvc, auditRepo := setupIPRiskTestRouter(t, db)

	// Create a policy group
	grp, err := policySvc.CreateGroup(context.Background(), policy.CreateGroupCommand{
		Name:      "TestGroup",
		GroupType: domain.GroupTypeSelect,
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	// Create a risk policy revision
	body, _ := json.Marshal(validRiskPolicyPayload())
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/ip-risk/policies", bytes.NewReader(body))
	postReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Idempotency-Key", "bind-policy-create-key")
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusCreated {
		t.Fatalf("create policy: %d", postRec.Code)
	}
	var polWrapper testDataResponse[domain.RiskPolicyRevision]
	_ = json.Unmarshal(postRec.Body.Bytes(), &polWrapper)
	pol := polWrapper.Data

	// 1. Bind group to policy revision
	bindPayload, _ := json.Marshal(map[string]any{
		"policy_revision_id": pol.RevisionID,
	})
	bindReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/ip-risk/bindings/%s", grp.ID), bytes.NewReader(bindPayload))
	bindReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("Idempotency-Key", "bind-key-1")
	bindRec := httptest.NewRecorder()
	router.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for bind group, got %d: %s", bindRec.Code, bindRec.Body.String())
	}

	var bindWrapper testDataResponse[domain.RiskPolicyGroupBinding]
	if err := json.Unmarshal(bindRec.Body.Bytes(), &bindWrapper); err != nil {
		t.Fatalf("unmarshal binding: %v", err)
	}
	if bindWrapper.Data.GroupID != grp.ID || bindWrapper.Data.PolicyRevisionID != pol.RevisionID {
		t.Fatalf("binding mismatch: %+v", bindWrapper.Data)
	}

	// 2. Query group binding
	getBindReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/ip-risk/bindings/%s", grp.ID), nil)
	getBindReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	getBindRec := httptest.NewRecorder()
	router.ServeHTTP(getBindRec, getBindReq)
	if getBindRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for get binding, got %d", getBindRec.Code)
	}

	// 3. Query policy group route alias: /api/v1/policies/groups/{id}/risk-policy
	aliasReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/policies/groups/%s/risk-policy", grp.ID), nil)
	aliasReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	aliasRec := httptest.NewRecorder()
	router.ServeHTTP(aliasRec, aliasReq)
	if aliasRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for group risk-policy alias, got %d: %s", aliasRec.Code, aliasRec.Body.String())
	}

	// 4. List all bindings
	listBindReq := httptest.NewRequest(http.MethodGet, "/api/v1/ip-risk/bindings", nil)
	listBindReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	listBindRec := httptest.NewRecorder()
	router.ServeHTTP(listBindRec, listBindReq)
	if listBindRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for list bindings, got %d", listBindRec.Code)
	}

	// 5. Unbind group
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/ip-risk/bindings/%s", grp.ID), nil)
	delReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	delReq.Header.Set("Idempotency-Key", "unbind-key-1")
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for unbind group, got %d: %s", delRec.Code, delRec.Body.String())
	}

	// Verify group is now unbound
	getAfterDel := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/ip-risk/bindings/%s", grp.ID), nil)
	getAfterDel.Header.Set("Authorization", "Bearer "+testAdminToken)
	getAfterDelRec := httptest.NewRecorder()
	router.ServeHTTP(getAfterDelRec, getAfterDel)
	if getAfterDelRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after unbind, got %d", getAfterDelRec.Code)
	}

	// 6. Verify audit logs for binding & unbinding
	events, _, err := auditRepo.List(context.Background(), domain.AuditFilter{})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	bindAudited, unbindAudited := false, false
	for _, ev := range events {
		if ev.Action == "risk_policy.bind_group" {
			bindAudited = true
		}
		if ev.Action == "risk_policy.unbind_group" {
			unbindAudited = true
		}
	}
	if !bindAudited || !unbindAudited {
		t.Fatalf("expected bind_group and unbind_group audit events, got bind=%v unbind=%v", bindAudited, unbindAudited)
	}
}

func TestIPRiskUnactivatedPolicyDoesNotAffectGroupMembers(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, policySvc, _ := setupIPRiskTestRouter(t, db)

	// Setup 2 nodes:
	// Node 1: low risk (score 20 -> allow)
	// Node 2: high risk (score 95 -> block)
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, created_at, updated_at)
		VALUES
		('node_1111111111111111', 'ss', 'Safe Node', 'sec://safe', '2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z'),
		('node_2222222222222222', 'ss', 'Risky Node', 'sec://risky', '2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("insert nodes: %v", err)
	}

	// Insert provider setting
	_, err = db.ExecContext(context.Background(), `
		INSERT OR REPLACE INTO ip_risk_provider_settings (
			provider, schema_version, secret_reference, max_concurrency,
			requests_per_minute, daily_request_budget, per_request_timeout_ms,
			max_response_bytes, created_at, updated_at
		) VALUES ('fixture', 'v1', 'secret://fixture', 1, 10, 100, 5, 1024,
			'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("insert provider settings: %v", err)
	}

	// Insert observations for both nodes
	safeScore, safeConf := 20, 90
	riskyScore, riskyConf := 95, 95
	obsRepo := sqlite.NewIPRiskObservationRepository(db)
	_ = obsRepo.Create(context.Background(), &domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         "node_1111111111111111",
		ExitIdentityDigest:    "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		Provider:              "fixture",
		ProviderSchemaVersion: "v1",
		ObservedAt:            time.Now().UTC().Add(-10 * time.Minute),
		ExpiresAt:             time.Now().UTC().Add(50 * time.Minute),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &safeScore,
		Confidence:            &safeConf,
		NetworkClass:          domain.NetworkClassResidential,
		EvidenceDigest:        "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		RedactedSummary:       "safe residential node",
	})
	_ = obsRepo.Create(context.Background(), &domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         "node_2222222222222222",
		ExitIdentityDigest:    "sha256:3333333333333333333333333333333333333333333333333333333333333333",
		Provider:              "fixture",
		ProviderSchemaVersion: "v1",
		ObservedAt:            time.Now().UTC().Add(-10 * time.Minute),
		ExpiresAt:             time.Now().UTC().Add(50 * time.Minute),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &riskyScore,
		Confidence:            &riskyConf,
		NetworkClass:          domain.NetworkClassDatacenter,
		EvidenceDigest:        "sha256:4444444444444444444444444444444444444444444444444444444444444444",
		RedactedSummary:       "high risk proxy node",
	})

	// Create group with both nodes
	safeNodeID := "node_1111111111111111"
	riskyNodeID := "node_2222222222222222"
	grp, err := policySvc.CreateGroup(context.Background(), policy.CreateGroupCommand{
		Name:      "ProtectedGroup",
		GroupType: domain.GroupTypeSelect,
		Edges: []policy.EdgeInput{
			{NodeLogicalID: &safeNodeID, Position: 0},
			{NodeLogicalID: &riskyNodeID, Position: 1},
		},
	})
	if err != nil {
		t.Fatalf("create group with nodes: %v", err)
	}

	// Create risk policy revision (starts inactive: active=false)
	body, _ := json.Marshal(validRiskPolicyPayload())
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/ip-risk/policies", bytes.NewReader(body))
	postReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Idempotency-Key", "members-test-create-policy")
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusCreated {
		t.Fatalf("create policy: %d", postRec.Code)
	}
	var polWrapper testDataResponse[domain.RiskPolicyRevision]
	_ = json.Unmarshal(postRec.Body.Bytes(), &polWrapper)
	pol := polWrapper.Data

	// Bind group to this UNACTIVATED policy revision
	bindPayload, _ := json.Marshal(map[string]any{
		"policy_revision_id": pol.RevisionID,
	})
	bindReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/ip-risk/bindings/%s", grp.ID), bytes.NewReader(bindPayload))
	bindReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("Idempotency-Key", "bind-unactive-key")
	bindRec := httptest.NewRecorder()
	router.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("bind group: %d", bindRec.Code)
	}

	// 1. When policy is UNACTIVATED: all members remain admitted, ZERO exclusions!
	evalReq1 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/ip-risk/groups/%s/members", grp.ID), nil)
	evalReq1.Header.Set("Authorization", "Bearer "+testAdminToken)
	evalRec1 := httptest.NewRecorder()
	router.ServeHTTP(evalRec1, evalReq1)
	if evalRec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for group evaluation with unactivated policy, got %d: %s", evalRec1.Code, evalRec1.Body.String())
	}

	var evalRes1 testDataResponse[map[string]any]
	_ = json.Unmarshal(evalRec1.Body.Bytes(), &evalRes1)
	policyActive1, _ := evalRes1.Data["policy_active"].(bool)
	if policyActive1 {
		t.Fatalf("expected policy_active to be false for unactivated policy")
	}
	admitted1, _ := evalRes1.Data["admitted_members"].([]any)
	excluded1, _ := evalRes1.Data["excluded_members"].([]any)
	if len(admitted1) != 2 || len(excluded1) != 0 {
		t.Fatalf("unactivated policy must not affect members: admitted=%d excluded=%d", len(admitted1), len(excluded1))
	}

	// 2. ACTIVATE the policy
	actReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/activate", pol.RevisionID), nil)
	actReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	actReq.Header.Set("Idempotency-Key", "activate-members-test-key")
	actRec := httptest.NewRecorder()
	router.ServeHTTP(actRec, actReq)
	if actRec.Code != http.StatusOK {
		t.Fatalf("activate policy: %d", actRec.Code)
	}

	// 3. When policy IS ACTIVATED: risky node (score 95) is blocked and EXCLUDED!
	evalReq2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/ip-risk/groups/%s/members", grp.ID), nil)
	evalReq2.Header.Set("Authorization", "Bearer "+testAdminToken)
	evalRec2 := httptest.NewRecorder()
	router.ServeHTTP(evalRec2, evalReq2)
	if evalRec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for group evaluation with activated policy, got %d: %s", evalRec2.Code, evalRec2.Body.String())
	}

	var evalRes2 testDataResponse[map[string]any]
	_ = json.Unmarshal(evalRec2.Body.Bytes(), &evalRes2)
	policyActive2, _ := evalRes2.Data["policy_active"].(bool)
	if !policyActive2 {
		t.Fatalf("expected policy_active to be true for activated policy")
	}
	admitted2, _ := evalRes2.Data["admitted_members"].([]any)
	excluded2, _ := evalRes2.Data["excluded_members"].([]any)
	if len(admitted2) != 1 || len(excluded2) != 1 {
		t.Fatalf("activated policy must exclude block members: admitted=%d excluded=%d (expected 1 and 1)", len(admitted2), len(excluded2))
	}
	if admitted2[0].(string) != safeNodeID {
		t.Fatalf("expected safe node %s to be admitted, got %v", safeNodeID, admitted2[0])
	}
	if excluded2[0].(string) != riskyNodeID {
		t.Fatalf("expected risky node %s to be excluded, got %v", riskyNodeID, excluded2[0])
	}

	// 4. DEACTIVATE the policy again: group members are once again unaffected
	deactReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/deactivate", pol.RevisionID), nil)
	deactReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	deactReq.Header.Set("Idempotency-Key", "deactivate-members-test-key")
	deactRec := httptest.NewRecorder()
	router.ServeHTTP(deactRec, deactReq)
	if deactRec.Code != http.StatusOK {
		t.Fatalf("deactivate policy: %d", deactRec.Code)
	}

	evalReq3 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/ip-risk/groups/%s/members", grp.ID), nil)
	evalReq3.Header.Set("Authorization", "Bearer "+testAdminToken)
	evalRec3 := httptest.NewRecorder()
	router.ServeHTTP(evalRec3, evalReq3)
	if evalRec3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK after deactivation, got %d", evalRec3.Code)
	}
	var evalRes3 testDataResponse[map[string]any]
	_ = json.Unmarshal(evalRec3.Body.Bytes(), &evalRes3)
	admitted3, _ := evalRes3.Data["admitted_members"].([]any)
	excluded3, _ := evalRes3.Data["excluded_members"].([]any)
	if len(admitted3) != 2 || len(excluded3) != 0 {
		t.Fatalf("deactivated policy must restore all members: admitted=%d excluded=%d", len(admitted3), len(excluded3))
	}
}

func TestIPRiskMutationsRequireIdempotencyKey(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, policySvc, _ := setupIPRiskTestRouter(t, db)

	group, err := policySvc.CreateGroup(context.Background(), policy.CreateGroupCommand{
		Name:      "IdempotencyGroup",
		GroupType: domain.GroupTypeSelect,
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	body, _ := json.Marshal(validRiskPolicyPayload())
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/ip-risk/policies", bytes.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Idempotency-Key", "mutation-policy-create")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create policy: %d: %s", createRec.Code, createRec.Body.String())
	}

	var policyResponse testDataResponse[domain.RiskPolicyRevision]
	if err := json.Unmarshal(createRec.Body.Bytes(), &policyResponse); err != nil {
		t.Fatalf("decode policy: %v", err)
	}

	reviewReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/review", policyResponse.Data.RevisionID), nil)
	reviewReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	reviewRec := httptest.NewRecorder()
	router.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("review without Idempotency-Key: expected 422, got %d", reviewRec.Code)
	}

	bindBody, _ := json.Marshal(map[string]string{"policy_revision_id": policyResponse.Data.RevisionID})
	bindReq := httptest.NewRequest(http.MethodPut, "/api/v1/ip-risk/bindings/"+group.ID, bytes.NewReader(bindBody))
	bindReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("Idempotency-Key", "mutation-binding")
	bindRec := httptest.NewRecorder()
	router.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("bind group: %d: %s", bindRec.Code, bindRec.Body.String())
	}

	unbindReq := httptest.NewRequest(http.MethodDelete, "/api/v1/ip-risk/bindings/"+group.ID, nil)
	unbindReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	unbindRec := httptest.NewRecorder()
	router.ServeHTTP(unbindRec, unbindReq)
	if unbindRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unbind without Idempotency-Key: expected 422, got %d", unbindRec.Code)
	}
}

func TestIPRiskAPIResponseDoesNotExposeSecretReferences(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, _, _ := setupIPRiskTestRouter(t, db)

	// Insert provider setting with secret reference in DB
	_, err := db.ExecContext(context.Background(), `
		INSERT OR REPLACE INTO ip_risk_provider_settings (
			provider, schema_version, secret_reference, max_concurrency,
			requests_per_minute, daily_request_budget, per_request_timeout_ms,
			max_response_bytes, created_at, updated_at
		) VALUES ('fixture', 'v1', 'secret://fixture-sensitive-vault-ref', 1, 10, 100, 5, 1024,
			'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("insert provider settings: %v", err)
	}

	// Create policy revision
	body, _ := json.Marshal(validRiskPolicyPayload())
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/ip-risk/policies", bytes.NewReader(body))
	postReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Idempotency-Key", "sec-test-create-policy")
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusCreated {
		t.Fatalf("create policy: %d", postRec.Code)
	}

	payload := postRec.Body.String()
	for _, forbidden := range []string{
		"secret://", "vault://", "fixture-sensitive-vault-ref", "api_key", "password", "token=",
	} {
		if strings.Contains(strings.ToLower(payload), forbidden) {
			t.Fatalf("response contains forbidden credential reference %q: %s", forbidden, payload)
		}
	}
}
