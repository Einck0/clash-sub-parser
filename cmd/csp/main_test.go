package main

import (
	"bytes"
	"context"
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
			name:       "legacy-inspect rejected as unknown command",
			args:       []string{"legacy-inspect"},
			wantCode:   1,
			wantStderr: "unknown command: legacy-inspect",
		},
		{
			name:       "legacy-import rejected as unknown command",
			args:       []string{"legacy-import"},
			wantCode:   1,
			wantStderr: "unknown command: legacy-import",
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
