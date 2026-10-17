package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
)

func TestRulesDesk_CategoriesAndRulesAPI(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPolicyTestRouter(t, db)
	ctx := context.Background()

	// Create active revision and a test group
	revID := domain.MustNewUUIDv7()
	_, err := db.ExecContext(ctx, `INSERT INTO configuration_revisions (id, state, content_digest, created_at) VALUES (?, 'active', 'test-digest', CURRENT_TIMESTAMP);`, revID)
	if err != nil {
		t.Fatalf("failed to insert active revision: %v", err)
	}

	groupID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `INSERT INTO node_groups (id, name, group_type, position, empty_fallback_pass, created_at, updated_at) VALUES (?, 'ProxyGroup', 'select', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);`, groupID)
	if err != nil {
		t.Fatalf("failed to insert test group: %v", err)
	}

	// 1. GET /api/v1/policies/categories (verify 14 preset categories)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/policies/categories", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /categories status %d: %s", rec.Code, rec.Body.String())
	}

	var catResp struct {
		Data []policy.RuleCategoryView `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catResp); err != nil {
		t.Fatalf("unmarshal categories error: %v", err)
	}
	if len(catResp.Data) != 14 {
		t.Fatalf("expected 14 preset categories, got %d", len(catResp.Data))
	}

	// 2. POST /api/v1/policies/categories
	createCatBody := []byte(`{"name":"MyCustomCat","sort_order":150}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policies/categories", bytes.NewReader(createCatBody))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /categories status %d: %s", rec.Code, rec.Body.String())
	}
	var createdCat struct {
		Data policy.RuleCategoryView `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createdCat)
	if createdCat.Data.Name != "MyCustomCat" {
		t.Fatalf("expected category name MyCustomCat, got %s", createdCat.Data.Name)
	}

	// 3. PUT /api/v1/policies/categories/{id}
	updateCatBody := []byte(`{"name":"MyCustomCatRenamed","sort_order":155}`)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/policies/categories/%s", createdCat.Data.ID), bytes.NewReader(updateCatBody))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /categories/{id} status %d: %s", rec.Code, rec.Body.String())
	}

	// 4. PUT /api/v1/policies/categories/reorder
	reorderBody := []byte(`{"order":["cat-01","cat-02","cat-03"]}`)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/policies/categories/reorder", bytes.NewReader(reorderBody))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /categories/reorder status %d: %s", rec.Code, rec.Body.String())
	}

	// 5. POST /api/v1/policies/rules with DIRECT builtin target
	ruleDirectBody := []byte(`{
		"category_id": "cat-01",
		"category": "广告/拦截",
		"name": "BlockAd",
		"builtin_target": "REJECT",
		"expression": "DOMAIN-SUFFIX,ad.example.com",
		"position": 0
	}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", bytes.NewReader(ruleDirectBody))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /rules with REJECT status %d: %s", rec.Code, rec.Body.String())
	}
	var createdRule1 struct {
		Data policy.PolicyRuleView `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createdRule1)
	if createdRule1.Data.BuiltinTarget != "REJECT" || createdRule1.Data.TargetGroupID != "" {
		t.Fatalf("unexpected rule view: %+v", createdRule1.Data)
	}

	// 6. POST /api/v1/policies/rules with Target Group
	ruleGroupBody := []byte(fmt.Sprintf(`{
		"category_id": "cat-13",
		"category": "常规代理",
		"name": "ProxyRule",
		"target_group_id": "%s",
		"expression": "DOMAIN-SUFFIX,google.com",
		"position": 1
	}`, groupID))
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policies/rules", bytes.NewReader(ruleGroupBody))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /rules with target_group status %d: %s", rec.Code, rec.Body.String())
	}
	var createdRule2 struct {
		Data policy.PolicyRuleView `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createdRule2)
	if createdRule2.Data.TargetGroupID != groupID || createdRule2.Data.BuiltinTarget != "" {
		t.Fatalf("unexpected rule view: %+v", createdRule2.Data)
	}

	// 7. GET /api/v1/policies/rules?category_id=cat-01
	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/rules?category_id=cat-01", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rules?category_id status %d: %s", rec.Code, rec.Body.String())
	}
	var rulesFilterResp ruleListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &rulesFilterResp)
	if len(rulesFilterResp.Data.PolicyRules) != 1 {
		t.Fatalf("expected 1 rule in cat-01, got %d", len(rulesFilterResp.Data.PolicyRules))
	}

	// 8. PUT /api/v1/policies/rules/{id} (toggle enabled)
	updateRuleBody := []byte(`{"enabled":false,"name":"BlockAdUpdated"}`)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/policies/rules/%s", createdRule1.Data.ID), bytes.NewReader(updateRuleBody))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /rules/{id} status %d: %s", rec.Code, rec.Body.String())
	}
	var updatedRule struct {
		Data policy.PolicyRuleView `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &updatedRule)
	if updatedRule.Data.Enabled != false || updatedRule.Data.Name != "BlockAdUpdated" {
		t.Fatalf("unexpected updated rule: %+v", updatedRule.Data)
	}

	// 9. Backfill all 470 rules
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/policies/rules/backfill?revision_id=%s", revID), nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /rules/backfill status %d: %s", rec.Code, rec.Body.String())
	}
	var bfResp struct {
		Data struct {
			Count  int    `json:"count"`
			Status string `json:"status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &bfResp)
	if bfResp.Data.Count != 470 {
		t.Fatalf("expected 470 backfilled rules, got %d", bfResp.Data.Count)
	}

	// Verify all 470 rules listed
	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/rules", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &rulesFilterResp)
	if len(rulesFilterResp.Data.PolicyRules) != 470 {
		t.Fatalf("expected 470 rules listed, got %d", len(rulesFilterResp.Data.PolicyRules))
	}

	// Verify validate passes without target group errors on builtin actions
	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/validate", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /validate status %d: %s", rec.Code, rec.Body.String())
	}

	// 10. Verify pagination limits for rules (max 1000) and groups (max 500)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/rules?kind=policy&page_size=1000", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rules?page_size=1000 status %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/rules?kind=policy&page_size=1001", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("GET /rules?page_size=1001 status %d, expected 422", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups?page_size=200", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /groups?page_size=200 status %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/policies/groups?page_size=501", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("GET /groups?page_size=501 status %d, expected 422", rec.Code)
	}
}
