package http_test

import (
	"bytes"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmptyPassHTTPRoundtripPublicationAndSharedValidation(t *testing.T) {
	db := newCleanSQLiteDB(t)
	ctx := context.Background()
	seedSamplePolicyData(t, db)
	repo := sqlite.NewPolicyRepository(db)
	rev := sqlite.NewRevisionRepository(db)
	nodes := sqlite.NewNodeRepository(db)
	filters := sqlite.NewNodeFilterRepository(db)
	pol := policy.NewService(repo, rev, nodes, sqlite.NewAuditRepository(db), filters)
	pub := publication.NewService(sqlite.NewPublicationRepository(db), nil, publication.WithPolicyRepository(repo), publication.WithRevisionRepository(rev), publication.WithNodeRepository(nodes), publication.WithNodeFilterRepository(filters), publication.WithNodeSourceRepository(sqlite.NewNodeSourceRepository(db)), publication.WithProbeObservationRepository(sqlite.NewProbeObservationRepository(db)))
	cfg := newTestRouterConfig(true)
	cfg.PolicyService = pol
	cfg.PublicationService = pub
	router := transporthttp.NewRouter(cfg)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	rec := request(http.MethodPost, "/api/v1/policies/groups", `{"name":"fixture-empty","group_type":"fallback"}`)
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var group groupDetailResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &group)
	if group.Data.EmptyFallbackPass {
		t.Fatal("default enabled")
	}
	id := group.Data.ID
	rec = request(http.MethodPatch, "/api/v1/policies/groups/"+id, `{"empty_fallback_pass":true}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body)
	}
	rec = request(http.MethodPatch, "/api/v1/policies/groups/"+id, `{"name":"fixture-empty"}`)
	_ = json.Unmarshal(rec.Body.Bytes(), &group)
	if !group.Data.EmptyFallbackPass {
		t.Fatal("omission lost opt-in")
	}
	rec = request(http.MethodGet, "/api/v1/policies/groups/"+id, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &group)
	if !group.Data.EmptyFallbackPass {
		t.Fatal("GET lost flag")
	}
	rec = request(http.MethodGet, "/api/v1/policies/groups", "")
	if !strings.Contains(rec.Body.String(), `"empty_fallback_pass":true`) {
		t.Fatal("LIST lost flag")
	}
	bad := request(http.MethodPatch, "/api/v1/policies/groups/"+id, `{"name":"bad","empty_fallback_pass":false,"node_filter":{"conditions":[{"field":"display_name","op":"regex","value":"("}]}}`)
	if bad.Code != 422 {
		t.Fatalf("bad filter: %d %s", bad.Code, bad.Body)
	}
	active, err := rev.GetActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE policy_rules SET position=1 WHERE revision_id=?`, active.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreatePolicyRule(ctx, &domain.PolicyRule{ID: domain.MustNewUUIDv7(), RevisionID: active.ID, TargetGroupID: id, Expression: "DOMAIN,fixture.invalid", Position: 0}); err != nil {
		t.Fatal(err)
	}
	check := request(http.MethodPost, "/api/v1/policies/validate", "")
	if check.Code != 200 {
		t.Fatal(check.Body)
	}
	var validation testDataResponse[policy.ValidationResult]
	_ = json.Unmarshal(check.Body.Bytes(), &validation)
	if !validation.Data.Valid {
		t.Fatalf("explicit PASS invalid: %s", check.Body)
	}
	before := int64(0)
	_ = db.QueryRow(`SELECT total_changes()`).Scan(&before)
	if _, err := pub.ResolvePolicySnapshot(ctx, active.ID); err != nil {
		t.Fatal(err)
	}
	after := int64(0)
	_ = db.QueryRow(`SELECT total_changes()`).Scan(&after)
	if before != after {
		t.Fatal("snapshot assembly wrote DB")
	}
	for _, mode := range []string{"strict", "compatible"} {
		preview, err := pub.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo, CompatMode: mode})
		if err != nil {
			t.Fatalf("%s preview: %v", mode, err)
		}
		if !strings.Contains(string(preview.Content), "empty-fallback: PASS") {
			t.Fatal("publication lost PASS")
		}
		published, err := pub.Publish(ctx, publication.PublishCommand{Target: domain.TargetMihomo, SnapshotID: preview.SnapshotID})
		if err != nil {
			t.Fatal(err)
		}
		frozen, err := pub.ResolveAndServe(ctx, published.Publication.ID, published.RawToken)
		if err != nil || !bytes.Equal(frozen.Content, preview.Content) {
			t.Fatalf("frozen content mismatch: %v", err)
		}
	}
	// One unchecked live-like group remains blocked; don't blanket enable it.
	rec = request(http.MethodPatch, "/api/v1/policies/groups/"+id, `{"empty_fallback_pass":false}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body)
	}
	check = request(http.MethodPost, "/api/v1/policies/validate", "")
	if !strings.Contains(check.Body.String(), "empty_routed_group") {
		t.Fatal("unchecked empty route no longer blocked")
	}
	// Evidence-dependent filtering uses the very same publication input provider.
	kind := domain.ProbeKindBaseline
	if err := filters.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{Spec: domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{Field: domain.FilterFieldProbeVerdict, Op: domain.FilterOpEquals, Value: "available", ProbeKind: &kind}}}, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	check = request(http.MethodPost, "/api/v1/policies/validate", "")
	if !strings.Contains(check.Body.String(), "filtered_nodes_empty") {
		t.Fatalf("validation omitted publication filter guard: %s", check.Body)
	}
}
