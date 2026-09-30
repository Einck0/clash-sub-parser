package http

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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
	// Defaults preserve the existing protected behavior for callers that do not load settings.
	AdminAuthEnabled  *bool
	ExportAuthEnabled *bool
}

// DynamicTokenHolder is a concurrency-safe atomic/mutex snapshot holder for the admin token verifier.
// It ensures that auth mode transitions, verification, and token rotations are completely thread-safe.
type DynamicTokenHolder struct {
	mu                sync.RWMutex
	verifier          string
	adminAuthEnabled  bool
	exportAuthEnabled bool
	settingsRepo      domain.SettingsRepository
	sessionStore      SessionStore
	hashCost          int
}

// NewDynamicTokenHolder creates and initializes a DynamicTokenHolder.
func NewDynamicTokenHolder(cfg TokenHolderConfig) *DynamicTokenHolder {
	cost := cfg.HashCost
	if cost <= 0 {
		cost = DefaultHashCost
	}

	holder := &DynamicTokenHolder{
		settingsRepo:      cfg.SettingsRepo,
		sessionStore:      cfg.SessionStore,
		hashCost:          cost,
		adminAuthEnabled:  true,
		exportAuthEnabled: true,
	}
	if cfg.AdminAuthEnabled != nil {
		holder.adminAuthEnabled = *cfg.AdminAuthEnabled
	}
	if cfg.ExportAuthEnabled != nil {
		holder.exportAuthEnabled = *cfg.ExportAuthEnabled
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

// Mode returns the effective admin security mode, retaining the legacy name.
func (h *DynamicTokenHolder) Mode() SecurityMode { return h.AdminMode() }

// AdminMode returns protected only when the admin switch is on and a verifier exists.
func (h *DynamicTokenHolder) AdminMode() SecurityMode {
	if h == nil {
		return SecurityModeOpen
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.adminAuthEnabled || h.verifier == "" {
		return SecurityModeOpen
	}
	return SecurityModeProtected
}

// ExportMode reports the independent export protection state.
func (h *DynamicTokenHolder) ExportMode() SecurityMode {
	if h.IsExportAuthRequired() {
		return SecurityModeProtected
	}
	return SecurityModeOpen
}

func (h *DynamicTokenHolder) IsExportAuthRequired() bool {
	if h == nil {
		return true
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.exportAuthEnabled
}

// SetAuthSwitches changes both in-memory switches as a single snapshot.
func (h *DynamicTokenHolder) SetAuthSwitches(admin, export bool) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.adminAuthEnabled, h.exportAuthEnabled = admin, export
	h.mu.Unlock()
}

// VerifyExportToken accepts a publication-specific token or the system master token.
func (h *DynamicTokenHolder) VerifyExportToken(candidate, pubTokenHash string) bool {
	if h == nil {
		return false
	}
	h.mu.RLock()
	required, verifier := h.exportAuthEnabled, h.verifier
	h.mu.RUnlock()
	if !required {
		return true
	}
	if candidate == "" {
		return false
	}
	digest := sha256.Sum256([]byte(candidate))
	if len(pubTokenHash) == hex.EncodedLen(len(digest)) &&
		subtle.ConstantTimeCompare([]byte(hex.EncodeToString(digest[:])), []byte(pubTokenHash)) == 1 {
		return true
	}
	return verifier != "" && VerifyToken(verifier, candidate)
}

// UpdateAuthSettings persists switches and an optional token change, then publishes
// the new verifier and switches together and invalidates sessions on token changes.
func (h *DynamicTokenHolder) UpdateAuthSettings(ctx context.Context, admin, export bool, token *string) error {
	if h == nil {
		return fmt.Errorf("dynamic token holder is nil")
	}
	var verifier *string
	if token != nil {
		v := ""
		if trimmed := strings.TrimSpace(*token); trimmed != "" {
			var err error
			v, err = HashTokenWithCost(trimmed, h.hashCost)
			if err != nil {
				return fmt.Errorf("failed to hash admin token: %w", err)
			}
		}
		verifier = &v
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.settingsRepo != nil {
		if err := h.settingsRepo.UpdateAuthSettings(ctx, admin, export, verifier); err != nil {
			return fmt.Errorf("failed to persist auth settings: %w", err)
		}
	}
	h.adminAuthEnabled, h.exportAuthEnabled = admin, export
	if verifier != nil {
		h.verifier = *verifier
		if h.sessionStore != nil {
			h.sessionStore.RevokeAll()
		}
	}
	return nil
}

// Verify checks a candidate plaintext token against the stored verifier.
// Returns false when no verifier is configured or when the candidate does not match.
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

// CurrentVerifier returns the active verifier snapshot (empty if no token is configured).
func (h *DynamicTokenHolder) CurrentVerifier() string {
	if h == nil {
		return ""
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.verifier
}

// UpdateToken updates or clears the system token while preserving both auth switches.
// It persists the verifier and invalidates sessions; the effective admin mode
// depends on the admin switch as well as whether a verifier is configured.
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

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.settingsRepo != nil {
		if err := h.settingsRepo.UpdateAdminToken(ctx, newVerifier); err != nil {
			return fmt.Errorf("failed to persist admin token: %w", err)
		}
	}
	h.verifier = newVerifier
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
