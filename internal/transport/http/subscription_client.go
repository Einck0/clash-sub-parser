package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
)

// isTemporaryStorageError identifies transient storage or connection errors that warrant a 503 + Retry-After.
func isTemporaryStorageError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "busy") ||
		strings.Contains(errStr, "locked") ||
		strings.Contains(errStr, "cannot start a transaction") ||
		strings.Contains(errStr, "resource temporarily unavailable") ||
		strings.Contains(errStr, "database is closed") ||
		strings.Contains(errStr, "driver: bad connection") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection refused")
}

// liveSubscriptionClientHandler serves client subscription export requests at /subscribe/v1/{id}.
func liveSubscriptionClientHandler(svc *publication.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")

		linkID := strings.TrimSpace(chi.URLParam(r, "id"))
		if linkID == "" {
			WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
			return
		}

		var tokenKeys []string
		var allTokens []string
		for key, vals := range r.URL.Query() {
			if strings.EqualFold(key, "token") {
				tokenKeys = append(tokenKeys, key)
				allTokens = append(allTokens, vals...)
			} else {
				WriteError(w, r, http.StatusUnprocessableEntity, "override_parameters_rejected",
					fmt.Sprintf("Query parameter %q is forbidden on fixed subscription links", key))
				return
			}
		}

		if len(tokenKeys) > 1 || len(allTokens) > 1 {
			WriteError(w, r, http.StatusBadRequest, "token_ambiguous",
				"Duplicate or repeated token parameter provided")
			return
		}

		queryToken := ""
		if len(allTokens) == 1 {
			queryToken = strings.TrimSpace(allTokens[0])
		}

		var bearerToken string
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if authHeader != "" {
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				bearerToken = strings.TrimSpace(authHeader[7:])
			} else {
				WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid Authorization header scheme")
				return
			}
		}

		if queryToken != "" && bearerToken != "" {
			WriteError(w, r, http.StatusUnauthorized, "token_ambiguous",
				"Ambiguous credentials: both query token and Bearer token provided")
			return
		}

		token := queryToken
		if token == "" {
			token = bearerToken
		}
		if token == "" {
			WriteError(w, r, http.StatusUnauthorized, "unauthorized",
				"Authentication credentials required")
			return
		}

		artifact, err := svc.GenerateLiveArtifact(r.Context(), linkID, token)
		if err != nil {
			switch {
			case errors.Is(err, publication.ErrLinkNotFound):
				WriteError(w, r, http.StatusNotFound, "subscription_link_not_found", "Subscription link not found")
			case errors.Is(err, publication.ErrLinkRevoked):
				WriteError(w, r, http.StatusForbidden, "subscription_link_revoked", "Subscription link has been revoked")
			case errors.Is(err, publication.ErrUnauthorized):
				WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid subscription token")
			case errors.Is(err, publication.ErrRateLimitExceeded):
				w.Header().Set("Retry-After", "5")
				WriteError(w, r, http.StatusServiceUnavailable, "concurrency_limit_exceeded", "Generation concurrency limit exceeded; retry shortly")
			case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
				w.Header().Set("Retry-After", "5")
				WriteError(w, r, http.StatusServiceUnavailable, "request_timeout", "Subscription generation timed out or was cancelled")
			default:
				if isTemporaryStorageError(err) {
					w.Header().Set("Retry-After", "5")
					WriteError(w, r, http.StatusServiceUnavailable, "database_busy", "Database busy or temporarily unavailable; retry shortly")
					return
				}

				var pfErr *publication.PreflightError
				if errors.As(err, &pfErr) {
					WriteError(w, r, http.StatusUnprocessableEntity, "preflight_rejected", "Subscription generation blocked by preflight checks")
					return
				}
				var capErr *publication.StrictCapabilityError
				if errors.As(err, &capErr) {
					WriteError(w, r, http.StatusUnprocessableEntity, capErr.Code, capErr.Message)
					return
				}
				var de *domain.DomainError
				if errors.As(err, &de) {
					status := http.StatusUnprocessableEntity
					switch de.Category {
					case domain.CategoryNotFound:
						status = http.StatusNotFound
					case domain.CategoryUnauthorized:
						status = http.StatusUnauthorized
					case domain.CategoryForbidden:
						status = http.StatusForbidden
					case domain.CategoryInternal:
						status = http.StatusInternalServerError
					case domain.CategoryValidation, domain.CategoryConflict:
						status = http.StatusUnprocessableEntity
					}
					WriteError(w, r, status, de.Code, de.Message)
					return
				}

				WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to generate subscription configuration")
			}
			return
		}

		// Conditional request evaluation: only checked after full authorization and artifact compilation
		ifNoneMatch := strings.TrimSpace(r.Header.Get("If-None-Match"))
		if ifNoneMatch != "" {
			digest := artifact.ContentDigest
			if ifNoneMatch == digest || ifNoneMatch == `"`+digest+`"` || ifNoneMatch == `W/"`+digest+`"` {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Referrer-Policy", "no-referrer")
				w.Header().Set("ETag", fmt.Sprintf("%q", digest))
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", artifact.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifact.Filename))
		w.Header().Set("ETag", fmt.Sprintf("%q", artifact.ContentDigest))
		w.Header().Set("X-Content-Digest", artifact.ContentDigest)
		w.Header().Set("X-Snapshot-Digest", artifact.SnapshotDigest)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(artifact.Content)
	}
}
