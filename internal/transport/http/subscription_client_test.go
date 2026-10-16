package http_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func seedFullLiveSubscriptionData(t *testing.T, db *sql.DB, target domain.CompilerTarget) (*domain.SubscriptionLink, string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Revision
	revID := "rev-client-1"
	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-rev-1', 'active', ?);
	`, revID, now.Format(time.RFC3339Nano))

	// 2. Admission rule
	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO admission_rules (id, revision_id, name, expression, action, position)
		VALUES ('ar-client-1', ?, 'Allow All', 'true', 'allow', 0);
	`, revID)

	// 3. Subscription & fetch
	subID := "sub-client-1"
	fetchID := "fetch-client-1"
	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES (?, 'Sub Client', 'secret://url', 1, '1', ?, ?);
	`, subID, now.Format(time.RFC3339), now.Format(time.RFC3339))

	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 'hash-1', 1, 1);
	`, fetchID, subID, now.Format(time.RFC3339Nano))

	// 4. Node
	nodeID := "node_0123456789abcdef"
	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, 'ss', 'Live Node 1', '1.2.3.4', 8388, '{"method":"aes-128-gcm","password":"secret"}', 1, ?, ?, 1);
	`, nodeID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))

	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?);
	`, nodeID, subID, fetchID)

	// 5. Node group & source (for Mihomo)
	groupID := domain.MustNewUUIDv7()
	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision)
		VALUES (?, 'Proxy', 'select', ?, ?, 0, 1);
	`, groupID, now.Format(time.RFC3339), now.Format(time.RFC3339))

	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO policy_group_sources (id, group_id, operation, kind, position, node_logical_id, created_at, updated_at)
		VALUES ('pgs-c-1', ?, 'add', 'manual_node', 0, ?, ?, ?);
	`, groupID, nodeID, now.Format(time.RFC3339), now.Format(time.RFC3339))

	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO policy_rules (id, revision_id, target_group_id, expression, position)
		VALUES ('pr-c-1', ?, ?, 'MATCH', 0);
	`, revID, groupID)

	// 6. Create subscription link
	rawToken := "pub_secret_client_token_1234567890abcdef"
	tokenHash := publication.HashToken(rawToken)
	linkID := "link-" + string(target) + "-1"
	link := &domain.SubscriptionLink{
		ID:        linkID,
		Label:     "Test Link " + string(target),
		Target:    target,
		Token:     rawToken,
		TokenHash: tokenHash,
		State:     domain.SubscriptionLinkStateActive,
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	linkRepo := sqlite.NewSubscriptionLinkRepository(db)
	_ = linkRepo.Create(ctx, link)

	return link, rawToken
}

func TestLiveSubscriptionClientEndpoint_AuthenticationAndRejections(t *testing.T) {
	router, db, _, _ := setupLiveSubTestRouter(t)

	link, rawToken := seedFullLiveSubscriptionData(t, db, domain.TargetMihomo)

	basePath := "/subscribe/v1/" + link.ID

	assertSecurityHeaders := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("expected Cache-Control: no-store, got %q", cc)
		}
		if ref := rec.Header().Get("Referrer-Policy"); ref != "no-referrer" {
			t.Fatalf("expected Referrer-Policy: no-referrer, got %q", ref)
		}
	}

	// 1. Valid request via Query parameter
	recQuery := doRequest(router, http.MethodGet, basePath+"?token="+rawToken, "", "")
	if recQuery.Code != http.StatusOK {
		t.Fatalf("GET via query token failed: %d, body: %s", recQuery.Code, recQuery.Body.String())
	}
	assertSecurityHeaders(t, recQuery)
	etag := recQuery.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header on 200 response")
	}

	// 2. Valid request via Authorization: Bearer <token>
	recBearer := doRequest(router, http.MethodGet, basePath, "", rawToken)
	if recBearer.Code != http.StatusOK {
		t.Fatalf("GET via Bearer token failed: %d, body: %s", recBearer.Code, recBearer.Body.String())
	}
	assertSecurityHeaders(t, recBearer)

	// 3. Ambiguous credentials: both query token and Bearer token -> 401 token_ambiguous
	recBoth := doRequest(router, http.MethodGet, basePath+"?token="+rawToken, "", rawToken)
	if recBoth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for both query and Bearer, got %d", recBoth.Code)
	}
	assertSecurityHeaders(t, recBoth)

	// 4. Duplicate query parameter: ?token=a&token=b -> 400 token_ambiguous
	recDup := doRequest(router, http.MethodGet, basePath+"?token="+rawToken+"&token=other", "", "")
	if recDup.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate query token, got %d", recDup.Code)
	}
	assertSecurityHeaders(t, recDup)

	// 5. Casing collision query parameter: ?token=a&TOKEN=b -> 400 token_ambiguous
	recCase := doRequest(router, http.MethodGet, basePath+"?token="+rawToken+"&TOKEN="+rawToken, "", "")
	if recCase.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for case duplicate token, got %d", recCase.Code)
	}
	assertSecurityHeaders(t, recCase)

	// 6. Forbidden override parameters: ?token=a&target=singbox -> 422 override_parameters_rejected
	recOverride := doRequest(router, http.MethodGet, basePath+"?token="+rawToken+"&target=singbox", "", "")
	if recOverride.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for forbidden override parameters, got %d", recOverride.Code)
	}
	assertSecurityHeaders(t, recOverride)

	// 7. Missing credentials -> 401 unauthorized
	recNoAuth := doRequest(router, http.MethodGet, basePath, "", "")
	if recNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing credentials, got %d", recNoAuth.Code)
	}
	assertSecurityHeaders(t, recNoAuth)

	// 8. Invalid token -> 401 unauthorized
	recBadToken := doRequest(router, http.MethodGet, basePath+"?token=wrong_token", "", "")
	if recBadToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad token, got %d", recBadToken.Code)
	}
	assertSecurityHeaders(t, recBadToken)

	// 9. Master token or Admin token used on client subscription -> 401 unauthorized
	recMaster := doRequest(router, http.MethodGet, basePath+"?token=admin-secret-token", "", "")
	if recMaster.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when master/admin token is used on client link, got %d", recMaster.Code)
	}
	assertSecurityHeaders(t, recMaster)

	// 10. Non-existent link ID -> 404
	recNotFound := doRequest(router, http.MethodGet, "/subscribe/v1/non-existent-link?token="+rawToken, "", "")
	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent link, got %d", recNotFound.Code)
	}
	assertSecurityHeaders(t, recNotFound)
}

func TestLiveSubscriptionClientEndpoint_ConditionalRequestAndRevocation(t *testing.T) {
	router, db, _, pubSvc := setupLiveSubTestRouter(t)

	link, rawToken := seedFullLiveSubscriptionData(t, db, domain.TargetSingBox)
	basePath := "/subscribe/v1/" + link.ID

	// 1. Initial request -> 200 OK with ETag
	rec := doRequest(router, http.MethodGet, basePath+"?token="+rawToken, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("initial GET failed: %d", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	// 2. Request with matching If-None-Match -> 304 Not Modified
	req304 := httptest.NewRequest(http.MethodGet, basePath+"?token="+rawToken, nil)
	req304.Header.Set("If-None-Match", etag)
	rec304 := httptest.NewRecorder()
	router.ServeHTTP(rec304, req304)

	if rec304.Code != http.StatusNotModified {
		t.Fatalf("expected 304 Not Modified, got %d", rec304.Code)
	}
	if cc := rec304.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("expected Cache-Control: no-store on 304, got %q", cc)
	}
	if ref := rec304.Header().Get("Referrer-Policy"); ref != "no-referrer" {
		t.Fatalf("expected Referrer-Policy: no-referrer on 304, got %q", ref)
	}

	// 3. Revoke link -> subsequent conditional request with matching ETag MUST return 403 (checked before 304)
	err := pubSvc.RevokeSubscriptionLink(context.Background(), link.ID, domain.ActorKindAdmin, "test-rev")
	if err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	reqRevoked := httptest.NewRequest(http.MethodGet, basePath+"?token="+rawToken, nil)
	reqRevoked.Header.Set("If-None-Match", etag)
	recRevoked := httptest.NewRecorder()
	router.ServeHTTP(recRevoked, reqRevoked)

	if recRevoked.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for revoked link on conditional request, got %d", recRevoked.Code)
	}
	if cc := recRevoked.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("expected Cache-Control: no-store on 403, got %q", cc)
	}
}

func TestLiveSubscriptionClientEndpoint_NonMihomoIgnoresEmptyGroup(t *testing.T) {
	router, db, _, _ := setupLiveSubTestRouter(t)

	// Seed for TargetSurge
	link, rawToken := seedFullLiveSubscriptionData(t, db, domain.TargetSurge)

	// Add an empty group to database
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision)
		VALUES ('grp-empty', 'Empty Group', 'select', datetime('now'), datetime('now'), 0, 1);
	`)
	if err != nil {
		t.Fatalf("insert empty group failed: %v", err)
	}

	// Request Surge subscription: empty group MUST NOT block pure nodes export
	rec := doRequest(router, http.MethodGet, "/subscribe/v1/"+link.ID+"?token="+rawToken, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Surge with empty group in DB, got %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestExportTokenCannotAccessAdminAPI(t *testing.T) {
	router, db, settingsRepo, pubSvc := setupLiveSubTestRouter(t)

	_, rawToken := seedFullLiveSubscriptionData(t, db, domain.TargetMihomo)

	// 1. Using the client subscription token as Bearer to call Admin API -> 403 invalid_token_scope
	recAdmin := doRequest(router, http.MethodGet, "/api/v1/subscription-links", "", rawToken)
	if recAdmin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 invalid_token_scope when export token accesses admin API, got %d body: %s", recAdmin.Code, recAdmin.Body.String())
	}

	// 2. Open Mode: even when Admin authentication is open/disabled, export token MUST STILL be forbidden
	openRouter := transporthttp.NewRouter(transporthttp.RouterConfig{
		SettingsRepository: settingsRepo,
		AuditRepository:    sqlite.NewAuditRepository(db),
		PublicationService: pubSvc,
		IsPublicationToken: pubSvc.IsPublicationToken,
	})
	recOpenAdmin := doRequest(openRouter, http.MethodGet, "/api/v1/subscription-links", "", rawToken)
	if recOpenAdmin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 invalid_token_scope in open mode when export token accesses admin API, got %d body: %s", recOpenAdmin.Code, recOpenAdmin.Body.String())
	}

	// But an anonymous request without the export token succeeds in open mode
	recOpenAnon := doRequest(openRouter, http.MethodGet, "/api/v1/subscription-links", "", "")
	if recOpenAnon.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for anonymous request in open mode, got %d", recOpenAnon.Code)
	}
}

func TestLiveSubscriptionClientEndpoint_RotateToken_OldTokenRejected_NewTokenAccepted(t *testing.T) {
	router, db, _, pubSvc := setupLiveSubTestRouter(t)

	link, oldToken := seedFullLiveSubscriptionData(t, db, domain.TargetMihomo)
	basePath := "/subscribe/v1/" + link.ID

	// 1. Initially old token works
	recOld1 := doRequest(router, http.MethodGet, basePath+"?token="+oldToken, "", "")
	if recOld1.Code != http.StatusOK {
		t.Fatalf("initial GET with old token failed: %d", recOld1.Code)
	}

	// 2. Rotate token
	updatedLink, err := pubSvc.RotateSubscriptionLinkToken(context.Background(), link.ID, domain.ActorKindAdmin, "test-rotate")
	if err != nil {
		t.Fatalf("RotateSubscriptionLinkToken failed: %v", err)
	}
	newToken := updatedLink.Token

	// 3. Old token MUST return 401 Unauthorized
	recOld2 := doRequest(router, http.MethodGet, basePath+"?token="+oldToken, "", "")
	if recOld2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for rotated old token, got %d", recOld2.Code)
	}

	// 4. New token MUST return 200 OK
	recNew := doRequest(router, http.MethodGet, basePath+"?token="+newToken, "", "")
	if recNew.Code != http.StatusOK {
		t.Fatalf("expected 200 for new token, got %d body: %s", recNew.Code, recNew.Body.String())
	}
}

func TestLiveSubscriptionClientEndpoint_ZeroExportableNodes_Returns422(t *testing.T) {
	router, db, _, _ := setupLiveSubTestRouter(t)

	link, rawToken := seedFullLiveSubscriptionData(t, db, domain.TargetSingBox)

	// Deactivate all nodes
	_, err := db.ExecContext(context.Background(), `UPDATE nodes SET active = 0;`)
	if err != nil {
		t.Fatalf("failed to deactivate nodes: %v", err)
	}

	rec := doRequest(router, http.MethodGet, "/subscribe/v1/"+link.ID+"?token="+rawToken, "", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity when 0 exportable nodes, got %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestLiveSubscriptionClientEndpoint_ConcurrentRequestsRaceSafety_And_ReadOnlyNoDraft(t *testing.T) {
	router, db, _, _ := setupLiveSubTestRouter(t)

	link, rawToken := seedFullLiveSubscriptionData(t, db, domain.TargetMihomo)
	basePath := "/subscribe/v1/" + link.ID + "?token=" + rawToken

	// Count revisions and publications before requests
	var revCountBefore, pubCountBefore int
	_ = db.QueryRow(`SELECT count(*) FROM configuration_revisions WHERE state = 'draft';`).Scan(&revCountBefore)
	_ = db.QueryRow(`SELECT count(*) FROM publications;`).Scan(&pubCountBefore)

	const concurrency = 10
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)
	etags := make([]string, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rec := doRequest(router, http.MethodGet, basePath, "", "")
			if rec.Code != http.StatusOK {
				errCh <- fmt.Errorf("goroutine %d got status %d: %s", idx, rec.Code, rec.Body.String())
				return
			}
			etag := rec.Header().Get("ETag")
			if etag == "" {
				errCh <- fmt.Errorf("goroutine %d missing ETag", idx)
				return
			}
			etags[idx] = etag
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}

	// Verify all concurrent requests produced identical deterministic ETag
	firstEtag := etags[0]
	for idx, etag := range etags {
		if etag != firstEtag {
			t.Fatalf("goroutine %d produced differing ETag: %s vs %s", idx, etag, firstEtag)
		}
	}

	// Verify ZERO drafts or publication records were created (strictly read-only)
	var revCountAfter, pubCountAfter int
	_ = db.QueryRow(`SELECT count(*) FROM configuration_revisions WHERE state = 'draft';`).Scan(&revCountAfter)
	_ = db.QueryRow(`SELECT count(*) FROM publications;`).Scan(&pubCountAfter)

	if revCountAfter != revCountBefore {
		t.Fatalf("client GET created drafts: before=%d after=%d", revCountBefore, revCountAfter)
	}
	if pubCountAfter != pubCountBefore {
		t.Fatalf("client GET persisted publications: before=%d after=%d", pubCountBefore, pubCountAfter)
	}
}

func TestLiveSubscriptionClientEndpoint_ContextCancellation(t *testing.T) {
	router, db, _, _ := setupLiveSubTestRouter(t)

	link, rawToken := seedFullLiveSubscriptionData(t, db, domain.TargetMihomo)
	basePath := "/subscribe/v1/" + link.ID + "?token=" + rawToken

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before executing

	req := httptest.NewRequest(http.MethodGet, basePath, nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	// Should not panic, should return cleanly
	router.ServeHTTP(rec, req)
}
