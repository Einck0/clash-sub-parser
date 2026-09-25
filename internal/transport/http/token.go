package http

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"clash-sub-parser/internal/domain"
)

const (
	// DefaultHashCost is the bcrypt work factor used for production verifiers.
	DefaultHashCost = bcrypt.DefaultCost
	// MinHashCost is the minimal bcrypt work factor used for fast unit tests.
	MinHashCost = bcrypt.MinCost
)

// AdminTokenHolder provides thread-safe dynamic admin token authentication, verification, and rotation.
type AdminTokenHolder interface {
	Mode() SecurityMode
	Verify(candidate string) bool
	UpdateToken(ctx context.Context, token string) error
	SetVerifier(verifier string)
	CurrentVerifier() string
}

// TokenHolderConfig defines initialization parameters for DynamicTokenHolder.
type TokenHolderConfig struct {
	InitialToken    string // plaintext token or existing verifier hash
	InitialVerifier string // directly set verifier hash
	SettingsRepo    domain.SettingsRepository
	SessionStore    SessionStore
	HashCost        int // bcrypt cost (default: DefaultHashCost)
}

// DynamicTokenHolder is a concurrency-safe atomic/mutex snapshot holder for the admin token verifier.
// It ensures that auth mode transitions, verification, and token rotations are completely thread-safe.
type DynamicTokenHolder struct {
	mu           sync.RWMutex
	verifier     string
	settingsRepo domain.SettingsRepository
	sessionStore SessionStore
	hashCost     int
}

// NewDynamicTokenHolder creates and initializes a DynamicTokenHolder.
func NewDynamicTokenHolder(cfg TokenHolderConfig) *DynamicTokenHolder {
	cost := cfg.HashCost
	if cost <= 0 {
		cost = DefaultHashCost
	}

	holder := &DynamicTokenHolder{
		settingsRepo: cfg.SettingsRepo,
		sessionStore: cfg.SessionStore,
		hashCost:     cost,
	}

	if trimmedVerifier := strings.TrimSpace(cfg.InitialVerifier); trimmedVerifier != "" {
		holder.verifier = trimmedVerifier
		return holder
	}

	trimmedToken := strings.TrimSpace(cfg.InitialToken)
	if trimmedToken != "" {
		if isBcryptHash(trimmedToken) {
			holder.verifier = trimmedToken
		} else {
			if hashed, err := HashTokenWithCost(trimmedToken, cost); err == nil {
				holder.verifier = hashed
			}
		}
	}

	return holder
}

// Mode returns the current SecurityMode (Open or Protected).
func (h *DynamicTokenHolder) Mode() SecurityMode {
	if h == nil {
		return SecurityModeOpen
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.verifier == "" {
		return SecurityModeOpen
	}
	return SecurityModeProtected
}

// Verify checks a candidate plaintext token in constant time against the stored verifier.
// Returns false if server is in Open Mode or if the candidate does not match.
func (h *DynamicTokenHolder) Verify(candidate string) bool {
	if h == nil {
		return false
	}
	h.mu.RLock()
	v := h.verifier
	h.mu.RUnlock()

	trimmed := strings.TrimSpace(candidate)
	if v == "" || trimmed == "" {
		return false
	}

	return VerifyToken(v, trimmed)
}

// SetVerifier directly updates the stored verifier snapshot without re-hashing or database write.
// Typically used during startup to load a previously persisted verifier from SQLite.
func (h *DynamicTokenHolder) SetVerifier(verifier string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.verifier = strings.TrimSpace(verifier)
	h.mu.Unlock()
}

// CurrentVerifier returns the active verifier snapshot (empty string if open mode).
func (h *DynamicTokenHolder) CurrentVerifier() string {
	if h == nil {
		return ""
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.verifier
}

// UpdateToken updates or clears the admin token.
// If token is empty: clears verifier, switches to OpenMode, persists to SQLite, revokes all sessions.
// If token is non-empty: generates cryptographic verifier, persists to SQLite, switches to ProtectedMode, revokes all sessions.
func (h *DynamicTokenHolder) UpdateToken(ctx context.Context, token string) error {
	if h == nil {
		return fmt.Errorf("dynamic token holder is nil")
	}

	trimmedToken := strings.TrimSpace(token)
	var newVerifier string
	if trimmedToken != "" {
		cost := h.hashCost
		if cost <= 0 {
			cost = DefaultHashCost
		}
		var err error
		newVerifier, err = HashTokenWithCost(trimmedToken, cost)
		if err != nil {
			return fmt.Errorf("failed to hash admin token: %w", err)
		}
	}

	// 1. Persist to SQLite database if repository is configured
	if h.settingsRepo != nil {
		if err := h.settingsRepo.UpdateAdminToken(ctx, newVerifier); err != nil {
			return fmt.Errorf("failed to persist admin token: %w", err)
		}
	}

	// 2. Concurrency-safe snapshot update
	h.mu.Lock()
	h.verifier = newVerifier
	h.mu.Unlock()

	// 3. Invalidate all existing cookie sessions
	if h.sessionStore != nil {
		h.sessionStore.RevokeAll()
	}

	return nil
}

// HashTokenWithCost produces a standard bcrypt verifier for a token.
// SHA-256 pre-hashing is applied to support arbitrary password lengths without the 72-byte bcrypt limit.
func HashTokenWithCost(token string, cost int) (string, error) {
	if cost <= 0 {
		cost = DefaultHashCost
	}
	digest := sha256.Sum256([]byte(token))
	hash, err := bcrypt.GenerateFromPassword(digest[:], cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// HashToken produces a standard bcrypt verifier with default production cost.
func HashToken(token string) (string, error) {
	return HashTokenWithCost(token, DefaultHashCost)
}

// VerifyToken validates candidate plaintext token against a verifier hash in constant time.
func VerifyToken(verifier, candidate string) bool {
	if verifier == "" || candidate == "" {
		return false
	}

	digest := sha256.Sum256([]byte(candidate))
	// 1. Verify against SHA-256 pre-hashed bcrypt hash
	if err := bcrypt.CompareHashAndPassword([]byte(verifier), digest[:]); err == nil {
		return true
	}

	// 2. Direct bcrypt check fallback if candidate <= 72 bytes (compatibility for standard raw bcrypt hashes)
	if len(candidate) <= 72 {
		if err := bcrypt.CompareHashAndPassword([]byte(verifier), []byte(candidate)); err == nil {
			return true
		}
	}

	return false
}

// isBcryptHash checks if a string matches the standard bcrypt hash prefix and length.
func isBcryptHash(s string) bool {
	return (strings.HasPrefix(s, "$2a$") || strings.HasPrefix(s, "$2b$") || strings.HasPrefix(s, "$2y$")) && len(s) == 60
}
