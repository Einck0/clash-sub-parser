package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

type subscriptionResponse struct {
	ID                 string                    `json:"id"`
	Name               string                    `json:"name"`
	SourceURLSecretRef string                    `json:"source_url_secret_ref"`
	Enabled            bool                      `json:"enabled"`
	IsPrimary          bool                      `json:"is_primary"`
	UserInfo           string                    `json:"userinfo,omitempty"`
	Config             domain.SubscriptionConfig `json:"config"`
	Revision           string                    `json:"revision"`
}

func subscriptionTestRouter(t *testing.T) http.Handler {
	t.Helper()
	cfg := sqlite.Config{
		Path:            "file:subscription_http_test?mode=memory&cache=shared",
		BusyTimeout:     time.Second,
		ForeignKeys:     true,
		WALMode:         false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	routerCfg := newTestRouterConfig(true)
	routerCfg.SubscriptionService = subscription.NewService(sqlite.NewSubscriptionRepository(db), sqlite.NewAuditRepository(db))
	return transporthttp.NewRouter(routerCfg)
}

func TestSubscriptionEndpointsEnforceSecurityAndReturnPlaintextURL(t *testing.T) {
	router := subscriptionTestRouter(t)
	secretRef := "https://example.com/subscriptions/primary?token=real-url"
	body := `{"name":"Primary","source_url_secret_ref":"` + secretRef + `","enabled":true,"refresh_policy":{"interval_seconds":3600}}`

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized POST status = %d, want 401", unauthorized.Code)
	}

	missingCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(body))
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRF.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: testValidSessionID})
	missingCSRFRecorder := httptest.NewRecorder()
	router.ServeHTTP(missingCSRFRecorder, missingCSRF)
	if missingCSRFRecorder.Code != http.StatusForbidden {
		t.Fatalf("cookie POST without CSRF status = %d, want 403", missingCSRFRecorder.Code)
	}

	create := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(body))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Authorization", "Bearer "+testAdminToken)
	created := httptest.NewRecorder()
	router.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}
	var createResponse testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(created.Body.Bytes(), &createResponse); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResponse.Data.SourceURLSecretRef != secretRef || createResponse.Data.ID == "" || createResponse.Data.Revision == "" {
		t.Fatalf("unexpected create response: %#v", createResponse.Data)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions?page=1&page_size=50&enabled_only=true&search_text=Primary", nil)
	list.Header.Set("Authorization", "Bearer "+testAdminToken)
	listed := httptest.NewRecorder()
	router.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), secretRef) {
		t.Fatalf("list status/body = %d %s", listed.Code, listed.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/"+createResponse.Data.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	got := httptest.NewRecorder()
	router.ServeHTTP(got, getReq)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), secretRef) {
		t.Fatalf("get status/body = %d %s", got.Code, got.Body.String())
	}

	patch := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+createResponse.Data.ID, strings.NewReader(`{"name":"Renamed"}`))
	patch.Header.Set("Content-Type", "application/json")
	patch.Header.Set("Authorization", "Bearer "+testAdminToken)
	patch.Header.Set("If-Match", "wrong-revision")
	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, patch)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("stale patch status = %d, body = %s", conflict.Code, conflict.Body.String())
	}

	updatedURL := "https://example.com/subscriptions/updated?token=new-url"
	patchUpdate := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+createResponse.Data.ID, strings.NewReader(`{"source_url_secret_ref":"`+updatedURL+`","config":{"cron_schedule":"0 0 * * *","auto_test":true}}`))
	patchUpdate.Header.Set("Content-Type", "application/json")
	patchUpdate.Header.Set("Authorization", "Bearer "+testAdminToken)
	patchUpdate.Header.Set("If-Match", createResponse.Data.Revision)
	patchRecorder := httptest.NewRecorder()
	router.ServeHTTP(patchRecorder, patchUpdate)
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("patch update status = %d, body = %s", patchRecorder.Code, patchRecorder.Body.String())
	}
	var patchResponse testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(patchRecorder.Body.Bytes(), &patchResponse); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if patchResponse.Data.SourceURLSecretRef != updatedURL {
		t.Fatalf("expected updated URL %q, got %q", updatedURL, patchResponse.Data.SourceURLSecretRef)
	}
	if patchResponse.Data.Config.CronSchedule != "0 0 * * *" || !patchResponse.Data.Config.AutoTest {
		t.Fatalf("expected updated config, got %#v", patchResponse.Data.Config)
	}

	missingRefreshKey := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/"+createResponse.Data.ID+"/refresh", nil)
	missingRefreshKey.Header.Set("Authorization", "Bearer "+testAdminToken)
	missingRefreshKeyRecorder := httptest.NewRecorder()
	router.ServeHTTP(missingRefreshKeyRecorder, missingRefreshKey)
	if missingRefreshKeyRecorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("refresh without idempotency key status = %d, want 422", missingRefreshKeyRecorder.Code)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/subscriptions/"+createResponse.Data.ID, nil)
	deleteRequest.Header.Set("Authorization", "Bearer "+testAdminToken)
	deleted := httptest.NewRecorder()
	router.ServeHTTP(deleted, deleteRequest)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
}

func TestSubscriptionConcurrentPatchRejection(t *testing.T) {
	router := subscriptionTestRouter(t)
	secretRef := "secret://subscriptions/concurrent?token=safe"
	body := `{"name":"ConcurrentBase","source_url_secret_ref":"` + secretRef + `","enabled":true,"refresh_policy":{"interval_seconds":3600}}`

	create := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(body))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Authorization", "Bearer "+testAdminToken)
	created := httptest.NewRecorder()
	router.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}
	var createResponse testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(created.Body.Bytes(), &createResponse); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	subID := createResponse.Data.ID
	baseRevision := createResponse.Data.Revision

	const n = 2
	start := make(chan struct{})
	type patchResult struct {
		code int
		body string
	}
	results := make(chan patchResult, n)

	for i := 0; i < n; i++ {
		go func(idx int) {
			<-start
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+subID, strings.NewReader(`{"name":"PatchWorker"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+testAdminToken)
			req.Header.Set("If-Match", baseRevision)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			results <- patchResult{code: rec.Code, body: rec.Body.String()}
		}(i)
	}

	close(start)

	var okCount, conflictCount int
	for i := 0; i < n; i++ {
		res := <-results
		switch res.code {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Errorf("unexpected status code %d: %s", res.code, res.body)
		}
	}

	if okCount != 1 || conflictCount != 1 {
		t.Fatalf("expected 1 OK (200) and 1 Conflict (409), got %d OK and %d Conflict", okCount, conflictCount)
	}
}

type subscriptionViewResponse struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	SourceURLSecretRef string  `json:"source_url_secret_ref"`
	LastRefreshedAt    *string `json:"last_refreshed_at"`
	LastRefreshOutcome *string `json:"last_refresh_outcome"`
}

func TestSubscriptionEndpointsReturnLastRefreshInfo(t *testing.T) {
	cfg := sqlite.Config{
		Path:            fmt.Sprintf("file:sub_refresh_http_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout:     time.Second,
		ForeignKeys:     true,
		WALMode:         false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	routerCfg := newTestRouterConfig(true)
	routerCfg.SubscriptionService = subscription.NewService(sqlite.NewSubscriptionRepository(db), sqlite.NewAuditRepository(db))
	router := transporthttp.NewRouter(routerCfg)

	// 1. Create a subscription
	body := `{"name":"RefreshTest","source_url_secret_ref":"https://example.com/sub","enabled":true,"refresh_policy":{"interval_seconds":3600}}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	var createdResp testDataResponse[subscriptionViewResponse]
	if err := json.Unmarshal(createRec.Body.Bytes(), &createdResp); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	subID := createdResp.Data.ID

	// Verify initially last_refreshed_at is null
	if !strings.Contains(createRec.Body.String(), `"last_refreshed_at":null`) {
		t.Fatalf("expected last_refreshed_at to be null in create response, got: %s", createRec.Body.String())
	}

	// GET by ID should also have last_refreshed_at null
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/"+subID, nil)
	getReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d", getRec.Code)
	}
	if !strings.Contains(getRec.Body.String(), `"last_refreshed_at":null`) {
		t.Fatalf("expected last_refreshed_at: null in get response, got: %s", getRec.Body.String())
	}

	// 2. Insert a finished fetch record with outcome "failed"
	finishedAt := "2026-09-29T03:14:15Z"
	_, err = db.Exec(`
		INSERT INTO subscription_fetches (
			id, subscription_id, started_at, finished_at, outcome,
			content_digest, redacted_error, nodes_parsed, nodes_valid
		) VALUES ('fetch-http-test-1', ?, '2026-09-29T03:14:00Z', ?, 'failed', 'digest1', 'some error', 0, 0);`,
		subID, finishedAt,
	)
	if err != nil {
		t.Fatalf("insert fetch failed: %v", err)
	}

	// 3. GET by ID now reflects latest finished fetch
	getReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/"+subID, nil)
	getReq2.Header.Set("Authorization", "Bearer "+testAdminToken)
	getRec2 := httptest.NewRecorder()
	router.ServeHTTP(getRec2, getReq2)
	if getRec2.Code != http.StatusOK {
		t.Fatalf("get status = %d", getRec2.Code)
	}
	var getResp2 testDataResponse[subscriptionViewResponse]
	if err := json.Unmarshal(getRec2.Body.Bytes(), &getResp2); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if getResp2.Data.LastRefreshedAt == nil || *getResp2.Data.LastRefreshedAt != finishedAt {
		t.Fatalf("expected LastRefreshedAt %s, got %v", finishedAt, getResp2.Data.LastRefreshedAt)
	}
	if getResp2.Data.LastRefreshOutcome == nil || *getResp2.Data.LastRefreshOutcome != "failed" {
		t.Fatalf("expected LastRefreshOutcome failed, got %v", getResp2.Data.LastRefreshOutcome)
	}

	// 4. List also reflects latest finished fetch
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions?page=1&page_size=10", nil)
	listReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRec.Code)
	}
	var listResp testDataResponse[transporthttp.PaginatedData[subscriptionViewResponse]]
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listResp.Data.Items) == 0 {
		t.Fatalf("expected non-empty list")
	}
	found := false
	for _, item := range listResp.Data.Items {
		if item.ID == subID {
			found = true
			if item.LastRefreshedAt == nil || *item.LastRefreshedAt != finishedAt {
				t.Fatalf("list item LastRefreshedAt mismatch: want %s, got %v", finishedAt, item.LastRefreshedAt)
			}
			if item.LastRefreshOutcome == nil || *item.LastRefreshOutcome != "failed" {
				t.Fatalf("list item LastRefreshOutcome mismatch: want failed, got %v", item.LastRefreshOutcome)
			}
		}
	}
	if !found {
		t.Fatalf("subID not found in list response")
	}
}

func TestSubscriptionPatch_BlankSecretRefRejected(t *testing.T) {
	cfg := sqlite.Config{
		Path:            fmt.Sprintf("file:sub_patch_blank_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout:     time.Second,
		ForeignKeys:     true,
		WALMode:         false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	routerCfg := newTestRouterConfig(true)
	routerCfg.SubscriptionService = subscription.NewService(sqlite.NewSubscriptionRepository(db), sqlite.NewAuditRepository(db))
	router := transporthttp.NewRouter(routerCfg)

	initialURL := "https://example.com/subscriptions/original?token=abc"
	createBody := `{"name":"OriginalSub","source_url_secret_ref":"` + initialURL + `","enabled":true,"refresh_policy":{"interval_seconds":3600}}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	var createdResp testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(createRec.Body.Bytes(), &createdResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	subID := createdResp.Data.ID
	initialRev := createdResp.Data.Revision

	// 1. Whitespace-only source_url_secret_ref must be rejected with 422 invalid_source_url_secret_ref
	for _, payload := range []string{
		`{"source_url_secret_ref":"   "}`,
		`{"source_url_secret_ref":""}`,
		`{"name":"ShouldNotApply","source_url_secret_ref":"  \t\n "}`,
	} {
		patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+subID, strings.NewReader(payload))
		patchReq.Header.Set("Content-Type", "application/json")
		patchReq.Header.Set("Authorization", "Bearer "+testAdminToken)
		patchReq.Header.Set("If-Match", initialRev)
		patchRec := httptest.NewRecorder()
		router.ServeHTTP(patchRec, patchReq)
		if patchRec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("patch payload %s status = %d, want 422; body = %s", payload, patchRec.Code, patchRec.Body.String())
		}
		if !strings.Contains(patchRec.Body.String(), `"invalid_source_url_secret_ref"`) {
			t.Fatalf("expected error code invalid_source_url_secret_ref, got body = %s", patchRec.Body.String())
		}
	}

	// Verify state and revision were not modified after failed PATCH attempts
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/"+subID, nil)
	getReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	var getResp testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getResp.Data.Name != "OriginalSub" || getResp.Data.SourceURLSecretRef != initialURL || getResp.Data.Revision != initialRev {
		t.Fatalf("subscription unexpectedly mutated after rejected PATCH: %+v", getResp.Data)
	}

	// 2. Omitting source_url_secret_ref while updating name must succeed and keep existing source_url_secret_ref
	validPatch := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+subID, strings.NewReader(`{"name":"RenamedWithoutURL"}`))
	validPatch.Header.Set("Content-Type", "application/json")
	validPatch.Header.Set("Authorization", "Bearer "+testAdminToken)
	validPatch.Header.Set("If-Match", initialRev)
	validRec := httptest.NewRecorder()
	router.ServeHTTP(validRec, validPatch)
	if validRec.Code != http.StatusOK {
		t.Fatalf("valid patch status = %d, body = %s", validRec.Code, validRec.Body.String())
	}
	var validResp testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(validRec.Body.Bytes(), &validResp); err != nil {
		t.Fatalf("decode valid patch response: %v", err)
	}
	if validResp.Data.Name != "RenamedWithoutURL" || validResp.Data.SourceURLSecretRef != initialURL {
		t.Fatalf("unexpected valid patch response: %+v", validResp.Data)
	}
}

func TestSubscriptionDelete_DeactivatesOrphanNodesAndPreservesSharedNodes(t *testing.T) {
	ctx := context.Background()
	cfg := sqlite.Config{
		Path:            fmt.Sprintf("file:sub_del_orphans_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout:     time.Second,
		ForeignKeys:     true,
		WALMode:         false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
	db, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	subRepo := sqlite.NewSubscriptionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)

	routerCfg := newTestRouterConfig(true)
	routerCfg.SubscriptionService = subscription.NewService(subRepo, sqlite.NewAuditRepository(db))
	router := transporthttp.NewRouter(routerCfg)

	createSub := func(name, url string) string {
		body := fmt.Sprintf(`{"name":%q,"source_url_secret_ref":%q,"enabled":true,"refresh_policy":{"interval_seconds":3600}}`, name, url)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create sub %s status = %d, body = %s", name, rec.Code, rec.Body.String())
		}
		var resp testDataResponse[subscriptionResponse]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode create response: %v", err)
		}
		return resp.Data.ID
	}

	subAID := createSub("Sub A", "https://example.com/a")
	subBID := createSub("Sub B", "https://example.com/b")

	now := domain.NowUTC()
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{
		{LogicalID: "orphan-candidate", Protocol: domain.ProtocolVMess, DisplayName: "Orphan Candidate", Server: "198.51.100.10", Port: 443, Active: true, CreatedAt: now, UpdatedAt: now},
		{LogicalID: "shared-node", Protocol: domain.ProtocolTrojan, DisplayName: "Shared Node", Server: "198.51.100.11", Port: 443, Active: true, CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("UpsertBatch failed: %v", err)
	}
	for _, src := range []domain.NodeSource{
		{NodeLogicalID: "orphan-candidate", SubscriptionID: subAID, LastSeenFetchID: "f1"},
		{NodeLogicalID: "shared-node", SubscriptionID: subAID, LastSeenFetchID: "f1"},
		{NodeLogicalID: "shared-node", SubscriptionID: subBID, LastSeenFetchID: "f2"},
	} {
		s := src
		if err := sourceRepo.Upsert(ctx, &s); err != nil {
			t.Fatalf("Upsert source failed: %v", err)
		}
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/subscriptions/"+subAID, nil)
	delReq.Header.Set("Authorization", "Bearer "+testAdminToken)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete subA status = %d, body = %s", delRec.Code, delRec.Body.String())
	}

	orphanNode, err := nodeRepo.GetByLogicalID(ctx, "orphan-candidate")
	if err != nil {
		t.Fatalf("GetByLogicalID orphan-candidate failed: %v", err)
	}
	if orphanNode.Active {
		t.Fatalf("expected orphan-candidate Active=false after deleting Sub A, got true")
	}

	sharedNode, err := nodeRepo.GetByLogicalID(ctx, "shared-node")
	if err != nil {
		t.Fatalf("GetByLogicalID shared-node failed: %v", err)
	}
	if !sharedNode.Active {
		t.Fatalf("expected shared-node Active=true because Sub B still owns it, got false")
	}
}

func TestSubscriptionPatch_PrimarySubscription(t *testing.T) {
	router := subscriptionTestRouter(t)

	// 1. Create Sub A and Sub B
	createSub := func(name string) (string, string) {
		t.Helper()
		payload := fmt.Sprintf(`{"name": "%s", "source_url_secret_ref": "https://example.com/%s"}`, name, name)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("failed to create %s: %d, body: %s", name, rec.Code, rec.Body.String())
		}
		var resp testDataResponse[subscriptionResponse]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse: %v", err)
		}
		return resp.Data.ID, resp.Data.Revision
	}

	subAID, revA := createSub("SubA")
	subBID, revB := createSub("SubB")

	// Helper to get subscription
	getSub := func(id string) subscriptionResponse {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/"+id, nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to get %s: %d", id, rec.Code)
		}
		var resp testDataResponse[subscriptionResponse]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse: %v", err)
		}
		return resp.Data
	}

	// 2. Neither is primary initially
	viewA := getSub(subAID)
	viewB := getSub(subBID)
	if viewA.IsPrimary || viewB.IsPrimary {
		t.Fatalf("neither should be primary initially, got A=%v B=%v", viewA.IsPrimary, viewB.IsPrimary)
	}

	// 3. PATCH Sub A to primary
	patchA := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+subAID, strings.NewReader(`{"is_primary": true}`))
	patchA.Header.Set("Authorization", "Bearer "+testAdminToken)
	patchA.Header.Set("If-Match", revA)
	patchA.Header.Set("Content-Type", "application/json")
	recA := httptest.NewRecorder()
	router.ServeHTTP(recA, patchA)
	if recA.Code != http.StatusOK {
		t.Fatalf("failed to patch Sub A: %d, body: %s", recA.Code, recA.Body.String())
	}
	var patchedAResp testDataResponse[subscriptionResponse]
	_ = json.Unmarshal(recA.Body.Bytes(), &patchedAResp)
	if !patchedAResp.Data.IsPrimary {
		t.Fatal("expected Sub A to be primary after patch")
	}

	// 4. PATCH Sub B to primary -> Mutual exclusion must clear Sub A
	patchB := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+subBID, strings.NewReader(`{"is_primary": true}`))
	patchB.Header.Set("Authorization", "Bearer "+testAdminToken)
	patchB.Header.Set("If-Match", revB)
	patchB.Header.Set("Content-Type", "application/json")
	recB := httptest.NewRecorder()
	router.ServeHTTP(recB, patchB)
	if recB.Code != http.StatusOK {
		t.Fatalf("failed to patch Sub B: %d, body: %s", recB.Code, recB.Body.String())
	}

	viewAAfter := getSub(subAID)
	viewBAfter := getSub(subBID)
	if !viewBAfter.IsPrimary {
		t.Fatal("expected Sub B to be primary")
	}
	if viewAAfter.IsPrimary {
		t.Fatal("expected Sub A to no longer be primary (mutual exclusion)")
	}

	// 5. Unset Sub B as primary
	patchBUnset := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+subBID, strings.NewReader(`{"is_primary": false}`))
	patchBUnset.Header.Set("Authorization", "Bearer "+testAdminToken)
	patchBUnset.Header.Set("If-Match", viewBAfter.Revision)
	patchBUnset.Header.Set("Content-Type", "application/json")
	recBUnset := httptest.NewRecorder()
	router.ServeHTTP(recBUnset, patchBUnset)
	if recBUnset.Code != http.StatusOK {
		t.Fatalf("failed to unset Sub B: %d", recBUnset.Code)
	}
	viewBFinal := getSub(subBID)
	if viewBFinal.IsPrimary {
		t.Fatal("expected Sub B to not be primary after unset")
	}
}

func TestSubscriptionProbePolicy_Endpoints(t *testing.T) {
	router := subscriptionTestRouter(t)

	// 1. POST with omitted probe_policy defaults to inherit
	createBody1 := `{"name":"PolicySub1","source_url_secret_ref":"https://example.com/sub1","config":{"rename_rules":[{"pattern":"p","replace":"r"}],"filter_rules":[{"type":"include","pattern":"US"}]}}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(createBody1))
	req1.Header.Set("Authorization", "Bearer "+testAdminToken)
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("POST sub1 status = %d, body = %s", rec1.Code, rec1.Body.String())
	}
	var resp1 testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(rec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("unmarshal resp1: %v", err)
	}
	if resp1.Data.Config.ProbePolicy == nil || resp1.Data.Config.ProbePolicy.Mode != domain.ProbePolicyModeInherit {
		t.Fatalf("expected probe_policy inherit on create, got %#v", resp1.Data.Config.ProbePolicy)
	}
	sub1ID := resp1.Data.ID
	sub1Rev := resp1.Data.Revision

	// 2. POST with custom probe_policy (interval: 120)
	createBody2 := `{"name":"PolicySub2","source_url_secret_ref":"https://example.com/sub2","config":{"probe_policy":{"mode":"custom","interval_seconds":120}}}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(createBody2))
	req2.Header.Set("Authorization", "Bearer "+testAdminToken)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("POST sub2 status = %d, body = %s", rec2.Code, rec2.Body.String())
	}
	var resp2 testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("unmarshal resp2: %v", err)
	}
	if resp2.Data.Config.ProbePolicy == nil || resp2.Data.Config.ProbePolicy.Mode != domain.ProbePolicyModeCustom || resp2.Data.Config.ProbePolicy.IntervalSeconds == nil || *resp2.Data.Config.ProbePolicy.IntervalSeconds != 120 {
		t.Fatalf("expected probe_policy custom(120), got %#v", resp2.Data.Config.ProbePolicy)
	}

	// 3. POST with invalid probe_policy interval below min (59s) returns 422
	createBodyBad := `{"name":"PolicySubBad","source_url_secret_ref":"https://example.com/subbad","config":{"probe_policy":{"mode":"custom","interval_seconds":59}}}`
	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(createBodyBad))
	reqBad.Header.Set("Authorization", "Bearer "+testAdminToken)
	reqBad.Header.Set("Content-Type", "application/json")
	recBad := httptest.NewRecorder()
	router.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for bad probe interval, got %d (body: %s)", recBad.Code, recBad.Body.String())
	}

	// 4. PATCH sub1 probe_policy to custom(300) without providing rename_rules or filter_rules:
	// must preserve rename_rules and filter_rules
	patchBody1 := `{"config":{"probe_policy":{"mode":"custom","interval_seconds":300}}}`
	patchReq1 := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+sub1ID, strings.NewReader(patchBody1))
	patchReq1.Header.Set("Authorization", "Bearer "+testAdminToken)
	patchReq1.Header.Set("If-Match", sub1Rev)
	patchReq1.Header.Set("Content-Type", "application/json")
	patchRec1 := httptest.NewRecorder()
	router.ServeHTTP(patchRec1, patchReq1)
	if patchRec1.Code != http.StatusOK {
		t.Fatalf("PATCH sub1 status = %d, body = %s", patchRec1.Code, patchRec1.Body.String())
	}
	var patchResp1 testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(patchRec1.Body.Bytes(), &patchResp1); err != nil {
		t.Fatalf("unmarshal patchResp1: %v", err)
	}
	if patchResp1.Data.Config.ProbePolicy.Mode != domain.ProbePolicyModeCustom || *patchResp1.Data.Config.ProbePolicy.IntervalSeconds != 300 {
		t.Fatalf("expected probe_policy custom(300), got %#v", patchResp1.Data.Config.ProbePolicy)
	}
	if len(patchResp1.Data.Config.RenameRules) != 1 || patchResp1.Data.Config.RenameRules[0].Pattern != "p" {
		t.Fatalf("expected rename_rules preserved, got %#v", patchResp1.Data.Config.RenameRules)
	}
	if len(patchResp1.Data.Config.FilterRules) != 1 || patchResp1.Data.Config.FilterRules[0].Pattern != "US" {
		t.Fatalf("expected filter_rules preserved, got %#v", patchResp1.Data.Config.FilterRules)
	}

	// 5. PATCH sub1 with explicit empty rename_rules: clears rename_rules, retains filter_rules and probe_policy
	patchBody2 := `{"config":{"rename_rules":[]}}`
	patchReq2 := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+sub1ID, strings.NewReader(patchBody2))
	patchReq2.Header.Set("Authorization", "Bearer "+testAdminToken)
	patchReq2.Header.Set("If-Match", patchResp1.Data.Revision)
	patchReq2.Header.Set("Content-Type", "application/json")
	patchRec2 := httptest.NewRecorder()
	router.ServeHTTP(patchRec2, patchReq2)
	if patchRec2.Code != http.StatusOK {
		t.Fatalf("PATCH sub1 rename_rules:[] status = %d, body = %s", patchRec2.Code, patchRec2.Body.String())
	}
	var patchResp2 testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(patchRec2.Body.Bytes(), &patchResp2); err != nil {
		t.Fatalf("unmarshal patchResp2: %v", err)
	}
	if len(patchResp2.Data.Config.RenameRules) != 0 {
		t.Fatalf("expected rename_rules cleared to empty, got %#v", patchResp2.Data.Config.RenameRules)
	}
	if len(patchResp2.Data.Config.FilterRules) != 1 {
		t.Fatalf("expected filter_rules preserved, got %#v", patchResp2.Data.Config.FilterRules)
	}
	if patchResp2.Data.Config.ProbePolicy.Mode != domain.ProbePolicyModeCustom || *patchResp2.Data.Config.ProbePolicy.IntervalSeconds != 300 {
		t.Fatalf("expected probe_policy preserved custom(300), got %#v", patchResp2.Data.Config.ProbePolicy)
	}

	// 6. PATCH with invalid mode returns 422
	patchBodyBad := `{"config":{"probe_policy":{"mode":"invalid_mode"}}}`
	patchReqBad := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+sub1ID, strings.NewReader(patchBodyBad))
	patchReqBad.Header.Set("Authorization", "Bearer "+testAdminToken)
	patchReqBad.Header.Set("If-Match", patchResp2.Data.Revision)
	patchReqBad.Header.Set("Content-Type", "application/json")
	patchRecBad := httptest.NewRecorder()
	router.ServeHTTP(patchRecBad, patchReqBad)
	if patchRecBad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid patch probe mode, got %d (body: %s)", patchRecBad.Code, patchRecBad.Body.String())
	}
}
