package http

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

// AdminAuthMiddleware validates admin credentials (Bearer token or session cookie)
// and strictly rejects unauthorized callers and publication export tokens.
// In SecurityModeOpen, requests are unconditionally allowed with admin context
// (unless bearing a publication export token, which is always rejected).
func AdminAuthMiddleware(cfg RouterConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			var token string
			if authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
					token = strings.TrimSpace(parts[1])
				}
			}

			// 1. 导出令牌安全隔离硬门禁：无论何种模式，发布令牌严禁访问管理端
			if token != "" && isPublicationToken(r.Context(), cfg, token) {
				WriteError(w, r, http.StatusForbidden, "invalid_token_scope", "Publication export token cannot access administration API")
				return
			}

			// 2. Open Mode 核心放行铁律：
			// 当服务端处于 Open Mode 时，不论请求头是否包含脏 Bearer Token 或历史 Cookie，
			// 一律作为 Open Mode 管理员无条件放行，彻底消除 401 死锁！
			if cfg.SecurityMode() == SecurityModeOpen {
				auth := &AuthContext{
					ActorKind:  ActorKindAdmin,
					Subject:    "admin",
					AuthMethod: AuthMethodOpenMode,
					Scopes:     []string{"admin"},
				}
				ctx := WithAuthContext(r.Context(), auth)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 3. Protected Mode：优先校验 Bearer Token（恒定时间比较）
			if token != "" {
				valid := false
				if cfg.TokenHolder != nil {
					valid = cfg.TokenHolder.Verify(token)
				} else if cfg.AdminToken != "" {
					valid = subtle.ConstantTimeCompare([]byte(token), []byte(cfg.AdminToken)) == 1
				}
				if valid {
					auth := &AuthContext{
						ActorKind:  ActorKindAdmin,
						Subject:    "admin",
						AuthMethod: AuthMethodBearer,
						Scopes:     []string{"admin"},
					}
					ctx := WithAuthContext(r.Context(), auth)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid authentication credentials")
				return
			}

			// 4. Protected Mode：其次校验 Session Cookie
			if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
				var session *SessionInfo
				var ok bool
				if cfg.SessionValidator != nil {
					session, ok = cfg.SessionValidator(cookie.Value)
				}
				if !ok && cfg.SessionStore != nil {
					session, ok = cfg.SessionStore.Get(cookie.Value)
				}
				if ok && session != nil {
					auth := &AuthContext{
						ActorKind:  ActorKindAdmin,
						Subject:    session.Subject,
						AuthMethod: AuthMethodCookie,
						Scopes:     []string{"admin"},
					}
					ctx := WithAuthContext(r.Context(), auth)
					ctx = WithSessionInfo(ctx, session)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid or expired session")
				return
			}

			// 5. Protected Mode：无凭据拒绝
			WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication credentials required")
		})
	}
}

// isPublicationToken checks if the provided token is recognized as a publication export token.
func isPublicationToken(ctx context.Context, cfg RouterConfig, token string) bool {
	if cfg.IsPublicationToken != nil && cfg.IsPublicationToken(ctx, token) {
		return true
	}
	if cfg.PublicationTokenValidator != nil {
		if ok, err := cfg.PublicationTokenValidator(ctx, "", token); err == nil && ok {
			return true
		}
	}
	return false
}

// CSRFMiddleware enforces CSRF validation for state-changing requests when authenticated via session cookie.
func CSRFMiddleware(cfg RouterConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only validate state-mutating HTTP methods
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				auth := GetAuthContext(r.Context())
				// CSRF only applies to ambient credential authentication (cookie sessions)
				if auth != nil && auth.AuthMethod == AuthMethodCookie {
					session := GetSessionInfo(r.Context())
					if session == nil {
						WriteError(w, r, http.StatusForbidden, "csrf_validation_failed", "No session associated with request")
						return
					}

					csrfToken := strings.TrimSpace(r.Header.Get(CSRFHeader))
					if csrfToken == "" {
						csrfToken = strings.TrimSpace(r.FormValue("csrf_token"))
					}

					if csrfToken == "" || csrfToken != session.CSRFToken {
						WriteError(w, r, http.StatusForbidden, "csrf_validation_failed", "Missing or invalid CSRF token")
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
