package http_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func setupPublicationTestRouter(t *testing.T, db *sql.DB) (http.Handler, *publication.Service) {
	t.Helper()
	routerCfg := newTestRouterConfig(true)
	pubRepo := sqlite.NewPublicationRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	revRepo := sqlite.NewRevisionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	riskBindingRepo := sqlite.NewRiskPolicyGroupBindingRepository(db)
	riskObsRepo := sqlite.NewIPRiskObservationRepository(db)

	ipriskSvc := iprisk.NewService(
		riskObsRepo,
		riskPolicyRepo,
		iprisk.WithBindingRepository(riskBindingRepo),
		iprisk.WithGroupRepository(policyRepo),
		iprisk.WithNodeRepository(nodeRepo),
		iprisk.WithAuditRepository(auditRepo),
	)

	pubSvc := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithIPRiskService(ipriskSvc),
		publication.WithRiskPolicyRepository(riskPolicyRepo),
		publication.WithRiskBindingRepository(riskBindingRepo),
		publication.WithRiskObservationRepository(riskObsRepo),
	)

	routerCfg.PublicationService = pubSvc
	routerCfg.IPRiskService = ipriskSvc
	routerCfg.RiskPolicyRepository = riskPolicyRepo
	routerCfg.RiskBindingRepository = riskBindingRepo
	routerCfg.PublicationTokenValidator = func(ctx context.Context, publicationID, token string) (bool, error) {
		return pubSvc.ValidateToken(ctx, publicationID, token)
	}
	routerCfg.IsPublicationToken = func(ctx context.Context, token string) bool {
		return pubSvc.IsPublicationToken(ctx, token)
	}
	routerCfg.AuditRepository = auditRepo

	return transporthttp.NewRouter(routerCfg), pubSvc
}

// seedSamplePolicyData sets up an active revision, a node, a policy group, and rules.
func seedSamplePolicyData(t *testing.T, db *sql.DB) (string, string) {
	t.Helper()
	ctx := context.Background()

	// 1. Insert Node
	nodeRepo := sqlite.NewNodeRepository(db)
	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolTrojan, "tokyo.example.com", 443, nil)
	err := nodeRepo.UpsertBatch(ctx, []domain.Node{
		{
			LogicalID:                 nodeID,
			Protocol:                  domain.ProtocolTrojan,
			DisplayName:               "Tokyo-01",
			Active:                    true,
			NormalizedConfigSecretRef: "secret://tokyo",
			UpdatedAt:                 domain.NowUTC(),
		},
	})
	if err != nil {
		t.Fatalf("failed to insert node: %v", err)
	}

	// 2. Insert Revision
	revRepo := sqlite.NewRevisionRepository(db)
	revID, _ := domain.NewUUIDv7()
	err = revRepo.Create(ctx, &domain.ConfigurationRevision{
		ID:            revID,
		ContentDigest: "sha256:rev-digest",
		State:         domain.RevisionStateDraft,
		CreatedAt:     domain.NowUTC(),
	})
	if err != nil {
		t.Fatalf("failed to create revision: %v", err)
	}
	if err := revRepo.SetActive(ctx, revID); err != nil {
		t.Fatalf("failed to set active revision: %v", err)
	}

	// 3. Insert Group and Edges
	policyRepo := sqlite.NewPolicyRepository(db)
	groupID, _ := domain.NewUUIDv7()
	edgeID, _ := domain.NewUUIDv7()
	err = policyRepo.CreateGroup(ctx, &domain.NodeGroup{
		ID:        groupID,
		Name:      "ProxyGroup",
		GroupType: domain.GroupTypeSelect,
		CreatedAt: domain.NowUTC(),
		UpdatedAt: domain.NowUTC(),
	})
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}
	err = policyRepo.SetEdgesForGroup(ctx, groupID, []domain.GroupEdge{
		{
			ID:            edgeID,
			ParentGroupID: groupID,
			NodeLogicalID: &nodeID,
			Position:      0,
		},
	})
	if err != nil {
		t.Fatalf("failed to set edges: %v", err)
	}

	// 4. Insert Policy Rule
	ruleID, _ := domain.NewUUIDv7()
	err = policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
		ID:            ruleID,
		RevisionID:    revID,
		TargetGroupID: groupID,
		Expression:    "MATCH",
		Position:      0,
	})
	if err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}

	return revID, nodeID
}

type publicationCreateResponse struct {
	Data struct {
		Publication struct {
			ID              string `json:"id"`
			Target          string `json:"target"`
			SnapshotDigest  string `json:"snapshot_digest"`
			CompilerVersion string `json:"compiler_version"`
			State           string `json:"state"`
		} `json:"publication"`
		RawToken       string `json:"raw_token"`
		ExportURL      string `json:"export_url"`
		ContentDigest  string `json:"content_digest"`
		SnapshotDigest string `json:"snapshot_digest"`
		ContentType    string `json:"content_type"`
		Filename       string `json:"filename"`
	} `json:"data"`
}

type previewResponse struct {
	Data struct {
		Target         string `json:"target"`
		SnapshotDigest string `json:"snapshot_digest"`
		ContentDigest  string `json:"content_digest"`
		Content        string `json:"content"`
		ContentType    string `json:"content_type"`
		Filename       string `json:"filename"`
	} `json:"data"`
}

func TestPublicationClientSubscriptionEndpoint(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, pubSvc := setupPublicationTestRouter(t, db)
	seedSamplePolicyData(t, db)

	// Publish for Clash
	ctx := context.Background()
	pubRes, err := pubSvc.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetClash,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-pub-clash",
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	pubID := pubRes.Publication.ID
	rawToken := pubRes.RawToken

	// 1. Success via query param token
	t.Run("success_via_query_param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", pubID, rawToken), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. body: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "yaml") {
			t.Fatalf("expected Content-Type yaml, got %s", rec.Header().Get("Content-Type"))
		}
		if rec.Header().Get("X-Content-Digest") != pubRes.ContentDigest {
			t.Fatalf("expected X-Content-Digest %s, got %s", pubRes.ContentDigest, rec.Header().Get("X-Content-Digest"))
		}
		if rec.Header().Get("X-Snapshot-Digest") != pubRes.SnapshotDigest {
			t.Fatalf("expected X-Snapshot-Digest %s, got %s", pubRes.SnapshotDigest, rec.Header().Get("X-Snapshot-Digest"))
		}
		if !strings.Contains(rec.Header().Get("Content-Disposition"), "clash.yaml") {
			t.Fatalf("expected Content-Disposition with clash.yaml, got %s", rec.Header().Get("Content-Disposition"))
		}
		body := rec.Body.String()
		if !strings.Contains(body, "proxies:") || !strings.Contains(body, "Tokyo-01") {
			t.Fatalf("served body missing expected clash configuration content: %s", body)
		}
	})

	// 2. Success via Authorization: Bearer token
	t.Run("success_via_bearer_header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s", pubID), nil)
		req.Header.Set("Authorization", "Bearer "+rawToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		if rec.Header().Get("X-Content-Digest") != pubRes.ContentDigest {
			t.Fatalf("expected X-Content-Digest %s, got %s", pubRes.ContentDigest, rec.Header().Get("X-Content-Digest"))
		}
	})

	// 3. Missing token returns 401 Unauthorized
	t.Run("missing_token_returns_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s", pubID), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rec.Code)
		}
	})

	// 4. Invalid token returns 401 Unauthorized
	t.Run("invalid_token_returns_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=wrong-token", pubID), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rec.Code)
		}
	})

	// 5. Unknown publication returns 404 Not Found
	t.Run("unknown_publication_returns_404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/0191e4a0-0000-7000-8000-000000000099?token=%s", rawToken), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", rec.Code)
		}
	})
}

func TestPublicationRevocationRejectionNoFallback(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, pubSvc := setupPublicationTestRouter(t, db)
	seedSamplePolicyData(t, db)

	ctx := context.Background()
	pubRes, err := pubSvc.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetSingBox,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-pub-singbox",
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	pubID := pubRes.Publication.ID
	rawToken := pubRes.RawToken

	// 1. Before revocation, publication is accessible
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", pubID, rawToken), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 before revocation, got %d", rec.Code)
	}

	// 2. Revoke publication via admin API
	revokeReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/publications/%s/revoke", pubID), nil)
	revokeReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	revokeRec := httptest.NewRecorder()
	router.ServeHTTP(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for revoke, got %d. body: %s", revokeRec.Code, revokeRec.Body.String())
	}

	// 3. After revocation, export URL MUST return 403 Forbidden (publication_revoked), NEVER fallback!
	reqAfter := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", pubID, rawToken), nil)
	recAfter := httptest.NewRecorder()
	router.ServeHTTP(recAfter, reqAfter)

	if recAfter.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 Forbidden for revoked publication, got %d. body: %s", recAfter.Code, recAfter.Body.String())
	}

	var errResp testErrorResponse
	if err := json.Unmarshal(recAfter.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if errResp.Code != "publication_revoked" {
		t.Fatalf("expected error code publication_revoked, got %s", errResp.Code)
	}
}

func TestPublicationTokenStrictlyForbiddenOnAdminAPI(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, pubSvc := setupPublicationTestRouter(t, db)
	seedSamplePolicyData(t, db)

	ctx := context.Background()
	pubRes, err := pubSvc.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetSurge,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-pub-surge",
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	rawToken := pubRes.RawToken

	adminRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/nodes"},
		{http.MethodGet, "/api/v1/policies/groups"},
		{http.MethodGet, fmt.Sprintf("/api/v1/publications/%s", pubRes.Publication.ID)},
		{http.MethodPost, "/api/v1/publications/preview"},
	}

	for _, route := range adminRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			req.Header.Set("Authorization", "Bearer "+rawToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403 Forbidden when export token accesses admin route %s, got status %d. body: %s", route.path, rec.Code, rec.Body.String())
			}

			var errResp testErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}
			if errResp.Code != "invalid_token_scope" {
				t.Fatalf("expected error code invalid_token_scope, got %s", errResp.Code)
			}
		})
	}
}

func TestAdminPreviewAndPublishConsistency(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPublicationTestRouter(t, db)
	seedSamplePolicyData(t, db)

	// 1. Preview via admin API
	previewBody := `{"target": "mihomo"}`
	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(previewBody))
	previewReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	previewReq.Header.Set("Content-Type", "application/json")
	previewRec := httptest.NewRecorder()
	router.ServeHTTP(previewRec, previewReq)

	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for preview, got %d. body: %s", previewRec.Code, previewRec.Body.String())
	}

	var previewResp previewResponse
	if err := json.Unmarshal(previewRec.Body.Bytes(), &previewResp); err != nil {
		t.Fatalf("failed to parse preview response: %v", err)
	}

	if previewResp.Data.Target != "mihomo" {
		t.Fatalf("expected target mihomo, got %s", previewResp.Data.Target)
	}
	if previewResp.Data.SnapshotDigest == "" || previewResp.Data.ContentDigest == "" {
		t.Fatal("expected non-empty snapshot and content digests in preview")
	}

	// 2. Publish via admin API
	publishBody := `{"target": "mihomo"}`
	publishReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications", strings.NewReader(publishBody))
	publishReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	publishReq.Header.Set("Content-Type", "application/json")
	publishRec := httptest.NewRecorder()
	router.ServeHTTP(publishRec, publishReq)

	if publishRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 for publish, got %d. body: %s", publishRec.Code, publishRec.Body.String())
	}

	var pubResp publicationCreateResponse
	if err := json.Unmarshal(publishRec.Body.Bytes(), &pubResp); err != nil {
		t.Fatalf("failed to parse publish response: %v", err)
	}

	// 3. Verify snapshot and content digests match between preview and publish
	if previewResp.Data.SnapshotDigest != pubResp.Data.SnapshotDigest {
		t.Fatalf("snapshot digest mismatch: preview=%s, publish=%s", previewResp.Data.SnapshotDigest, pubResp.Data.SnapshotDigest)
	}
	if previewResp.Data.ContentDigest != pubResp.Data.ContentDigest {
		t.Fatalf("content digest mismatch: preview=%s, publish=%s", previewResp.Data.ContentDigest, pubResp.Data.ContentDigest)
	}

	// 4. Retrieve publication detail via GET /api/v1/publications/{id}
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/publications/%s", pubResp.Data.Publication.ID), nil)
	getReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for GET publication, got %d. body: %s", getRec.Code, getRec.Body.String())
	}

	// Verify raw token is NOT exposed in GET publication detail
	if strings.Contains(getRec.Body.String(), pubResp.Data.RawToken) {
		t.Fatalf("raw export token must NOT be exposed in GET publication detail: %s", getRec.Body.String())
	}
}

func TestAdminPublicationEndpointsAuthGuardrails(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPublicationTestRouter(t, db)

	// 1. Unauthenticated request to preview -> 401
	req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{"target": "clash"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
	}

	// 2. Cookie session without CSRF to create publication -> 403
	req = httptest.NewRequest(http.MethodPost, "/api/v1/publications", strings.NewReader(`{"target": "clash"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: testValidSessionID})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for missing CSRF, got %d", rec.Code)
	}

	// 3. Cookie session without CSRF to revoke -> 403
	req = httptest.NewRequest(http.MethodPost, "/api/v1/publications/pub-123/revoke", nil)
	req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: testValidSessionID})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for missing CSRF on revoke, got %d", rec.Code)
	}
}

type preflightResponse struct {
	Data struct {
		Allowed        bool   `json:"allowed"`
		PolicyRevision string `json:"policy_revision,omitempty"`
		SnapshotDigest string `json:"snapshot_digest,omitempty"`
		Diagnostics    []struct {
			Severity string `json:"severity"`
			Code     string `json:"code"`
			Message  string `json:"message"`
			Target   string `json:"target,omitempty"`
		} `json:"diagnostics,omitempty"`
	} `json:"data"`
}

func TestAdminPublicationPreflightEndpoint(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPublicationTestRouter(t, db)
	seedSamplePolicyData(t, db)

	// 1. POST /api/v1/publications/preflight -> 200 OK with allowed=true
	body := `{"target": "clash"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preflight", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for preflight, got %d. body: %s", rec.Code, rec.Body.String())
	}

	var resp preflightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode preflight response: %v", err)
	}
	if !resp.Data.Allowed {
		t.Fatalf("expected allowed=true, got false with diagnostics: %+v", resp.Data.Diagnostics)
	}

	// 2. GET /api/v1/publications/preflight?target=clash -> 200 OK
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/publications/preflight?target=clash", nil)
	getReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for GET preflight, got %d. body: %s", getRec.Code, getRec.Body.String())
	}

	// 3. Invalid target -> 422
	invReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preflight", strings.NewReader(`{"target": "invalid-compiler"}`))
	invReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	invReq.Header.Set("Content-Type", "application/json")
	invRec := httptest.NewRecorder()
	router.ServeHTTP(invRec, invReq)

	if invRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid target, got %d", invRec.Code)
	}
}

func TestAdminPublicationPreflightRecomputationConflict(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPublicationTestRouter(t, db)
	_, nodeID := seedSamplePolicyData(t, db)

	// 1. Create provider settings and activate a risk policy (score > 60 -> Block, reviewAction -> Block)
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	provSettings := &domain.IPRiskProviderSettings{
		Provider: "ipinfo", SchemaVersion: "v1", Enabled: true,
		SecretReference: "secret://fixture", MaxConcurrency: 1,
		RequestsPerMinute: 10, DailyRequestBudget: 100,
		PerRequestTimeout: 1500 * time.Millisecond, MaxResponseBytes: 1024,
	}
	if err := providerRepo.Upsert(context.Background(), provSettings); err != nil {
		t.Fatalf("failed to insert provider settings: %v", err)
	}

	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	policyRevID := domain.MustNewUUIDv7()
	policyRev := domain.RiskPolicyRevision{
		RiskPolicy: domain.RiskPolicy{
			RevisionID: policyRevID,
			ProviderSelection: domain.RiskProviderSelection{
				Mode: domain.RiskFusionSingleProvider,
				Providers: []domain.ProviderRef{
					{Provider: "ipinfo", SchemaVersion: "v1"},
				},
			},
			MaxObservationAge: 24 * time.Hour,
			MinimumConfidence: 50,
			ScoreBands: []domain.ScoreBand{
				{Min: 0, Max: 25, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
				{Min: 26, Max: 60, Band: domain.RiskBandMedium, Action: domain.RiskActionReview},
				{Min: 61, Max: 85, Band: domain.RiskBandHigh, Action: domain.RiskActionBlock},
				{Min: 86, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
			},
			UnknownAction:  domain.RiskActionReview,
			ConflictAction: domain.RiskActionReview,
			ReviewAction:   domain.RiskActionBlock,
		},
		Active:    true,
		CreatedAt: time.Now().UTC(),
	}
	if err := riskPolicyRepo.Create(context.Background(), &policyRev); err != nil {
		t.Fatalf("failed to create risk policy: %v", err)
	}

	// 2. Add high-risk observation for nodeID (score=80 -> Block)
	obsRepo := sqlite.NewIPRiskObservationRepository(db)
	obsID := domain.MustNewUUIDv7()
	score80 := 80
	conf90 := 90
	h := sha256.Sum256([]byte(obsID + "ipinfo" + "v1"))
	now := time.Now().UTC()
	obs := domain.IPRiskObservation{
		ID:                    obsID,
		NodeLogicalID:         nodeID,
		ExitIdentityDigest:    "sha256:" + strings.Repeat("e", 64),
		Provider:              "ipinfo",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(12 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &score80,
		Confidence:            &conf90,
		NetworkClass:          domain.NetworkClassResidential,
		EvidenceDigest:        "sha256:" + hex.EncodeToString(h[:]),
		RedactedSummary:       "high risk node observation",
	}
	if err := obsRepo.Create(context.Background(), &obs); err != nil {
		t.Fatalf("failed to insert observation: %v", err)
	}

	// 3. Standalone preflight returns 200 with allowed=false and diagnostic
	preflightReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preflight", strings.NewReader(`{"target": "clash"}`))
	preflightReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	preflightReq.Header.Set("Content-Type", "application/json")
	preflightRec := httptest.NewRecorder()
	router.ServeHTTP(preflightRec, preflightReq)

	if preflightRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for preflight, got %d", preflightRec.Code)
	}
	var preflightRes preflightResponse
	if err := json.Unmarshal(preflightRec.Body.Bytes(), &preflightRes); err != nil {
		t.Fatalf("failed to decode preflight response: %v", err)
	}
	if preflightRes.Data.Allowed {
		t.Fatal("expected preflight to report allowed=false for blocked node")
	}
	if len(preflightRes.Data.Diagnostics) == 0 {
		t.Fatal("expected diagnostics in preflight response")
	}

	// 4. POST /api/v1/publications MUST be intercepted with HTTP 409 Conflict
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications", strings.NewReader(`{"target": "clash"}`))
	pubReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	pubReq.Header.Set("Content-Type", "application/json")
	pubRec := httptest.NewRecorder()
	router.ServeHTTP(pubRec, pubReq)

	if pubRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when publishing blocked node, got %d. body: %s", pubRec.Code, pubRec.Body.String())
	}

	var errResp testErrorResponse
	if err := json.Unmarshal(pubRec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if errResp.Code != "publication_preflight_rejected" {
		t.Fatalf("expected error code publication_preflight_rejected, got %s", errResp.Code)
	}

	// 5. Verify NO publication was created in the database
	var pubCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM publications").Scan(&pubCount); err != nil {
		t.Fatalf("failed to count publications: %v", err)
	}
	if pubCount != 0 {
		t.Fatalf("expected 0 publications after preflight rejection, got %d", pubCount)
	}
}

func TestExistingPublicationImmutableToSubsequentObservationsHTTP(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPublicationTestRouter(t, db)
	_, nodeID := seedSamplePolicyData(t, db)

	// 1. Create provider settings and activate a risk policy
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	provSettings := &domain.IPRiskProviderSettings{
		Provider: "ipinfo", SchemaVersion: "v1", Enabled: true,
		SecretReference: "secret://fixture", MaxConcurrency: 1,
		RequestsPerMinute: 10, DailyRequestBudget: 100,
		PerRequestTimeout: 1500 * time.Millisecond, MaxResponseBytes: 1024,
	}
	if err := providerRepo.Upsert(context.Background(), provSettings); err != nil {
		t.Fatalf("failed to insert provider settings: %v", err)
	}

	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	policyRevID := domain.MustNewUUIDv7()
	policyRev := domain.RiskPolicyRevision{
		RiskPolicy: domain.RiskPolicy{
			RevisionID: policyRevID,
			ProviderSelection: domain.RiskProviderSelection{
				Mode: domain.RiskFusionSingleProvider,
				Providers: []domain.ProviderRef{
					{Provider: "ipinfo", SchemaVersion: "v1"},
				},
			},
			MaxObservationAge: 24 * time.Hour,
			MinimumConfidence: 50,
			ScoreBands: []domain.ScoreBand{
				{Min: 0, Max: 25, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
				{Min: 26, Max: 60, Band: domain.RiskBandMedium, Action: domain.RiskActionReview},
				{Min: 61, Max: 85, Band: domain.RiskBandHigh, Action: domain.RiskActionBlock},
				{Min: 86, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
			},
			UnknownAction:  domain.RiskActionReview,
			ConflictAction: domain.RiskActionReview,
			ReviewAction:   domain.RiskActionBlock,
		},
		Active:    true,
		CreatedAt: time.Now().UTC(),
	}
	if err := riskPolicyRepo.Create(context.Background(), &policyRev); err != nil {
		t.Fatalf("failed to create risk policy: %v", err)
	}

	// 2. Add initial low-risk observation (score=10 -> allow)
	obsRepo := sqlite.NewIPRiskObservationRepository(db)
	obsID1 := domain.MustNewUUIDv7()
	score10 := 10
	conf90 := 90
	h1 := sha256.Sum256([]byte(obsID1 + "ipinfo" + "v1"))
	now := time.Now().UTC()
	obs1 := domain.IPRiskObservation{
		ID:                    obsID1,
		NodeLogicalID:         nodeID,
		ExitIdentityDigest:    "sha256:" + strings.Repeat("1", 64),
		Provider:              "ipinfo",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(12 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &score10,
		Confidence:            &conf90,
		NetworkClass:          domain.NetworkClassResidential,
		EvidenceDigest:        "sha256:" + hex.EncodeToString(h1[:]),
		RedactedSummary:       "low risk observation",
	}
	if err := obsRepo.Create(context.Background(), &obs1); err != nil {
		t.Fatalf("failed to insert initial observation: %v", err)
	}

	// 3. Create initial publication via admin API -> 201 Created
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications", strings.NewReader(`{"target": "clash"}`))
	pubReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	pubReq.Header.Set("Content-Type", "application/json")
	pubRec := httptest.NewRecorder()
	router.ServeHTTP(pubRec, pubReq)

	if pubRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for initial publication, got %d. body: %s", pubRec.Code, pubRec.Body.String())
	}

	var pubResp publicationCreateResponse
	if err := json.Unmarshal(pubRec.Body.Bytes(), &pubResp); err != nil {
		t.Fatalf("failed to decode publish response: %v", err)
	}
	initialPubID := pubResp.Data.Publication.ID
	rawToken := pubResp.Data.RawToken
	initialContentDigest := pubResp.Data.ContentDigest

	// 4. Verify initial publication is served via client subscription endpoint
	clientReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", initialPubID, rawToken), nil)
	clientRec := httptest.NewRecorder()
	router.ServeHTTP(clientRec, clientReq)
	if clientRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK serving publication, got %d", clientRec.Code)
	}
	if clientRec.Header().Get("X-Content-Digest") != initialContentDigest {
		t.Fatalf("expected content digest %s, got %s", initialContentDigest, clientRec.Header().Get("X-Content-Digest"))
	}
	initialBody := clientRec.Body.String()

	// 5. Subsequent observation arrives with high risk (score=95 -> block)
	obsID2 := domain.MustNewUUIDv7()
	score95 := 95
	h2 := sha256.Sum256([]byte(obsID2 + "ipinfo" + "v1"))
	obs2 := domain.IPRiskObservation{
		ID:                    obsID2,
		NodeLogicalID:         nodeID,
		ExitIdentityDigest:    "sha256:" + strings.Repeat("2", 64),
		Provider:              "ipinfo",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now.Add(1 * time.Hour),
		ExpiresAt:             now.Add(13 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &score95,
		Confidence:            &conf90,
		NetworkClass:          domain.NetworkClassResidential,
		EvidenceDigest:        "sha256:" + hex.EncodeToString(h2[:]),
		RedactedSummary:       "high risk subsequent observation",
	}
	if err := obsRepo.Create(context.Background(), &obs2); err != nil {
		t.Fatalf("failed to insert high-risk observation: %v", err)
	}

	// 6. Client subscription endpoint MUST continue serving the EXISTING immutable publication unchanged!
	clientReq2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", initialPubID, rawToken), nil)
	clientRec2 := httptest.NewRecorder()
	router.ServeHTTP(clientRec2, clientReq2)
	if clientRec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK serving existing publication, got %d", clientRec2.Code)
	}
	if clientRec2.Header().Get("X-Content-Digest") != initialContentDigest {
		t.Fatalf("existing publication content digest changed after subsequent observation! %s vs %s", initialContentDigest, clientRec2.Header().Get("X-Content-Digest"))
	}
	if clientRec2.Body.String() != initialBody {
		t.Fatal("existing publication body was altered by subsequent observation!")
	}

	// 7. Attempting to create a NEW publication now MUST be rejected with HTTP 409 Conflict
	newPubReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications", strings.NewReader(`{"target": "clash"}`))
	newPubReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	newPubReq.Header.Set("Content-Type", "application/json")
	newPubRec := httptest.NewRecorder()
	router.ServeHTTP(newPubRec, newPubReq)

	if newPubRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when publishing after risk change, got %d. body: %s", newPubRec.Code, newPubRec.Body.String())
	}

	// 8. The existing publication MUST NOT be replaced, modified, or revoked
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/publications/%s", initialPubID), nil)
	getReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET existing publication, got %d", getRec.Code)
	}
	var detailResp struct {
		Data struct {
			Publication struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"publication"`
			ContentDigest string `json:"content_digest"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("failed to decode GET publication response: %v", err)
	}
	if detailResp.Data.Publication.State != string(domain.PublicationStateActive) {
		t.Fatalf("expected existing publication to remain active, got %s", detailResp.Data.Publication.State)
	}
	if detailResp.Data.ContentDigest != initialContentDigest {
		t.Fatalf("existing publication content digest changed: %s vs %s", detailResp.Data.ContentDigest, initialContentDigest)
	}
}

func TestAdminPreview_CapabilityBoundaries(t *testing.T) {
	// Subcase 1: Target Clash cannot render Hysteria2 node protocol -> 422 unsupported_target_capability
	t.Run("Hysteria2ProtocolNotSupportedByClash", func(t *testing.T) {
		db := newCleanSQLiteDB(t)
		router, _ := setupPublicationTestRouter(t, db)
		ctx := context.Background()

		nodeRepo := sqlite.NewNodeRepository(db)
		nodeID := domain.ComputeNodeLogicalID(domain.ProtocolHysteria2, "hy2.example.com", 443, nil)
		err := nodeRepo.UpsertBatch(ctx, []domain.Node{
			{
				LogicalID:                 nodeID,
				Protocol:                  domain.ProtocolHysteria2,
				DisplayName:               "Hy2-01",
				Active:                    true,
				NormalizedConfigSecretRef: "secret://hy2",
				UpdatedAt:                 domain.NowUTC(),
			},
		})
		if err != nil {
			t.Fatalf("failed to insert node: %v", err)
		}

		revRepo := sqlite.NewRevisionRepository(db)
		revID, _ := domain.NewUUIDv7()
		err = revRepo.Create(ctx, &domain.ConfigurationRevision{
			ID:            revID,
			ContentDigest: "sha256:hy2-rev-digest",
			State:         domain.RevisionStateActive,
			CreatedAt:     domain.NowUTC(),
		})
		if err != nil {
			t.Fatalf("failed to insert revision: %v", err)
		}

		policyRepo := sqlite.NewPolicyRepository(db)
		groupID, _ := domain.NewUUIDv7()
		err = policyRepo.CreateGroup(ctx, &domain.NodeGroup{
			ID:        groupID,
			Name:      "PROXY",
			GroupType: domain.GroupTypeSelect,
		})
		if err != nil {
			t.Fatalf("failed to insert group: %v", err)
		}

		// Request preview for target "surge" (which does not support hysteria2)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{"target": "surge"}`))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 Unprocessable Entity, got %d. body: %s", rec.Code, rec.Body.String())
		}
		var errResp transporthttp.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to parse error response: %v", err)
		}
		if errResp.Code != "unsupported_target_capability" {
			t.Fatalf("expected error code unsupported_target_capability, got %s", errResp.Code)
		}
		if !strings.Contains(errResp.Message, "hysteria2") || !strings.Contains(errResp.Message, "protocol is not supported") {
			t.Fatalf("expected message to mention unsupported protocol, got %s", errResp.Message)
		}

		// Same setup for target "clash" and "mihomo" (which support hysteria2) -> 200 OK
		for _, target := range []string{"clash", "mihomo"} {
			reqTarget := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{"target": "`+target+`"}`))
			reqTarget.Header.Set("Authorization", "Bearer "+testAdminToken)
			reqTarget.Header.Set("Content-Type", "application/json")
			recTarget := httptest.NewRecorder()
			router.ServeHTTP(recTarget, reqTarget)

			if recTarget.Code != http.StatusOK {
				t.Fatalf("expected 200 OK for %s with hysteria2, got %d. body: %s", target, recTarget.Code, recTarget.Body.String())
			}
		}
	})

	// Subcase 2: Target Quantumult-X cannot render url-test group type -> 422 unsupported_target_capability
	t.Run("URLTestGroupNotSupportedByQuantumultX", func(t *testing.T) {
		db := newCleanSQLiteDB(t)
		router, _ := setupPublicationTestRouter(t, db)
		ctx := context.Background()

		nodeRepo := sqlite.NewNodeRepository(db)
		nodeID := domain.ComputeNodeLogicalID(domain.ProtocolTrojan, "trojan.example.com", 443, nil)
		err := nodeRepo.UpsertBatch(ctx, []domain.Node{
			{
				LogicalID:                 nodeID,
				Protocol:                  domain.ProtocolTrojan,
				DisplayName:               "Trojan-01",
				Active:                    true,
				NormalizedConfigSecretRef: "secret://trojan",
				UpdatedAt:                 domain.NowUTC(),
			},
		})
		if err != nil {
			t.Fatalf("failed to insert node: %v", err)
		}

		revRepo := sqlite.NewRevisionRepository(db)
		revID, _ := domain.NewUUIDv7()
		err = revRepo.Create(ctx, &domain.ConfigurationRevision{
			ID:            revID,
			ContentDigest: "sha256:qx-rev-digest",
			State:         domain.RevisionStateActive,
			CreatedAt:     domain.NowUTC(),
		})
		if err != nil {
			t.Fatalf("failed to insert revision: %v", err)
		}

		policyRepo := sqlite.NewPolicyRepository(db)
		groupID, _ := domain.NewUUIDv7()
		err = policyRepo.CreateGroup(ctx, &domain.NodeGroup{
			ID:        groupID,
			Name:      "AUTO",
			GroupType: domain.GroupTypeURLTest, // Quantumult-X only supports Select
		})
		if err != nil {
			t.Fatalf("failed to insert group: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{"target": "qx"}`))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 Unprocessable Entity, got %d. body: %s", rec.Code, rec.Body.String())
		}
		var errResp transporthttp.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to parse error response: %v", err)
		}
		if errResp.Code != "unsupported_target_capability" {
			t.Fatalf("expected error code unsupported_target_capability, got %s", errResp.Code)
		}
		if !strings.Contains(errResp.Message, "urltest") || !strings.Contains(errResp.Message, "policy group type is not supported") {
			t.Fatalf("expected message to mention unsupported group type, got %s", errResp.Message)
		}
	})

	// Subcase 3: Target Clash cannot render GEOSITE routing rule -> 422 unsupported_target_capability
	t.Run("GEOSITERuleNotSupportedByClash", func(t *testing.T) {
		db := newCleanSQLiteDB(t)
		router, _ := setupPublicationTestRouter(t, db)
		ctx := context.Background()

		nodeRepo := sqlite.NewNodeRepository(db)
		nodeID := domain.ComputeNodeLogicalID(domain.ProtocolTrojan, "trojan.example.com", 443, nil)
		err := nodeRepo.UpsertBatch(ctx, []domain.Node{
			{
				LogicalID:                 nodeID,
				Protocol:                  domain.ProtocolTrojan,
				DisplayName:               "Trojan-01",
				Active:                    true,
				NormalizedConfigSecretRef: "secret://trojan",
				UpdatedAt:                 domain.NowUTC(),
			},
		})
		if err != nil {
			t.Fatalf("failed to insert node: %v", err)
		}

		revRepo := sqlite.NewRevisionRepository(db)
		revID, _ := domain.NewUUIDv7()
		err = revRepo.Create(ctx, &domain.ConfigurationRevision{
			ID:            revID,
			ContentDigest: "sha256:rule-rev-digest",
			State:         domain.RevisionStateActive,
			CreatedAt:     domain.NowUTC(),
		})
		if err != nil {
			t.Fatalf("failed to insert revision: %v", err)
		}

		policyRepo := sqlite.NewPolicyRepository(db)
		groupID, _ := domain.NewUUIDv7()
		err = policyRepo.CreateGroup(ctx, &domain.NodeGroup{
			ID:        groupID,
			Name:      "PROXY",
			GroupType: domain.GroupTypeSelect,
		})
		if err != nil {
			t.Fatalf("failed to insert group: %v", err)
		}

		ruleID, _ := domain.NewUUIDv7()
		err = policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
			ID:            ruleID,
			RevisionID:    revID,
			TargetGroupID: groupID,
			Expression:    "GEOSITE,category-ads-all", // Surge does not support GEOSITE
			Position:      0,
		})
		if err != nil {
			t.Fatalf("failed to insert rule: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{"target": "surge"}`))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 Unprocessable Entity, got %d. body: %s", rec.Code, rec.Body.String())
		}
		var errResp transporthttp.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to parse error response: %v", err)
		}
		if errResp.Code != "unsupported_target_capability" {
			t.Fatalf("expected error code unsupported_target_capability, got %s", errResp.Code)
		}
		if !strings.Contains(errResp.Message, "GEOSITE") || !strings.Contains(errResp.Message, "routing rule kind is not supported") {
			t.Fatalf("expected message to mention unsupported rule kind, got %s", errResp.Message)
		}
	})
}

func TestAdminPreview_InputAndRevisionBoundaries(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPublicationTestRouter(t, db)

	// 1. Invalid JSON body -> 422 invalid_json
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{invalid json`))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for invalid JSON, got %d", rec.Code)
		}
		var errResp transporthttp.ErrorResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp.Code != "invalid_json" {
			t.Fatalf("expected code invalid_json, got %s", errResp.Code)
		}
	}

	// 2. Missing target -> 422 invalid_target
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for missing target, got %d", rec.Code)
		}
		var errResp transporthttp.ErrorResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp.Code != "invalid_target" {
			t.Fatalf("expected code invalid_target, got %s", errResp.Code)
		}
	}

	// 3. Unknown target -> 422 unsupported_target
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{"target": "unknown-compiler"}`))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for unknown target, got %d", rec.Code)
		}
		var errResp transporthttp.ErrorResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp.Code != "unsupported_target" {
			t.Fatalf("expected code unsupported_target, got %s", errResp.Code)
		}
	}

	// 4. No active revision in DB -> 409 no_active_revision
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{"target": "clash"}`))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 for no active revision, got %d", rec.Code)
		}
		var errResp transporthttp.ErrorResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp.Code != "no_active_revision" {
			t.Fatalf("expected code no_active_revision, got %s", errResp.Code)
		}
	}

	// 5. Non-existent revision_id -> 404 revision_not_found
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{"target": "clash", "revision_id": "0195c100-0000-7000-8000-000000000000"}`))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for non-existent revision_id, got %d", rec.Code)
		}
		var errResp transporthttp.ErrorResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp.Code != "revision_not_found" {
			t.Fatalf("expected code revision_not_found, got %s", errResp.Code)
		}
	}
}

func TestAdminPublish_CapabilityBoundaries(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _ := setupPublicationTestRouter(t, db)
	ctx := context.Background()

	// Seed Hysteria2 node and active revision
	nodeRepo := sqlite.NewNodeRepository(db)
	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolHysteria2, "hy2.example.com", 443, nil)
	err := nodeRepo.UpsertBatch(ctx, []domain.Node{
		{
			LogicalID:                 nodeID,
			Protocol:                  domain.ProtocolHysteria2,
			DisplayName:               "Hy2-01",
			Active:                    true,
			NormalizedConfigSecretRef: "secret://hy2",
			UpdatedAt:                 domain.NowUTC(),
		},
	})
	if err != nil {
		t.Fatalf("failed to insert node: %v", err)
	}

	revRepo := sqlite.NewRevisionRepository(db)
	revID, _ := domain.NewUUIDv7()
	err = revRepo.Create(ctx, &domain.ConfigurationRevision{
		ID:            revID,
		ContentDigest: "sha256:hy2-publish-digest",
		State:         domain.RevisionStateActive,
		CreatedAt:     domain.NowUTC(),
	})
	if err != nil {
		t.Fatalf("failed to insert revision: %v", err)
	}

	policyRepo := sqlite.NewPolicyRepository(db)
	groupID, _ := domain.NewUUIDv7()
	err = policyRepo.CreateGroup(ctx, &domain.NodeGroup{
		ID:        groupID,
		Name:      "PROXY",
		GroupType: domain.GroupTypeSelect,
	})
	if err != nil {
		t.Fatalf("failed to insert group: %v", err)
	}

	// POST /api/v1/publications with target Surge (which does not support hysteria2)
	publishReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications", strings.NewReader(`{"target": "surge"}`))
	publishReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	publishReq.Header.Set("Content-Type", "application/json")
	publishRec := httptest.NewRecorder()
	router.ServeHTTP(publishRec, publishReq)

	if publishRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for publish capability error, got %d. body: %s", publishRec.Code, publishRec.Body.String())
	}
	var errResp transporthttp.ErrorResponse
	if err := json.Unmarshal(publishRec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Code != "unsupported_target_capability" {
		t.Fatalf("expected code unsupported_target_capability, got %s", errResp.Code)
	}
}

func TestWriteDomainError_CapabilityErrorAndUnexpectedError(t *testing.T) {
	// 1. CapabilityError maps to 422 unsupported_target_capability
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		capErr := &compiler.CapabilityError{
			Target:   domain.TargetClash,
			Location: "nodes[0]",
			Feature:  "hysteria2",
			Reason:   "protocol is not supported",
		}
		transporthttp.WriteDomainError(rec, req, capErr)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for CapabilityError, got %d", rec.Code)
		}
		var errResp transporthttp.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to parse error response: %v", err)
		}
		if errResp.Code != "unsupported_target_capability" {
			t.Fatalf("expected code unsupported_target_capability, got %s", errResp.Code)
		}
		if !strings.Contains(errResp.Message, "hysteria2") {
			t.Fatalf("expected message to contain feature, got %s", errResp.Message)
		}
	}

	// 2. Unexpected error maps to 500 internal_error without leaking details
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		unexpectedErr := errors.New("raw unredacted db connection string postgres://secret@host/db")
		transporthttp.WriteDomainError(rec, req, unexpectedErr)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 for unexpected error, got %d", rec.Code)
		}
		var errResp transporthttp.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to parse error response: %v", err)
		}
		if errResp.Code != "internal_error" {
			t.Fatalf("expected code internal_error, got %s", errResp.Code)
		}
		if strings.Contains(errResp.Message, "postgres") || strings.Contains(errResp.Message, "secret") {
			t.Fatalf("raw error details leaked in 500 message: %s", errResp.Message)
		}
	}
}

