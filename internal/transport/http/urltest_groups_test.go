package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
)

func TestURLTestGroupSaveReadPreviewAndExport(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)
	router, _ := setupPolicyTestRouter(t, db)
	request := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s %s: expected %d, got %d: %s", method, path, want, rec.Code, rec.Body.String())
		}
		return rec
	}
	readGroup := func(rec *httptest.ResponseRecorder) policy.GroupView {
		t.Helper()
		var response groupDetailResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Data
	}
	created := readGroup(request(http.MethodPost, "/api/v1/policies/groups", `{"name":"Auto","group_type":"urltest","urltest_interval":120,"urltest_tolerance":0,"sources":[{"operation":"add","kind":"typed_builtin","builtin_name":"DIRECT","position":0}]}`, http.StatusCreated))
	assertSettings := func(group policy.GroupView, interval, tolerance int) {
		t.Helper()
		if group.URLTestInterval == nil || *group.URLTestInterval != interval || group.URLTestTolerance == nil || *group.URLTestTolerance != tolerance {
			t.Fatalf("lost URL-Test settings: %+v", group)
		}
	}
	assertSettings(created, 120, 0)
	path := "/api/v1/policies/groups/" + created.ID
	assertSettings(readGroup(request(http.MethodGet, path, "", http.StatusOK)), 120, 0)
	// Metadata-only updates preserve explicitly configured measurements.
	assertSettings(readGroup(request(http.MethodPatch, path, `{"name":"Auto renamed"}`, http.StatusOK)), 120, 0)
	request(http.MethodPatch, path, `{"urltest_interval":-1}`, http.StatusUnprocessableEntity)
	request(http.MethodPatch, path, `{"urltest_tolerance":60001}`, http.StatusUnprocessableEntity)
	request(http.MethodPatch, path, `{"urltest_interval":1.5}`, http.StatusUnprocessableEntity)
	assertSettings(readGroup(request(http.MethodGet, path, "", http.StatusOK)), 120, 0)

	previewBody := `{"group_id":"` + created.ID + `","group_type":"urltest","urltest_interval":60,"urltest_tolerance":10,"sources":[{"operation":"add","kind":"typed_builtin","builtin_name":"DIRECT","position":0}],"target":"mihomo"}`
	preview := request(http.MethodPost, "/api/v1/policies/groups/preview", previewBody, http.StatusOK)
	var previewResponse struct {
		Data policy.GroupPreviewResponse `json:"data"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &previewResponse); err != nil {
		t.Fatal(err)
	}
	if previewResponse.Data.ContentDigest == nil || *previewResponse.Data.ContentDigest == "" {
		t.Fatalf("missing draft compile digest: %s", preview.Body.String())
	}
	secondPreview := request(http.MethodPost, "/api/v1/policies/groups/preview", strings.Replace(previewBody, `"urltest_interval":60`, `"urltest_interval":0`, 1), http.StatusOK)
	var secondPreviewResponse struct {
		Data policy.GroupPreviewResponse `json:"data"`
	}
	if err := json.Unmarshal(secondPreview.Body.Bytes(), &secondPreviewResponse); err != nil {
		t.Fatal(err)
	}
	if secondPreviewResponse.Data.ContentDigest == nil || *secondPreviewResponse.Data.ContentDigest == *previewResponse.Data.ContentDigest {
		t.Fatal("draft measurement change did not reach the compiler")
	}
	request(http.MethodPost, "/api/v1/policies/groups/preview", strings.Replace(previewBody, `"urltest_interval":60`, `"urltest_interval":-1`, 1), http.StatusUnprocessableEntity)
	assertSettings(readGroup(request(http.MethodGet, path, "", http.StatusOK)), 120, 0)

	repo := sqlite.NewPolicyRepository(db)
	compileSaved := func(interval, tolerance int) *resolver.ResolvedPolicySnapshot {
		t.Helper()
		groups, err := repo.ListGroups(ctx)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := resolver.New().Resolve(ctx, resolver.ResolveInput{Groups: groups})
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Groups) != 1 || snapshot.Groups[0].URLTestInterval == nil || *snapshot.Groups[0].URLTestInterval != interval || snapshot.Groups[0].URLTestTolerance == nil || *snapshot.Groups[0].URLTestTolerance != tolerance {
			t.Fatalf("resolver lost settings: %+v", snapshot.Groups)
		}
		result, err := compiler.CompileMihomo(ctx, snapshot)
		if err != nil {
			t.Fatal(err)
		}
		var yamlResult struct {
			Groups []struct {
				Interval  *int   `yaml:"interval"`
				Tolerance *int   `yaml:"tolerance"`
				URL       string `yaml:"url"`
			} `yaml:"proxy-groups"`
		}
		if err := yaml.Unmarshal(result.Content, &yamlResult); err != nil {
			t.Fatal(err)
		}
		if len(yamlResult.Groups) != 1 || yamlResult.Groups[0].Interval == nil || *yamlResult.Groups[0].Interval != interval || yamlResult.Groups[0].Tolerance == nil || *yamlResult.Groups[0].Tolerance != tolerance || yamlResult.Groups[0].URL != "https://www.gstatic.com/generate_204" {
			t.Fatalf("incorrect YAML export: %s", result.Content)
		}
		return snapshot
	}
	before := compileSaved(120, 0)
	assertSettings(readGroup(request(http.MethodPatch, path, `{"urltest_interval":60,"urltest_tolerance":10}`, http.StatusOK)), 60, 10)
	after := compileSaved(60, 10)
	if before.InputDigest == after.InputDigest || before.SnapshotDigest == after.SnapshotDigest {
		t.Fatal("measurement changes did not invalidate resolver digests")
	}
	assertSettings(readGroup(request(http.MethodPatch, path, `{"urltest_interval":0,"urltest_tolerance":0}`, http.StatusOK)), 0, 0)
	compileSaved(0, 0)
	// Changing type clears irrelevant URL-Test settings rather than blocking the editor.
	changed := readGroup(request(http.MethodPatch, path, `{"group_type":"select"}`, http.StatusOK))
	if changed.URLTestInterval != nil || changed.URLTestTolerance != nil {
		t.Fatalf("non-urltest type retained measurement settings: %+v", changed)
	}
	request(http.MethodPost, "/api/v1/policies/groups", `{"name":"manual","group_type":"select","urltest_interval":60}`, http.StatusUnprocessableEntity)
}

func TestURLTestTypeChangeEditorHTTPPayload(t *testing.T) {
	for _, target := range []domain.GroupType{domain.GroupTypeSelect, domain.GroupTypeFallback, domain.GroupTypeLoadBalance} {
		t.Run(string(target), func(t *testing.T) {
			db := newCleanSQLiteDB(t)
			router, _ := setupPolicyTestRouter(t, db)
			request := func(method, path, body string, want int) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+testAdminToken)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code != want {
					t.Fatalf("%s %s returned %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
				}
				return rec
			}
			created := request(http.MethodPost, "/api/v1/policies/groups", `{"name":"Auto","group_type":"urltest","urltest_interval":120,"urltest_tolerance":20,"sources":[{"operation":"add","kind":"typed_builtin","builtin_name":"DIRECT","position":0}]}`, http.StatusCreated)
			var response groupDetailResponse
			if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			original := response.Data
			path := "/api/v1/policies/groups/" + original.ID
			request(http.MethodPut, path, fmt.Sprintf(`{"group_type":%q,"urltest_tolerance":0}`, target), http.StatusUnprocessableEntity)
			readback := request(http.MethodGet, path, "", http.StatusOK)
			if err := json.Unmarshal(readback.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Data.GroupType != domain.GroupTypeURLTest || response.Data.Revision != original.Revision || response.Data.URLTestInterval == nil || *response.Data.URLTestInterval != 120 || response.Data.URLTestTolerance == nil || *response.Data.URLTestTolerance != 20 {
				t.Fatalf("explicit invalid input changed saved group: %+v", response.Data)
			}
			// Use the real UI save shape: type, sources and revision, no URL-Test fields.
			body, err := json.Marshal(struct {
				GroupType        domain.GroupType      `json:"group_type"`
				ExpectedRevision int64                 `json:"expected_revision"`
				Sources          []domain.MemberSource `json:"sources"`
			}{target, original.Revision, original.Sources})
			if err != nil {
				t.Fatal(err)
			}
			request(http.MethodPut, path, string(body), http.StatusOK)
			readback = request(http.MethodGet, path, "", http.StatusOK)
			// Decode afresh so omitted pointer fields cannot reuse the prior JSON values.
			var changed groupDetailResponse
			if err := json.Unmarshal(readback.Body.Bytes(), &changed); err != nil {
				t.Fatal(err)
			}
			if changed.Data.GroupType != target || changed.Data.URLTestInterval != nil || changed.Data.URLTestTolerance != nil || changed.Data.Revision != original.Revision+1 || len(changed.Data.Sources) != 1 || changed.Data.Sources[0].ID != original.Sources[0].ID {
				t.Fatalf("type change did not persist cleanly: %+v", changed.Data)
			}
		})
	}
}

func TestURLTestUnsetDefaultsAndOtherGroupExports(t *testing.T) {
	for _, groupType := range []domain.GroupType{domain.GroupTypeURLTest, domain.GroupTypeSelect, domain.GroupTypeFallback, domain.GroupTypeLoadBalance} {
		t.Run(string(groupType), func(t *testing.T) {
			snapshot := &resolver.ResolvedPolicySnapshot{Groups: []resolver.ResolvedGroup{{ID: "auto", Name: "Auto", GroupType: groupType, Members: []resolver.ResolvedGroupMember{{Kind: resolver.MemberKindBuiltin, TargetID: "DIRECT", DisplayName: "DIRECT"}}}}}
			result, err := compiler.CompileMihomo(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			out := string(result.Content)
			if groupType == domain.GroupTypeURLTest {
				if !strings.Contains(out, "interval: 300") || !strings.Contains(out, "tolerance: 50") {
					t.Fatalf("defaults changed: %s", out)
				}
			} else if strings.Contains(out, "tolerance:") {
				t.Fatalf("tolerance leaked to %s: %s", groupType, out)
			}
		})
	}
}
