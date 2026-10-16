package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/subscription"
)

type shutdownReconciler struct {
	run func(context.Context) (*inventory.ReconcileResult, error)
}

func (r shutdownReconciler) ReconcileSubscription(ctx context.Context, _ string) (*inventory.ReconcileResult, error) {
	return r.run(ctx)
}

// A manual refresh is an HTTP-owned worker, not a subscription-scheduler worker.
// Hold its cancellation cleanup past Shutdown's deadline to exercise that boundary.
func TestServeManualRefreshDatabaseLifetime(t *testing.T) {
	for _, mode := range []string{"graceful", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := listener.Addr().String()
			_ = listener.Close()
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, cancelled, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			cleanupResult := make(chan error, 1)
			serveDone := make(chan int, 1)
			var releaseOnce sync.Once
			releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
			configured := make(chan struct{})
			var db *sql.DB
			var subID string
			var closeCalls atomic.Int32
			var stdout, stderr bytes.Buffer
			hooks := &serveDependencies{
				shutdownTimeout: 3 * time.Second,
				configureServices: func(handle *sql.DB, services *applicationServices) {
					db = handle
					sub, err := services.subService.Create(context.Background(), subscription.CreateSubscriptionCommand{
						Name: "shutdown fixture", SourceURLSecretRef: "https://synthetic.invalid/subscription", Enabled: true,
					})
					if err != nil {
						panic(err)
					}
					subID = sub.ID
					services.subService.SetReconciler(shutdownReconciler{run: func(ctx context.Context) (*inventory.ReconcileResult, error) {
						defer close(finished)
						close(entered)
						<-ctx.Done()
						close(cancelled)
						<-release
						// Real post-cancellation cleanup must still be able to use SQLite.
						_, err := handle.ExecContext(context.Background(), "CREATE TABLE manual_refresh_cleanup (done INTEGER)")
						cleanupResult <- err
						return nil, ctx.Err()
					}})
					close(configured)
				},
				closeDBFn: func(handle *sql.DB) error { closeCalls.Add(1); return handle.Close() },
			}
			if mode == "timeout" {
				hooks.shutdownTimeout = 50 * time.Millisecond
			}
			dbPath := filepath.Join(t.TempDir(), "manual.db")
			go func() {
				serveDone <- runServeWithDependencies(parent, []string{"-addr", addr, "-db", dbPath, "-admin-token=", "-disable-subscription-scheduler", "-disable-probe-coordinator"}, &stdout, &stderr, hooks)
			}()
			defer func() {
				cancel()
				releaseHandler()
				if db != nil {
					// Failure deliberately leaves the handle open; the test owns final cleanup.
					<-finished
					_ = db.Close()
				}
			}()
			wait := func(ch <-chan struct{}, name string) {
				t.Helper()
				select {
				case <-ch:
				case <-time.After(5 * time.Second):
					t.Fatalf("timed out waiting for %s", name)
				}
			}
			wait(configured, "services")
			requestDone := make(chan error, 1)
			client := &http.Client{Timeout: 5 * time.Second}
			defer client.CloseIdleConnections()
			deadline := time.Now().Add(5 * time.Second)
			for {
				resp, err := client.Get("http://" + addr + "/healthz")
				if err == nil {
					_ = resp.Body.Close()
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("listener did not become ready")
				}
				time.Sleep(10 * time.Millisecond)
			}
			requestCtx, cancelRequest := context.WithCancel(context.Background())
			defer cancelRequest()
			go func() {
				req, _ := http.NewRequestWithContext(requestCtx, http.MethodPost, "http://"+addr+"/api/v1/subscriptions/"+subID+"/refresh", nil)
				req.Header.Set("Idempotency-Key", "shutdown-manual-fixture")
				resp, err := client.Do(req)
				if err == nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
				requestDone <- err
			}()
			wait(entered, "manual refresh")
			cancel()
			cancelRequest()
			wait(cancelled, "handler cancellation")
			if mode == "graceful" {
				select {
				case code := <-serveDone:
					t.Fatalf("serve returned %d before HTTP cleanup", code)
				default:
				}
				if closeCalls.Load() != 0 {
					t.Fatal("database closed while manual HTTP handler was active")
				}
				releaseHandler()
			}
			var code int
			select {
			case code = <-serveDone:
			case <-time.After(5 * time.Second):
				t.Fatal("serve did not return")
			}
			if mode == "graceful" {
				if code != 0 || closeCalls.Load() != 1 || db.Ping() == nil {
					t.Fatalf("graceful exit=%d close calls=%d, want closed DB; stderr=%s", code, closeCalls.Load(), stderr.String())
				}
			} else {
				if code != 1 || closeCalls.Load() != 0 {
					t.Errorf("failed HTTP shutdown exit=%d close calls=%d, want 1/0", code, closeCalls.Load())
				}
				if err := db.Ping(); err != nil {
					t.Errorf("DB not retained after uncertain HTTP shutdown: %v", err)
				}
				if strings.Contains(stdout.String(), "stopped cleanly") || strings.Contains(stdout.String(), "probe scheduler drained") {
					t.Errorf("HTTP failure claimed unrelated drain/clean success: %s", stdout.String())
				}
				releaseHandler()
			}
			wait(finished, "handler cleanup")
			if err := <-cleanupResult; err != nil {
				t.Errorf("post-cancellation SQLite cleanup failed: %v", err)
			}
			select {
			case err := <-requestDone:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("manual request transport = %v, want cancellation", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("manual request did not finish")
			}
		})
	}
}
