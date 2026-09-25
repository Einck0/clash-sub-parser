package http

import (
	"context"
	"strings"
)

// SecurityMode represents the operating security mode of the control plane.
type SecurityMode string

const (
	SecurityModeOpen      SecurityMode = "open"
	SecurityModeProtected SecurityMode = "protected"
)

// SecurityMode returns the effective security mode based on RouterConfig.
func (cfg RouterConfig) SecurityMode() SecurityMode {
	if cfg.TokenHolder != nil {
		return cfg.TokenHolder.Mode()
	}
	if strings.TrimSpace(cfg.AdminToken) == "" {
		return SecurityModeOpen
	}
	return SecurityModeProtected
}

// AuthStatusData defines the payload returned by GET /api/v1/auth/status.
type AuthStatusData struct {
	Mode          SecurityMode `json:"mode"`
	Authenticated bool         `json:"authenticated"`
	Subject       string       `json:"subject"`
}

// LoginRequest defines the payload for POST /api/v1/auth/login.
type LoginRequest struct {
	Token string `json:"token"`
}

// LoginResponseData defines the payload returned upon successful login.
type LoginResponseData struct {
	Mode          SecurityMode `json:"mode"`
	Authenticated bool         `json:"authenticated"`
	Subject       string       `json:"subject"`
	Token         string       `json:"token,omitempty"`
	CSRFToken     string       `json:"csrf_token,omitempty"`
	Message       string       `json:"message"`
}

// ActorKind represents the classification of the caller identity.
type ActorKind string

const (
	ActorKindAdmin             ActorKind = "admin"
	ActorKindPublicationExport ActorKind = "publication_export"
	ActorKindSystem            ActorKind = "system"
	ActorKindAnonymous         ActorKind = "anonymous"
)

// AuthMethod indicates the mechanism used to authenticate the request.
type AuthMethod string

const (
	AuthMethodBearer           AuthMethod = "bearer"
	AuthMethodCookie           AuthMethod = "cookie"
	AuthMethodPublicationToken AuthMethod = "publication_token"
	AuthMethodOpenMode         AuthMethod = "open_mode"
)

const (
	SessionCookieName = "csp_session"
	CSRFHeader        = "X-CSRF-Token"
)

// AuthContext encapsulates caller identity, authorization scope, and authentication details.
type AuthContext struct {
	ActorKind  ActorKind  `json:"actor_kind"`
	Subject    string     `json:"subject"`
	AuthMethod AuthMethod `json:"auth_method"`
	Scopes     []string   `json:"scopes,omitempty"`
}

// SessionInfo encapsulates authenticated session data and the associated CSRF token.
type SessionInfo struct {
	SessionID string `json:"session_id"`
	Subject   string `json:"subject"`
	CSRFToken string `json:"csrf_token"`
}

// PublicationTokenValidatorFunc validates publication export tokens for a given publication ID.
type PublicationTokenValidatorFunc func(ctx context.Context, publicationID, token string) (bool, error)

// SessionValidatorFunc validates a session cookie value and returns the session info.
type SessionValidatorFunc func(sessionID string) (*SessionInfo, bool)
