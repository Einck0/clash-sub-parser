package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/platform"

	_ "modernc.org/sqlite"
)

func TestMainRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "default run",
			args:       []string{},
			wantCode:   0,
			wantStdout: "Clash Sub Parser Control Plane",
		},
		{
			name:       "version flag",
			args:       []string{"--version"},
			wantCode:   0,
			wantStdout: platform.Version,
		},
		{
			name:       "version shorthand",
			args:       []string{"-v"},
			wantCode:   0,
			wantStdout: platform.Version,
		},
		{
			name:       "version subcommand",
			args:       []string{"version"},
			wantCode:   0,
			wantStdout: platform.Version,
		},
		{
			name:       "legacy-inspect requires source",
			args:       []string{"legacy-inspect"},
			wantCode:   2,
			wantStderr: "legacy-inspect: --source is required",
		},
		{
			name:       "legacy-import requires source",
			args:       []string{"legacy-import"},
			wantCode:   2,
			wantStderr: "legacy-import: --source is required",
		},
		{
			name:       "unknown command",
			args:       []string{"unknown-subcommand"},
			wantCode:   1,
			wantStderr: "unknown command: unknown-subcommand",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tc.args, &stdout, &stderr)

			if code != tc.wantCode {
				t.Fatalf("expected exit code %d, got %d. Stderr: %s", tc.wantCode, code, stderr.String())
			}

			if tc.wantStdout != "" && !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("expected stdout to contain %q, got %q", tc.wantStdout, stdout.String())
			}

			if tc.wantStderr != "" && !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("expected stderr to contain %q, got %q", tc.wantStderr, stderr.String())
			}
		})
	}
}

func TestLegacyInspectCLI(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "legacy_cli_test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite test database: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE subscriptions (
			id INTEGER PRIMARY KEY,
			name TEXT,
			url TEXT,
			password TEXT
		);
		INSERT INTO subscriptions VALUES (1, 'TestSub', 'https://example.invalid', 'secret_pass_12345');
	`)
	if err != nil {
		db.Close()
		t.Fatalf("failed to seed test database: %v", err)
	}
	_ = db.Close()

	t.Run("legacy-inspect with --source and text format", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"legacy-inspect", "--source", dbPath}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "CSP Legacy SQLite Inspection Report") {
			t.Errorf("expected report header, got: %s", out)
		}
		if !strings.Contains(out, "Schema Fingerprint:") {
			t.Errorf("expected schema fingerprint in text output, got: %s", out)
		}
		if !strings.Contains(out, "subscriptions") {
			t.Errorf("expected table name 'subscriptions', got: %s", out)
		}
		if strings.Contains(out, "secret_pass_12345") {
			t.Errorf("leak detected: text output contains secret data value")
		}
	})

	t.Run("legacy-inspect with -s and JSON format", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"legacy-inspect", "-s", dbPath, "--format", "json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr.String())
		}
		out := stdout.String()

		var report map[string]interface{}
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatalf("expected valid json output, got error: %v. Raw: %s", err, out)
		}
		if report["schema_fingerprint"] == nil || report["schema_fingerprint"] == "" {
			t.Errorf("missing schema_fingerprint in json: %v", report)
		}
		if report["total_tables"] != float64(1) {
			t.Errorf("expected 1 table, got %v", report["total_tables"])
		}
		if strings.Contains(out, "secret_pass_12345") {
			t.Errorf("leak detected: json output contains secret data value")
		}
	})

	t.Run("legacy-inspect with positional source argument", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"legacy-inspect", dbPath}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "CSP Legacy SQLite Inspection Report") {
			t.Errorf("expected report header with positional arg, got: %s", stdout.String())
		}
	})

	t.Run("legacy-inspect with output file", func(t *testing.T) {
		outFile := filepath.Join(tempDir, "report_output.json")
		var stdout, stderr bytes.Buffer
		code := run([]string{"legacy-inspect", "-s", dbPath, "-f", "json", "-o", outFile}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr.String())
		}
		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		if !strings.Contains(string(data), "schema_fingerprint") {
			t.Errorf("expected json in output file, got: %s", string(data))
		}
	})

	t.Run("legacy-inspect invalid format", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"legacy-inspect", "-s", dbPath, "--format", "yaml"}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("expected exit code 2 for invalid format, got %d", code)
		}
		if !strings.Contains(stderr.String(), "unknown format") {
			t.Errorf("expected unknown format error, got: %s", stderr.String())
		}
	})

	t.Run("legacy-inspect non-existent file", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"legacy-inspect", "-s", filepath.Join(tempDir, "missing.db")}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("expected exit code 1 for missing file, got %d", code)
		}
		if !strings.Contains(stderr.String(), "cannot access legacy database") {
			t.Errorf("expected missing database error, got: %s", stderr.String())
		}
	})
}

func TestLegacyImportCLI(t *testing.T) {
	tempDir := t.TempDir()
	sourceDB := filepath.Join(tempDir, "legacy_source.db")
	targetDB := filepath.Join(tempDir, "csp_target.db")

	db, err := sql.Open("sqlite", sourceDB)
	if err != nil {
		t.Fatalf("failed to create sqlite source: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE subscriptions (
			id INTEGER PRIMARY KEY,
			name TEXT,
			url TEXT,
			update_interval INTEGER,
			enabled BOOLEAN,
			is_primary BOOLEAN
		);
		INSERT INTO subscriptions VALUES (1, 'MainFeed', 'https://user:pass123@sub.example.invalid/feed?token=tok999', 3600, 1, 1);
		CREATE TABLE node_groups (
			id INTEGER PRIMARY KEY,
			name TEXT,
			kind TEXT,
			group_type TEXT,
			sort_order INTEGER,
			include_group_ids TEXT
		);
		INSERT INTO node_groups VALUES (1, 'ProxyGroup', 'select', 'select', 1, '[]');
		CREATE TABLE rules (
			id INTEGER PRIMARY KEY,
			name TEXT,
			type TEXT,
			value TEXT,
			proxy TEXT,
			sort_order INTEGER,
			enabled BOOLEAN
		);
		INSERT INTO rules VALUES (1, 'GoogleRule', 'DOMAIN-SUFFIX', 'google.com', 'ProxyGroup', 1, 1);
	`)
	if err != nil {
		db.Close()
		t.Fatalf("failed to seed source: %v", err)
	}
	db.Close()

	t.Run("legacy-import requires target when not dry-run", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"legacy-import", "--source", sourceDB}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("expected exit code 2, got %d. Stderr: %s", code, stderr.String())
		}
		if !strings.Contains(stderr.String(), "legacy-import: --target is required") {
			t.Errorf("expected missing target error, got: %s", stderr.String())
		}
	})

	t.Run("legacy-import with dry-run and text format", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"legacy-import", "--source", sourceDB, "--dry-run"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "DRY-RUN PREVIEW (NO WRITES)") {
			t.Errorf("expected dry run header, got: %s", out)
		}
		if !strings.Contains(out, "Target Revision State: draft") {
			t.Errorf("expected draft state, got: %s", out)
		}
		if strings.Contains(out, "pass123") || strings.Contains(out, "tok999") {
			t.Errorf("secret leaked in dry-run CLI stdout: %s", out)
		}
	})

	t.Run("legacy-import actual import with json format and report file", func(t *testing.T) {
		reportFile := filepath.Join(tempDir, "import_report.json")
		var stdout, stderr bytes.Buffer
		code := run([]string{
			"legacy-import",
			"-s", sourceDB,
			"-t", targetDB,
			"-f", "json",
			"-r", reportFile,
		}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr.String())
		}

		reportData, err := os.ReadFile(reportFile)
		if err != nil {
			t.Fatalf("failed to read report file: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(reportData, &parsed); err != nil {
			t.Fatalf("failed to parse json report: %v", err)
		}
		if parsed["target_revision_state"] != "draft" {
			t.Errorf("expected draft revision state, got: %v", parsed["target_revision_state"])
		}

		// Verify target DB has records
		targetCheck, err := sql.Open("sqlite", targetDB)
		if err != nil {
			t.Fatal(err)
		}
		defer targetCheck.Close()

		var subCount, revCount int
		if err := targetCheck.QueryRow("SELECT COUNT(*) FROM subscriptions").Scan(&subCount); err != nil {
			t.Fatal(err)
		}
		if subCount != 1 {
			t.Errorf("expected 1 subscription imported, got %d", subCount)
		}
		if err := targetCheck.QueryRow("SELECT COUNT(*) FROM configuration_revisions WHERE state = 'draft'").Scan(&revCount); err != nil {
			t.Fatal(err)
		}
		if revCount != 1 {
			t.Errorf("expected 1 draft revision, got %d", revCount)
		}
	})

	t.Run("legacy-import unknown format", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"legacy-import", "-s", sourceDB, "--dry-run", "-f", "xml"}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("expected exit code 2, got %d", code)
		}
		if !strings.Contains(stderr.String(), "unknown format") {
			t.Errorf("expected unknown format error, got: %s", stderr.String())
		}
	})

	t.Run("legacy-import non-existent source", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"legacy-import", "-s", filepath.Join(tempDir, "not_exist.db"), "--dry-run"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "cannot access legacy source database") {
			t.Errorf("expected cannot access error, got: %s", stderr.String())
		}
	})
}

func TestServeCLI(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "serve_test.db")

	t.Run("serve help flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"serve", "--help"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0 on --help, got %d", code)
		}
	})

	t.Run("serve starts up and responds to healthz and readyz with graceful shutdown", func(t *testing.T) {
		// Find an available local port
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to find free port: %v", err)
		}
		addr := l.Addr().String()
		_ = l.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var stdout, stderr bytes.Buffer
		serveDone := make(chan int, 1)

		go func() {
			exitCode := runServeWithContext(ctx, []string{"-addr", addr, "-db", dbPath}, &stdout, &stderr)
			serveDone <- exitCode
		}()

		baseURL := fmt.Sprintf("http://%s", addr)
		client := &http.Client{Timeout: 2 * time.Second}

		// Poll until server is responding
		var healthzOK, readyzOK bool
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)

			// Check /healthz
			resp, err := client.Get(baseURL + "/healthz")
			if err != nil {
				continue
			}
			if resp.StatusCode == http.StatusOK {
				var body map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
					data, ok := body["data"].(map[string]any)
					if ok && data["status"] == "ok" {
						healthzOK = true
					}
				}
			}
			_ = resp.Body.Close()

			// Check /readyz
			resp, err = client.Get(baseURL + "/readyz")
			if err == nil {
				if resp.StatusCode == http.StatusOK {
					var body map[string]any
					if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
						data, ok := body["data"].(map[string]any)
						if ok && data["ready"] == true {
							readyzOK = true
						}
					}
				}
				_ = resp.Body.Close()
			}

			if healthzOK && readyzOK {
				break
			}
		}

		if !healthzOK {
			t.Fatalf("/healthz check failed or timed out. Stdout: %s, Stderr: %s", stdout.String(), stderr.String())
		}
		if !readyzOK {
			t.Fatalf("/readyz check failed or timed out. Stdout: %s, Stderr: %s", stdout.String(), stderr.String())
		}

		// Verify 410 Gone interceptor on legacy endpoint
		resp410, err := client.Get(baseURL + "/yaml")
		if err != nil {
			t.Fatalf("failed to query /yaml: %v", err)
		}
		if resp410.StatusCode != http.StatusGone {
			t.Errorf("expected /yaml to return 410 Gone, got %d", resp410.StatusCode)
		}
		_ = resp410.Body.Close()

		// Trigger graceful shutdown
		cancel()

		select {
		case code := <-serveDone:
			if code != 0 {
				t.Fatalf("expected graceful exit code 0, got %d. Stderr: %s", code, stderr.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatal("server shutdown timed out")
		}

		// Verify database was migrated and created on disk
		if _, err := os.Stat(dbPath); err != nil {
			t.Fatalf("expected db file to exist on disk: %v", err)
		}

		// Verify warning emitted when credential master key is unconfigured
		if !strings.Contains(stderr.String(), "warning: node credential master key not configured") {
			t.Errorf("expected warning about unconfigured credential master key, got stderr: %s", stderr.String())
		}
	})

	t.Run("serve fails immediately with invalid node credential key without leaking key", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidDBPath := filepath.Join(tempDir, "invalid_key.db")
		secretKey := "this-is-an-invalid-node-credential-key-secret-9999"
		t.Setenv("CSP_NODE_CREDENTIAL_KEY", secretKey)

		var stdout, stderr bytes.Buffer
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		exitCode := runServeWithContext(ctx, []string{"-addr", "127.0.0.1:0", "-db", invalidDBPath}, &stdout, &stderr)
		if exitCode != 1 {
			t.Fatalf("expected exit code 1 on invalid node credential key, got %d", exitCode)
		}

		if !strings.Contains(stderr.String(), "node credential vault initialization failed") {
			t.Errorf("expected stderr to contain vault initialization failure, got: %s", stderr.String())
		}
		if strings.Contains(stderr.String(), secretKey) {
			t.Errorf("security violation: stderr leaked credential secret: %s", stderr.String())
		}
		if strings.Contains(stdout.String(), secretKey) {
			t.Errorf("security violation: stdout leaked credential secret: %s", stdout.String())
		}
	})

	t.Run("serve starts up successfully with valid node credential key", func(t *testing.T) {
		tempDir := t.TempDir()
		validDBPath := filepath.Join(tempDir, "valid_key.db")
		validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		t.Setenv("CSP_NODE_CREDENTIAL_KEY", validKey)

		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to find free port: %v", err)
		}
		addr := l.Addr().String()
		_ = l.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var stdout, stderr bytes.Buffer
		serveDone := make(chan int, 1)

		go func() {
			exitCode := runServeWithContext(ctx, []string{"-addr", addr, "-db", validDBPath}, &stdout, &stderr)
			serveDone <- exitCode
		}()

		baseURL := fmt.Sprintf("http://%s", addr)
		client := &http.Client{Timeout: 2 * time.Second}

		var healthzOK bool
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
			resp, err := client.Get(baseURL + "/healthz")
			if err != nil {
				continue
			}
			if resp.StatusCode == http.StatusOK {
				healthzOK = true
				_ = resp.Body.Close()
				break
			}
			_ = resp.Body.Close()
		}

		if !healthzOK {
			t.Fatalf("/healthz check failed with valid key. Stdout: %s, Stderr: %s", stdout.String(), stderr.String())
		}

		cancel()

		select {
		case code := <-serveDone:
			if code != 0 {
				t.Fatalf("expected graceful exit code 0, got %d. Stderr: %s", code, stderr.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatal("server shutdown timed out")
		}

		if strings.Contains(stderr.String(), "warning: node credential master key not configured") {
			t.Errorf("did not expect unconfigured warning when key is set, got: %s", stderr.String())
		}
		if strings.Contains(stderr.String(), validKey) {
			t.Errorf("security violation: stderr leaked valid credential key: %s", stderr.String())
		}
		if strings.Contains(stdout.String(), validKey) {
			t.Errorf("security violation: stdout leaked valid credential key: %s", stdout.String())
		}
	})
}
