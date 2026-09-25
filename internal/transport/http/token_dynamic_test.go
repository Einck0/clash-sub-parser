package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	transporthttp "clash-sub-parser/internal/transport/http"
)

type mockSettingsRepo struct {
	mu       sync.Mutex
	settings domain.Settings
}

func newMockSettingsRepo(initialVerifier string) *mockSettingsRepo {
	s := domain.DefaultSettings()
	s.AdminToken = initialVerifier
	return &mockSettingsRepo{settings: s}
}

func (m *mockSettingsRepo) Get(ctx context.Context) (*domain.Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := m.settings
	return &copied, nil
}

func (m *mockSettingsRepo) Update(ctx context.Context, s *domain.Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = *s
	return nil
}

func (m *mockSettingsRepo) UpdateAdminToken(ctx context.Context, verifier string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.AdminToken = verifier
	return nil
}

func TestDynamicAdminTokenLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newMockSettingsRepo("")
	sessionStore := transporthttp.NewMemorySessionStore(24 * time.Hour)

	tokenHolder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		SettingsRepo: repo,
		SessionStore: sessionStore,
		HashCost:     transporthttp.MinHashCost, // fast test cost
	})

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		TokenHolder:        tokenHolder,
		SettingsRepository: repo,
		SessionStore:       sessionStore,
	})

	// 1. Initial status: Open Mode
	reqStatus := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	recStatus := httptest.NewRecorder()
	router.ServeHTTP(recStatus, reqStatus)
	if recStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/v1/auth/status in open mode, got %d", recStatus.Code)
	}
	if tokenHolder.Mode() != transporthttp.SecurityModeOpen {
		t.Fatalf("expected SecurityModeOpen, got %v", tokenHolder.Mode())
	}

	// 2. Set Admin Token via POST /api/v1/settings/admin-token
	setPayload := `{"token": "initial-secret-12345"}`
	reqSet := httptest.NewRequest(http.MethodPost, "/api/v1/settings/admin-token", bytes.NewBufferString(setPayload))
	reqSet.Header.Set("Content-Type", "application/json")
	recSet := httptest.NewRecorder()
	router.ServeHTTP(recSet, reqSet)
	if recSet.Code != http.StatusOK {
		t.Fatalf("expected 200 from setting admin token, got %d: %s", recSet.Code, recSet.Body.String())
	}

	// Verify Mode switched to Protected
	if tokenHolder.Mode() != transporthttp.SecurityModeProtected {
		t.Fatalf("expected SecurityModeProtected after setting token, got %v", tokenHolder.Mode())
	}
	st, err := repo.Get(ctx)
	if err != nil || st.AdminToken == "" {
		t.Fatalf("expected verifier persisted in repository, got %v, err: %v", st, err)
	}

	// 3. Protected mode: unauthenticated request to /api/v1/settings/admin-token rejected with 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/v1/settings/admin-token", nil)
	recUnauth := httptest.NewRecorder()
	router.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthenticated in protected mode, got %d", recUnauth.Code)
	}

	// 4. Authenticate with Bearer token
	reqBearer := httptest.NewRequest(http.MethodGet, "/api/v1/settings/admin-token", nil)
	reqBearer.Header.Set("Authorization", "Bearer initial-secret-12345")
	recBearer := httptest.NewRecorder()
	router.ServeHTTP(recBearer, reqBearer)
	if recBearer.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid bearer token, got %d: %s", recBearer.Code, recBearer.Body.String())
	}

	// 5. Login to obtain session cookie
	loginPayload := `{"token": "initial-secret-12345"}`
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(loginPayload))
	reqLogin.Header.Set("Content-Type", "application/json")
	recLogin := httptest.NewRecorder()
	router.ServeHTTP(recLogin, reqLogin)
	if recLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 from login, got %d", recLogin.Code)
	}
	var sessionCookie *http.Cookie
	for _, c := range recLogin.Result().Cookies() {
		if c.Name == transporthttp.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("expected session cookie in login response")
	}

	// Verify session cookie works for auth status
	reqCookie := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	reqCookie.AddCookie(sessionCookie)
	recCookie := httptest.NewRecorder()
	router.ServeHTTP(recCookie, reqCookie)
	if recCookie.Code != http.StatusOK {
		t.Fatalf("expected 200 from auth status with session cookie, got %d", recCookie.Code)
	}

	// 6. Rotate token to new-secret-99999
	rotatePayload := `{"token": "new-secret-99999"}`
	reqRotate := httptest.NewRequest(http.MethodPost, "/api/v1/settings/admin-token", bytes.NewBufferString(rotatePayload))
	reqRotate.Header.Set("Authorization", "Bearer initial-secret-12345")
	reqRotate.Header.Set("Content-Type", "application/json")
	recRotate := httptest.NewRecorder()
	router.ServeHTTP(recRotate, reqRotate)
	if recRotate.Code != http.StatusOK {
		t.Fatalf("expected 200 rotating admin token, got %d: %s", recRotate.Code, recRotate.Body.String())
	}

	// 7. Verification of rotation effects:
	// a. Old Bearer token must immediately be rejected
	reqOldBearer := httptest.NewRequest(http.MethodGet, "/api/v1/settings/admin-token", nil)
	reqOldBearer.Header.Set("Authorization", "Bearer initial-secret-12345")
	recOldBearer := httptest.NewRecorder()
	router.ServeHTTP(recOldBearer, reqOldBearer)
	if recOldBearer.Code != http.StatusUnauthorized {
		t.Fatalf("expected old bearer token to be 401 unauthorized, got %d", recOldBearer.Code)
	}

	// b. Previous session cookie must be invalidated
	reqOldCookie := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	reqOldCookie.AddCookie(sessionCookie)
	recOldCookie := httptest.NewRecorder()
	router.ServeHTTP(recOldCookie, reqOldCookie)
	var statusBody transporthttp.SuccessResponse[transporthttp.AuthStatusData]
	if err := json.NewDecoder(recOldCookie.Body).Decode(&statusBody); err != nil {
		t.Fatalf("failed to parse auth status response: %v", err)
	}
	if statusBody.Data.Authenticated {
		t.Fatalf("expected invalidated session to result in authenticated=false")
	}

	// c. New token works
	reqNewBearer := httptest.NewRequest(http.MethodGet, "/api/v1/settings/admin-token", nil)
	reqNewBearer.Header.Set("Authorization", "Bearer new-secret-99999")
	recNewBearer := httptest.NewRecorder()
	router.ServeHTTP(recNewBearer, reqNewBearer)
	if recNewBearer.Code != http.StatusOK {
		t.Fatalf("expected 200 with new bearer token, got %d", recNewBearer.Code)
	}

	// 8. Clear token (empty string) switches back to Open Mode
	clearPayload := `{"token": ""}`
	reqClear := httptest.NewRequest(http.MethodPost, "/api/v1/settings/admin-token", bytes.NewBufferString(clearPayload))
	reqClear.Header.Set("Authorization", "Bearer new-secret-99999")
	reqClear.Header.Set("Content-Type", "application/json")
	recClear := httptest.NewRecorder()
	router.ServeHTTP(recClear, reqClear)
	if recClear.Code != http.StatusOK {
		t.Fatalf("expected 200 clearing token, got %d: %s", recClear.Code, recClear.Body.String())
	}
	if tokenHolder.Mode() != transporthttp.SecurityModeOpen {
		t.Fatalf("expected SecurityModeOpen after clearing token, got %v", tokenHolder.Mode())
	}
}

func TestDynamicTokenConcurrencyRace(t *testing.T) {
	repo := newMockSettingsRepo("")
	sessionStore := transporthttp.NewMemorySessionStore(24 * time.Hour)
	holder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		SettingsRepo: repo,
		SessionStore: sessionStore,
		HashCost:     transporthttp.MinHashCost,
	})

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				tok := fmt.Sprintf("token-%d-%d", id, j)
				if id%2 == 0 {
					_ = holder.UpdateToken(context.Background(), tok)
				}
				_ = holder.Mode()
				_ = holder.Verify(tok)
			}
		}(i)
	}

	wg.Wait()
}
