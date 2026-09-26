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

		// Verify no credential master key warning is emitted
		if strings.Contains(stderr.String(), "node credential master key") {
			t.Errorf("did not expect credential master key warning, got stderr: %s", stderr.String())
		}
	})
}
