package inventory_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

// Exercise the actual reconcile path without a network or production data.
func TestScaleReconcileRepeated(t *testing.T) {
	db, subRepo, svc, fetcher := setupOriginalTestDB(t)
	id := domain.MustNewUUIDv7()
	url := "https://scale.invalid/sub"
	createOrigSub(t, subRepo, id, "scale", url)
	var config strings.Builder
	config.WriteString("proxies:\n")
	for i := 0; i < 1001; i++ {
		fmt.Fprintf(&config, "  - {name: node-%d, type: ss, server: 192.0.2.1, port: %d, cipher: aes-128-gcm, password: synthetic}\n", i, 10000+i)
	}
	body := []byte(config.String())
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: body, ContentDigest: "scale-synthetic"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	for iteration := 0; iteration < 3; iteration++ {
		started := time.Now()
		before := db.Stats()
		result, err := svc.ReconcileSubscription(ctx, id)
		after := db.Stats()
		t.Logf("reconcile=%d elapsed=%s pool_waits=%d pool_wait_duration=%s error=%v", iteration, time.Since(started), after.WaitCount-before.WaitCount, after.WaitDuration-before.WaitDuration, err)
		if err != nil || result == nil || result.NodesValid != 1001 {
			t.Fatalf("reconcile %d: result=%+v error=%v", iteration, result, err)
		}
		var sources, nodes int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM node_sources WHERE subscription_id = ?`, id).Scan(&sources); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM nodes`).Scan(&nodes); err != nil {
			t.Fatal(err)
		}
		if sources != 1001 || nodes != 1001 {
			t.Fatalf("iteration %d: sources=%d nodes=%d", iteration, sources, nodes)
		}
		orig, err := svc.GetOriginalConfig(ctx, id)
		if err != nil || orig == nil || !bytes.Equal(orig.Body, body) {
			t.Fatalf("iteration %d: original mismatch: %v", iteration, err)
		}
	}
}
