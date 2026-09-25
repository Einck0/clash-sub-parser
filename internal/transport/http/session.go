package http

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// SessionStore defines the contract for session lifecycle management.
type SessionStore interface {
	Create(subject string) (*SessionInfo, error)
	Get(sessionID string) (*SessionInfo, bool)
	Revoke(sessionID string)
	RevokeAll()
}

type memorySessionItem struct {
	info      *SessionInfo
	expiresAt time.Time
}

// MemorySessionStore provides a thread-safe in-memory session manager.
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]memorySessionItem
	ttl      time.Duration
}

// NewMemorySessionStore creates a new in-memory session store with specified TTL.
func NewMemorySessionStore(ttl time.Duration) *MemorySessionStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &MemorySessionStore{
		sessions: make(map[string]memorySessionItem),
		ttl:      ttl,
	}
}

// Create generates a cryptographically secure session ID and CSRF token, saving the session.
func (s *MemorySessionStore) Create(subject string) (*SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bID := make([]byte, 32)
	if _, err := rand.Read(bID); err != nil {
		return nil, fmt.Errorf("failed to generate session id: %w", err)
	}
	sessionID := hex.EncodeToString(bID)

	bCSRF := make([]byte, 32)
	if _, err := rand.Read(bCSRF); err != nil {
		return nil, fmt.Errorf("failed to generate csrf token: %w", err)
	}
	csrfToken := hex.EncodeToString(bCSRF)

	info := &SessionInfo{
		SessionID: sessionID,
		Subject:   subject,
		CSRFToken: csrfToken,
	}

	s.sessions[sessionID] = memorySessionItem{
		info:      info,
		expiresAt: time.Now().Add(s.ttl),
	}
	return info, nil
}

// Get retrieves an active session by ID. If expired, it is removed and returns false.
func (s *MemorySessionStore) Get(sessionID string) (*SessionInfo, bool) {
	if sessionID == "" {
		return nil, false
	}
	s.mu.RLock()
	item, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if !ok || time.Now().After(item.expiresAt) {
		if ok {
			s.Revoke(sessionID)
		}
		return nil, false
	}
	return item.info, true
}

// Revoke removes a session by ID.
func (s *MemorySessionStore) Revoke(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

// RevokeAll clears all active sessions.
func (s *MemorySessionStore) RevokeAll() {
	s.mu.Lock()
	s.sessions = make(map[string]memorySessionItem)
	s.mu.Unlock()
}
