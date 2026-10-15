package inventory_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

const routeTestYAML = "proxies:\n  - {name: Route, type: ss, server: node.example, port: 8388, cipher: aes-128-gcm, password: fixture}\n"

func TestSubscriptionFetchRouteLiveSQLiteHTTP(t *testing.T) {
	ctx := context.Background()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	settings := sqlite.NewSettingsRepository(db)
	var originHits, envHits, customHits, overrideHits atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { originHits.Add(1); fmt.Fprint(w, routeTestYAML) }))
	defer origin.Close()
	var block atomic.Bool
	entered, release := make(chan struct{}, 1), make(chan struct{})
	envProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envHits.Add(1)
		if block.Load() {
			entered <- struct{}{}
			<-release
		}
		fmt.Fprint(w, routeTestYAML)
	}))
	defer envProxy.Close()
	customProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		customHits.Add(1)
		if r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("fixture-user:fixture-password")) {
			t.Error("proxy credentials not transmitted")
		}
		fmt.Fprint(w, routeTestYAML)
	}))
	defer customProxy.Close()
	overrideProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { overrideHits.Add(1); fmt.Fprint(w, routeTestYAML) }))
	defer overrideProxy.Close()
	// Loopback target exemption is ONLY a test fixture; production defaults retain public-IP policy.
	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{"127.0.0.1"}
	_ = policy.AddAllowedProxy("http://127.0.0.1:1")
	client := fetch.NewClientWithPolicy(policy)
	svc := inventory.NewService(db, subs, fetches, nodes, sources, client, inventory.WithFetchSettingsRepository(settings), inventory.WithDefaultFetchProxy(envProxy.URL))
	sub := createTestSub(t, subs, domain.MustNewUUIDv7(), "Route", origin.URL, true)
	run := func() {
		t.Helper()
		result, err := svc.ReconcileSubscription(ctx, sub.ID)
		if err != nil || result.NodesValid != 1 {
			t.Fatalf("reconcile: %+v %v", result, err)
		}
	}
	set := func(mode string, url *string) {
		t.Helper()
		if err := settings.UpdateSubscriptionFetch(ctx, mode, url); err != nil {
			t.Fatal(err)
		}
	}
	run()
	if envHits.Load() != 1 || originHits.Load() != 0 {
		t.Fatal("environment did not use inherited proxy")
	}
	t.Setenv("HTTP_PROXY", envProxy.URL)
	t.Setenv("HTTPS_PROXY", envProxy.URL)
	set("direct", nil)
	run()
	if originHits.Load() != 1 || envHits.Load() != 1 {
		t.Fatal("direct inherited environment proxy")
	}
	customURL := strings.Replace(customProxy.URL, "://", "://fixture-user:fixture-password@", 1)
	set("proxy", &customURL)
	run()
	if customHits.Load() != 1 {
		t.Fatal("runtime custom proxy not applied")
	}
	override := *sub
	override.RefreshPolicy.FetchProxyRef = overrideProxy.URL
	if err := subs.Update(ctx, &override); err != nil {
		t.Fatal(err)
	}
	// Another private proxy is NOT implicitly trusted by a previous/custom configuration.
	result, err := svc.ReconcileSubscription(ctx, sub.ID)
	de, ok := domain.AsDomainError(err)
	if err == nil || !ok || de.Code != "proxy_ssrf_blocked" || result.Outcome != domain.FetchOutcomeFailed || overrideHits.Load() != 0 {
		t.Fatalf("untrusted override: %+v %v", result, err)
	}
	set("environment", nil)
	if err := subs.Update(ctx, sub); err != nil {
		t.Fatal(err)
	}
	// One in-flight fetch keeps its initial policy/route while a save changes the next fetch.
	block.Store(true)
	done := make(chan error, 1)
	go func() { _, err := svc.ReconcileSubscription(ctx, sub.ID); done <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight proxy not reached")
	}
	set("proxy", &customURL)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	run()
	if envHits.Load() != 2 || customHits.Load() != 2 {
		t.Fatalf("incoherent routing env=%d custom=%d", envHits.Load(), customHits.Load())
	}
	// Explicit override has precedence (here the previously configured env proxy); old private trust must not accumulate.
	override.RefreshPolicy.FetchProxyRef = envProxy.URL
	if err := subs.Update(ctx, &override); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ReconcileSubscription(ctx, sub.ID)
	de, ok = domain.AsDomainError(err)
	if !ok || de.Code != "proxy_ssrf_blocked" {
		t.Fatalf("old proxy remained trusted: %v", err)
	}
	// A direct subscription override is usable when it is the exact currently trusted relay.
	override.RefreshPolicy.FetchProxyRef = customURL
	if err := subs.Update(ctx, &override); err != nil {
		t.Fatal(err)
	}
	run()
	if customHits.Load() != 3 {
		t.Fatal("explicit override ignored")
	}
	var sourceCount, nodeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM node_sources WHERE subscription_id=?", sub.ID).Scan(&sourceCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&nodeCount); err != nil {
		t.Fatal(err)
	}
	if sourceCount != 1 || nodeCount != 1 {
		t.Fatalf("failed fetch mutated assets: sources=%d nodes=%d", sourceCount, nodeCount)
	}
}
