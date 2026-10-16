package e2e_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

// Uses only an in-memory migrated database and httptest; never contacts an upstream or production.
func TestManualAndPrunedInventoryPolicyExportAndPrefixImmutability(t *testing.T) {
	h := setupTestHarness(t)
	ctx := context.Background()
	now := domain.NowUTC()
	// Deterministic synthetic 32-byte keys, unrelated to the historical WARP account.
	privateKey := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("p", 32)))
	publicKey := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("q", 32)))
	manual := domain.Node{LogicalID: "manual-warp-fixture", Protocol: domain.ProtocolWireGuard, DisplayName: "WARP-manual-fixture", Server: "198.51.100.8", Port: 2408,
		Credentials: domain.InboundProtocolCredential{PrivateKey: privateKey, PublicKey: publicKey, LocalAddress: []string{"10.0.0.2/32"}}, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := h.NodeRepo.UpsertBatch(ctx, []domain.Node{manual}); err != nil {
		t.Fatal(err)
	}
	url := "https://pruned.example/feed"
	payload := func(name, server string) *fetch.Response {
		return &fetch.Response{StatusCode: 200, ContentType: "text/yaml", ContentDigest: name, Body: []byte(fmt.Sprintf("proxies:\n  - name: %q\n    type: ss\n    server: %s\n    port: 8388\n    cipher: aes-128-gcm\n    password: synthetic-fixture\n", name, server))}
	}
	sub, err := h.SubService.Create(ctx, subscription.CreateSubscriptionCommand{Name: "Pruning fixture", SourceURLSecretRef: url, Enabled: true, Prefix: "A "})
	if err != nil {
		t.Fatal(err)
	}
	h.Fetcher.setResponse(url, payload("WARP-pruned-fixture", "198.51.100.10"))
	if _, err := h.InvService.ReconcileSubscription(ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	h.Fetcher.setResponse(url, payload("WARP-current-fixture", "198.51.100.11"))
	if _, err := h.InvService.ReconcileSubscription(ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	pattern := "WARP"
	group, err := h.PolicyService.CreateGroup(ctx, policy.CreateGroupCommand{Name: "WARP", GroupType: domain.GroupTypeSelect,
		Sources: []domain.MemberSource{{Kind: domain.SourceKindRegex, Operation: domain.OperationAdd, Pattern: &pattern}}})
	if err != nil {
		t.Fatal(err)
	}
	rev, err := h.PolicyService.EnsureActiveRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.PolicyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{ID: domain.MustNewUUIDv7(), RevisionID: rev.ID, TargetGroupID: group.ID, Expression: "MATCH"}); err != nil {
		t.Fatal(err)
	}
	preview, err := h.PublicationService.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo})
	if err != nil {
		t.Fatal(err)
	}
	content := string(preview.Content)
	if !strings.Contains(content, manual.DisplayName) || !strings.Contains(content, "A WARP-current-fixture") || strings.Contains(content, "WARP-pruned-fixture") {
		t.Fatalf("wrong policy/export scope: %s", content)
	}
	published, err := h.PublicationService.Publish(ctx, publication.PublishCommand{Target: domain.TargetMihomo, SnapshotID: preview.SnapshotID})
	if err != nil {
		t.Fatal(err)
	}
	original, err := h.Request(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", published.Publication.ID, published.RawToken), nil, nil, nil)
	if err != nil || original.StatusCode != 200 {
		t.Fatalf("export: %v %v", original, err)
	}
	prefix := "B "
	updated, err := h.SubService.Update(ctx, subscription.UpdateSubscriptionCommand{ID: sub.ID, Revision: sub.Revision, Prefix: &prefix})
	if err != nil {
		t.Fatal(err)
	}
	newPreview, err := h.PublicationService.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo})
	if err != nil || !strings.Contains(string(newPreview.Content), "B WARP-current-fixture") {
		t.Fatalf("retroactive prefix missing: %v", err)
	}
	if err := h.SubService.Delete(ctx, subscription.DeleteSubscriptionCommand{ID: updated.ID}); err != nil {
		t.Fatal(err)
	}
	manualPreview, err := h.PublicationService.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manualPreview.Content), manual.DisplayName) || strings.Contains(string(manualPreview.Content), "current-fixture") || strings.Contains(string(manualPreview.Content), "pruned-fixture") {
		t.Fatalf("manual node not retained alone in policy/export: %s", manualPreview.Content)
	}
	after, err := h.Request(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", published.Publication.ID, published.RawToken), nil, nil, nil)
	if err != nil || after.StatusCode != 200 || string(after.Body) != string(original.Body) {
		t.Fatal("prefix/delete changed immutable publication")
	}
}
