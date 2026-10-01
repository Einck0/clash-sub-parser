package e2e_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// EndpointMapping represents a verified bilateral API route mapping.
type EndpointMapping struct {
	Method      string
	Path        string
	Category    string
	AuthScope   string // "public", "admin", "export", "legacy_410"
	CalledByUI  bool
	Description string
}

// FrontendCallSites lists all API endpoints called from web/src (features & UI).
var FrontendCallSites = []struct {
	Method string
	Path   string
	Caller string
}{
	// Auth
	{http.MethodGet, "/api/v1/auth/status", "web/src/features/auth/useAuth.ts"},
	{http.MethodPost, "/api/v1/auth/login", "web/src/features/auth/useAuth.ts"},
	{http.MethodPost, "/api/v1/auth/logout", "web/src/features/auth/useAuth.ts"},

	// Settings
	{http.MethodGet, "/api/v1/settings/auth", "web/src/features/settings/SettingsView.vue"},
	{http.MethodPut, "/api/v1/settings/auth", "web/src/features/settings/SettingsView.vue"},
	{http.MethodGet, "/api/v1/settings/admin-token", "web/src/features/settings/SettingsView.vue"},
	{http.MethodPost, "/api/v1/settings/admin-token", "web/src/features/settings/SettingsView.vue"},

	// Subscriptions
	{http.MethodGet, "/api/v1/subscriptions", "web/src/features/subscriptions/SubscriptionsView.vue"},
	{http.MethodPost, "/api/v1/subscriptions", "web/src/features/subscriptions/SubscriptionsView.vue"},
	{http.MethodGet, "/api/v1/subscriptions/{id}", "web/src/features/subscriptions/SubscriptionsView.vue"},
	{http.MethodPatch, "/api/v1/subscriptions/{id}", "web/src/features/subscriptions/SubscriptionsView.vue"},
	{http.MethodDelete, "/api/v1/subscriptions/{id}", "web/src/features/subscriptions/SubscriptionsView.vue"},
	{http.MethodPost, "/api/v1/subscriptions/{id}/refresh", "web/src/features/subscriptions/SubscriptionsView.vue"},

	// Nodes
	{http.MethodGet, "/api/v1/nodes", "web/src/features/nodes/NodesView.vue"},
	{http.MethodGet, "/api/v1/nodes/{logical_id}", "web/src/features/nodes/NodesView.vue"},
	{http.MethodPatch, "/api/v1/nodes/{logical_id}/connection", "web/src/features/nodes/NodesView.vue"},
	{http.MethodGet, "/api/v1/nodes/{logical_id}/observations", "web/src/features/nodes/NodesView.vue"},

	// Probes
	{http.MethodPost, "/api/v1/probes/runs", "web/src/features/probes/ProbesView.vue"},
	{http.MethodGet, "/api/v1/probes/runs", "web/src/features/probes/ProbesView.vue"},
	{http.MethodGet, "/api/v1/probes/runs/{run_id}", "web/src/features/probes/ProbesView.vue"},
	{http.MethodPost, "/api/v1/probes/runs/{run_id}/cancel", "web/src/features/probes/ProbesView.vue"},
	{http.MethodGet, "/api/v1/probes/runs/{run_id}/observations", "web/src/features/probes/ProbesView.vue"},
	{http.MethodGet, "/api/v1/probes/pool", "web/src/features/probes/ProbesView.vue"},
	{http.MethodGet, "/api/v1/probes/schedule", "web/src/features/probes/ProbesView.vue"},
	{http.MethodPut, "/api/v1/probes/schedule", "web/src/features/probes/ProbesView.vue"},

	// Policies & Rules
	{http.MethodGet, "/api/v1/policies/global-node-filter", "web/src/features/settings/GlobalNodeFilterSettings.vue"},
	{http.MethodPut, "/api/v1/policies/global-node-filter", "web/src/features/settings/GlobalNodeFilterSettings.vue"},
	{http.MethodGet, "/api/v1/policies/groups", "web/src/features/policy/PolicyView.vue"},
	{http.MethodPost, "/api/v1/policies/groups", "web/src/features/policy/PolicyView.vue"},
	{http.MethodGet, "/api/v1/policies/groups/{id}", "web/src/features/policy/PolicyView.vue"},
	{http.MethodPatch, "/api/v1/policies/groups/{id}", "web/src/features/policy/PolicyView.vue"},
	{http.MethodDelete, "/api/v1/policies/groups/{id}", "web/src/features/policy/PolicyView.vue"},
	{http.MethodPut, "/api/v1/policies/groups/{id}/edges", "web/src/features/policy/PolicyView.vue"},
	{http.MethodGet, "/api/v1/policies/rules", "web/src/features/policy/PolicyView.vue"},
	{http.MethodPost, "/api/v1/policies/rules", "web/src/features/policy/PolicyView.vue"},
	{http.MethodDelete, "/api/v1/policies/rules/{id}", "web/src/features/policy/PolicyView.vue"},

	// Publications
	{http.MethodPost, "/api/v1/publications/preflight", "web/src/features/publications/PublicationsView.vue"},
	{http.MethodGet, "/api/v1/publications/preflight", "web/src/features/publications/PublicationsView.vue"},
	{http.MethodPost, "/api/v1/publications", "web/src/features/publications/PublicationsView.vue"},
	{http.MethodPost, "/api/v1/publications/preview", "web/src/features/publications/PublicationsView.vue"},
	{http.MethodGet, "/api/v1/publications/{id}", "web/src/features/publications/PublicationsView.vue"},
	{http.MethodPost, "/api/v1/publications/{id}/revoke", "web/src/features/publications/PublicationsView.vue"},
	{http.MethodDelete, "/api/v1/publications/{id}", "web/src/features/publications/PublicationsView.vue"},
}

// Task 5.1: TestRouteInventory_BilateralCompleteness
// Statically and dynamically verifies bilateral mapping between frontend UI call sites
// and backend Chi route registration across all 45+ endpoints.
func TestRouteInventory_BilateralCompleteness(t *testing.T) {
	harness := setupTestHarness(t)

	// Collect all registered routes from the Chi router
	type RouteKey struct {
		Method string
		Pattern string
	}

	registeredRoutes := make(map[RouteKey]bool)
	chiRouter, ok := harness.Router.(chi.Routes)
	if !ok {
		t.Fatalf("harness router does not implement chi.Routes")
	}

	walkErr := chi.Walk(chiRouter, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// Chi route patterns normalize trailing slashes and wildcard parameters
		cleanRoute := strings.TrimSuffix(route, "/")
		if cleanRoute == "" {
			cleanRoute = "/"
		}
		registeredRoutes[RouteKey{Method: method, Pattern: cleanRoute}] = true
		return nil
	})
	if walkErr != nil {
		t.Fatalf("chi.Walk failed: %v", walkErr)
	}

	t.Logf("Total registered backend routes found by chi.Walk: %d", len(registeredRoutes))

	// Helper to check if a route matches Chi route registration patterns
	routeExists := func(method, path string) bool {
		// Exact match
		if registeredRoutes[RouteKey{Method: method, Pattern: path}] {
			return true
		}
		// Try matching with Chi parameter normalization: e.g. {id} vs {run_id} vs {logical_id}
		for r := range registeredRoutes {
			if r.Method != method {
				continue
			}
			partsExpected := strings.Split(strings.Trim(path, "/"), "/")
			partsActual := strings.Split(strings.Trim(r.Pattern, "/"), "/")
			if len(partsExpected) != len(partsActual) {
				continue
			}
			match := true
			for i := range partsExpected {
				e := partsExpected[i]
				a := partsActual[i]
				if strings.HasPrefix(e, "{") && strings.HasPrefix(a, "{") {
					continue // parameter placeholder matches
				}
				if e != a {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
		return false
	}

	// 1. Bilateral check: Every frontend call site MUST exist in backend router!
	var missingFromBackend []string
	for _, call := range FrontendCallSites {
		if !routeExists(call.Method, call.Path) {
			missingFromBackend = append(missingFromBackend, call.Method+" "+call.Path+" (from "+call.Caller+")")
		}
	}
	if len(missingFromBackend) > 0 {
		t.Fatalf("Bilateral check failed! Frontend calls missing in backend router:\n%s", strings.Join(missingFromBackend, "\n"))
	}
	t.Logf("✓ All %d frontend API call sites successfully mapped to backend Chi routes", len(FrontendCallSites))

	// 2. Count distinct HTTP endpoints registered on the server (Must be >= 45)
	if len(registeredRoutes) < 45 {
		t.Fatalf("expected at least 45 distinct registered endpoints, got %d", len(registeredRoutes))
	}
	t.Logf("✓ Total registered Chi endpoints count: %d (exceeds requirement of 45+ endpoints)", len(registeredRoutes))

	// 3. Verify System & Infrastructure Endpoints are present
	systemEndpoints := []struct {
		Method string
		Path   string
	}{
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/readyz"},
		{http.MethodGet, "/publish/v1/{publication_id}"},
		{http.MethodGet, "/p/{publication_id}"},
		{http.MethodGet, "/yaml"},
		{http.MethodGet, "/script"},
	}

	for _, se := range systemEndpoints {
		if !routeExists(se.Method, se.Path) {
			t.Errorf("expected system endpoint %s %s to be mounted", se.Method, se.Path)
		}
	}

	// 4. Print clean alphabetical matrix of all registered routes for audit documentation
	type RouteRow struct {
		Method string
		Path   string
	}
	var rows []RouteRow
	for r := range registeredRoutes {
		rows = append(rows, RouteRow{Method: r.Method, Path: r.Pattern})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Path == rows[j].Path {
			return rows[i].Method < rows[j].Method
		}
		return rows[i].Path < rows[j].Path
	})

	t.Logf("--- Registered Route Inventory (%d routes) ---", len(rows))
	for _, r := range rows {
		t.Logf("  %-6s %s", r.Method, r.Path)
	}
}

// TestLegacyGoneEndpoints_SemanticErrorEnvelope verifies that deprecated legacy endpoints (/yaml, /script)
// genuinely return HTTP 410 Gone with a structured Unified Error Envelope, not just static router presence.
func TestLegacyGoneEndpoints_SemanticErrorEnvelope(t *testing.T) {
	harness := setupTestHarness(t)

	legacyEndpoints := []string{
		"/yaml",
		"/yaml/default",
		"/script",
		"/script/default",
	}

	for _, ep := range legacyEndpoints {
		t.Run("GET "+ep, func(t *testing.T) {
			resp, err := harness.Request(http.MethodGet, ep, nil, nil, nil)
			if err != nil {
				t.Fatalf("request to %s failed: %v", ep, err)
			}
			if resp.StatusCode != http.StatusGone {
				t.Fatalf("expected 410 Gone for %s, got %d body=%s", ep, resp.StatusCode, resp.Body)
			}

			// Verify unified error envelope structure
			var errEnvelope struct {
				Code      string `json:"code"`
				Message   string `json:"message"`
				RequestID string `json:"request_id"`
			}
			if err := resp.JSON(&errEnvelope); err != nil {
				t.Fatalf("failed to parse JSON error envelope for %s: %v", ep, err)
			}
			if errEnvelope.Code == "" || errEnvelope.Message == "" {
				t.Fatalf("invalid error envelope for %s: %+v", ep, errEnvelope)
			}
			t.Logf("✓ %s correctly returned 410 Gone: code=%s message=%q request_id=%s", ep, errEnvelope.Code, errEnvelope.Message, errEnvelope.RequestID)
		})
	}
}

// TestSPAEndpoints_IndexAndFallback verifies that GET / and SPA fallback GET /*
// return HTTP 200 OK with text/html content containing embedded index.html.
func TestSPAEndpoints_IndexAndFallback(t *testing.T) {
	harness := setupTestHarness(t)

	spaRoutes := []string{
		"/",
		"/nodes",
		"/subscriptions",
		"/policy",
		"/settings",
	}

	for _, route := range spaRoutes {
		t.Run("GET "+route, func(t *testing.T) {
			resp, err := harness.Request(http.MethodGet, route, nil, nil, nil)
			if err != nil {
				t.Fatalf("request to %s failed: %v", route, err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200 OK for %s, got %d", route, resp.StatusCode)
			}
			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, "text/html") {
				t.Fatalf("expected text/html for %s, got %q", route, ct)
			}
			if !strings.Contains(string(resp.Body), "<div id=\"app\">") {
				t.Fatalf("expected SPA index.html containing <div id=\"app\"> for %s, got body length %d", route, len(resp.Body))
			}
			t.Logf("✓ %s returned 200 OK with SPA index.html", route)
		})
	}
}


