package http

import (
	"net/http"
)

// Recoverer recovers from panics and writes a sanitized 500 error response.
// Internal secrets, paths, and stack traces are not leaked to the caller.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				WriteError(w, r, http.StatusInternalServerError, "internal_error", "An internal server error occurred")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
