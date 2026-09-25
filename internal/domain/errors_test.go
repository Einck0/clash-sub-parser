package domain_test

import (
	"errors"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestDomainErrorCreationAndCategorization(t *testing.T) {
	errVal := domain.NewValidationError("invalid_field", "field 'port' must be between 1 and 65535")
	if errVal.Category != domain.CategoryValidation {
		t.Errorf("expected CategoryValidation, got %s", errVal.Category)
	}
	if errVal.Code != "invalid_field" {
		t.Errorf("expected code invalid_field, got %s", errVal.Code)
	}
	if !strings.Contains(errVal.Error(), "field 'port'") {
		t.Errorf("Error() output missing message: %s", errVal.Error())
	}

	errNotFound := domain.NewNotFoundError("node_not_found", "node does not exist")
	if errNotFound.Category != domain.CategoryNotFound {
		t.Errorf("expected CategoryNotFound, got %s", errNotFound.Category)
	}

	errConflict := domain.NewConflictError("idempotency_conflict", "duplicate probe run with same idempotency key")
	if errConflict.Category != domain.CategoryConflict {
		t.Errorf("expected CategoryConflict, got %s", errConflict.Category)
	}

	errSecurity := domain.NewSecurityError("ssrf_detected", "private address target rejected")
	if errSecurity.Category != domain.CategorySecurity {
		t.Errorf("expected CategorySecurity, got %s", errSecurity.Category)
	}

	// Test IsDomainError and AsDomainError
	var stdErr error = errVal
	if !domain.IsDomainError(stdErr) {
		t.Error("IsDomainError should return true for DomainError")
	}

	extracted, ok := domain.AsDomainError(stdErr)
	if !ok || extracted.Code != "invalid_field" {
		t.Errorf("AsDomainError failed to extract domain error: %v", extracted)
	}

	otherErr := errors.New("generic error")
	if domain.IsDomainError(otherErr) {
		t.Error("IsDomainError should return false for generic errors")
	}
}

func TestSecretRedaction(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "url with password",
			input:    "failed to connect to https://alice:secretPass123@sub.example.com/sub.yaml",
			expected: "https://alice:***@sub.example.com/sub.yaml",
		},
		{
			name:     "bearer token in header or log",
			input:    "request authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.secretpayload",
			expected: "Bearer ***",
		},
		{
			name:     "token query param",
			input:    "GET /api?token=abc123456789xyz&page=1",
			expected: "/api?token=***&page=1",
		},
		{
			name:     "private key payload",
			input:    "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----",
			expected: "[REDACTED_PRIVATE_KEY]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			redacted := domain.RedactSensitiveInfo(tc.input)
			if !strings.Contains(redacted, tc.expected) {
				t.Errorf("redaction failed.\nInput: %s\nExpected substring: %s\nGot: %s", tc.input, tc.expected, redacted)
			}
			// Plaintext secrets must not remain in output.
			if strings.Contains(redacted, "secretPass123") ||
				strings.Contains(redacted, "secretpayload") ||
				strings.Contains(redacted, "abc123456789xyz") ||
				strings.Contains(redacted, "MIIEowIBAAKCAQEA") {
				t.Errorf("sensitive plaintext remained in redacted output: %s", redacted)
			}
		})
	}
}
