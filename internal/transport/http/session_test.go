package http_test

import (
	"sync"
	"testing"
	"time"

	transporthttp "clash-sub-parser/internal/transport/http"
)

func TestMemorySessionStore_CreateAndGet(t *testing.T) {
	store := transporthttp.NewMemorySessionStore(1 * time.Hour)

	info, err := store.Create("admin")
	if err != nil {
		t.Fatalf("unexpected error creating session: %v", err)
	}

	if info == nil {
		t.Fatal("expected non-nil SessionInfo")
	}
	if info.SessionID == "" {
		t.Fatal("expected non-empty SessionID")
	}
	if len(info.SessionID) != 64 { // 32 bytes hex encoded = 64 chars
		t.Errorf("expected 64 character hex session ID, got %d chars (%s)", len(info.SessionID), info.SessionID)
	}
	if info.CSRFToken == "" {
		t.Fatal("expected non-empty CSRFToken")
	}
	if len(info.CSRFToken) != 64 {
		t.Errorf("expected 64 character hex CSRF token, got %d chars (%s)", len(info.CSRFToken), info.CSRFToken)
	}
	if info.Subject != "admin" {
		t.Errorf("expected Subject 'admin', got %q", info.Subject)
	}

	retrieved, ok := store.Get(info.SessionID)
	if !ok || retrieved == nil {
		t.Fatalf("failed to retrieve session %q", info.SessionID)
	}
	if retrieved.SessionID != info.SessionID {
		t.Errorf("expected SessionID %q, got %q", info.SessionID, retrieved.SessionID)
	}
	if retrieved.CSRFToken != info.CSRFToken {
		t.Errorf("expected CSRFToken %q, got %q", info.CSRFToken, retrieved.CSRFToken)
	}
	if retrieved.Subject != info.Subject {
		t.Errorf("expected Subject %q, got %q", info.Subject, retrieved.Subject)
	}
}

func TestMemorySessionStore_GetNonExistent(t *testing.T) {
	store := transporthttp.NewMemorySessionStore(1 * time.Hour)

	if _, ok := store.Get(""); ok {
		t.Error("expected Get with empty session ID to return false")
	}
	if _, ok := store.Get("non-existent-id"); ok {
		t.Error("expected Get with non-existent session ID to return false")
	}
}

func TestMemorySessionStore_TTLExpiration(t *testing.T) {
	// Store with very short TTL
	store := transporthttp.NewMemorySessionStore(30 * time.Millisecond)

	info, err := store.Create("admin")
	if err != nil {
		t.Fatalf("unexpected error creating session: %v", err)
	}

	// Immediate get should succeed
	if _, ok := store.Get(info.SessionID); !ok {
		t.Fatal("expected session to be immediately retrievable")
	}

	// Sleep past expiration
	time.Sleep(50 * time.Millisecond)

	// Post-expiration get should fail and clean up
	if _, ok := store.Get(info.SessionID); ok {
		t.Fatal("expected expired session to return false")
	}
}

func TestMemorySessionStore_Revoke(t *testing.T) {
	store := transporthttp.NewMemorySessionStore(1 * time.Hour)

	info, err := store.Create("admin")
	if err != nil {
		t.Fatalf("unexpected error creating session: %v", err)
	}

	store.Revoke(info.SessionID)

	if _, ok := store.Get(info.SessionID); ok {
		t.Fatal("expected revoked session to return false")
	}
}

func TestMemorySessionStore_RevokeAll(t *testing.T) {
	store := transporthttp.NewMemorySessionStore(1 * time.Hour)

	info1, _ := store.Create("admin1")
	info2, _ := store.Create("admin2")

	store.RevokeAll()

	if _, ok := store.Get(info1.SessionID); ok {
		t.Error("expected session 1 to be revoked after RevokeAll")
	}
	if _, ok := store.Get(info2.SessionID); ok {
		t.Error("expected session 2 to be revoked after RevokeAll")
	}
}

func TestMemorySessionStore_ConcurrentAccess(t *testing.T) {
	store := transporthttp.NewMemorySessionStore(1 * time.Hour)
	concurrency := 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			info, err := store.Create("admin")
			if err != nil {
				t.Errorf("failed concurrent create: %v", err)
				return
			}
			if _, ok := store.Get(info.SessionID); !ok {
				t.Errorf("failed concurrent get for %s", info.SessionID)
			}
		}()
	}

	wg.Wait()
}
