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
