package http

import (
	"net/http"
	"strings"

	"clash-sub-parser/internal/domain"
)

const RequestIDHeader = "X-Request-ID"

// RequestID middleware generates or propagates the X-Request-ID header and context value.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := strings.TrimSpace(r.Header.Get(RequestIDHeader))
		// If missing, too long, or containing control characters, generate a new UUIDv7.
		if reqID == "" || len(reqID) > 128 || strings.ContainsAny(reqID, "\r\n\t") {
			reqID = domain.MustNewUUIDv7()
		}

		ctx := WithRequestID(r.Context(), reqID)
		w.Header().Set(RequestIDHeader, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
