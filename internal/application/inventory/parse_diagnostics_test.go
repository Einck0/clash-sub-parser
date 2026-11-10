package inventory_test

import (
	"context"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

func TestParseDiagnosticsPersistPartialAndTotalFailureWithoutSecrets(t *testing.T) {
	ctx := context.Background()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	fetcher := newMockFetcher()
	svc := inventory.NewService(db, subs, fetches, nodes, sources, fetcher)
	id, url := domain.MustNewUUIDv7(), "https://parse-diagnostics.invalid/feed"
	createTestSub(t, subs, id, "Diagnostics", url, true)
	valid := " - {name: Valid, type: ss, server: 192.0.2.33, port: 8388, cipher: aes-128-gcm, password: SECRET_PASSWORD_SENTINEL}\n"
	bad := " - SECRET_SCALAR_SENTINEL\n - {type: SECRET_PROTOCOL_SENTINEL, name: SECRET_NAME_SENTINEL}\n - {type: wireguard, server: 192.0.2.1, port: 51820, private-key: SECRET_KEY_SENTINEL, public-key: pub, ip: SECRET_IP_SENTINEL}\n"
	body := []byte("proxies:\n" + valid + bad)
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: body, ContentDigest: "partial-diagnostics"})
	partial, err := svc.ReconcileSubscription(ctx, id)
	if err != nil || partial == nil || partial.Outcome != domain.FetchOutcomePartial || partial.NodesParsed != 4 || partial.NodesValid != 1 {
		t.Fatalf("partial result=%+v error=%v", partial, err)
	}
	checkStored := func(fetchID, diagnostic string, parsed, valid int) {
		t.Helper()
		var stored string
		var gotParsed, gotValid int
		if err := db.QueryRowContext(ctx, "SELECT redacted_error, nodes_parsed, nodes_valid FROM subscription_fetches WHERE id = ?", fetchID).Scan(&stored, &gotParsed, &gotValid); err != nil {
			t.Fatal(err)
		}
		if stored != diagnostic || gotParsed != parsed || gotValid != valid {
			t.Fatalf("persisted diagnostic/count mismatch: %s %d/%d", stored, gotParsed, gotValid)
		}
		for _, field := range []string{"invalid_entry", "unsupported_protocol", "invalid_field", "local_address"} {
			if !strings.Contains(stored, field) {
				t.Errorf("diagnostic missing %s", field)
			}
		}
		if strings.Contains(stored, "SECRET") || strings.Contains(stored, "192.0.2") || len(stored) > 8192 {
			t.Fatal("unsafe/unbounded diagnostic")
		}
	}
	checkStored(partial.FetchID, partial.RedactedError, 4, 1)
	original, err := svc.GetOriginalConfig(ctx, id)
	if err != nil || string(original.Body) != string(body) {
		t.Fatalf("partial original not preserved: %v", err)
	}

	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte("proxies:\n" + bad), ContentDigest: "failed-diagnostics"})
	failed, err := svc.ReconcileSubscription(ctx, id)
	if err == nil || failed == nil || failed.Outcome != domain.FetchOutcomeFailed || failed.NodesParsed != 3 || failed.NodesValid != 0 {
		t.Fatalf("total failure=%+v error=%v", failed, err)
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatal("returned error leaked subscription")
	}
	checkStored(failed.FetchID, failed.RedactedError, 3, 0)
	original, err = svc.GetOriginalConfig(ctx, id)
	if err != nil || string(original.Body) != string(body) {
		t.Fatalf("failure replaced last successful original: %v", err)
	}
	var lastFetch string
	if err := db.QueryRowContext(ctx, "SELECT last_seen_fetch_id FROM node_sources WHERE subscription_id = ?", id).Scan(&lastFetch); err != nil || lastFetch != partial.FetchID {
		t.Fatalf("failure changed source: %q %v", lastFetch, err)
	}
}
