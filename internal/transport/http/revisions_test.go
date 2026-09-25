package http_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func setupRevisionTestRouter(t *testing.T, db *sql.DB) (http.Handler, *revision.Service, domain.RevisionRepository, domain.AuditRepository) {
	t.Helper()
	routerCfg := newTestRouterConfig(true)
	revRepo := sqlite.NewRevisionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)

	revSvc := revision.NewService(revRepo, auditRepo, revision.WithPolicyRepository(policyRepo))
	routerCfg.RevisionService = revSvc
	routerCfg.AuditRepository = auditRepo

	return transporthttp.NewRouter(routerCfg), revSvc, revRepo, auditRepo
}

func insertTestRevision(t *testing.T, db *sql.DB, id, digest string, state domain.ConfigurationRevisionState) {
	t.Helper()
	const query = `
		INSERT INTO configuration_revisions (id, content_digest, state, created_at)
		VALUES (?, ?, ?, ?);
	`
	if _, err := db.Exec(query, id, digest, string(state), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to insert test revision %s: %v", id, err)
	}
}

func insertTestPolicyRule(t *testing.T, db *sql.DB, id, revID, targetGroupID, expr string, pos int) {
	t.Helper()
	const query = `
		INSERT INTO policy_rules (id, revision_id, target_group_id, expression, position)
		VALUES (?, ?, ?, ?, ?);
	`
	if _, err := db.Exec(query, id, revID, targetGroupID, expr, pos); err != nil {
		t.Fatalf("failed to insert test policy rule %s: %v", id, err)
	}
}

func insertTestAdmissionRule(t *testing.T, db *sql.DB, id, revID, name, expr, action string, pos int) {
	t.Helper()
	const query = `
		INSERT INTO admission_rules (id, revision_id, name, expression, action, position)
		VALUES (?, ?, ?, ?, ?, ?);
	`
	if _, err := db.Exec(query, id, revID, name, expr, action, pos); err != nil {
		t.Fatalf("failed to insert test admission rule %s: %v", id, err)
	}
}

func TestRevisionsAuthAndCSRFGuardrails(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, _, _ := setupRevisionTestRouter(t, db)

	draftID := domain.MustNewUUIDv7()
	insertTestRevision(t, db, draftID, "digest-draft-auth", domain.RevisionStateDraft)

	// 1. Unauthenticated GET /revisions must return 401 Unauthorized
	req := httptest.NewRequest(http.MethodGet, "/api/v1/revisions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated GET /api/v1/revisions, got %d", rec.Code)
	}

	// 2. Publication token must be forbidden on admin API
	req = httptest.NewRequest(http.MethodGet, "/api/v1/revisions", nil)
	req.Header.Set("Authorization", "Bearer "+testPublicationToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Fatalf("expected 401 Unauthorized or 403 Forbidden for publication token on admin API, got %d", rec.Code)
	}

	// 3. State-changing request via cookie session without CSRF must return 403 Forbidden
	req = httptest.NewRequest(http.MethodPost, "/api/v1/revisions/"+draftID+"/review", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: testValidSessionID})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cookie mutation without CSRF token, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/revisions/"+draftID+"/activate", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: testValidSessionID})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cookie mutation without CSRF token on activate, got %d", rec.Code)
	}

	// 4. Valid Bearer token authentication succeeds
	req = httptest.NewRequest(http.MethodGet, "/api/v1/revisions", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid admin bearer token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevisionsListAndDetailAPI(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, _, _ := setupRevisionTestRouter(t, db)

	rev1 := domain.MustNewUUIDv7()
	rev2 := domain.MustNewUUIDv7()
	rev3 := domain.MustNewUUIDv7()

	insertTestRevision(t, db, rev1, "digest-rev-1", domain.RevisionStateArchived)
	insertTestRevision(t, db, rev2, "digest-rev-2", domain.RevisionStateActive)
	insertTestRevision(t, db, rev3, "digest-rev-3", domain.RevisionStateDraft)

	// Create group and rules for rev3
	grpID := domain.MustNewUUIDv7()
	const insertGrp = `INSERT INTO node_groups (id, name, group_type, created_at, updated_at) VALUES (?, 'DefaultGroup', 'select', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');`
	if _, err := db.Exec(insertGrp, grpID); err != nil {
		t.Fatal(err)
	}
	insertTestPolicyRule(t, db, domain.MustNewUUIDv7(), rev3, grpID, "MATCH", 0)
	insertTestAdmissionRule(t, db, domain.MustNewUUIDv7(), rev3, "RejectEmpty", "protocol != ''", "allow", 0)

	// 1. List all revisions
	req := httptest.NewRequest(http.MethodGet, "/api/v1/revisions", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var listResp struct {
		Data struct {
			Items    []domain.ConfigurationRevision `json:"items"`
			Page     int                            `json:"page"`
			PageSize int                            `json:"page_size"`
			Total    int                            `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if listResp.Data.Total != 3 || len(listResp.Data.Items) != 3 {
		t.Fatalf("expected 3 total revisions, got total=%d len=%d", listResp.Data.Total, len(listResp.Data.Items))
	}

	// 2. Filter by draft
	req = httptest.NewRequest(http.MethodGet, "/api/v1/revisions?state=draft", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var draftListResp struct {
		Data struct {
			Items []domain.ConfigurationRevision `json:"items"`
			Total int                            `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &draftListResp); err != nil {
		t.Fatal(err)
	}
	if draftListResp.Data.Total != 1 || len(draftListResp.Data.Items) != 1 || draftListResp.Data.Items[0].ID != rev3 {
		t.Fatalf("expected 1 draft revision with ID %s, got %+v", rev3, draftListResp.Data)
	}

	// 3. Get Active revision
	req = httptest.NewRequest(http.MethodGet, "/api/v1/revisions/active", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var activeResp struct {
		Data domain.ConfigurationRevision `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &activeResp); err != nil {
		t.Fatal(err)
	}
	if activeResp.Data.ID != rev2 || activeResp.Data.State != domain.RevisionStateActive {
		t.Fatalf("expected active revision rev2, got %+v", activeResp.Data)
	}

	// 4. Get Revision Detail (rev3) with rules
	req = httptest.NewRequest(http.MethodGet, "/api/v1/revisions/"+rev3, nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var detailResp struct {
		Data struct {
			ID             string                 `json:"id"`
			State          string                 `json:"state"`
			PolicyRules    []domain.PolicyRule    `json:"policy_rules"`
			AdmissionRules []domain.AdmissionRule `json:"admission_rules"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detailResp); err != nil {
		t.Fatal(err)
	}
	if detailResp.Data.ID != rev3 || len(detailResp.Data.PolicyRules) != 1 || len(detailResp.Data.AdmissionRules) != 1 {
		t.Fatalf("expected revision detail with 1 policy rule and 1 admission rule, got %+v", detailResp.Data)
	}
}

func TestDraftReviewAndActivationWorkflow(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, revRepo, auditRepo := setupRevisionTestRouter(t, db)

	// Simulate offline legacy-import producing an unactivated draft configuration revision
	draftID := domain.MustNewUUIDv7()
	insertTestRevision(t, db, draftID, "digest-imported-draft", domain.RevisionStateDraft)

	// 1. Isolation check: active endpoint must return 404 since only draft exists
	req := httptest.NewRequest(http.MethodGet, "/api/v1/revisions/active", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for active revision before activation, got %d", rec.Code)
	}

	// 2. Review draft endpoint: POST /api/v1/revisions/{id}/review
	req = httptest.NewRequest(http.MethodPost, "/api/v1/revisions/"+draftID+"/review", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on review draft, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Verify that review does NOT activate the draft
	active, err := revRepo.GetActive(context.Background())
	if err == nil || active != nil {
		t.Fatalf("unactivated draft must not participate as active after review: got=%+v", active)
	}

	// Verify review audit event in database
	reviewEvents, _, err := auditRepo.List(context.Background(), domain.AuditFilter{
		Action:     "revision.review",
		Pagination: domain.Pagination{Page: 1, PageSize: 10},
	})
	if err != nil || len(reviewEvents) != 1 || reviewEvents[0].Result != domain.AuditResultSuccess {
		t.Fatalf("expected 1 successful revision.review audit event, got len=%d err=%v", len(reviewEvents), err)
	}

	// 4. Activate draft endpoint: POST /api/v1/revisions/{id}/activate
	req = httptest.NewRequest(http.MethodPost, "/api/v1/revisions/"+draftID+"/activate", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on activate draft, got %d: %s", rec.Code, rec.Body.String())
	}

	var activateResp struct {
		Data domain.ConfigurationRevision `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &activateResp); err != nil {
		t.Fatal(err)
	}
	if activateResp.Data.ID != draftID || activateResp.Data.State != domain.RevisionStateActive {
		t.Fatalf("expected activated revision to have state active, got %+v", activateResp.Data)
	}

	// 5. Verify that active endpoint now returns the newly activated revision
	req = httptest.NewRequest(http.MethodGet, "/api/v1/revisions/active", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on active revision after activation, got %d", rec.Code)
	}
	var activeAfterResp struct {
		Data domain.ConfigurationRevision `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &activeAfterResp); err != nil {
		t.Fatal(err)
	}
	if activeAfterResp.Data.ID != draftID {
		t.Fatalf("expected active revision to be %s, got %s", draftID, activeAfterResp.Data.ID)
	}

	// 6. Verify activation audit event in database
	activateEvents, _, err := auditRepo.List(context.Background(), domain.AuditFilter{
		Action:     "revision.activate",
		Pagination: domain.Pagination{Page: 1, PageSize: 10},
	})
	if err != nil || len(activateEvents) != 1 || activateEvents[0].Result != domain.AuditResultSuccess {
		t.Fatalf("expected 1 successful revision.activate audit event, got len=%d err=%v", len(activateEvents), err)
	}

	// 7. Activating an already active revision returns 409 Conflict
	req = httptest.NewRequest(http.MethodPost, "/api/v1/revisions/"+draftID+"/activate", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when activating already active revision, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDraftIsolationFromConsumers(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, revRepo, _ := setupRevisionTestRouter(t, db)

	// 1. Initial state: active-1 is active
	active1ID := domain.MustNewUUIDv7()
	insertTestRevision(t, db, active1ID, "digest-active-1", domain.RevisionStateActive)

	// 2. Draft-2 is imported or created
	draft2ID := domain.MustNewUUIDv7()
	insertTestRevision(t, db, draft2ID, "digest-draft-2", domain.RevisionStateDraft)

	// Verify consumers reading active revision get active-1, NEVER draft-2
	active, err := revRepo.GetActive(context.Background())
	if err != nil || active.ID != active1ID {
		t.Fatalf("consumer must see active-1, got %+v err=%v", active, err)
	}

	// Verify HTTP GET /revisions/active returns active-1
	req := httptest.NewRequest(http.MethodGet, "/api/v1/revisions/active", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var resp struct {
		Data domain.ConfigurationRevision `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ID != active1ID {
		t.Fatalf("expected active-1 ID %s, got %s", active1ID, resp.Data.ID)
	}

	// 3. Explicit activation of draft-2
	req = httptest.NewRequest(http.MethodPost, "/api/v1/revisions/"+draft2ID+"/activate", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on activate, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Verify consumers now get draft-2, and active-1 has transitioned to archived
	active, err = revRepo.GetActive(context.Background())
	if err != nil || active.ID != draft2ID {
		t.Fatalf("consumer must now see draft-2, got %+v err=%v", active, err)
	}

	oldActive, err := revRepo.GetByID(context.Background(), active1ID)
	if err != nil || oldActive.State != domain.RevisionStateArchived {
		t.Fatalf("previous active-1 must be archived, got state %s", oldActive.State)
	}
}

func TestRevisionsConcurrentRequestsRaceSafety(t *testing.T) {
	db := newCleanSQLiteDB(t)
	router, _, _, _ := setupRevisionTestRouter(t, db)

	draftIDs := make([]string, 10)
	for i := 0; i < 10; i++ {
		id := domain.MustNewUUIDv7()
		insertTestRevision(t, db, id, fmt.Sprintf("digest-conc-%d", i), domain.RevisionStateDraft)
		draftIDs[i] = id
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			draftID := draftIDs[idx%len(draftIDs)]

			// Concurrent GET /revisions
			reqList := httptest.NewRequest(http.MethodGet, "/api/v1/revisions", nil)
			reqList.Header.Set("Authorization", "Bearer "+testAdminToken)
			recList := httptest.NewRecorder()
			router.ServeHTTP(recList, reqList)

			// Concurrent GET /revisions/{id}
			reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/revisions/"+draftID, nil)
			reqGet.Header.Set("Authorization", "Bearer "+testAdminToken)
			recGet := httptest.NewRecorder()
			router.ServeHTTP(recGet, reqGet)

			// Concurrent POST /revisions/{id}/review
			reqRev := httptest.NewRequest(http.MethodPost, "/api/v1/revisions/"+draftID+"/review", strings.NewReader(`{}`))
			reqRev.Header.Set("Authorization", "Bearer "+testAdminToken)
			reqRev.Header.Set("Content-Type", "application/json")
			recRev := httptest.NewRecorder()
			router.ServeHTTP(recRev, reqRev)

			// Concurrent POST /revisions/{id}/activate
			reqAct := httptest.NewRequest(http.MethodPost, "/api/v1/revisions/"+draftID+"/activate", strings.NewReader(`{}`))
			reqAct.Header.Set("Authorization", "Bearer "+testAdminToken)
			reqAct.Header.Set("Content-Type", "application/json")
			recAct := httptest.NewRecorder()
			router.ServeHTTP(recAct, reqAct)
		}(i)
	}
	wg.Wait()
}
