package main_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestSingleBinary_EndToEndLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping single-binary e2e test in short mode")
	}

	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "clash-sub-parser")

	// 1. Build the binary with CGO_ENABLED=0 and stripped symbols
	buildCmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", binPath, ".")
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	buildCmd.Dir = "."
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\nOutput: %s", err, string(buildOut))
	}

	// Verify binary exists and size is reasonable (>20MB due to embedded assets, <60MB)
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("stat binary failed: %v", err)
	}
	sizeMB := float64(info.Size()) / (1024 * 1024)
	t.Logf("Compiled binary size: %.2f MB", sizeMB)
	if sizeMB < 15.0 || sizeMB > 60.0 {
		t.Errorf("binary size %.2f MB outside expected 15-60 MB range", sizeMB)
	}

	// 2. Start binary on an ephemeral port
	port := getFreePort(t)
	dbPath := filepath.Join(tempDir, "test.db")

	cmd := exec.Command(binPath, "-port", fmt.Sprintf("%d", port), "-db", dbPath, "-bind", "127.0.0.1")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}

	// Ensure cleanup
	processExited := make(chan error, 1)
	go func() {
		processExited <- cmd.Wait()
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Wait for server ready with polling on /health
	client := &http.Client{Timeout: 2 * time.Second}
	ready := false
	for i := 0; i < 40; i++ {
		time.Sleep(100 * time.Millisecond)
		resp, err := client.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			ready = true
			_ = resp.Body.Close()
			break
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
	if !ready {
		_ = cmd.Process.Kill()
		t.Fatalf("server failed to start within timeout.\nStderr: %s\nStdout: %s", stderr.String(), stdout.String())
	}

	// 3. Test GET /health
	t.Run("GET /health", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("GET /health failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got: %d", resp.StatusCode)
		}
		var res map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode health response: %v", err)
		}
		if res["status"] != "ok" {
			t.Errorf("expected status: ok, got: %v", res["status"])
		}
	})

	// 4. Test GET / (Root SPA Index)
	t.Run("GET / Root SPA", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/")
		if err != nil {
			t.Fatalf("GET / failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got: %d", resp.StatusCode)
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "text/html") {
			t.Errorf("expected text/html, got: %s", ct)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "<html") {
			t.Errorf("expected HTML content in root response")
		}
	})

	// 5. Test GET /subscriptions (SPA Fallback)
	t.Run("GET /subscriptions Fallback", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/subscriptions")
		if err != nil {
			t.Fatalf("GET /subscriptions failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got: %d", resp.StatusCode)
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "text/html") {
			t.Errorf("expected text/html, got: %s", ct)
		}
	})

	// 6. Test GET /assets/ static file
	t.Run("GET /assets/index-CetPrhTk.js", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/assets/index-CetPrhTk.js")
		if err != nil {
			t.Fatalf("GET asset failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got: %d", resp.StatusCode)
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "application/javascript") {
			t.Errorf("expected javascript, got: %s", ct)
		}
	})

	// 7. Test Export /clash
	t.Run("GET /clash export", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/clash")
		if err != nil {
			t.Fatalf("GET /clash failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got: %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "proxies:") {
			t.Errorf("expected clash config body")
		}
	})

	// 8. Test Graceful Shutdown on SIGTERM
	t.Run("Graceful Shutdown on SIGTERM", func(t *testing.T) {
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("failed to send SIGTERM: %v", err)
		}

		select {
		case err := <-processExited:
			if err != nil {
				t.Fatalf("process exited with non-zero status: %v", err)
			}
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatalf("process failed to exit gracefully within 10 seconds")
		}
	})
}
