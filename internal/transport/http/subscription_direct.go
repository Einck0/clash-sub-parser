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

// resolveDirectTarget maps friendly path names to compiler targets.
func resolveDirectTarget(param string) (domain.CompilerTarget, bool) {
	switch strings.ToLower(strings.TrimSpace(param)) {
	case "mihomo", "clash":
		return domain.TargetMihomo, true
	case "singbox", "sing-box":
		return domain.TargetSingBox, true
	case "surge":
		return domain.TargetSurge, true
	case "qx", "quantumult-x", "quantumultx":
		return domain.TargetQuantumultX, true
	default:
		return "", false
	}
}

// directSubscriptionHandler serves clean, direct client subscription export requests.
// Supports both fixed routes (/clash, /singbox, etc.) and wildcard routes (/sub/{target}).
func directSubscriptionHandler(svc *publication.Service, holder AdminTokenHolder, fixedTarget domain.CompilerTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")

		// 1. Resolve target
		target := fixedTarget
		if target == "" {
			targetParam := chi.URLParam(r, "target")
			resolved, ok := resolveDirectTarget(targetParam)
			if !ok {
				WriteError(w, r, http.StatusNotFound, "unsupported_target", fmt.Sprintf("Unsupported target %q", targetParam))
				return
			}
			target = resolved
		}

		// 2. Authentication check against unified Settings Token
		authRequired := false
		if holder != nil {
			authRequired = holder.IsExportAuthRequired() && holder.AdminMode() == SecurityModeProtected
		}

		if authRequired {
			queryToken := strings.TrimSpace(r.URL.Query().Get("token"))
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			var bearerToken string
			if authHeader != "" {
				if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
					bearerToken = strings.TrimSpace(authHeader[7:])
				} else {
					WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid Authorization header scheme")
					return
				}
			}

			if queryToken != "" && bearerToken != "" && queryToken != bearerToken {
				WriteError(w, r, http.StatusUnauthorized, "token_ambiguous", "Conflicting tokens in query and authorization header")
				return
			}

			candidate := queryToken
			if candidate == "" {
				candidate = bearerToken
			}

			if candidate == "" || !holder.Verify(candidate) {
				WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid or missing token")
				return
			}
		}

		// 3. Live on-request compilation
		artifact, err := svc.GenerateTargetArtifact(r.Context(), target)
		if err != nil {
			switch {
			case errors.Is(err, publication.ErrRateLimitExceeded):
				WriteError(w, r, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many concurrent requests")
			case isTemporaryStorageError(err):
				w.Header().Set("Retry-After", "3")
				WriteError(w, r, http.StatusServiceUnavailable, "temporary_storage_error", "Database is temporarily busy, please retry")
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				WriteError(w, r, http.StatusGatewayTimeout, "timeout", "Request timed out during generation")
			default:
				var de *domain.DomainError
				if errors.As(err, &de) {
					status := http.StatusInternalServerError
					switch de.Category {
					case domain.CategoryNotFound:
						status = http.StatusNotFound
					case domain.CategoryUnauthorized:
						status = http.StatusUnauthorized
					case domain.CategoryForbidden:
						status = http.StatusForbidden
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

		// 4. Conditional caching evaluation (ETag / If-None-Match)
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

		// 5. Successful response
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
