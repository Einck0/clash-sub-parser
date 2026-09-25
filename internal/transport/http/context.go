package http

import (
	"context"
)

type contextKey string

const (
	requestIDContextKey contextKey = "csp_request_id"
	authContextKey      contextKey = "csp_auth_context"
	sessionContextKey   contextKey = "csp_session_context"
)

// WithRequestID injects the request ID into the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if reqID, ok := ctx.Value(requestIDContextKey).(string); ok {
		return reqID
	}
	return ""
}

// WithAuthContext injects the AuthContext into the context.
func WithAuthContext(ctx context.Context, auth *AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey, auth)
}

// GetAuthContext retrieves the AuthContext from the context.
func GetAuthContext(ctx context.Context) *AuthContext {
	if ctx == nil {
		return nil
	}
	if auth, ok := ctx.Value(authContextKey).(*AuthContext); ok {
		return auth
	}
	return nil
}

// WithSessionInfo injects the SessionInfo into the context.
func WithSessionInfo(ctx context.Context, session *SessionInfo) context.Context {
	return context.WithValue(ctx, sessionContextKey, session)
}

// GetSessionInfo retrieves the SessionInfo from the context.
func GetSessionInfo(ctx context.Context) *SessionInfo {
	if ctx == nil {
		return nil
	}
	if session, ok := ctx.Value(sessionContextKey).(*SessionInfo); ok {
		return session
	}
	return nil
}
