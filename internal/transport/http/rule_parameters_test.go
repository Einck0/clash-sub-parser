package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/policy"
)

func TestPolicyRuleParameterErrorsReachHTTPWithoutWriting(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, svc := setupPolicyTestRouter(t, db)
	active, err := svc.EnsureActiveRevision(context.Background())
	if err != nil || active == nil {
		t.Fatalf("active revision setup failed: %v", err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	assertRejected := func(rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
		var response testErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Code != "unsupported_rule_parameter" || !strings.Contains(response.Message, "remove the option") {
			t.Fatalf("missing actionable parameter diagnostic: %+v", response)
		}
	}

	assertRejected(request(http.MethodPost, "/api/v1/policies/rules", fmt.Sprintf(`{"revision_id":%q,"builtin_target":"DIRECT","expression":"DOMAIN-SUFFIX,example.org,no-resolve"}`, active.ID)))
	list, err := svc.ListRules(context.Background(), policy.ListRulesQuery{RevisionID: active.ID})
	if err != nil || len(list.PolicyRules) != 0 {
		t.Fatalf("rejected create changed rules: %+v, %v", list, err)
	}

	created := request(http.MethodPost, "/api/v1/policies/rules", fmt.Sprintf(`{"revision_id":%q,"builtin_target":"DIRECT","expression":"IP-CIDR,192.0.2.0/24,no-resolve"}`, active.ID))
	if created.Code != http.StatusCreated {
		t.Fatalf("legal IP no-resolve rejected: %d: %s", created.Code, created.Body.String())
	}
	var response struct {
		Data policy.PolicyRuleView `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	assertRejected(request(http.MethodPut, "/api/v1/policies/rules/"+response.Data.ID, `{"expression":"DOMAIN-SUFFIX,example.org,no-resolve"}`))
	list, err = svc.ListRules(context.Background(), policy.ListRulesQuery{RevisionID: active.ID})
	if err != nil || len(list.PolicyRules) != 1 || list.PolicyRules[0].Expression != "IP-CIDR,192.0.2.0/24,no-resolve" {
		t.Fatalf("rejected update changed saved rule: %+v, %v", list, err)
	}
}
