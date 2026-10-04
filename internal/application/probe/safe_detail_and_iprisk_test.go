package probe_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
)

type dummyAuditRepo struct{}

func newCleanSQLiteDB(t *testing.T) *sql.DB {
	t.Helper()
	cfg := sqlite.Config{
		Path:            fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()),
		BusyTimeout:     time.Second,
		ForeignKeys:     true,
		WALMode:         false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedProviderSettings(t *testing.T, db *sql.DB, provider, schemaVersion string) {
	t.Helper()
	now := domain.NowUTC().Format(time.RFC3339)
	const q = `
	INSERT OR IGNORE INTO ip_risk_provider_settings (
		provider, schema_version, enabled, secret_reference, max_concurrency,
		requests_per_minute, daily_request_budget, per_request_timeout_ms,
		max_response_bytes, created_at, updated_at
	) VALUES (?, ?, 1, 'secret://iprisk/test', 5, 60, 1000, 3000, 65536, ?, ?);`
	if _, err := db.Exec(q, provider, schemaVersion, now, now); err != nil {
		t.Fatalf("failed to seed provider settings: %v", err)
	}
}

func TestSafeDetail_AllowlistAndNoSensitiveLeakage(t *testing.T) {
	// Verify SanitizeSafeDetail explicitly strips sensitive keys and retains ONLY allowlist
	rawSensitive := map[string]any{
		"stage":     "baseline",
		"code":      "timeout",
		"core":      "mihomo",
		"transport": "ss",
		"status":    504,
		"timeout":   3000,
		"err":       "dial tcp 1.2.3.4:443: i/o timeout?token=secret123&password=supersecret",
		"url":       "https://secret.server.com/api?uuid=00000000-0000-0000-0000-000000000000",
		"body":      "sensitive response body containing token",
		"password":  "mypassword",
	}

	sanitized := domain.SanitizeSafeDetail(rawSensitive)
	if sanitized.Stage != "baseline" || sanitized.Code != "timeout" || sanitized.Core != "mihomo" ||
		sanitized.Transport != "ss" || sanitized.Status != 504 || sanitized.Timeout != 3000 {
		t.Fatalf("unexpected sanitized allowlist fields: %+v", sanitized)
	}

	// Verify ProbeObservation persistence with SafeDetail
	db := newCleanSQLiteDB(t)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	runRepo := sqlite.NewProbeRunRepository(db)

	now := domain.NowUTC()
	node := domain.Node{
		LogicalID:   "node-safe-test",
		Protocol:    domain.ProtocolSS,
		DisplayName: "Safe Test",
		Server:      "1.2.3.4",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{Password: "secret", Method: "aes-256-gcm"},
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := nodeRepo.UpsertBatch(context.Background(), []domain.Node{node}); err != nil {
		t.Fatal(err)
	}

	runID := "run_" + domain.MustNewUUIDv7()
	run := domain.ProbeRun{
		ID:        runID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := runRepo.Create(context.Background(), &run); err != nil {
		t.Fatal(err)
	}

	obs := domain.ProbeObservation{
		ID:              domain.MustNewUUIDv7(),
		ProbeRunID:      runID,
		NodeLogicalID:   node.LogicalID,
		Kind:            domain.ProbeKindBaseline,
		Verdict:         domain.VerdictError,
		EvidenceDigest:  "sha256:digest_fail",
		ObservedAt:      now,
		LatencyMS:       3000,
		RedactedSummary: "timeout",
		SafeDetail:      &sanitized,
	}
	if err := obsRepo.Create(context.Background(), &obs); err != nil {
		t.Fatalf("failed to create observation with SafeDetail: %v", err)
	}

	// Retrieve from DB and verify EvidenceData has safe_detail
	stored, err := obsRepo.GetByID(context.Background(), obs.ID)
	if err != nil {
		t.Fatalf("failed to get observation: %v", err)
	}
	if stored.SafeDetail == nil {
		t.Fatalf("expected SafeDetail to be restored from EvidenceData")
	}
	if stored.SafeDetail.Stage != "baseline" || stored.SafeDetail.Code != "timeout" || stored.SafeDetail.Timeout != 3000 {
		t.Fatalf("restored SafeDetail mismatch: %+v", stored.SafeDetail)
	}
	// Verify raw evidence JSON has NO sensitive keys
	if strings.Contains(stored.EvidenceData, "password") || strings.Contains(stored.EvidenceData, "supersecret") ||
		strings.Contains(stored.EvidenceData, "uuid") || strings.Contains(stored.EvidenceData, "token") {
		t.Fatalf("EvidenceData leaked sensitive values: %s", stored.EvidenceData)
	}
}

func TestIPRiskObservation_OneToManyLinkingAndBaselineExclusion(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)
	seedProviderSettings(t, db, "scamalytics", "v1")
	seedProviderSettings(t, db, "ipinfo", "v1")

	nodeRepo := sqlite.NewNodeRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	runRepo := sqlite.NewProbeRunRepository(db)
	riskObsRepo := sqlite.NewIPRiskObservationRepository(db)

	now := domain.NowUTC()
	nodeID := "0123456789abcdef0123456789abcdef"
	node := domain.Node{
		LogicalID:   nodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "IPRisk Test",
		Server:      "1.2.3.5",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{Password: "secret", Method: "aes-256-gcm"},
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}

	runID := "run_" + domain.MustNewUUIDv7()
	run := domain.ProbeRun{ID: runID, CreatedAt: now, UpdatedAt: now}
	if err := runRepo.Create(ctx, &run); err != nil {
		t.Fatal(err)
	}

	validDigest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	// 1. Probe observation with kind = ip_risk
	ipRiskProbeObs := domain.ProbeObservation{
		ID:              domain.MustNewUUIDv7(),
		ProbeRunID:      runID,
		NodeLogicalID:   node.LogicalID,
		Kind:            domain.ProbeKindIPRisk,
		Verdict:         domain.VerdictAvailable,
		EvidenceDigest:  validDigest,
		ObservedAt:      now,
		LatencyMS:       200,
		RedactedSummary: "clean IP",
	}
	if err := obsRepo.Create(ctx, &ipRiskProbeObs); err != nil {
		t.Fatal(err)
	}

	// Link two IP risk observations to the SAME kind=ip_risk probe observation (1:N)
	r1 := domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         node.LogicalID,
		ExitIdentityDigest:    validDigest,
		Provider:              "scamalytics",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(24 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		NetworkClass:          domain.NetworkClassDatacenter,
		EvidenceDigest:        validDigest,
		ProbeObservationID:    &ipRiskProbeObs.ID,
	}
	r2 := domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         node.LogicalID,
		ExitIdentityDigest:    validDigest,
		Provider:              "ipinfo",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(24 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		NetworkClass:          domain.NetworkClassResidential,
		EvidenceDigest:        validDigest,
		ProbeObservationID:    &ipRiskProbeObs.ID,
	}
	if err := riskObsRepo.Create(ctx, &r1); err != nil {
		t.Fatalf("failed to create ip_risk_observation 1: %v", err)
	}
	if err := riskObsRepo.Create(ctx, &r2); err != nil {
		t.Fatalf("failed to create ip_risk_observation 2: %v", err)
	}

	// 2. Probe observation with kind = baseline
	baselineProbeObs := domain.ProbeObservation{
		ID:              domain.MustNewUUIDv7(),
		ProbeRunID:      runID,
		NodeLogicalID:   node.LogicalID,
		Kind:            domain.ProbeKindBaseline,
		Verdict:         domain.VerdictAvailable,
		EvidenceDigest:  validDigest,
		ObservedAt:      now,
		LatencyMS:       100,
		RedactedSummary: "baseline pass",
	}
	if err := obsRepo.Create(ctx, &baselineProbeObs); err != nil {
		t.Fatal(err)
	}

	// Attempting to link ip_risk_observations to baseline MUST fail with invalid_probe_kind_for_ip_risk
	rInvalid := domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         node.LogicalID,
		ExitIdentityDigest:    validDigest,
		Provider:              "scamalytics",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(24 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		NetworkClass:          domain.NetworkClassDatacenter,
		EvidenceDigest:        validDigest,
		ProbeObservationID:    &baselineProbeObs.ID, // INVALID: kind is baseline!
	}
	err := riskObsRepo.Create(ctx, &rInvalid)
	if err == nil {
		t.Fatalf("expected error when linking ip_risk_observation to kind='baseline', got nil")
	}
	if !strings.Contains(err.Error(), "invalid_probe_kind_for_ip_risk") {
		t.Fatalf("expected invalid_probe_kind_for_ip_risk error, got: %v", err)
	}

	// 3. Historical IP risk observation with ProbeObservationID = NULL is preserved
	rLegacy := domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         node.LogicalID,
		ExitIdentityDigest:    validDigest,
		Provider:              "scamalytics",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(24 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		NetworkClass:          domain.NetworkClassDatacenter,
		EvidenceDigest:        validDigest,
		ProbeObservationID:    nil, // NULL
	}
	if err := riskObsRepo.Create(ctx, &rLegacy); err != nil {
		t.Fatalf("failed to create legacy ip_risk_observation with NULL probe_observation_id: %v", err)
	}
}

type probeMockRoundTripper func(req *http.Request) (*http.Response, error)

func (f probeMockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRunner_IPRiskProbing_ActualExecutionAndLinkage(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)
	seedProviderSettings(t, db, "scamalytics", "v1")
	seedProviderSettings(t, db, "ipinfo", "v1")

	nodeRepo := sqlite.NewNodeRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	runRepo := sqlite.NewProbeRunRepository(db)
	riskObsRepo := sqlite.NewIPRiskObservationRepository(db)

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	now := domain.NowUTC()
	nodeID := "0123456789abcdef0123456789abcdef"
	node := domain.Node{
		LogicalID:          nodeID,
		Protocol:           domain.ProtocolSS,
		DisplayName:        "IPRisk Probing Node",
		Server:             "1.2.3.4",
		Port:               8388,
		Credentials:        domain.InboundProtocolCredential{Password: "secret123", Method: "aes-256-gcm"},
		Active:             true,
		ConnectionRevision: 2,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	v2 := domain.NodeConnectionVersion{
		NodeLogicalID:       nodeID,
		ConnectionRevision:  2,
		EffectiveConfigJSON: `{"server":"1.2.3.4","port":8388}`,
		ConfigFingerprint:   "sha256:fp2",
		CreatedAt:           now,
	}
	if err := sqlite.NewNodeConnectionVersionRepository(db).Save(ctx, &v2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE node_connection_heads SET connection_revision = 2 WHERE logical_id = ?;", nodeID); err != nil {
		t.Fatal(err)
	}

	subID := "sub-iprisk-01"
	fetchID := "fetch-iprisk-01"
	nowStr := now.Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at) VALUES (?, 'Test Sub', 'secret', 1, 'rev1', ?, ?);`, subID, nowStr, nowStr); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO subscription_fetches (id, subscription_id, started_at, finished_at, outcome) VALUES (?, ?, ?, ?, 'success');`, fetchID, subID, nowStr, nowStr); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES (?, ?, ?);`, nodeID, subID, fetchID); err != nil {
		t.Fatal(err)
	}

	scamalyticsHTML := `<html>
IP Fraud Risk API
Line 1
Line 2
Line 3
"score": "15",
"risk": "low"
</html>`

	mockClient := &http.Client{
		Transport: probeMockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.String(), "cloudflare.com/cdn-cgi/trace") {
				traceBody := "ip=198.51.100.1\nloc=HK\ncolo=HKG\n"
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(traceBody)),
					Header:     make(http.Header),
				}, nil
			}
			if strings.Contains(req.URL.String(), "scamalytics.com/ip/") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(scamalyticsHTML)),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	runner := probe.NewDefaultRunner(
		nodeRepo,
		obsRepo,
		sched,
		runRepo,
		probe.WithIPRiskObservationRepository(riskObsRepo),
		probe.WithNodeDialer(func(ctx context.Context, n domain.Node) (*http.Client, func() error, error) {
			return mockClient, func() error { return nil }, nil
		}),
	)

	runID := "run_" + domain.MustNewUUIDv7()
	run := domain.ProbeRun{
		ID:             runID,
		IdempotencyKey: "key_" + runID,
		ActorScope:     "admin",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := runRepo.Create(ctx, &run); err != nil {
		t.Fatal(err)
	}

	err = runner.Run(ctx, &run, []string{nodeID}, []domain.ProbeKind{domain.ProbeKindIPRisk})
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	// 1. Verify ProbeObservation created with kind=ip_risk and connection_revision=2
	observations, err := obsRepo.ListByRun(ctx, runID)
	if err != nil || len(observations) != 1 {
		t.Fatalf("expected 1 probe observation, got %d (err: %v)", len(observations), err)
	}
	probeObs := observations[0]
	if probeObs.Kind != domain.ProbeKindIPRisk || probeObs.Verdict != domain.VerdictAvailable {
		t.Fatalf("expected kind=ip_risk and verdict=available, got kind=%s verdict=%s", probeObs.Kind, probeObs.Verdict)
	}
	if probeObs.ConnectionRevision == nil || *probeObs.ConnectionRevision != 2 {
		val := int64(-1)
		if probeObs.ConnectionRevision != nil {
			val = *probeObs.ConnectionRevision
		}
		t.Fatalf("expected ConnectionRevision=2, got %d", val)
	}
	if probeObs.RiskScore != "15% low" {
		t.Fatalf("expected RiskScore='15%% low', got %q", probeObs.RiskScore)
	}

	// 2. Verify IPRiskObservation created and linked to probe observation
	riskObsList, total, err := riskObsRepo.List(ctx, domain.IPRiskObservationFilter{NodeLogicalID: nodeID})
	if err != nil || total != 1 || len(riskObsList) != 1 {
		t.Fatalf("expected 1 ip risk observation, got %d (err: %v)", total, err)
	}
	riskObs := riskObsList[0]
	if riskObs.ProbeObservationID == nil || *riskObs.ProbeObservationID != probeObs.ID {
		t.Fatalf("expected ProbeObservationID linking to %s, got %v", probeObs.ID, riskObs.ProbeObservationID)
	}
	if riskObs.Provider != "scamalytics" || riskObs.Status != domain.IPRiskStatusAvailable {
		t.Fatalf("expected provider=scamalytics status=available, got provider=%s status=%s", riskObs.Provider, riskObs.Status)
	}
	if riskObs.Score == nil || *riskObs.Score != 15 {
		t.Fatalf("expected Score=15, got %v", riskObs.Score)
	}
	if riskObs.Confidence == nil || *riskObs.Confidence != 80 {
		t.Fatalf("expected Confidence=80, got %v", riskObs.Confidence)
	}

	// 3. Multi-provider test: add a second provider (e.g. ipinfo) linking to the SAME probe observation
	r2 := domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         nodeID,
		ExitIdentityDigest:    probeObs.EvidenceDigest,
		Provider:              "ipinfo",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(24 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		NetworkClass:          domain.NetworkClassResidential,
		EvidenceDigest:        probeObs.EvidenceDigest,
		ProbeObservationID:    &probeObs.ID,
	}
	if err := riskObsRepo.Create(ctx, &r2); err != nil {
		t.Fatalf("failed to create second provider observation linking to same probe obs: %v", err)
	}

	// 4. SafeDetail on error: dial failure must generate SafeDetail without secrets and with real core version
	failingRunner := probe.NewDefaultRunner(
		nodeRepo,
		obsRepo,
		sched,
		runRepo,
		probe.WithIPRiskObservationRepository(riskObsRepo),
		probe.WithNodeDialer(func(ctx context.Context, n domain.Node) (*http.Client, func() error, error) {
			return nil, nil, fmt.Errorf("dial tcp 1.2.3.4:8388: connect: connection refused?password=mysecrettoken")
		}),
	)
	failRunID := "run_fail_" + domain.MustNewUUIDv7()
	failRun := domain.ProbeRun{
		ID:             failRunID,
		IdempotencyKey: "key_" + failRunID,
		ActorScope:     "admin",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_ = runRepo.Create(ctx, &failRun)
	_ = failingRunner.Run(ctx, &failRun, []string{nodeID}, []domain.ProbeKind{domain.ProbeKindIPRisk})

	failObservations, err := obsRepo.ListByRun(ctx, failRunID)
	if err != nil || len(failObservations) != 1 {
		t.Fatalf("expected 1 failed observation, got %d", len(failObservations))
	}
	failObs := failObservations[0]
	if failObs.SafeDetail == nil {
		t.Fatalf("expected SafeDetail to be populated on failure")
	}
	if failObs.SafeDetail.Stage != "dial" {
		t.Fatalf("expected stage=dial, got %s", failObs.SafeDetail.Stage)
	}
	// Verify raw evidence JSON has NO sensitive keys
	if strings.Contains(failObs.EvidenceData, "password") || strings.Contains(failObs.EvidenceData, "mysecrettoken") {
		t.Fatalf("SafeDetail EvidenceData leaked secrets: %s", failObs.EvidenceData)
	}
}

func TestRunner_And_Service_NoticeExclusion(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)

	nodeRepo := sqlite.NewNodeRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	runRepo := sqlite.NewProbeRunRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	payloadRepo := sqlite.NewSubscriptionPayloadRepository(db)
	entryRepo := sqlite.NewSubscriptionEntryRepository(db)

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	now := domain.NowUTC()

	// 1. Subscription & Payload
	sub := domain.Subscription{
		ID:                 "sub-notice-test",
		Name:               "Notice Sub",
		SourceURLSecretRef: "secret://sub",
		Enabled:            true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := subRepo.Create(ctx, &sub); err != nil {
		t.Fatal(err)
	}
	fetch := domain.SubscriptionFetch{
		ID:             "fetch-notice-1",
		SubscriptionID: sub.ID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:sub",
	}
	if err := fetchRepo.Create(ctx, &fetch); err != nil {
		t.Fatal(err)
	}
	payload := domain.SubscriptionPayload{
		ID:             "payload-notice-1",
		SubscriptionID: sub.ID,
		FetchID:        fetch.ID,
		ContentDigest:  "sha256:sub",
		BodyBlob:       []byte("proxies"),
		HTTPStatus:     200,
		CreatedAt:      now,
	}
	if err := payloadRepo.Save(ctx, &payload); err != nil {
		t.Fatal(err)
	}

	// Node 1: Normal proxy (active = 1)
	node1 := domain.Node{
		LogicalID:   "node-proxy-1",
		Protocol:    domain.ProtocolSS,
		DisplayName: "Normal Proxy 1",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	entry1 := domain.SubscriptionEntry{
		ID:             "entry-1",
		PayloadID:      payload.ID,
		SubscriptionID: sub.ID,
		Ordinal:        0,
		SourceKey:      "p1",
		RawName:        "Normal Proxy 1",
		Protocol:       domain.ProtocolSS,
		Server:         "1.1.1.1",
		Port:           8388,
		EntryKind:      domain.EntryKindProxy,
		NodeLogicalID:  &node1.LogicalID,
		CreatedAt:      now,
	}

	// Node 2: Notice pseudo-node (active = 1 in nodes table for test, entry_kind = 'notice')
	node2 := domain.Node{
		LogicalID:   "node-notice-2",
		Protocol:    domain.ProtocolVMess,
		DisplayName: "Notice 127.0.0.1",
		Server:      "127.0.0.1",
		Port:        1,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	entry2 := domain.SubscriptionEntry{
		ID:             "entry-2",
		PayloadID:      payload.ID,
		SubscriptionID: sub.ID,
		Ordinal:        1,
		SourceKey:      "n2",
		RawName:        "剩余流量: 100GB",
		Protocol:       domain.ProtocolVMess,
		Server:         "127.0.0.1",
		Port:           1,
		EntryKind:      domain.EntryKindNotice,
		NodeLogicalID:  &node2.LogicalID,
		CreatedAt:      now,
	}

	// Node 3: Notice pseudo-node with user_kind_override = 'proxy'
	node3 := domain.Node{
		LogicalID:   "node-override-3",
		Protocol:    domain.ProtocolVMess,
		DisplayName: "Overridden Notice",
		Server:      "127.0.0.1",
		Port:        1,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	proxyKind := domain.EntryKindProxy
	entry3 := domain.SubscriptionEntry{
		ID:               "entry-3",
		PayloadID:        payload.ID,
		SubscriptionID:   sub.ID,
		Ordinal:          2,
		SourceKey:        "n3",
		RawName:          "Manual Proxy",
		Protocol:         domain.ProtocolVMess,
		Server:           "127.0.0.1",
		Port:             1,
		EntryKind:        domain.EntryKindNotice,
		UserKindOverride: &proxyKind,
		NodeLogicalID:    &node3.LogicalID,
		CreatedAt:        now,
	}

	// Node 4: Old inactive proxy (active = 0) with user_kind_override = 'proxy' (must NOT auto-activate)
	node4 := domain.Node{
		LogicalID:   "node-inactive-4",
		Protocol:    domain.ProtocolSS,
		DisplayName: "Inactive Proxy",
		Server:      "4.4.4.4",
		Port:        8388,
		Active:      false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	entry4 := domain.SubscriptionEntry{
		ID:               "entry-4",
		PayloadID:        payload.ID,
		SubscriptionID:   sub.ID,
		Ordinal:          3,
		SourceKey:        "p4",
		RawName:          "Inactive Proxy",
		Protocol:         domain.ProtocolSS,
		Server:           "4.4.4.4",
		Port:             8388,
		EntryKind:        domain.EntryKindProxy,
		UserKindOverride: &proxyKind,
		NodeLogicalID:    &node4.LogicalID,
		CreatedAt:        now,
	}

	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node1, node2, node3, node4}); err != nil {
		t.Fatal(err)
	}
	for _, nid := range []string{node1.LogicalID, node2.LogicalID, node3.LogicalID, node4.LogicalID} {
		if _, err := db.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES (?, ?, ?);`, nid, sub.ID, fetch.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := entryRepo.SaveBatch(ctx, []domain.SubscriptionEntry{entry1, entry2, entry3, entry4}); err != nil {
		t.Fatal(err)
	}

	// 2. Test NodeFilter SQL with ExcludeNotices: true
	activeNonNotices, total, err := nodeRepo.List(ctx, domain.NodeFilter{
		ActiveOnly:     true,
		ExcludeNotices: true,
	})
	if err != nil {
		t.Fatalf("nodeRepo.List failed: %v", err)
	}
	// Node 1 (normal active proxy) and Node 3 (active proxy override) should be included.
	// Node 2 (notice) must be excluded.
	// Node 4 (inactive proxy) must be excluded because active=false.
	if total != 2 || len(activeNonNotices) != 2 {
		t.Fatalf("expected 2 active non-notice nodes, got %d (nodes: %+v)", total, activeNonNotices)
	}
	hasNode1, hasNode2, hasNode3, hasNode4 := false, false, false, false
	for _, n := range activeNonNotices {
		if n.LogicalID == node1.LogicalID {
			hasNode1 = true
		}
		if n.LogicalID == node2.LogicalID {
			hasNode2 = true
		}
		if n.LogicalID == node3.LogicalID {
			hasNode3 = true
		}
		if n.LogicalID == node4.LogicalID {
			hasNode4 = true
		}
	}
	if !hasNode1 || hasNode2 || !hasNode3 || hasNode4 {
		t.Fatalf("unexpected nodes returned: node1=%v, node2=%v (expected false), node3=%v, node4=%v (expected false)",
			hasNode1, hasNode2, hasNode3, hasNode4)
	}

	// 3. Test Runner in runAll mode (nodeIDs = nil) excludes notices
	var probedMu sync.Mutex
	var probedNodes []string
	runner := probe.NewDefaultRunner(
		nodeRepo,
		obsRepo,
		sched,
		runRepo,
		probe.WithNodeDialer(func(ctx context.Context, n domain.Node) (*http.Client, func() error, error) {
			probedMu.Lock()
			probedNodes = append(probedNodes, n.LogicalID)
			probedMu.Unlock()
			return mockHTTPClient(http.StatusNoContent, "", nil), func() error { return nil }, nil
		}),
	)

	runAllID := "run_all_" + domain.MustNewUUIDv7()
	runAll := domain.ProbeRun{
		ID:             runAllID,
		IdempotencyKey: "key_" + runAllID,
		ActorScope:     "admin",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_ = runRepo.Create(ctx, &runAll)
	if err := runner.Run(ctx, &runAll, nil, []domain.ProbeKind{domain.ProbeKindBaseline}); err != nil {
		t.Fatalf("runner.Run runAll failed: %v", err)
	}
	probedMu.Lock()
	if len(probedNodes) != 2 {
		t.Fatalf("expected runner runAll to probe 2 nodes, probed %d: %v", len(probedNodes), probedNodes)
	}
	for _, pid := range probedNodes {
		if pid == node2.LogicalID || pid == node4.LogicalID {
			t.Fatalf("runner runAll probed forbidden node: %s", pid)
		}
	}
	probedNodes = nil
	probedMu.Unlock()

	// 4. Test Runner in selected mode (passing nodeIDs including node2 notice) excludes notice!
	runSelID := "run_sel_" + domain.MustNewUUIDv7()
	runSel := domain.ProbeRun{
		ID:             runSelID,
		IdempotencyKey: "key_" + runSelID,
		ActorScope:     "admin",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_ = runRepo.Create(ctx, &runSel)
	selectedIDs := []string{node1.LogicalID, node2.LogicalID, node3.LogicalID}
	if err := runner.Run(ctx, &runSel, selectedIDs, []domain.ProbeKind{domain.ProbeKindBaseline}); err != nil {
		t.Fatalf("runner.Run selected failed: %v", err)
	}
	probedMu.Lock()
	if len(probedNodes) != 2 {
		t.Fatalf("expected runner selected to probe 2 nodes (node2 excluded), probed %d: %v", len(probedNodes), probedNodes)
	}
	for _, pid := range probedNodes {
		if pid == node2.LogicalID {
			t.Fatalf("runner selected probed forbidden notice node: %s", pid)
		}
	}
	probedMu.Unlock()

	// 5. Test probe.Service status query excludes notice
	probeSvc := probe.NewService(runRepo, probe.WithNodeRepository(nodeRepo))
	status, err := probeSvc.GetPoolStatus(ctx)
	if err != nil {
		t.Fatalf("GetPoolStatus failed: %v", err)
	}
	if status.TotalCount != 2 {
		t.Fatalf("expected probe service status TotalCount=2, got %d", status.TotalCount)
	}
}
