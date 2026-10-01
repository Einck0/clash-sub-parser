package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

// TestAPISemantic_IPRiskModules tests the complete semantic lifecycle of the IP Risk module:
// Policy CRUD, review, activation, deactivation, bindings, group binding aliases, and member evaluations.
// Note: Uses a deterministic mock safe provider ('fixture') for testing without external public internet dependence.
func TestAPISemantic_IPRiskModules(t *testing.T) {
	harness := setupTestHarness(t)
	ctx := context.Background()

	// 0. Enable Admin Auth
	harness.TokenHolder.SetAuthSwitches(true, true)

	// Seed test fixture provider settings in DB
	nowStr := domain.NowUTC().Format(time.RFC3339)
	_, err := harness.DB.ExecContext(ctx, `
		INSERT OR IGNORE INTO ip_risk_provider_settings (
			provider, schema_version, secret_reference, max_concurrency,
			requests_per_minute, daily_request_budget, per_request_timeout_ms,
			max_response_bytes, created_at, updated_at
		) VALUES ('fixture', 'v1', 'secret://fixture', 1, 10, 100, 5, 1024, ?, ?);
	`, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert provider settings: %v", err)
	}

	// 1. POST /api/v1/ip-risk/policies: Create Draft Risk Policy Revision (requires Idempotency-Key)
	riskPayload := map[string]any{
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

	headers := map[string]string{"Idempotency-Key": "e2e-risk-create-pol-01"}
	resp, err := harness.AuthRequest(http.MethodPost, "/api/v1/ip-risk/policies", riskPayload, headers)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/ip-risk/policies failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	var createdPol struct {
		Data struct {
			RevisionID string `json:"revision_id"`
		} `json:"data"`
	}
	if err := resp.JSON(&createdPol); err != nil || createdPol.Data.RevisionID == "" {
		t.Fatalf("unmarshal created risk policy failed: %v", err)
	}
	polID := createdPol.Data.RevisionID

	// DB verification: policy exists in `risk_policy_revisions`
	var dbPolCount int
	_ = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM risk_policy_revisions WHERE id = ?", polID).Scan(&dbPolCount)
	if dbPolCount != 1 {
		t.Fatalf("risk policy row not inserted in DB")
	}

	// 2. GET /api/v1/ip-risk/policies: List Policies
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/ip-risk/policies", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/ip-risk/policies failed: %d", resp.StatusCode)
	}

	// 3. GET /api/v1/ip-risk/policies/{id}: Get Specific Policy
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/ip-risk/policies/"+polID, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/ip-risk/policies/%s failed: %d", polID, resp.StatusCode)
	}

	// 4. POST /api/v1/ip-risk/policies/{id}/review: Review Policy
	revHeaders := map[string]string{"Idempotency-Key": "e2e-risk-rev-pol-01"}
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/review", polID), nil, revHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/ip-risk/policies/%s/review failed: %d", polID, resp.StatusCode)
	}

	// 5. POST /api/v1/ip-risk/policies/{id}/activate: Activate Policy
	actHeaders := map[string]string{"Idempotency-Key": "e2e-risk-act-pol-01"}
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/activate", polID), nil, actHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/ip-risk/policies/%s/activate failed: %d", polID, resp.StatusCode)
	}

	// 6. GET /api/v1/ip-risk/policies/active: Get Active Policy
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/ip-risk/policies/active", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/ip-risk/policies/active failed: %d", resp.StatusCode)
	}

	// Create a test policy group for binding testing
	testGroupID := domain.MustNewUUIDv7()
	_, err = harness.DB.ExecContext(ctx, `
		INSERT INTO node_groups (id, name, group_type, created_at, updated_at)
		VALUES (?, 'Risk Test Group', 'select', ?, ?);
	`, testGroupID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert test group: %v", err)
	}

	// 7. POST /api/v1/ip-risk/bindings: Bind Group to Policy
	bindPayload := map[string]any{
		"group_id":           testGroupID,
		"policy_revision_id": polID,
	}
	bindHeaders := map[string]string{"Idempotency-Key": "e2e-risk-bind-01"}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/ip-risk/bindings", bindPayload, bindHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/ip-risk/bindings failed: %d body=%s", resp.StatusCode, resp.Body)
	}

	// 8. GET /api/v1/ip-risk/bindings: List Bindings
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/ip-risk/bindings", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/ip-risk/bindings failed: %d", resp.StatusCode)
	}

	// 9. GET /api/v1/ip-risk/bindings/{group_id}: Get Group Binding
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/ip-risk/bindings/"+testGroupID, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/ip-risk/bindings/%s failed: %d", testGroupID, resp.StatusCode)
	}

	// 10. PUT /api/v1/ip-risk/bindings/{group_id}: Update Group Binding
	putHeaders := map[string]string{"Idempotency-Key": "e2e-risk-bind-put-01"}
	resp, err = harness.AuthRequest(http.MethodPut, "/api/v1/ip-risk/bindings/"+testGroupID, map[string]any{
		"policy_revision_id": polID,
	}, putHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/ip-risk/bindings/%s failed: %d", testGroupID, resp.StatusCode)
	}

	// 11. GET /api/v1/ip-risk/groups/{group_id}/binding: Alias GET
	resp, err = harness.AuthRequest(http.MethodGet, fmt.Sprintf("/api/v1/ip-risk/groups/%s/binding", testGroupID), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/ip-risk/groups/%s/binding failed: %d", testGroupID, resp.StatusCode)
	}

	// 12. PUT /api/v1/ip-risk/groups/{group_id}/binding: Alias PUT
	aliasPutHeaders := map[string]string{"Idempotency-Key": "e2e-risk-alias-put-01"}
	resp, err = harness.AuthRequest(http.MethodPut, fmt.Sprintf("/api/v1/ip-risk/groups/%s/binding", testGroupID), map[string]any{
		"policy_revision_id": polID,
	}, aliasPutHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/ip-risk/groups/%s/binding failed: %d", testGroupID, resp.StatusCode)
	}

	// 13. POST /api/v1/ip-risk/groups/{group_id}/evaluate: Evaluate Group Members
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/groups/%s/evaluate", testGroupID), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/ip-risk/groups/%s/evaluate failed: %d body=%s", testGroupID, resp.StatusCode, resp.Body)
	}

	// 14. GET /api/v1/ip-risk/groups/{group_id}/members: Evaluate Group Members GET Alias
	resp, err = harness.AuthRequest(http.MethodGet, fmt.Sprintf("/api/v1/ip-risk/groups/%s/members", testGroupID), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/ip-risk/groups/%s/members failed: %d body=%s", testGroupID, resp.StatusCode, resp.Body)
	}

	// 15. DELETE /api/v1/ip-risk/groups/{group_id}/binding: Alias Unbind
	aliasUnbindHeaders := map[string]string{"Idempotency-Key": "e2e-risk-alias-unbind-01"}
	resp, err = harness.AuthRequest(http.MethodDelete, fmt.Sprintf("/api/v1/ip-risk/groups/%s/binding", testGroupID), nil, aliasUnbindHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/v1/ip-risk/groups/%s/binding failed: %d", testGroupID, resp.StatusCode)
	}

	// Re-bind to test main DELETE endpoint
	rebindHeaders := map[string]string{"Idempotency-Key": "e2e-risk-rebind-01"}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/ip-risk/bindings", bindPayload, rebindHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("rebind failed: %d", resp.StatusCode)
	}

	// 16. DELETE /api/v1/ip-risk/bindings/{group_id}: Main Unbind
	unbindHeaders := map[string]string{"Idempotency-Key": "e2e-risk-unbind-main-01"}
	resp, err = harness.AuthRequest(http.MethodDelete, "/api/v1/ip-risk/bindings/"+testGroupID, nil, unbindHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/v1/ip-risk/bindings/%s failed: %d", testGroupID, resp.StatusCode)
	}

	// 17. POST /api/v1/ip-risk/policies/{id}/deactivate: Deactivate Risk Policy
	deactHeaders := map[string]string{"Idempotency-Key": "e2e-risk-deact-pol-01"}
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/deactivate", polID), nil, deactHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/ip-risk/policies/%s/deactivate failed: %d body=%s", polID, resp.StatusCode, resp.Body)
	}

	// Verify in DB: active = 0
	var dbActive int
	_ = harness.DB.QueryRowContext(ctx, "SELECT active FROM risk_policy_revisions WHERE id = ?", polID).Scan(&dbActive)
	if dbActive != 0 {
		t.Fatalf("expected policy active=0 after deactivation, got %d", dbActive)
	}
}
