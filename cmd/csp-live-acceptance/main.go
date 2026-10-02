package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"clash-sub-parser/internal/domain"
)

// Exit codes following deterministic fail-closed contract.
const (
	ExitCodeSuccess = 0 // All semantic checks passed (PASS)
	ExitCodeFailure = 1 // Any assertion failed, schema mismatch, or error occurred (FAIL)
	ExitCodeBlocked = 2 // Protected mode requires credentials, but none available (BLOCKED)
)

// AcceptanceReport structures the complete output of the live acceptance suite.
type AcceptanceReport struct {
	Timestamp          string             `json:"timestamp"`
	TargetAddr         string             `json:"target_addr"`
	AuthMode           string             `json:"auth_mode"`
	Authenticated      bool               `json:"authenticated"`
	ProtectedStatus    string             `json:"protected_status"`
	Verdict            string             `json:"verdict"` // PASS | FAIL | BLOCKED
	UnauthenticatedOps []AssertionRecord  `json:"unauthenticated_ops"`
	AuthenticatedOps   []AssertionRecord  `json:"authenticated_ops,omitempty"`
	SubscriptionsAudit *SubscriptionsInfo `json:"subscriptions_audit,omitempty"`
	NodesAudit         *NodesInfo         `json:"nodes_audit,omitempty"`
	RefreshAudit       []RefreshRecord    `json:"refresh_audit,omitempty"`
	ProbeAudit         []ProbeRecord      `json:"probe_audit,omitempty"`
	ExternalActionable []string           `json:"external_actionable,omitempty"`
	Errors             []string           `json:"errors,omitempty"`
}

// AssertionRecord captures individual endpoint checks.
type AssertionRecord struct {
	Endpoint   string `json:"endpoint"`
	Method     string `json:"method"`
	StatusCode int    `json:"status_code"`
	Success    bool   `json:"success"`
	Detail     string `json:"detail"`
}

// SubscriptionsInfo summarizes subscriptions discovered.
type SubscriptionsInfo struct {
	Count        int      `json:"count"`
	EnabledCount int      `json:"enabled_count"`
	IDs          []string `json:"ids"`
}

// NodesInfo summarizes nodes discovered across all pages.
type NodesInfo struct {
	Total           int `json:"total"`
	Active          int `json:"active"`
	Inactive        int `json:"inactive"`
	PagesFetched    int `json:"pages_fetched"`
	ItemsCount      int `json:"items_count"`
	DuplicatesCount int `json:"duplicates_count"`
}

// RefreshRecord captures the outcome of a bounded subscription refresh.
type RefreshRecord struct {
	SubscriptionID     string `json:"subscription_id"`
	Outcome            string `json:"outcome"`
	NodesParsed        int    `json:"nodes_parsed"`
	NodesValid         int    `json:"nodes_valid"`
	RedactedError      string `json:"redacted_error,omitempty"`
	InventoryPreserved bool   `json:"inventory_preserved"`
}

// ProbeRecord captures the outcome of a bounded node probe.
type ProbeRecord struct {
	NodeID             string  `json:"node_id"`
	DisplayName        string  `json:"display_name,omitempty"`
	Stage              string  `json:"stage,omitempty"`
	LatencyMs          float64 `json:"latency_ms,omitempty"`
	Reachable          bool    `json:"reachable"`
	Error              string  `json:"error,omitempty"`
	RunID              string  `json:"run_id,omitempty"`
	Verdict            string  `json:"verdict,omitempty"`
	ConnectionRevision *int64  `json:"connection_revision,omitempty"`
}

// NodeSnapshot represents immutable inventory state for identity comparison.
type NodeSnapshot struct {
	LogicalID          string   `json:"logical_id"`
	Active             bool     `json:"active"`
	ConnectionRevision int64    `json:"connection_revision"`
	Sources            []string `json:"sources"`
}

type nodeViewItem struct {
	LogicalID          string              `json:"logical_id"`
	DisplayName        string              `json:"display_name"`
	Protocol           string              `json:"protocol"`
	Active             bool                `json:"active"`
	ConnectionRevision int64               `json:"connection_revision,omitempty"`
	Sources            []domain.NodeSource `json:"sources,omitempty"`
}

type SubscriptionItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type observationItem struct {
	ID                 string `json:"id"`
	ProbeRunID         string `json:"probe_run_id"`
	NodeLogicalID      string `json:"node_logical_id"`
	Kind               string `json:"kind"`
	Verdict            string `json:"verdict"`
	EvidenceDigest     string `json:"evidence_digest"`
	ObservedAt         string `json:"observed_at"`
	LatencyMs          int64  `json:"latency_ms"`
	RedactedSummary    string `json:"redacted_summary"`
	ConnectionRevision *int64 `json:"connection_revision,omitempty"`
}

func main() {
	exitCode := run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(exitCode)
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("csp-live-acceptance", flag.ContinueOnError)
	flags.SetOutput(stderr)

	addr := flags.String("addr", "http://127.0.0.1:18080", "CSP target base URL")
	token := flags.String("token", "", "Admin token (optional, directly passed - INSECURE, prefer -token-env or -token-file)")
	tokenEnv := flags.String("token-env", "CSP_ADMIN_TOKEN", "Environment variable name for admin token")
	tokenFile := flags.String("token-file", "", "Path to file containing admin token (must be regular file, owned by user, 0600)")
	maxSubs := flags.Int("max-subs", 3, "Max subscriptions to refresh during bounded live run (<=3)")
	maxProbes := flags.Int("max-probes", 6, "Max representative nodes to probe (<=6)")
	timeout := flags.Duration("timeout", 10*time.Minute, "Overall acceptance run timeout")
	liveRefresh := flags.Bool("live-refresh", false, "Execute bounded subscription refresh (frozen budget <= 3)")
	liveProbe := flags.Bool("live-probe", false, "Execute bounded representative node probe (frozen budget <= 6, <=2 serial)")
	outputPath := flags.String("output", "acceptance-report.json", "Path to save JSON acceptance report")

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return ExitCodeSuccess
		}
		fmt.Fprintf(stderr, "flag parse error: %v\n", err)
		return ExitCodeFailure
	}

	if *token != "" {
		fmt.Fprintf(stderr, "WARNING: passing token via command-line flag -token is insecure and visible in system process listings. Use -token-env or -token-file instead.\n")
	}

	if *maxSubs > 3 {
		*maxSubs = 3
	}
	if *maxProbes > 6 {
		*maxProbes = 6
	}

	runStartTime := time.Now()
	overallRunDeadline := runStartTime.Add(*timeout)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	baseURL := strings.TrimRight(*addr, "/")
	client := &http.Client{Timeout: 45 * time.Second}

	report := &AcceptanceReport{
		Timestamp:          time.Now().UTC().Format(time.RFC3339),
		TargetAddr:         sanitizeURL(baseURL),
		ProtectedStatus:    "PENDING",
		UnauthenticatedOps: make([]AssertionRecord, 0),
		Errors:             make([]string, 0),
		Verdict:            "PASS",
	}

	allPassed := true

	fmt.Fprintf(stdout, "=== CSP Live Semantic Acceptance Suite ===\n")
	fmt.Fprintf(stdout, "Target:            %s\n", report.TargetAddr)
	fmt.Fprintf(stdout, "Frozen Budget:     Max %d sub refreshes, Max %d node probes, Timeout %s\n", *maxSubs, *maxProbes, *timeout)
	fmt.Fprintf(stdout, "Live Modes:        Refresh=%t, Probe=%t (batches <=2 serial)\n", *liveRefresh, *liveProbe)
	fmt.Fprintf(stdout, "-------------------------------------------\n")

	// --------------------------------------------------------------------------
	// Phase 1: Unauthenticated Baseline & Schema Checks
	// --------------------------------------------------------------------------
	fmt.Fprintf(stdout, "[Phase 1] Executing Unauthenticated Baseline Checks...\n")

	// 1. /healthz
	healthzRec := checkEndpoint(ctx, client, http.MethodGet, baseURL+"/healthz", "", func(body []byte, status int) (bool, string) {
		if status != http.StatusOK {
			return false, fmt.Sprintf("expected 200, got %d", status)
		}
		var parsed struct {
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return false, fmt.Sprintf("invalid json: %v", err)
		}
		if parsed.Data.Status != "ok" {
			return false, fmt.Sprintf("expected status 'ok', got %q", parsed.Data.Status)
		}
		return true, "status=ok"
	})
	report.UnauthenticatedOps = append(report.UnauthenticatedOps, healthzRec)
	fmt.Fprintf(stdout, "  /healthz:           %s (%s)\n", passFail(healthzRec.Success), healthzRec.Detail)
	if !healthzRec.Success {
		allPassed = false
		report.Errors = append(report.Errors, "healthz check failed: "+healthzRec.Detail)
	}

	// 2. /readyz
	readyzRec := checkEndpoint(ctx, client, http.MethodGet, baseURL+"/readyz", "", func(body []byte, status int) (bool, string) {
		if status != http.StatusOK {
			return false, fmt.Sprintf("expected 200, got %d", status)
		}
		var parsed struct {
			Data struct {
				Ready          bool `json:"ready"`
				RequiredTables int  `json:"required_tables"`
				SchemaVersion  int  `json:"schema_version"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return false, fmt.Sprintf("invalid json: %v", err)
		}
		if !parsed.Data.Ready || parsed.Data.RequiredTables < 14 || parsed.Data.SchemaVersion < 13 {
			return false, fmt.Sprintf("ready=%t, tables=%d (expected>=14), schema=%d (expected>=13)",
				parsed.Data.Ready, parsed.Data.RequiredTables, parsed.Data.SchemaVersion)
		}
		return true, fmt.Sprintf("ready=%t, tables=%d, schema=%d", parsed.Data.Ready, parsed.Data.RequiredTables, parsed.Data.SchemaVersion)
	})
	report.UnauthenticatedOps = append(report.UnauthenticatedOps, readyzRec)
	fmt.Fprintf(stdout, "  /readyz:            %s (%s)\n", passFail(readyzRec.Success), readyzRec.Detail)
	if !readyzRec.Success {
		allPassed = false
		report.Errors = append(report.Errors, "readyz check failed: "+readyzRec.Detail)
	}

	// 3. /api/v1/auth/status
	var authMode string
	authStatusRec := checkEndpoint(ctx, client, http.MethodGet, baseURL+"/api/v1/auth/status", "", func(body []byte, status int) (bool, string) {
		if status != http.StatusOK {
			return false, fmt.Sprintf("expected 200, got %d", status)
		}
		var parsed struct {
			Data struct {
				Mode          string `json:"mode"`
				AdminMode     string `json:"admin_mode"`
				ExportMode    string `json:"export_mode"`
				Authenticated bool   `json:"authenticated"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return false, fmt.Sprintf("invalid json: %v", err)
		}
		authMode = parsed.Data.Mode
		return true, fmt.Sprintf("mode=%s, admin_mode=%s, export_mode=%s, auth=%t",
			parsed.Data.Mode, parsed.Data.AdminMode, parsed.Data.ExportMode, parsed.Data.Authenticated)
	})
	report.UnauthenticatedOps = append(report.UnauthenticatedOps, authStatusRec)
	report.AuthMode = authMode
	fmt.Fprintf(stdout, "  /api/v1/auth/status: %s (%s)\n", passFail(authStatusRec.Success), authStatusRec.Detail)
	if !authStatusRec.Success {
		allPassed = false
		report.Errors = append(report.Errors, "unauthenticated auth/status check failed: "+authStatusRec.Detail)
	}

	// --------------------------------------------------------------------------
	// Phase 2: Credential Discovery
	// --------------------------------------------------------------------------
	fmt.Fprintf(stdout, "\n[Phase 2] Discovering Legally Managed Credentials...\n")
	resolvedToken := strings.TrimSpace(*token)
	if resolvedToken == "" && *tokenFile != "" {
		secToken, err := readTokenFileSecurely(*tokenFile)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: token file security verification failed: %v\n", err)
			return ExitCodeFailure
		}
		resolvedToken = secToken
	}
	if resolvedToken == "" && *tokenEnv != "" {
		resolvedToken = strings.TrimSpace(os.Getenv(*tokenEnv))
	}

	if resolvedToken == "" && authMode == "protected" {
		fmt.Fprintf(stdout, "  [NOTICE] Auth mode is protected but no legal managed token is available.\n")
		fmt.Fprintf(stdout, "  [NOTICE] Protected management endpoints paused per zero-bypass policy.\n")
		report.ProtectedStatus = "PAUSED_CREDENTIALS_REQUIRED"
		report.Verdict = "BLOCKED"
		report.ExternalActionable = []string{
			"Option 1: Provide production CSP_ADMIN_TOKEN via secure environment variable or credential file.",
			"Option 2: Review and grant operator approval for authorized credential injection.",
		}
		if err := writeReport(*outputPath, report); err != nil {
			fmt.Fprintf(stderr, "ERROR: failed to save acceptance report: %v\n", err)
			return ExitCodeFailure
		}
		return ExitCodeBlocked
	}

	if resolvedToken != "" {
		fmt.Fprintf(stdout, "  ✓ Legal managed token discovered (length: %d, value: [REDACTED])\n", len(resolvedToken))
	} else {
		fmt.Fprintf(stdout, "  ✓ Running in open mode without credentials\n")
	}

	// --------------------------------------------------------------------------
	// Phase 3: Authenticated Management API Semantics (Read-Only)
	// --------------------------------------------------------------------------
	fmt.Fprintf(stdout, "\n[Phase 3] Auditing Authenticated API Response Semantics (Read-Only)...\n")
	report.AuthenticatedOps = make([]AssertionRecord, 0)

	// 1. Authenticated auth/status
	authCheckRec := checkEndpoint(ctx, client, http.MethodGet, baseURL+"/api/v1/auth/status", resolvedToken, func(body []byte, status int) (bool, string) {
		if status != http.StatusOK {
			return false, fmt.Sprintf("expected 200, got %d", status)
		}
		var parsed struct {
			Data struct {
				Authenticated bool `json:"authenticated"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return false, fmt.Sprintf("invalid json: %v", err)
		}
		report.Authenticated = parsed.Data.Authenticated
		if authMode == "protected" && !parsed.Data.Authenticated {
			return false, "expected authenticated=true in protected mode with valid token"
		}
		return true, fmt.Sprintf("authenticated=%t", parsed.Data.Authenticated)
	})
	report.AuthenticatedOps = append(report.AuthenticatedOps, authCheckRec)
	fmt.Fprintf(stdout, "  Auth Verification:  %s (%s)\n", passFail(authCheckRec.Success), authCheckRec.Detail)
	if !authCheckRec.Success {
		allPassed = false
		report.Errors = append(report.Errors, "authenticated auth verification failed: "+authCheckRec.Detail)
	}

	// 2. /api/v1/settings/auth
	settingsRec := checkEndpoint(ctx, client, http.MethodGet, baseURL+"/api/v1/settings/auth", resolvedToken, func(body []byte, status int) (bool, string) {
		if status != http.StatusOK {
			return false, fmt.Sprintf("expected 200, got %d", status)
		}
		var parsed struct {
			Data struct {
				AdminAuthEnabled  bool   `json:"admin_auth_enabled"`
				ExportAuthEnabled bool   `json:"export_auth_enabled"`
				TokenConfigured   bool   `json:"token_configured"`
				AdminMode         string `json:"admin_mode"`
				ExportMode        string `json:"export_mode"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return false, fmt.Sprintf("invalid json: %v", err)
		}
		return true, fmt.Sprintf("admin_auth=%t, export_auth=%t, token_configured=%t, admin_mode=%s, export_mode=%s",
			parsed.Data.AdminAuthEnabled, parsed.Data.ExportAuthEnabled, parsed.Data.TokenConfigured,
			parsed.Data.AdminMode, parsed.Data.ExportMode)
	})
	report.AuthenticatedOps = append(report.AuthenticatedOps, settingsRec)
	fmt.Fprintf(stdout, "  /api/v1/settings/auth: %s (%s)\n", passFail(settingsRec.Success), settingsRec.Detail)
	if !settingsRec.Success {
		allPassed = false
		report.Errors = append(report.Errors, "settings/auth check failed: "+settingsRec.Detail)
	}

	// 3. /api/v1/subscriptions (all pages, duplicate and pagination check)
	subs, totalSubs, subIDs, enabledSubIDs, subsErr := fetchAllSubscriptions(ctx, client, baseURL, resolvedToken)
	subsSuccess := subsErr == nil
	subsDetail := ""
	if subsSuccess {
		subsDetail = fmt.Sprintf("total=%d, fetched=%d, enabled=%d", totalSubs, len(subs), len(enabledSubIDs))
		report.SubscriptionsAudit = &SubscriptionsInfo{
			Count:        totalSubs,
			EnabledCount: len(enabledSubIDs),
			IDs:          subIDs,
		}
	} else {
		subsDetail = subsErr.Error()
		allPassed = false
		report.Errors = append(report.Errors, "subscriptions audit failed: "+subsErr.Error())
	}
	subsRec := AssertionRecord{
		Endpoint:   sanitizeURL(baseURL + "/api/v1/subscriptions"),
		Method:     http.MethodGet,
		StatusCode: 200,
		Success:    subsSuccess,
		Detail:     subsDetail,
	}
	report.AuthenticatedOps = append(report.AuthenticatedOps, subsRec)
	fmt.Fprintf(stdout, "  /api/v1/subscriptions: %s (%s)\n", passFail(subsRec.Success), subsRec.Detail)

	// 4. /api/v1/nodes (all pages, active as bool, pagination, duplicate check)
	allNodes, totalNodes, activeNodeIDs, pagesFetched, nodesErr := fetchAllNodes(ctx, client, baseURL, resolvedToken)
	nodesSuccess := nodesErr == nil
	nodesDetail := ""
	if nodesSuccess {
		activeCount := len(activeNodeIDs)
		inactiveCount := len(allNodes) - activeCount
		nodesDetail = fmt.Sprintf("total=%d, active=%d, inactive=%d, pages=%d",
			totalNodes, activeCount, inactiveCount, pagesFetched)
		report.NodesAudit = &NodesInfo{
			Total:           totalNodes,
			Active:          activeCount,
			Inactive:        inactiveCount,
			PagesFetched:    pagesFetched,
			ItemsCount:      len(allNodes),
			DuplicatesCount: 0,
		}
	} else {
		nodesDetail = nodesErr.Error()
		allPassed = false
		report.Errors = append(report.Errors, "nodes audit failed: "+nodesErr.Error())
	}
	nodesRec := AssertionRecord{
		Endpoint:   sanitizeURL(baseURL + "/api/v1/nodes"),
		Method:     http.MethodGet,
		StatusCode: 200,
		Success:    nodesSuccess,
		Detail:     nodesDetail,
	}
	report.AuthenticatedOps = append(report.AuthenticatedOps, nodesRec)
	fmt.Fprintf(stdout, "  /api/v1/nodes:      %s (%s)\n", passFail(nodesRec.Success), nodesRec.Detail)

	// 5. /api/v1/policies/rules & /api/v1/policies/groups
	rulesRec := checkEndpoint(ctx, client, http.MethodGet, baseURL+"/api/v1/policies/rules", resolvedToken, func(body []byte, status int) (bool, string) {
		if status != http.StatusOK {
			return false, fmt.Sprintf("expected 200, got %d", status)
		}
		var parsed struct {
			Data struct {
				Items []any `json:"items"`
				Total int   `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return false, fmt.Sprintf("invalid json: %v", err)
		}
		return true, fmt.Sprintf("rules_count=%d", parsed.Data.Total)
	})
	report.AuthenticatedOps = append(report.AuthenticatedOps, rulesRec)
	fmt.Fprintf(stdout, "  /api/v1/policies/rules:  %s (%s)\n", passFail(rulesRec.Success), rulesRec.Detail)
	if !rulesRec.Success {
		allPassed = false
		report.Errors = append(report.Errors, "policies/rules check failed: "+rulesRec.Detail)
	}

	groupsRec := checkEndpoint(ctx, client, http.MethodGet, baseURL+"/api/v1/policies/groups", resolvedToken, func(body []byte, status int) (bool, string) {
		if status != http.StatusOK {
			return false, fmt.Sprintf("expected 200, got %d", status)
		}
		var parsed struct {
			Data struct {
				Items []any `json:"items"`
				Total int   `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return false, fmt.Sprintf("invalid json: %v", err)
		}
		return true, fmt.Sprintf("groups_count=%d", parsed.Data.Total)
	})
	report.AuthenticatedOps = append(report.AuthenticatedOps, groupsRec)
	fmt.Fprintf(stdout, "  /api/v1/policies/groups: %s (%s)\n", passFail(groupsRec.Success), groupsRec.Detail)
	if !groupsRec.Success {
		allPassed = false
		report.Errors = append(report.Errors, "policies/groups check failed: "+groupsRec.Detail)
	}

	// 6. /api/v1/publications/preflight
	pubRec := checkEndpoint(ctx, client, http.MethodGet, baseURL+"/api/v1/publications/preflight?target=mihomo", resolvedToken, func(body []byte, status int) (bool, string) {
		if status != http.StatusOK {
			return false, fmt.Sprintf("expected 200, got %d", status)
		}
		var parsed struct {
			Data struct {
				Allowed     bool  `json:"allowed"`
				Diagnostics []any `json:"diagnostics"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return false, fmt.Sprintf("invalid json: %v", err)
		}
		return true, fmt.Sprintf("preflight_allowed=%t, diagnostics=%d, export_check=not_applicable (no prior publication)",
			parsed.Data.Allowed, len(parsed.Data.Diagnostics))
	})
	report.AuthenticatedOps = append(report.AuthenticatedOps, pubRec)
	fmt.Fprintf(stdout, "  /api/v1/publications/preflight: %s (%s)\n", passFail(pubRec.Success), pubRec.Detail)
	if !pubRec.Success {
		allPassed = false
		report.Errors = append(report.Errors, "publications/preflight check failed: "+pubRec.Detail)
	}

	// --------------------------------------------------------------------------
	// Phase 4: Bounded Live Execution (Refresh & Probe)
	// --------------------------------------------------------------------------
	if *liveRefresh {
		fmt.Fprintf(stdout, "\n[Phase 4a] Executing Bounded Live Subscription Refresh (Frozen Budget)...\n")
		report.RefreshAudit = make([]RefreshRecord, 0)
		toRefresh := enabledSubIDs
		if len(toRefresh) > *maxSubs {
			toRefresh = toRefresh[:*maxSubs]
		}

		for _, subID := range toRefresh {
			// Extract baseline inventory snapshot for subID with canonical sorted sources
			preSnapshot := getSubInventory(allNodes, subID)

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/subscriptions/"+subID+"/refresh", nil)
			if err != nil {
				allPassed = false
				report.Errors = append(report.Errors, fmt.Sprintf("refresh req create failed for sub %s: %v", subID, err))
				continue
			}
			req.Header.Set("Idempotency-Key", fmt.Sprintf("live-acceptance-sub-refresh-%s-%d", subID, time.Now().UnixNano()))
			if resolvedToken != "" {
				req.Header.Set("Authorization", "Bearer "+resolvedToken)
			}

			refreshTimeoutClient := &http.Client{Timeout: 45 * time.Second}
			resp, err := refreshTimeoutClient.Do(req)

			outcome := "unknown"
			nodesParsed := 0
			nodesValid := 0
			redactedErr := ""
			status := 0

			if err != nil {
				outcome = "network_error"
				redactedErr = domain.RedactSensitiveInfo(err.Error())
			} else {
				status = resp.StatusCode
				respBody, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				if status == http.StatusOK {
					var parsed struct {
						Data struct {
							Outcome       string `json:"outcome"`
							NodesParsed   int    `json:"nodes_parsed"`
							NodesValid    int    `json:"nodes_valid"`
							RedactedError string `json:"redacted_error"`
						} `json:"data"`
					}
					if jErr := json.Unmarshal(respBody, &parsed); jErr == nil {
						outcome = parsed.Data.Outcome
						nodesParsed = parsed.Data.NodesParsed
						nodesValid = parsed.Data.NodesValid
						redactedErr = domain.RedactSensitiveInfo(parsed.Data.RedactedError)
					}
				} else {
					var errParsed struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					}
					_ = json.Unmarshal(respBody, &errParsed)
					outcome = "failed"
					if errParsed.Code != "" {
						outcome = errParsed.Code
					}
					redactedErr = domain.RedactSensitiveInfo(errParsed.Message)
				}
			}

			// Post refresh inventory snapshot: re-fetch nodes across all pages
			postNodes, _, _, _, postNodesErr := fetchAllNodes(ctx, client, baseURL, resolvedToken)
			preserved := false
			if postNodesErr != nil {
				// Cannot verify inventory preservation -> fail closed!
				allPassed = false
				preserved = false
				report.Errors = append(report.Errors, fmt.Sprintf("post-refresh node query failed for sub %s: %v", subID, postNodesErr))
			} else {
				postSnapshot := getSubInventory(postNodes, subID)
				allNodes = postNodes // keep updated

				if outcome == "success" {
					// Successful refresh must preserve or legally update valid nodes
					preserved = true
				} else {
					// Failure case: prior inventory identity, active, connection revision AND sources must remain intact
					preserved = compareInventoryExact(preSnapshot, postSnapshot)
					if !preserved {
						allPassed = false
						report.Errors = append(report.Errors, fmt.Sprintf("CRITICAL: inventory clobbered, ID changed, or sources dropped on sub %s refresh failure", subID))
					}
				}
			}

			rec := RefreshRecord{
				SubscriptionID:     subID,
				Outcome:            outcome,
				NodesParsed:        nodesParsed,
				NodesValid:         nodesValid,
				RedactedError:      redactedErr,
				InventoryPreserved: preserved,
			}
			report.RefreshAudit = append(report.RefreshAudit, rec)
			fmt.Fprintf(stdout, "  Sub %s: outcome=%s, parsed=%d, valid=%d, inventory_preserved=%t\n",
				subID, rec.Outcome, rec.NodesParsed, rec.NodesValid, rec.InventoryPreserved)
		}
	}

	if *liveProbe {
		fmt.Fprintf(stdout, "\n[Phase 4b] Executing Bounded Representative Node Probes (Frozen Budget, <=2 Serial)...\n")
		report.ProbeAudit = make([]ProbeRecord, 0)
		toProbe := activeNodeIDs
		if len(toProbe) > *maxProbes {
			toProbe = toProbe[:*maxProbes]
		}

		if len(toProbe) == 0 {
			fmt.Fprintf(stdout, "  [NOTICE] No active nodes available for representative probe.\n")
		} else {
			// Build node metadata lookup
			nodeMetaMap := make(map[string]nodeViewItem)
			for _, n := range allNodes {
				nodeMetaMap[n.LogicalID] = n
			}

			// Batching: strictly <= 2 nodes per batch, executed serially
			const probeBatchSize = 2
			var batches [][]string
			for i := 0; i < len(toProbe); i += probeBatchSize {
				end := i + probeBatchSize
				if end > len(toProbe) {
					end = len(toProbe)
				}
				batches = append(batches, toProbe[i:end])
			}

			overallProbeDeadline := overallRunDeadline

			for batchIdx, batchNodes := range batches {
				if time.Now().After(overallProbeDeadline) {
					allPassed = false
					report.Errors = append(report.Errors, fmt.Sprintf("probe run budget exhausted before batch %d/%d", batchIdx+1, len(batches)))
					break
				}
				remaining := time.Until(overallProbeDeadline)
				batchTimeout := 60 * time.Second
				if remaining < batchTimeout {
					batchTimeout = remaining
				}
				batchDeadline := time.Now().Add(batchTimeout)

				fmt.Fprintf(stdout, "  [Batch %d/%d] Dispatching probe for %d nodes: %v...\n",
					batchIdx+1, len(batches), len(batchNodes), batchNodes)

				reqBody, _ := json.Marshal(map[string]any{
					"node_logical_ids": batchNodes,
					"kinds":            []string{"baseline"},
					"deadline":         batchDeadline.UTC().Format(time.RFC3339),
				})

				req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/probes/runs", bytes.NewReader(reqBody))
				if err != nil {
					allPassed = false
					report.Errors = append(report.Errors, fmt.Sprintf("failed to create probe run request for batch %d: %v", batchIdx+1, err))
					continue
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", fmt.Sprintf("live-acceptance-probe-run-b%d-%d", batchIdx+1, time.Now().UnixNano()))
				if resolvedToken != "" {
					req.Header.Set("Authorization", "Bearer "+resolvedToken)
				}
				resp, pErr := client.Do(req)
				if pErr != nil {
					allPassed = false
					report.Errors = append(report.Errors, fmt.Sprintf("probe run dispatch network error for batch %d: %v", batchIdx+1, pErr))
					continue
				}
				respBody, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				var parsed struct {
					Data struct {
						RunID string `json:"run_id"`
					} `json:"data"`
				}
				_ = json.Unmarshal(respBody, &parsed)
				runID := parsed.Data.RunID

				if runID == "" {
					allPassed = false
					report.Errors = append(report.Errors, fmt.Sprintf("probe run dispatch returned no run_id for batch %d (status %d)", batchIdx+1, resp.StatusCode))
					continue
				}

				fmt.Fprintf(stdout, "    Dispatched RunID: %s (polling up to %s)...\n", runID, batchTimeout)
				runState, waitOk := pollProbeRunTerminal(ctx, client, baseURL, resolvedToken, runID, batchDeadline)
				if !waitOk {
					allPassed = false
					report.Errors = append(report.Errors, fmt.Sprintf("probe run %s timed out waiting for terminal state (last state: %s)", runID, runState))
					continue
				}
				if runState != "succeeded" {
					allPassed = false
					report.Errors = append(report.Errors, fmt.Sprintf("probe run %s finished with non-successful state %s", runID, runState))
				}

				obsList, obsErr := fetchAllRunObservations(ctx, client, baseURL, resolvedToken, runID)
				if obsErr != nil {
					allPassed = false
					report.Errors = append(report.Errors, fmt.Sprintf("probe run %s observations fetch failed: %v", runID, obsErr))
					continue
				}
				if len(obsList) == 0 {
					allPassed = false
					report.Errors = append(report.Errors, fmt.Sprintf("probe run %s completed with state %s but returned 0 observations (fail-closed)", runID, runState))
					continue
				}

				// Verification of observations against requested batchNodes:
				obsByNode := make(map[string]observationItem)
				for _, obs := range obsList {
					if obs.ProbeRunID != runID {
						allPassed = false
						report.Errors = append(report.Errors, fmt.Sprintf("observation %s has mismatched probe_run_id %s (expected %s)", obs.ID, obs.ProbeRunID, runID))
					}
					// Check if node is in batchNodes
					foundInBatch := false
					for _, bn := range batchNodes {
						if bn == obs.NodeLogicalID {
							foundInBatch = true
							break
						}
					}
					if !foundInBatch {
						allPassed = false
						report.Errors = append(report.Errors, fmt.Sprintf("foreign node %s found in observations for run %s", obs.NodeLogicalID, runID))
					}

					// Verify connection_revision
					if targetMeta, ok := nodeMetaMap[obs.NodeLogicalID]; ok {
						if obs.ConnectionRevision == nil || *obs.ConnectionRevision != targetMeta.ConnectionRevision {
							allPassed = false
							report.Errors = append(report.Errors, fmt.Sprintf("observation connection_revision mismatch for node %s: got %v, expected %d",
								obs.NodeLogicalID, obs.ConnectionRevision, targetMeta.ConnectionRevision))
						}
					}

					obsByNode[obs.NodeLogicalID] = obs
				}

				// Every target node in batchNodes must have an observation
				for _, bn := range batchNodes {
					obs, exists := obsByNode[bn]
					if !exists {
						allPassed = false
						report.Errors = append(report.Errors, fmt.Sprintf("missing observation for requested node %s in run %s", bn, runID))
						continue
					}
					reachable := obs.Verdict == "healthy" || obs.Verdict == "available"
					if !reachable {
						allPassed = false
						report.Errors = append(report.Errors, fmt.Sprintf("node %s probe failed with verdict %s (%s)", bn, obs.Verdict, obs.RedactedSummary))
					}
					dispName := ""
					if meta, ok := nodeMetaMap[bn]; ok {
						dispName = meta.DisplayName
					}
					rec := ProbeRecord{
						NodeID:             bn,
						DisplayName:        dispName,
						Stage:              obs.RedactedSummary,
						LatencyMs:          float64(obs.LatencyMs),
						Reachable:          reachable,
						Error:              obs.RedactedSummary,
						RunID:              runID,
						Verdict:            obs.Verdict,
						ConnectionRevision: obs.ConnectionRevision,
					}
					report.ProbeAudit = append(report.ProbeAudit, rec)
					fmt.Fprintf(stdout, "    Node %s: verdict=%s, reachable=%t, latency=%.1fms\n",
						bn, obs.Verdict, reachable, float64(obs.LatencyMs))
				}
			}
		}
	}

	// --------------------------------------------------------------------------
	// Final Verdict Convergence (Fail-Closed)
	// --------------------------------------------------------------------------
	if allPassed {
		report.ProtectedStatus = "VERIFIED"
		report.Verdict = "PASS"
	} else {
		report.ProtectedStatus = "FAILED"
		report.Verdict = "FAIL"
	}

	if err := writeReport(*outputPath, report); err != nil {
		fmt.Fprintf(stderr, "ERROR: failed to save acceptance report: %v\n", err)
		return ExitCodeFailure
	}

	if allPassed {
		fmt.Fprintf(stdout, "\n=== Acceptance Complete: All Semantics Verified (PASS) ===\n")
		return ExitCodeSuccess
	}

	fmt.Fprintf(stdout, "\n=== Acceptance Completed with Errors (FAIL) ===\n")
	for _, e := range report.Errors {
		fmt.Fprintf(stderr, "  - %s\n", e)
	}
	return ExitCodeFailure
}

func checkEndpoint(ctx context.Context, client *http.Client, method, urlStr, token string, assertFn func([]byte, int) (bool, string)) AssertionRecord {
	cleanURL := sanitizeURL(urlStr)
	req, err := http.NewRequestWithContext(ctx, method, urlStr, nil)
	if err != nil {
		return AssertionRecord{
			Endpoint:   cleanURL,
			Method:     method,
			StatusCode: 0,
			Success:    false,
			Detail:     domain.RedactSensitiveInfo(err.Error()),
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return AssertionRecord{
			Endpoint:   cleanURL,
			Method:     method,
			StatusCode: 0,
			Success:    false,
			Detail:     domain.RedactSensitiveInfo(err.Error()),
		}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	success, detail := assertFn(body, resp.StatusCode)
	return AssertionRecord{
		Endpoint:   cleanURL,
		Method:     method,
		StatusCode: resp.StatusCode,
		Success:    success,
		Detail:     domain.RedactSensitiveInfo(detail),
	}
}

func fetchAllSubscriptions(ctx context.Context, client *http.Client, baseURL, token string) ([]SubscriptionItem, int, []string, []string, error) {
	allSubs := make([]SubscriptionItem, 0)
	subIDs := make([]string, 0)
	enabledSubIDs := make([]string, 0)
	seen := make(map[string]bool)
	page := 1
	total := 0

	for {
		urlStr := fmt.Sprintf("%s/api/v1/subscriptions?page=%d&page_size=100", baseURL, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, 0, nil, nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, 0, nil, nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, 0, nil, nil, fmt.Errorf("GET /api/v1/subscriptions status %d: %s",
				resp.StatusCode, domain.RedactSensitiveInfo(string(body)))
		}

		var parsed struct {
			Data struct {
				Items []SubscriptionItem `json:"items"`
				Total int                `json:"total"`
				Page  int                `json:"page"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, 0, nil, nil, fmt.Errorf("decode subscriptions JSON: %w", err)
		}

		if page > 1 && parsed.Data.Total != total {
			return nil, 0, nil, nil, fmt.Errorf("pagination inconsistency: total subscriptions changed from %d to %d on page %d", total, parsed.Data.Total, page)
		}
		total = parsed.Data.Total

		if len(parsed.Data.Items) == 0 && len(allSubs) < total {
			return nil, 0, nil, nil, fmt.Errorf("pagination ended prematurely: fetched %d of total %d on page %d", len(allSubs), total, page)
		}

		for _, s := range parsed.Data.Items {
			if seen[s.ID] {
				return nil, 0, nil, nil, fmt.Errorf("pagination inconsistency: duplicate subscription ID across pages: %s", s.ID)
			}
			seen[s.ID] = true
			allSubs = append(allSubs, s)
			subIDs = append(subIDs, s.ID)
			if s.Enabled {
				enabledSubIDs = append(enabledSubIDs, s.ID)
			}
		}

		if len(parsed.Data.Items) == 0 || len(allSubs) >= total {
			break
		}
		page++
	}

	return allSubs, total, subIDs, enabledSubIDs, nil
}

func fetchAllNodes(ctx context.Context, client *http.Client, baseURL, token string) ([]nodeViewItem, int, []string, int, error) {
	allNodes := make([]nodeViewItem, 0)
	activeNodeIDs := make([]string, 0)
	seen := make(map[string]bool)
	page := 1
	total := 0

	for {
		urlStr := fmt.Sprintf("%s/api/v1/nodes?page=%d&page_size=100", baseURL, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, 0, nil, page, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, 0, nil, page, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, 0, nil, page, fmt.Errorf("GET /api/v1/nodes status %d: %s",
				resp.StatusCode, domain.RedactSensitiveInfo(string(body)))
		}

		var parsed struct {
			Data struct {
				Items []nodeViewItem `json:"items"`
				Total int            `json:"total"`
				Page  int            `json:"page"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, 0, nil, page, fmt.Errorf("decode nodes JSON: %w", err)
		}

		if page > 1 && parsed.Data.Total != total {
			return nil, 0, nil, page, fmt.Errorf("pagination inconsistency: total nodes changed from %d to %d on page %d", total, parsed.Data.Total, page)
		}
		total = parsed.Data.Total

		if len(parsed.Data.Items) == 0 && len(allNodes) < total {
			return nil, 0, nil, page, fmt.Errorf("pagination ended prematurely: fetched %d of total %d on page %d", len(allNodes), total, page)
		}

		for _, n := range parsed.Data.Items {
			if seen[n.LogicalID] {
				return nil, 0, nil, page, fmt.Errorf("pagination inconsistency: duplicate node logical_id across pages: %s", n.LogicalID)
			}
			seen[n.LogicalID] = true
			if n.Active {
				activeNodeIDs = append(activeNodeIDs, n.LogicalID)
			}
			allNodes = append(allNodes, n)
		}

		if len(parsed.Data.Items) == 0 || len(allNodes) >= total {
			break
		}
		page++
	}

	return allNodes, total, activeNodeIDs, page, nil
}

func getSubInventory(nodes []nodeViewItem, subID string) map[string]NodeSnapshot {
	m := make(map[string]NodeSnapshot)
	for _, n := range nodes {
		belongs := false
		for _, s := range n.Sources {
			if s.SubscriptionID == subID {
				belongs = true
				break
			}
		}
		if belongs {
			srcs := make([]string, 0, len(n.Sources))
			for _, src := range n.Sources {
				srcs = append(srcs, fmt.Sprintf("%s:%s", src.SubscriptionID, src.LastSeenFetchID))
			}
			sort.Strings(srcs)
			m[n.LogicalID] = NodeSnapshot{
				LogicalID:          n.LogicalID,
				Active:             n.Active,
				ConnectionRevision: n.ConnectionRevision,
				Sources:            srcs,
			}
		}
	}
	return m
}

func compareInventoryExact(pre, post map[string]NodeSnapshot) bool {
	if len(pre) != len(post) {
		return false
	}
	for id, preNode := range pre {
		postNode, ok := post[id]
		if !ok {
			return false // Lost logical ID or swapped IDs
		}
		if postNode.Active != preNode.Active {
			return false // Active status unexpectedly changed on failure
		}
		if postNode.ConnectionRevision != preNode.ConnectionRevision {
			return false // Revision changed
		}
		if len(preNode.Sources) != len(postNode.Sources) {
			return false // Sources count changed
		}
		for i := range preNode.Sources {
			if preNode.Sources[i] != postNode.Sources[i] {
				return false // Specific source association missing or altered
			}
		}
	}
	return true
}

func pollProbeRunTerminal(ctx context.Context, client *http.Client, baseURL, token, runID string, deadline time.Time) (string, bool) {
	pollInterval := 500 * time.Millisecond
	runState := ""

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/probes/runs/"+runID, nil)
		if err != nil {
			break
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var parsed struct {
			Data struct {
				State string `json:"state"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err == nil {
			runState = parsed.Data.State
			// Terminal states: succeeded, failed, cancelled, expired
			if runState == "succeeded" || runState == "failed" || runState == "cancelled" || runState == "expired" {
				return runState, true
			}
		}
		time.Sleep(pollInterval)
	}

	// Timeout occurred before terminal state -> cancel probe run per bounded governance
	cancelReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/probes/runs/"+runID+"/cancel", nil)
	if token != "" {
		cancelReq.Header.Set("Authorization", "Bearer "+token)
	}
	if cResp, cErr := client.Do(cancelReq); cErr == nil {
		cResp.Body.Close()
	}
	return runState, false
}

func fetchAllRunObservations(ctx context.Context, client *http.Client, baseURL, token, runID string) ([]observationItem, error) {
	allObs := make([]observationItem, 0)
	seen := make(map[string]bool)
	page := 1
	total := 0

	for {
		obsURL := fmt.Sprintf("%s/api/v1/probes/runs/%s/observations?page=%d&page_size=100", baseURL, runID, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, obsURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create observations request: %w", err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("observations network error: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("observations returned HTTP %d: %s", resp.StatusCode, domain.RedactSensitiveInfo(string(body)))
		}

		var parsed struct {
			Data struct {
				Items []observationItem `json:"items"`
				Total int               `json:"total"`
				Page  int               `json:"page"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode observations JSON: %w", err)
		}

		if page > 1 && parsed.Data.Total != total {
			return nil, fmt.Errorf("pagination inconsistency: total observations changed from %d to %d on page %d", total, parsed.Data.Total, page)
		}
		total = parsed.Data.Total

		if len(parsed.Data.Items) == 0 && len(allObs) < total {
			return nil, fmt.Errorf("observations pagination ended prematurely: fetched %d of total %d on page %d", len(allObs), total, page)
		}

		for _, item := range parsed.Data.Items {
			if seen[item.ID] {
				return nil, fmt.Errorf("pagination inconsistency: duplicate observation ID across pages: %s", item.ID)
			}
			seen[item.ID] = true
			allObs = append(allObs, item)
		}

		if len(parsed.Data.Items) == 0 || len(allObs) >= total {
			break
		}
		page++
	}

	return allObs, nil
}

func readTokenFileSecurely(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("stat token file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("insecure token file: symlinks are not permitted: %s", path)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("insecure token file: must be a regular file: %s", path)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if stat.Uid != uint32(os.Getuid()) {
			return "", fmt.Errorf("insecure token file: owned by UID %d, must be owned by current UID %d: %s", stat.Uid, os.Getuid(), path)
		}
	}
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		return "", fmt.Errorf("insecure token file: permissions %04o allow group/other access, expected <= 0600: %s", perm, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func sanitizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return domain.RedactSensitiveInfo(raw)
	}
	u.User = nil
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			if strings.Contains(strings.ToLower(k), "token") || strings.Contains(strings.ToLower(k), "secret") || strings.Contains(strings.ToLower(k), "key") {
				q.Set(k, "***")
			}
		}
		u.RawQuery = q.Encode()
	}
	return domain.RedactSensitiveInfo(u.String())
}

func passFail(success bool) string {
	if success {
		return "✓ PASS"
	}
	return "✗ FAIL"
}

func writeReport(path string, report *AcceptanceReport) error {
	cleanPath := filepath.Clean(path)

	// Check if destination file exists and is a symlink or owned by someone else
	if info, err := os.Lstat(cleanPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("insecure report path: destination is a symlink: %s", cleanPath)
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			if stat.Uid != uint32(os.Getuid()) {
				return fmt.Errorf("insecure report path: destination owned by UID %d, expected current UID %d: %s", stat.Uid, os.Getuid(), cleanPath)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat report path: %w", err)
	}

	dir := filepath.Dir(cleanPath)
	if dir != "" && dir != "." {
		if dirInfo, err := os.Lstat(dir); err == nil {
			if dirInfo.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("insecure report directory: parent directory is a symlink: %s", dir)
			}
			if stat, ok := dirInfo.Sys().(*syscall.Stat_t); ok {
				if stat.Uid != uint32(os.Getuid()) {
					return fmt.Errorf("insecure report directory: parent directory owned by UID %d, expected current UID %d: %s", stat.Uid, os.Getuid(), dir)
				}
			}
		} else if os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return fmt.Errorf("failed to create report directory %s: %w", dir, err)
			}
		} else {
			return fmt.Errorf("stat report directory %s: %w", dir, err)
		}
	}

	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal acceptance report: %w", err)
	}

	f, err := os.OpenFile(cleanPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open report file %s: %w", cleanPath, err)
	}
	defer f.Close()

	if _, err := f.Write(raw); err != nil {
		return fmt.Errorf("failed to write acceptance report to %s: %w", cleanPath, err)
	}
	return nil
}
