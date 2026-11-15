package inventory_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

func TestInventoryTargetCompatibilityIsReadScopedAndRefreshesAfterSave(t *testing.T) {
	ctx := context.Background()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	fetcher := newMockFetcher()
	id, url := domain.MustNewUUIDv7(), "https://compatibility.invalid/feed"
	createTestSub(t, subs, id, "Compatibility", url, true)
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, ContentDigest: "compatibility", Body: []byte(`proxies:
 - {name: Legacy, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: secret-sentinel, skip-cert-verify: true}
 - {name: Valid, type: ss, server: 192.0.2.2, port: 8388, cipher: aes-128-gcm, password: test}
`)})
	svc := inventory.NewService(db, subs, fetches, nodes, sources, fetcher)
	result, err := svc.ReconcileSubscription(ctx, id)
	if err != nil || result == nil || result.NodesValid != 2 || result.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	views, total, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 2 || len(views) != 2 {
		t.Fatalf("views=%+v total=%d err=%v", views, total, err)
	}
	var legacyID string
	for _, view := range views {
		want := "compatible"
		if view.DisplayName == "Legacy" {
			want = "incompatible"
			legacyID = view.LogicalID
		}
		capability := view.TargetCompatibility[domain.TargetMihomo]
		if capability.Status != want || !view.Active || len(view.Sources) != 1 {
			t.Fatalf("unexpected view: %+v", view)
		}
		if view.DisplayName == "Legacy" {
			if len(capability.Diagnostics) == 0 || capability.Diagnostics[0].Target != "mihomo" {
				t.Fatalf("missing scoped diagnosis: %+v", capability)
			}
			encoded, _ := json.Marshal(capability)
			if strings.Contains(string(encoded), "secret-sentinel") {
				t.Fatal("target diagnosis leaked credentials")
			}
		}
	}
	if legacyID == "" {
		t.Fatal("legacy node disappeared")
	}
	detail, err := svc.GetNodeDetail(ctx, legacyID)
	if err != nil || detail == nil || detail.ToNodeView().TargetCompatibility[domain.TargetMihomo].Status != "incompatible" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	updated, err := svc.UpdateNodeConnection(ctx, inventory.UpdateNodeConnectionCommand{
		LogicalID: legacyID,
		Patch:     inventory.NodePatchRequest{Transport: map[string]string{}},
	})
	if err != nil || updated == nil || updated.ToNodeView().TargetCompatibility[domain.TargetMihomo].Status != "compatible" {
		t.Fatalf("saved detail=%+v err=%v", updated, err)
	}
	reloaded, err := svc.GetNodeDetail(ctx, legacyID)
	if err != nil || reloaded == nil || reloaded.ToNodeView().TargetCompatibility[domain.TargetMihomo].Status != "compatible" {
		t.Fatalf("reloaded detail=%+v err=%v", reloaded, err)
	}
}

func TestInventoryFormatErrorsAreAuditableDisplayOnlyEntries(t *testing.T) {
	ctx := context.Background()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	fetcher := newMockFetcher()
	id, url := domain.MustNewUUIDv7(), "https://format.invalid/feed"
	createTestSub(t, subs, id, "Format errors", url, true)
	svc := inventory.NewService(db, subs, fetches, nodes, sources, fetcher)
	for i, tc := range []struct {
		body, reason string
		outcome      domain.FetchOutcome
	}{
		{"proxies:\n - {name: Valid, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: test}\n - {name: SECRET_NAME, type: ss, server: 192.0.2.2, cipher: aes-128-gcm, password: SECRET_PASSWORD}\n", "partial_subscription", domain.FetchOutcomePartial},
		{"proxies:\n - SECRET_SCALAR\n", "parse_failed", domain.FetchOutcomeFailed},
		{"proxies: []\n", "parse_failed", domain.FetchOutcomeFailed},
	} {
		fetcher.setResponse(url, &fetch.Response{StatusCode: 200, ContentDigest: "format", Body: []byte(tc.body)})
		result, reconcileErr := svc.ReconcileSubscription(ctx, id)
		if result == nil || result.Outcome != tc.outcome || !strings.Contains(result.RedactedError, "reason_code:"+tc.reason) || (reconcileErr != nil) != (tc.outcome == domain.FetchOutcomeFailed) {
			t.Fatalf("wrong persisted scenario: result=%+v err=%v", result, reconcileErr)
		}
		// Mixed precision makes the older whole-second timestamp sort after later fractions as text.
		startedAt := []string{"2026-11-15T00:00:00Z", "2026-11-15T00:00:00.500Z", "2026-11-15T00:00:00.750Z"}[i]
		if _, err := db.ExecContext(ctx, `UPDATE subscription_fetches SET started_at=? WHERE id=?`, startedAt, result.FetchID); err != nil {
			t.Fatal(err)
		}
		entries, err := svc.ListLatestEntries(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		var errors int
		for _, entry := range entries {
			if entry.EffectiveKind() != domain.EntryKindFormatError {
				continue
			}
			errors++
			if entry.NodeLogicalID != nil || entry.SubscriptionID != id || !strings.Contains(entry.ClassificationReason, result.FetchID) || entry.ParsedConfigJSON != "" || entry.Server != "" || entry.Port != 0 {
				t.Fatalf("error lost display-only provenance: %+v", entry)
			}
			var provenance struct {
				Format string `json:"format"`
				Reason string `json:"reason"`
				Field  string `json:"field"`
			}
			if err := json.Unmarshal([]byte(entry.SourceProvenanceJSON), &provenance); err != nil || provenance.Format == "" || provenance.Reason == "" || provenance.Field == "" {
				t.Fatalf("missing structured safe provenance: %+v err=%v", provenance, err)
			}
			encoded, _ := json.Marshal(entry)
			for _, private := range []string{"SECRET", "192.0.2.2", url} {
				if strings.Contains(string(encoded), private) {
					t.Fatal("format diagnosis leaked source text")
				}
			}
			proxy := domain.EntryKindProxy
			if _, err := svc.OverrideEntryKind(ctx, entry.ID, &proxy, "", "test"); err == nil {
				t.Fatal("format error can be promoted to proxy")
			}
		}
		if errors != 1 {
			t.Fatalf("expected one visible format error, got %+v", entries)
		}
		active, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
		if err != nil || total != 1 || len(active) != 1 || active[0].DisplayName != "Valid" {
			t.Fatalf("format error changed valid membership: %+v total=%d err=%v", active, total, err)
		}
	}
}

func TestFormatErrorIsAValidDisplayKindButNotAnOverride(t *testing.T) {
	if !domain.EntryKindFormatError.IsValid() || domain.EntryKindFormatError.IsOverrideValid() {
		t.Fatal("format errors need a distinct display-only contract")
	}
	kind := domain.EntryKindFormatError
	entry := domain.SubscriptionEntry{EntryKind: domain.EntryKindProxy, UserKindOverride: &kind}
	if entry.EffectiveKind() != domain.EntryKindProxy {
		t.Fatal("format error was accepted as an override")
	}
	proxy := domain.EntryKindProxy
	stored := domain.SubscriptionEntry{EntryKind: domain.EntryKindFormatError, UserKindOverride: &proxy}
	if stored.EffectiveKind() != domain.EntryKindFormatError {
		t.Fatal("stored format error was promoted by a historical override")
	}
}

func TestInventoryStoredMalformedNodeHasGlobalFormatError(t *testing.T) {
	for _, node := range []domain.Node{
		{LogicalID: "legacy-corrupt", DisplayName: "Missing credentials", Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 8388},
		{LogicalID: "legacy-unnamed", Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 8388, Credentials: domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "test"}},
	} {
		view := inventory.ToNodeView(node)
		for target, status := range view.TargetCompatibility {
			if status.Status != "format_error" || len(status.Diagnostics) != 1 || status.Diagnostics[0].Code != "format_error" {
				t.Fatalf("%s: corrupt node was confused with target support: %+v", target, status)
			}
		}
	}
}

func TestInventoryTargetCompatibilityDoesNotApplyMihomoStatusToOtherTargets(t *testing.T) {
	view := inventory.ToNodeView(domain.Node{
		LogicalID: "vless", DisplayName: "VLESS", Protocol: domain.ProtocolVLESS,
		Server: "192.0.2.1", Port: 443,
		Credentials: domain.InboundProtocolCredential{UUID: "00000000-0000-4000-8000-000000000001"},
	})
	if view.TargetCompatibility[domain.TargetMihomo].Status != "compatible" || view.TargetCompatibility[domain.TargetSurge].Status != "incompatible" {
		t.Fatalf("target status is not independent: %+v", view.TargetCompatibility)
	}
}
