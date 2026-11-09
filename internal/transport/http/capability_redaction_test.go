package http_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	transporthttp "clash-sub-parser/internal/transport/http"
)

// TestWriteDomainError_CapabilityError_RedactsSensitiveURLAndPassword verifies that
// sensitive URLs with embedded passwords and query secret parameters in CapabilityError
// are properly redacted without altering the 422 status or code.
func TestWriteDomainError_CapabilityError_RedactsSensitiveURLAndPassword(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test-capability-error", nil)
	req.Header.Set("X-Request-ID", "req-cap-leak-check-001")

	secretPassword := "super_secret_password_42"
	secretQueryToken := "sensitive_query_token_99"
	sensitiveURL := fmt.Sprintf("https://admin:%s@proxy.example.com/feed?token=%s", secretPassword, secretQueryToken)

	capErr := &compiler.CapabilityError{
		Target:   domain.TargetSurge,
		Location: "nodes[0].url",
		Feature:  "custom_upstream",
		Reason:   fmt.Sprintf("failed connecting to %s", sensitiveURL),
	}

	transporthttp.WriteDomainError(rec, req, capErr)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422, got %d", rec.Code)
	}

	var errResp transporthttp.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON response: %v, raw body: %s", err, rec.Body.String())
	}

	if errResp.Code != "unsupported_target_capability" {
		t.Fatalf("expected code unsupported_target_capability, got %q", errResp.Code)
	}
	if errResp.RequestID != "req-cap-leak-check-001" {
		t.Fatalf("expected request_id req-cap-leak-check-001, got %q", errResp.RequestID)
	}

	rawBody := rec.Body.String()
	if strings.Contains(rawBody, secretPassword) {
		t.Fatalf("CRITICAL: raw body leaks secret password: %s", rawBody)
	}
	if strings.Contains(rawBody, secretQueryToken) {
		t.Fatalf("CRITICAL: raw body leaks secret query token: %s", rawBody)
	}
	if strings.Contains(errResp.Message, secretPassword) {
		t.Fatalf("CRITICAL: message leaks secret password: %s", errResp.Message)
	}
	if strings.Contains(errResp.Message, secretQueryToken) {
		t.Fatalf("CRITICAL: message leaks secret query token: %s", errResp.Message)
	}

	// The shared redactor masks all URL userinfo, including the username.
	if strings.Contains(errResp.Message, "admin") {
		t.Fatalf("message leaks URL username: %s", errResp.Message)
	}
	// Verify standard redaction markers and non-sensitive URL structure.
	expectedRedactedURL := "https://***@proxy.example.com/feed?token=***"
	if !strings.Contains(errResp.Message, expectedRedactedURL) {
		t.Fatalf("expected message to contain redacted URL %q, got: %s", expectedRedactedURL, errResp.Message)
	}
	if !strings.Contains(errResp.Message, "target surge cannot render custom_upstream at nodes[0].url") {
		t.Fatalf("expected message to preserve target, feature, and location, got: %s", errResp.Message)
	}
}

// TestWriteDomainError_CapabilityError_RedactsBearerToken verifies that
// Bearer authorization tokens in CapabilityError are redacted.
func TestWriteDomainError_CapabilityError_RedactsBearerToken(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test-capability-bearer", nil)

	secretBearer := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.sensitive_payload"
	capErr := &compiler.CapabilityError{
		Target:   domain.TargetMihomo,
		Location: "providers[0]",
		Feature:  "auth_header",
		Reason:   fmt.Sprintf("upstream rejected Bearer %s", secretBearer),
	}

	transporthttp.WriteDomainError(rec, req, capErr)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422, got %d", rec.Code)
	}

	var errResp transporthttp.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if strings.Contains(rec.Body.String(), secretBearer) {
		t.Fatalf("CRITICAL: raw body leaks Bearer token: %s", rec.Body.String())
	}
	if !strings.Contains(errResp.Message, "Bearer ***") {
		t.Fatalf("expected message to redact Bearer token to 'Bearer ***', got: %s", errResp.Message)
	}
	if errResp.Code != "unsupported_target_capability" {
		t.Fatalf("expected code unsupported_target_capability, got %q", errResp.Code)
	}
}

// TestWriteDomainError_CapabilityError_RedactsIPAddress verifies that
// sensitive IPv4 and IPv6 addresses in CapabilityError are redacted.
func TestWriteDomainError_CapabilityError_RedactsIPAddress(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test-capability-ip", nil)

	sensitiveIP := "198.51.100.77"
	capErr := &compiler.CapabilityError{
		Target:   domain.TargetSingBox,
		Location: "outbounds[0].server",
		Feature:  "direct_routing",
		Reason:   fmt.Sprintf("cannot route to endpoint %s", sensitiveIP),
	}

	transporthttp.WriteDomainError(rec, req, capErr)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422, got %d", rec.Code)
	}

	var errResp transporthttp.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if strings.Contains(rec.Body.String(), sensitiveIP) {
		t.Fatalf("CRITICAL: raw body leaks IP address %s: %s", sensitiveIP, rec.Body.String())
	}
	if !strings.Contains(errResp.Message, "[REDACTED_IP]") {
		t.Fatalf("expected message to contain '[REDACTED_IP]', got: %s", errResp.Message)
	}
}

// TestWriteDomainError_CapabilityError_RedactsDatabaseDetails verifies that
// internal database errors leaking into CapabilityError are scrubbed.
func TestWriteDomainError_CapabilityError_RedactsDatabaseDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test-capability-db", nil)

	capErr := &compiler.CapabilityError{
		Target:   domain.TargetQuantumultX,
		Location: "filter_rules",
		Feature:  "database_backed_filter",
		Reason:   "sqlite3: database table locked (SQL logic error)",
	}

	transporthttp.WriteDomainError(rec, req, capErr)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422, got %d", rec.Code)
	}

	var errResp transporthttp.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if strings.Contains(rec.Body.String(), "sqlite3:") || strings.Contains(rec.Body.String(), "SQL logic error") {
		t.Fatalf("CRITICAL: database internals leaked in response: %s", rec.Body.String())
	}
	if errResp.Message != "A database constraint or integrity error occurred" {
		t.Fatalf("expected scrubbed message, got: %s", errResp.Message)
	}
	if errResp.Code != "unsupported_target_capability" {
		t.Fatalf("expected code unsupported_target_capability, got %q", errResp.Code)
	}
}

// TestWriteDomainError_CapabilityError_PreservesStandardCapabilityError verifies that
// non-sensitive, valid capability errors retain full detail without mutation.
func TestWriteDomainError_CapabilityError_PreservesStandardCapabilityError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test-capability-normal", nil)

	capErr := &compiler.CapabilityError{
		Target:   domain.TargetSurge,
		Location: "nodes[0]",
		Feature:  "hysteria2",
		Reason:   "protocol is not supported",
	}

	transporthttp.WriteDomainError(rec, req, capErr)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422, got %d", rec.Code)
	}

	var errResp transporthttp.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if errResp.Code != "unsupported_target_capability" {
		t.Fatalf("expected code unsupported_target_capability, got %q", errResp.Code)
	}
	expectedMsg := "target surge cannot render hysteria2 at nodes[0]: protocol is not supported"
	if errResp.Message != expectedMsg {
		t.Fatalf("expected message %q, got %q", expectedMsg, errResp.Message)
	}
}

// TestWriteDomainError_CapabilityError_Wrapped verifies that
// errors wrapping *compiler.CapabilityError are unwrapped and sanitized.
func TestWriteDomainError_CapabilityError_Wrapped(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test-capability-wrapped", nil)

	secretToken := "secret_token_in_wrapped_err"
	capErr := &compiler.CapabilityError{
		Target:   domain.TargetMihomo,
		Location: "nodes[2]",
		Feature:  "tuic",
		Reason:   fmt.Sprintf("unsupported cipher with token ?secret=%s", secretToken),
	}
	wrappedErr := fmt.Errorf("compilation step failed: %w", capErr)

	transporthttp.WriteDomainError(rec, req, wrappedErr)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422, got %d", rec.Code)
	}

	var errResp transporthttp.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if strings.Contains(rec.Body.String(), secretToken) {
		t.Fatalf("CRITICAL: raw body leaks secret from wrapped error: %s", rec.Body.String())
	}
	if errResp.Code != "unsupported_target_capability" {
		t.Fatalf("expected code unsupported_target_capability, got %q", errResp.Code)
	}
	if !strings.Contains(errResp.Message, "?secret=***") {
		t.Fatalf("expected query secret to be redacted, got: %s", errResp.Message)
	}
}
