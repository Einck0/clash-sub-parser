package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"

	"gopkg.in/yaml.v3"
)

// Exercise API writes/readback, real SQLite facts, Resolver and the actual /clash
// handler in one isolated fixture. No production data or external peer is used.
func TestDirectSubscription_RuleEnabledLifecycle(t *testing.T) {
	exportRouter, db, _ := setupDirectSubRouter(t, "", false, false)
	adminRouter, _ := setupPolicyTestRouter(t, db)
	ctx := context.Background()
	request := func(method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		adminRouter.ServeHTTP(rec, req)
		if rec.Code != status {
			t.Fatalf("%s %s: want %d, got %d: %s", method, path, status, rec.Code, rec.Body.String())
		}
		return rec
	}
	create := func(body string) policy.PolicyRuleView {
		t.Helper()
		rec := request(http.MethodPost, "/api/v1/policies/rules", body, http.StatusCreated)
		var response struct {
			Data policy.PolicyRuleView `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Data
	}
	ad := create(`{"expression":"DOMAIN-SUFFIX,local.adguard.org","builtin_target":"REJECT","category":"ads","position":0}`)
	if !ad.Enabled {
		t.Fatal("omitted enabled must still default to true")
	}
	create(`{"expression":"DOMAIN,kept.example.org","builtin_target":"DIRECT","category":"custom","position":1}`)
	disabledTerminal := create(`{"expression":"FINAL","builtin_target":"REJECT","category":"disabled","position":0,"enabled":false}`)
	if disabledTerminal.Enabled {
		t.Fatal("explicit disabled create was not saved")
	}
	dnsRepo := sqlite.NewDNSSettingsRepository(db)
	dns, err := dnsRepo.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dns.RawYAML = "enable: true\nnameserver:\n  - 1.1.1.1\nenhanced-mode: fake-ip\nfake-ip-filter:\n  - local.adguard.org\n  - kept.example.org\n"
	dns.Enabled = true
	if err := dnsRepo.Update(ctx, dns, dns.Revision); err != nil {
		t.Fatal(err)
	}

	var onETag, offETag string
	for step, enabled := range []bool{true, false, true} {
		request(http.MethodPut, "/api/v1/policies/rules/"+ad.ID, fmt.Sprintf(`{"enabled":%t}`, enabled), http.StatusOK)
		rec := request(http.MethodGet, "/api/v1/policy/rules", "", http.StatusOK)
		var listed ruleListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
			t.Fatal(err)
		}
		if len(listed.Data.PolicyRules) != 4 {
			t.Fatalf("toggle changed stored rule count: %+v", listed.Data)
		}
		found := false
		for _, rule := range listed.Data.PolicyRules {
			if rule.ID == ad.ID {
				want := ad
				want.Enabled = enabled
				if !reflect.DeepEqual(rule, want) {
					t.Fatalf("API lost rule state/identity: want %+v, got %+v", want, rule)
				}
				found = true
			}
		}
		if !found {
			t.Fatal("disabled record disappeared from API")
		}
		stored, err := sqlite.NewPolicyRepository(db).GetPolicyRule(ctx, ad.ID)
		if err != nil || stored == nil || stored.Enabled != enabled || stored.Expression != ad.Expression || stored.BuiltinTarget != ad.BuiltinTarget || stored.Position != ad.Position {
			t.Fatalf("repository readback mismatch: %+v, %v", stored, err)
		}
		facts, err := sqlite.NewConsistentFactsLoader(db).LoadConsistentFacts(ctx, "", domain.TargetMihomo, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if len(facts.PolicyRules) != 4 {
			t.Fatalf("facts must retain disabled records: %+v", facts.PolicyRules)
		}
		found = false
		for _, rule := range facts.PolicyRules {
			if rule.ID == ad.ID {
				found = true
				if rule.Enabled != enabled || rule.Expression != ad.Expression || rule.BuiltinTarget != ad.BuiltinTarget || rule.Position != ad.Position {
					t.Fatalf("consistent facts lost saved state: %+v", rule)
				}
			}
		}
		if !found {
			t.Fatal("disabled record disappeared from facts")
		}

		exportReq := httptest.NewRequest(http.MethodGet, "/clash", nil)
		if step == 1 {
			exportReq.Header.Set("If-None-Match", onETag)
		}
		if step == 2 {
			exportReq.Header.Set("If-None-Match", offETag)
		}
		export := httptest.NewRecorder()
		exportRouter.ServeHTTP(export, exportReq)
		if export.Code != http.StatusOK {
			t.Fatalf("export step %d: %d %s", step, export.Code, export.Body.String())
		}
		var config struct {
			Rules []string `yaml:"rules"`
			DNS   struct {
				FakeIPFilter []string `yaml:"fake-ip-filter"`
			} `yaml:"dns"`
		}
		if err := yaml.Unmarshal(export.Body.Bytes(), &config); err != nil {
			t.Fatal(err)
		}
		wantRules := []string{"DOMAIN,kept.example.org,DIRECT", "MATCH,Proxy"}
		if enabled {
			wantRules = append([]string{"DOMAIN-SUFFIX,local.adguard.org,REJECT"}, wantRules...)
		}
		if !reflect.DeepEqual(config.Rules, wantRules) {
			t.Fatalf("enabled=%v: rules want %v, got %v", enabled, wantRules, config.Rules)
		}
		if !reflect.DeepEqual(config.DNS.FakeIPFilter, []string{"local.adguard.org", "kept.example.org"}) {
			t.Fatalf("unrelated DNS config changed: %+v", config.DNS)
		}
		etag := export.Header().Get("ETag")
		if etag == "" {
			t.Fatal("missing content ETag")
		}
		switch step {
		case 0:
			onETag = etag
		case 1:
			offETag = etag
			if offETag == onETag {
				t.Fatal("content ETag ignored disabled rule")
			}
		case 2:
			if etag != onETag {
				t.Fatal("re-enable did not restore content ETag")
			}
		}
	}
}
