package http

import (
	"net/http"
)

const (
	LegacyEndpointRemovedCode    = "legacy_endpoint_removed"
	LegacyEndpointRemovedMessage = "This endpoint has been permanently removed in CSP 1.0. Please use the versioned API at /api/v1/."
)

// Legacy410Handler returns a hard HTTP 410 Gone for legacy endpoints with unified error payload.
func Legacy410Handler(w http.ResponseWriter, r *http.Request) {
	// Explicitly remove any Location header to prevent accidental redirects
	w.Header().Del("Location")
	WriteError(w, r, http.StatusGone, LegacyEndpointRemovedCode, LegacyEndpointRemovedMessage)
}
