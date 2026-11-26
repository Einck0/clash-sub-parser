package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/policy"
	"gopkg.in/yaml.v3"
)

func TestDirectSubscriptionURLTestConfiguredMeasurements(t *testing.T) {
	exportRouter, db, _ := setupDirectSubRouter(t, "", false, false)
	adminRouter, _ := setupPolicyTestRouter(t, db)
	create := httptest.NewRequest(http.MethodPost, "/api/v1/policies/groups", strings.NewReader(`{"name":"Auto","group_type":"urltest","urltest_interval":120,"urltest_tolerance":0,"sources":[{"operation":"add","kind":"typed_builtin","builtin_name":"DIRECT","position":0}]}`))
	create.Header.Set("Authorization", "Bearer "+testAdminToken)
	create.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, create)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create URL-Test group: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data policy.GroupView `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.URLTestInterval == nil || *created.Data.URLTestInterval != 120 || created.Data.URLTestTolerance == nil || *created.Data.URLTestTolerance != 0 {
		t.Fatalf("create lost settings: %+v", created.Data)
	}
	for _, expected := range []struct {
		body                string
		interval, tolerance int
	}{{"", 120, 0}, {`{"urltest_interval":60,"urltest_tolerance":20}`, 60, 20}, {`{"urltest_interval":0,"urltest_tolerance":0}`, 0, 0}} {
		if expected.body != "" {
			update := httptest.NewRequest(http.MethodPatch, "/api/v1/policies/groups/"+created.Data.ID, strings.NewReader(expected.body))
			update.Header.Set("Authorization", "Bearer "+testAdminToken)
			update.Header.Set("Content-Type", "application/json")
			rec = httptest.NewRecorder()
			adminRouter.ServeHTTP(rec, update)
			if rec.Code != http.StatusOK {
				t.Fatalf("update URL-Test group: %d %s", rec.Code, rec.Body.String())
			}
		}
		rec = httptest.NewRecorder()
		exportRouter.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/clash", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("direct export: %d %s", rec.Code, rec.Body.String())
		}
		var config struct {
			Groups []struct {
				Name      string `yaml:"name"`
				Type      string `yaml:"type"`
				Interval  *int   `yaml:"interval"`
				Tolerance *int   `yaml:"tolerance"`
			} `yaml:"proxy-groups"`
		}
		if err := yaml.Unmarshal(rec.Body.Bytes(), &config); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, group := range config.Groups {
			if group.Name == "Auto" {
				found = true
				if group.Type != "url-test" || group.Interval == nil || *group.Interval != expected.interval || group.Tolerance == nil || *group.Tolerance != expected.tolerance {
					t.Fatalf("publication export lost settings: %+v", group)
				}
			}
		}
		if !found {
			t.Fatalf("publication omitted Auto: %s", rec.Body.String())
		}
	}
}
