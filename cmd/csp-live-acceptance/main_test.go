package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRun_BaselineSuccessOpenMode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"mode":"open","admin_mode":"disabled","export_mode":"disabled","authenticated":false}}`))
		case "/api/v1/settings/auth":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"admin_auth_enabled":false,"export_auth_enabled":false,"token_configured":false,"admin_mode":"disabled","export_mode":"disabled"}}`))
		case "/api/v1/subscriptions":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"items":[{"id":"sub-1","name":"Sub 1","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/nodes":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"items":[{"logical_id":"node-1","display_name":"Node 1","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-1","node_logical_id":"node-1"}]}],"total":1,"page":1}}`))
		case "/api/v1/policies/rules":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeSuccess {
		t.Fatalf("expected ExitCodeSuccess (0), got %d; stderr: %s", code, stderr.String())
	}

	reportData, err := os.ReadFile(outReport)
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}

	var report AcceptanceReport
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatalf("parse report JSON: %v", err)
	}

	if report.Verdict != "PASS" {
		t.Errorf("expected Verdict PASS, got %q", report.Verdict)
	}
	if report.ProtectedStatus != "VERIFIED" {
		t.Errorf("expected ProtectedStatus VERIFIED, got %q", report.ProtectedStatus)
	}
	if report.NodesAudit == nil || report.NodesAudit.Active != 1 {
		t.Errorf("expected 1 active node in report, got %+v", report.NodesAudit)
	}
}

func TestRun_ProtectedModeNoToken_Blocked(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"protected","admin_mode":"token","export_mode":"token","authenticated":false}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-output", outReport, "-token-env", "NON_EXISTENT_ENV_TOKEN"}, &stdout, &stderr)
	if code != ExitCodeBlocked {
		t.Fatalf("expected ExitCodeBlocked (2), got %d; stdout: %s", code, stdout.String())
	}

	reportData, err := os.ReadFile(outReport)
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}

	var report AcceptanceReport
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatalf("parse report JSON: %v", err)
	}

	if report.Verdict != "BLOCKED" {
		t.Errorf("expected Verdict BLOCKED, got %q", report.Verdict)
	}
	if report.ProtectedStatus != "PAUSED_CREDENTIALS_REQUIRED" {
		t.Errorf("expected ProtectedStatus PAUSED_CREDENTIALS_REQUIRED, got %q", report.ProtectedStatus)
	}
	if len(report.ExternalActionable) == 0 {
		t.Errorf("expected external actionable options in blocked report")
	}
}

func TestRun_ProtectedModeValidToken_Success(t *testing.T) {
	validToken := "valid-secret-admin-token-12345"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		isAuthenticated := authHeader == "Bearer "+validToken

		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(fmt.Sprintf(`{"data":{"mode":"protected","admin_mode":"token","export_mode":"token","authenticated":%t}}`, isAuthenticated)))
		case "/api/v1/settings/auth":
			if !isAuthenticated {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":"unauthorized","message":"missing or invalid token"}`))
				return
			}
			w.Write([]byte(`{"data":{"admin_auth_enabled":true,"export_auth_enabled":true,"token_configured":true,"admin_mode":"token","export_mode":"token"}}`))
		case "/api/v1/subscriptions":
			if !isAuthenticated {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"data":{"items":[{"id":"sub-p1","name":"Sub P1","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/nodes":
			if !isAuthenticated {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"data":{"items":[{"logical_id":"node-p1","display_name":"Node P1","active":true,"connection_revision":2,"sources":[{"subscription_id":"sub-p1","node_logical_id":"node-p1"}]}],"total":1,"page":1}}`))
		case "/api/v1/policies/rules":
			if !isAuthenticated {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			if !isAuthenticated {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			if !isAuthenticated {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")
	tokenFile := filepath.Join(outDir, "token.secret")
	if err := os.WriteFile(tokenFile, []byte(validToken), 0600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-token-file", tokenFile, "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeSuccess {
		t.Fatalf("expected ExitCodeSuccess (0), got %d; stderr: %s", code, stderr.String())
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if report.Verdict != "PASS" {
		t.Errorf("expected Verdict PASS, got %q", report.Verdict)
	}
	if !report.Authenticated {
		t.Errorf("expected Authenticated=true in report")
	}
}

func TestRun_PublicBaselineFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"data":{"status":"degraded"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":false,"required_tables":10,"schema_version":10}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open","authenticated":false}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1), got %d", code)
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if report.Verdict != "FAIL" {
		t.Errorf("expected Verdict FAIL, got %q", report.Verdict)
	}
	if len(report.Errors) == 0 {
		t.Errorf("expected errors in report on baseline failure")
	}
}

func TestRun_InvalidJSONOrSchemaMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			// Returning corrupted JSON
			w.Write([]byte(`{"data": { corrupted-json...`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) on bad JSON, got %d", code)
	}
}

func TestRun_MultiPageNodesAndFirstPageNoActive(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-all","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/nodes":
			page := r.URL.Query().Get("page")
			if page == "1" || page == "" {
				// Page 1: 100 inactive nodes
				items := make([]map[string]any, 0, 100)
				for i := 1; i <= 100; i++ {
					items = append(items, map[string]any{
						"logical_id": fmt.Sprintf("inact-node-%d", i),
						"active":     false,
						"sources":    []map[string]any{{"subscription_id": "sub-all"}},
					})
				}
				data := map[string]any{"data": map[string]any{"items": items, "total": 120, "page": 1}}
				_ = json.NewEncoder(w).Encode(data)
				return
			}
			// Page 2: 20 active nodes
			items := make([]map[string]any, 0, 20)
			for i := 1; i <= 20; i++ {
				items = append(items, map[string]any{
					"logical_id": fmt.Sprintf("act-node-%d", i),
					"active":     true,
					"sources":    []map[string]any{{"subscription_id": "sub-all"}},
				})
			}
			data := map[string]any{"data": map[string]any{"items": items, "total": 120, "page": 2}}
			_ = json.NewEncoder(w).Encode(data)
			return
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeSuccess {
		t.Fatalf("expected ExitCodeSuccess (0), got %d; stderr: %s", code, stderr.String())
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)

	if report.NodesAudit == nil {
		t.Fatalf("expected NodesAudit in report")
	}
	if report.NodesAudit.Total != 120 {
		t.Errorf("expected Total=120, got %d", report.NodesAudit.Total)
	}
	if report.NodesAudit.Active != 20 {
		t.Errorf("expected Active=20, got %d", report.NodesAudit.Active)
	}
	if report.NodesAudit.Inactive != 100 {
		t.Errorf("expected Inactive=100, got %d", report.NodesAudit.Inactive)
	}
	if report.NodesAudit.PagesFetched < 2 {
		t.Errorf("expected at least 2 pages fetched, got %d", report.NodesAudit.PagesFetched)
	}
}

func TestRun_RefreshPreservation_FailureKeepsPriorInventory(t *testing.T) {
	var mu sync.Mutex
	refreshCount := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-flaky","name":"Flaky Sub","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/subscriptions/sub-flaky/refresh":
			refreshCount++
			// Simulate upstream 500 error
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":"fetch_failed","message":"upstream dial timeout: dial tcp 198.51.100.1:443"}`))
		case "/api/v1/nodes":
			// Both pre and post refresh return the same intact inventory
			w.Write([]byte(`{"data":{"items":[{"logical_id":"node-pres-1","active":true,"connection_revision":5,"sources":[{"subscription_id":"sub-flaky"}]}],"total":1,"page":1}}`))
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-live-refresh", "-output", outReport}, &stdout, &stderr)
	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)

	if len(report.RefreshAudit) != 1 {
		t.Fatalf("expected 1 refresh audit record, got %d", len(report.RefreshAudit))
	}
	rec := report.RefreshAudit[0]
	if !rec.InventoryPreserved {
		t.Errorf("expected InventoryPreserved=true on failure preservation")
	}
	if rec.Outcome != "fetch_failed" {
		t.Errorf("expected outcome 'fetch_failed', got %q", rec.Outcome)
	}
	if code != ExitCodeSuccess {
		t.Errorf("expected ExitCodeSuccess when inventory is preserved, got %d", code)
	}
}

func TestRun_RefreshFailure_EqualCountIDSwap_ClobberDetected(t *testing.T) {
	var mu sync.Mutex
	refreshCalled := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-swap","name":"Swap Sub","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/subscriptions/sub-swap/refresh":
			refreshCalled = true
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":"fetch_failed","message":"parse error"}`))
		case "/api/v1/nodes":
			if !refreshCalled {
				// Pre-refresh: node-orig-1 and node-orig-2 (count 2)
				w.Write([]byte(`{"data":{"items":[
					{"logical_id":"node-orig-1","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-swap"}]},
					{"logical_id":"node-orig-2","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-swap"}]}
				],"total":2,"page":1}}`))
				return
			}
			// Post-refresh: node-clobbered-1 and node-clobbered-2 (count 2, equal count, but IDs changed!)
			w.Write([]byte(`{"data":{"items":[
				{"logical_id":"node-clobbered-1","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-swap"}]},
				{"logical_id":"node-clobbered-2","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-swap"}]}
			],"total":2,"page":1}}`))
			return
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-live-refresh", "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) on ID clobber during failure, got %d", code)
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)

	if len(report.RefreshAudit) != 1 {
		t.Fatalf("expected 1 refresh record, got %d", len(report.RefreshAudit))
	}
	if report.RefreshAudit[0].InventoryPreserved {
		t.Errorf("CRITICAL BUG: equal count with swapped IDs must NOT be marked inventory_preserved!")
	}
	if report.Verdict != "FAIL" {
		t.Errorf("expected Verdict FAIL, got %q", report.Verdict)
	}
}

func TestRun_RefreshFailure_SourcesLostOrAltered_FailClosed(t *testing.T) {
	var mu sync.Mutex
	refreshCalled := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-drop-source","name":"Drop Source Sub","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/subscriptions/sub-drop-source/refresh":
			refreshCalled = true
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":"fetch_failed","message":"parse error"}`))
		case "/api/v1/nodes":
			if !refreshCalled {
				// Pre-refresh: node has source association
				w.Write([]byte(`{"data":{"items":[
					{"logical_id":"node-src-1","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-drop-source","last_seen_fetch_id":"fetch-1"}]}
				],"total":1,"page":1}}`))
				return
			}
			// Post-refresh: node has same logical_id, active, and connection_revision, but sources are empty (dropped!)
			w.Write([]byte(`{"data":{"items":[
				{"logical_id":"node-src-1","active":true,"connection_revision":1,"sources":[]}
			],"total":1,"page":1}}`))
			return
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-live-refresh", "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) when sources are dropped, got %d", code)
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if report.Verdict != "FAIL" {
		t.Errorf("expected Verdict FAIL on lost sources, got %q", report.Verdict)
	}
	if len(report.RefreshAudit) > 0 && report.RefreshAudit[0].InventoryPreserved {
		t.Errorf("InventoryPreserved must be false when sources are dropped")
	}
}

func TestRun_ProbeRun_Batching_Max2Serial_AllCovered(t *testing.T) {
	// Test 5 nodes: verify they are executed in batches of size <= 2 serially!
	var mu sync.Mutex
	var runsDispatched int32
	batchSizes := make([]int, 0)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-1","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/nodes":
			// 5 active nodes
			items := make([]map[string]any, 0, 5)
			for i := 1; i <= 5; i++ {
				items = append(items, map[string]any{
					"logical_id":          fmt.Sprintf("node-batched-%d", i),
					"active":              true,
					"connection_revision": int64(10 + i),
					"sources":             []map[string]any{{"subscription_id": "sub-1"}},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": items, "total": 5, "page": 1}})
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		case "/api/v1/probes/runs":
			var body struct {
				NodeLogicalIDs []string `json:"node_logical_ids"`
				Kinds          []string `json:"kinds"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.NodeLogicalIDs) > 2 {
				t.Errorf("CRITICAL VIOLATION: batch size exceeded 2 nodes! got %d", len(body.NodeLogicalIDs))
			}
			batchSizes = append(batchSizes, len(body.NodeLogicalIDs))
			idx := atomic.AddInt32(&runsDispatched, 1)
			runID := fmt.Sprintf("run-batch-%d", idx)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"run_id": runID, "state": "pending"}})
		default:
			if strings.HasPrefix(r.URL.Path, "/api/v1/probes/runs/run-batch-") {
				parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/probes/runs/"), "/")
				runID := parts[0]
				if len(parts) == 1 {
					// GET run state
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": runID, "state": "succeeded"}})
					return
				}
				if len(parts) == 2 && parts[1] == "observations" {
					// Return observations corresponding to this run's batch
					var runIdx int
					_, _ = fmt.Sscanf(runID, "run-batch-%d", &runIdx)
					var nodesForRun []string
					if runIdx == 1 {
						nodesForRun = []string{"node-batched-1", "node-batched-2"}
					} else if runIdx == 2 {
						nodesForRun = []string{"node-batched-3", "node-batched-4"}
					} else {
						nodesForRun = []string{"node-batched-5"}
					}

					obs := make([]map[string]any, 0, len(nodesForRun))
					for _, nid := range nodesForRun {
						var rev int64
						if nid == "node-batched-1" {
							rev = 11
						} else if nid == "node-batched-2" {
							rev = 12
						} else if nid == "node-batched-3" {
							rev = 13
						} else if nid == "node-batched-4" {
							rev = 14
						} else {
							rev = 15
						}
						obs = append(obs, map[string]any{
							"id":                  "obs-" + nid,
							"probe_run_id":        runID,
							"node_logical_id":     nid,
							"kind":                "baseline",
							"verdict":             "healthy",
							"evidence_digest":     "sha256:digest-" + nid,
							"latency_ms":          15,
							"redacted_summary":    "handshake_ok",
							"connection_revision": rev,
						})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": obs, "total": len(obs), "page": 1}})
					return
				}
			}
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-live-probe", "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeSuccess {
		t.Fatalf("expected ExitCodeSuccess (0), got %d; stderr: %s", code, stderr.String())
	}

	mu.Lock()
	defer mu.Unlock()

	// 5 nodes with batch size 2 -> 3 batches of sizes [2, 2, 1]
	if len(batchSizes) != 3 {
		t.Fatalf("expected 3 probe runs dispatched for 5 nodes, got %d (%v)", len(batchSizes), batchSizes)
	}
	if batchSizes[0] != 2 || batchSizes[1] != 2 || batchSizes[2] != 1 {
		t.Errorf("expected batches [2, 2, 1], got %v", batchSizes)
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if len(report.ProbeAudit) != 5 {
		t.Fatalf("expected 5 probe records in audit, got %d", len(report.ProbeAudit))
	}
	for _, pr := range report.ProbeAudit {
		if !pr.Reachable || pr.Verdict != "healthy" {
			t.Errorf("node %s probe record not healthy: %+v", pr.NodeID, pr)
		}
	}
}

func TestRun_ProbeRun_ForeignNodeObservation_FailClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-1","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/nodes":
			w.Write([]byte(`{"data":{"items":[{"logical_id":"node-target-1","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-1"}]}],"total":1,"page":1}}`))
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		case "/api/v1/probes/runs":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data":{"run_id":"run-foreign","state":"pending"}}`))
		case "/api/v1/probes/runs/run-foreign":
			w.Write([]byte(`{"data":{"id":"run-foreign","state":"succeeded"}}`))
		case "/api/v1/probes/runs/run-foreign/observations":
			// Returns observation for foreign node NOT in target list
			w.Write([]byte(`{"data":{"items":[{
				"id":"obs-foreign",
				"probe_run_id":"run-foreign",
				"node_logical_id":"unrelated-foreign-node",
				"kind":"baseline",
				"verdict":"healthy",
				"connection_revision":1
			}],"total":1,"page":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-live-probe", "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) on foreign node observation, got %d", code)
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if report.Verdict != "FAIL" {
		t.Errorf("expected Verdict FAIL on foreign node, got %q", report.Verdict)
	}
}

func TestRun_ProbeRun_MismatchedRevision_FailClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-1","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/nodes":
			// Target node has connection_revision = 5
			w.Write([]byte(`{"data":{"items":[{"logical_id":"node-rev-target","active":true,"connection_revision":5,"sources":[{"subscription_id":"sub-1"}]}],"total":1,"page":1}}`))
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		case "/api/v1/probes/runs":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data":{"run_id":"run-mismatch-rev","state":"pending"}}`))
		case "/api/v1/probes/runs/run-mismatch-rev":
			w.Write([]byte(`{"data":{"id":"run-mismatch-rev","state":"succeeded"}}`))
		case "/api/v1/probes/runs/run-mismatch-rev/observations":
			// Observation has outdated connection_revision = 4 (expected 5)
			w.Write([]byte(`{"data":{"items":[{
				"id":"obs-stale",
				"probe_run_id":"run-mismatch-rev",
				"node_logical_id":"node-rev-target",
				"kind":"baseline",
				"verdict":"healthy",
				"connection_revision":4
			}],"total":1,"page":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-live-probe", "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) on revision mismatch, got %d", code)
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if report.Verdict != "FAIL" {
		t.Errorf("expected Verdict FAIL on revision mismatch, got %q", report.Verdict)
	}
}

func TestRun_ProbeRun_MissingNodeObservation_FailClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-1","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/nodes":
			// 2 target nodes in batch
			w.Write([]byte(`{"data":{"items":[
				{"logical_id":"node-pair-1","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-1"}]},
				{"logical_id":"node-pair-2","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-1"}]}
			],"total":2,"page":1}}`))
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		case "/api/v1/probes/runs":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data":{"run_id":"run-missing-obs","state":"pending"}}`))
		case "/api/v1/probes/runs/run-missing-obs":
			w.Write([]byte(`{"data":{"id":"run-missing-obs","state":"succeeded"}}`))
		case "/api/v1/probes/runs/run-missing-obs/observations":
			// Only node-pair-1 returned observation; node-pair-2 missing!
			w.Write([]byte(`{"data":{"items":[{
				"id":"obs-only-1",
				"probe_run_id":"run-missing-obs",
				"node_logical_id":"node-pair-1",
				"kind":"baseline",
				"verdict":"healthy",
				"connection_revision":1
			}],"total":1,"page":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-live-probe", "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) when target observation is missing, got %d", code)
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if report.Verdict != "FAIL" {
		t.Errorf("expected Verdict FAIL on missing observation, got %q", report.Verdict)
	}
}

func TestRun_ProbeRun_NodeUnreachable_FailClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-1","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/nodes":
			w.Write([]byte(`{"data":{"items":[{"logical_id":"node-unreachable-1","active":true,"connection_revision":2,"sources":[{"subscription_id":"sub-1"}]}],"total":1,"page":1}}`))
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		case "/api/v1/probes/runs":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data":{"run_id":"run-unreach","state":"pending"}}`))
		case "/api/v1/probes/runs/run-unreach":
			w.Write([]byte(`{"data":{"id":"run-unreach","state":"succeeded"}}`))
		case "/api/v1/probes/runs/run-unreach/observations":
			// Verdict is unreachable (handshake failed)
			w.Write([]byte(`{"data":{"items":[{
				"id":"obs-unreach",
				"probe_run_id":"run-unreach",
				"node_logical_id":"node-unreachable-1",
				"kind":"baseline",
				"verdict":"unreachable",
				"redacted_summary":"dial tcp connection timeout",
				"connection_revision":2
			}],"total":1,"page":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-live-probe", "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) when node is unreachable, got %d", code)
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if report.Verdict != "FAIL" {
		t.Errorf("expected Verdict FAIL when node probe is unreachable, got %q", report.Verdict)
	}
}

func TestRun_ProbeRun_RunFailed_FailClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-1","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/nodes":
			w.Write([]byte(`{"data":{"items":[{"logical_id":"node-fail-run","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-1"}]}],"total":1,"page":1}}`))
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		case "/api/v1/probes/runs":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data":{"run_id":"run-state-fail","state":"pending"}}`))
		case "/api/v1/probes/runs/run-state-fail":
			// Terminal state is failed
			w.Write([]byte(`{"data":{"id":"run-state-fail","state":"failed"}}`))
		case "/api/v1/probes/runs/run-state-fail/observations":
			w.Write([]byte(`{"data":{"items":[{
				"id":"obs-fail",
				"probe_run_id":"run-state-fail",
				"node_logical_id":"node-fail-run",
				"kind":"baseline",
				"verdict":"failed",
				"connection_revision":1
			}],"total":1,"page":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-live-probe", "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) when probe run fails, got %d", code)
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if report.Verdict != "FAIL" {
		t.Errorf("expected Verdict FAIL when run failed, got %q", report.Verdict)
	}
}

func TestRun_ProbeRun_ObservationsMultiPagePagination(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[{"id":"sub-1","enabled":true}],"total":1,"page":1}}`))
		case "/api/v1/nodes":
			w.Write([]byte(`{"data":{"items":[
				{"logical_id":"node-paged-1","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-1"}]},
				{"logical_id":"node-paged-2","active":true,"connection_revision":1,"sources":[{"subscription_id":"sub-1"}]}
			],"total":2,"page":1}}`))
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		case "/api/v1/probes/runs":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data":{"run_id":"run-multipage-obs","state":"pending"}}`))
		case "/api/v1/probes/runs/run-multipage-obs":
			w.Write([]byte(`{"data":{"id":"run-multipage-obs","state":"succeeded"}}`))
		case "/api/v1/probes/runs/run-multipage-obs/observations":
			page := r.URL.Query().Get("page")
			if page == "1" || page == "" {
				// Page 1 returns node-paged-1 observation (total 2)
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
					"items": []map[string]any{{
						"id":                  "obs-p1",
						"probe_run_id":        "run-multipage-obs",
						"node_logical_id":     "node-paged-1",
						"kind":                "baseline",
						"verdict":             "healthy",
						"connection_revision": 1,
					}},
					"total": 2,
					"page":  1,
				}})
				return
			}
			// Page 2 returns node-paged-2 observation
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"items": []map[string]any{{
					"id":                  "obs-p2",
					"probe_run_id":        "run-multipage-obs",
					"node_logical_id":     "node-paged-2",
					"kind":                "baseline",
					"verdict":             "healthy",
					"connection_revision": 1,
				}},
				"total": 2,
				"page":  2,
			}})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-live-probe", "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeSuccess {
		t.Fatalf("expected ExitCodeSuccess (0), got %d; stderr: %s", code, stderr.String())
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if len(report.ProbeAudit) != 2 {
		t.Fatalf("expected 2 probe records retrieved across pages, got %d", len(report.ProbeAudit))
	}
}

func TestRun_PaginationInconsistency_DuplicatesFailClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/nodes":
			page := r.URL.Query().Get("page")
			if page == "1" || page == "" {
				w.Write([]byte(`{"data":{"items":[{"logical_id":"duplicate-node-id","active":true}],"total":2,"page":1}}`))
				return
			}
			// Page 2 returns the SAME logical_id -> duplicate inconsistency
			w.Write([]byte(`{"data":{"items":[{"logical_id":"duplicate-node-id","active":true}],"total":2,"page":2}}`))
			return
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-output", outReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) on duplicate node ID in pagination, got %d", code)
	}

	reportData, _ := os.ReadFile(outReport)
	var report AcceptanceReport
	_ = json.Unmarshal(reportData, &report)
	if report.Verdict != "FAIL" {
		t.Errorf("expected Verdict FAIL on pagination duplicates, got %q", report.Verdict)
	}
}

func TestRun_Security_TokenFileSymlink_Rejected(t *testing.T) {
	tempDir := t.TempDir()
	realToken := filepath.Join(tempDir, "real_token.secret")
	_ = os.WriteFile(realToken, []byte("valid-token"), 0600)

	symlinkToken := filepath.Join(tempDir, "symlink_token.secret")
	if err := os.Symlink(realToken, symlinkToken); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-token-file", symlinkToken}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) when token-file is a symlink, got %d", code)
	}
	if !strings.Contains(stderr.String(), "symlinks are not permitted") {
		t.Errorf("expected symlink rejection error in stderr, got: %s", stderr.String())
	}
}

func TestRun_Security_TokenFileGroupReadable_Rejected(t *testing.T) {
	tempDir := t.TempDir()
	insecureToken := filepath.Join(tempDir, "insecure_token.secret")
	// Write with 0644 permissions (group/other readable)
	_ = os.WriteFile(insecureToken, []byte("valid-token"), 0644)

	var stdout, stderr bytes.Buffer
	code := run([]string{"-token-file", insecureToken}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) when token-file is group/other readable, got %d", code)
	}
	if !strings.Contains(stderr.String(), "allow group/other access") {
		t.Errorf("expected permission rejection error in stderr, got: %s", stderr.String())
	}
}

func TestRun_Security_ReportFileSymlink_Rejected(t *testing.T) {
	tempDir := t.TempDir()
	realFile := filepath.Join(tempDir, "target.json")
	_ = os.WriteFile(realFile, []byte("{}"), 0600)

	symlinkReport := filepath.Join(tempDir, "symlink_report.json")
	if err := os.Symlink(realFile, symlinkReport); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"status":"ok"}}`))
	}))
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-output", symlinkReport}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Fatalf("expected ExitCodeFailure (1) when output destination is a symlink, got %d", code)
	}
	if !strings.Contains(stderr.String(), "destination is a symlink") {
		t.Errorf("expected symlink destination error in stderr, got: %s", stderr.String())
	}
}

func TestRun_RedactionCanary(t *testing.T) {
	secretCanary := "CANARY_SECRET_TOKEN_DO_NOT_LEAK_xyz987"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			// Echo error containing secret
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf(`{"code":"internal_error","message":"failed with secret token=%s"}`, secretCanary)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	outDir := t.TempDir()
	outReport := filepath.Join(outDir, "report.json")

	var stdout, stderr bytes.Buffer
	_ = run([]string{"-addr", ts.URL, "-token", secretCanary, "-output", outReport}, &stdout, &stderr)

	stdoutStr := stdout.String()
	stderrStr := stderr.String()
	if strings.Contains(stdoutStr, secretCanary) {
		t.Errorf("SECURITY LEAK: secret canary found in stdout: %s", stdoutStr)
	}
	if strings.Contains(stderrStr, secretCanary) {
		t.Errorf("SECURITY LEAK: secret canary found in stderr: %s", stderrStr)
	}

	reportData, err := os.ReadFile(outReport)
	if err == nil {
		reportStr := string(reportData)
		if strings.Contains(reportStr, secretCanary) {
			t.Errorf("SECURITY LEAK: secret canary found in saved report: %s", reportStr)
		}
	}
}

func TestRun_WriteFileFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Write([]byte(`{"data":{"status":"ok"}}`))
		case "/readyz":
			w.Write([]byte(`{"data":{"ready":true,"required_tables":14,"schema_version":13}}`))
		case "/api/v1/auth/status":
			w.Write([]byte(`{"data":{"mode":"open"}}`))
		case "/api/v1/settings/auth":
			w.Write([]byte(`{"data":{"admin_auth_enabled":false}}`))
		case "/api/v1/subscriptions":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/nodes":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/rules":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/policies/groups":
			w.Write([]byte(`{"data":{"items":[],"total":0}}`))
		case "/api/v1/publications/preflight":
			w.Write([]byte(`{"data":{"allowed":true,"diagnostics":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Attempt to write report to an impossible location: /dev/null/report.json
	invalidReportPath := "/dev/null/report.json"

	var stdout, stderr bytes.Buffer
	code := run([]string{"-addr", ts.URL, "-output", invalidReportPath}, &stdout, &stderr)
	if code != ExitCodeFailure {
		t.Errorf("expected ExitCodeFailure (1) when report saving fails, got %d", code)
	}
}

func TestRun_RunnerScriptScratchIsolation(t *testing.T) {
	// Test scripts/run_authenticated_acceptance.sh via subprocess
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	scriptPath := filepath.Join(repoRoot, "scripts", "run_authenticated_acceptance.sh")

	// Custom private scratch directory
	scratchDir := t.TempDir()
	outReport := filepath.Join(scratchDir, "out-report.json")

	cmd := exec.Command("bash", scriptPath, "-h")
	cmd.Env = append(os.Environ(), fmt.Sprintf("CSP_ACCEPTANCE_SCRATCH=%s", scratchDir))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("runner execution failed: %v, output: %s", err, string(output))
	}
	if !strings.Contains(string(output), "Usage of") && !strings.Contains(string(output), "-addr") {
		t.Errorf("expected help output from runner, got: %s", string(output))
	}

	// Verify that the binary build artifact in scratch was cleaned up by trap
	binPath := filepath.Join(scratchDir, "csp-live-acceptance")
	if _, err := os.Stat(binPath); !os.IsNotExist(err) {
		t.Errorf("expected binary in scratch to be cleaned up after execution, but it still exists at %s", binPath)
	}
	_ = outReport
}
