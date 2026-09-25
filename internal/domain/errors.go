package domain

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// ErrorCategory classifies domain errors for machine handling and HTTP status mapping.
type ErrorCategory string

const (
	CategoryValidation   ErrorCategory = "validation"
	CategoryNotFound     ErrorCategory = "not_found"
	CategoryConflict     ErrorCategory = "conflict"
	CategoryUnauthorized ErrorCategory = "unauthorized"
	CategoryForbidden    ErrorCategory = "forbidden"
	CategorySecurity     ErrorCategory = "security"
	CategoryInternal     ErrorCategory = "internal"
)

// DomainError represents a structured, safe domain error.
// It guarantees that internal secrets are redacted and machine codes are stable.
type DomainError struct {
	Code     string            `json:"code"`
	Message  string            `json:"message"`
	Category ErrorCategory     `json:"category"`
	Details  map[string]string `json:"details,omitempty"`
}

func (e *DomainError) Error() string {
	if len(e.Details) > 0 {
		return fmt.Sprintf("[%s/%s] %s (details: %v)", e.Category, e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s/%s] %s", e.Category, e.Code, e.Message)
}

// NewDomainError creates a new DomainError ensuring the message is redacted.
func NewDomainError(code, message string, category ErrorCategory) *DomainError {
	return &DomainError{
		Code:     code,
		Message:  RedactSensitiveInfo(message),
		Category: category,
	}
}

// NewValidationError creates a validation domain error.
func NewValidationError(code, message string) *DomainError {
	return NewDomainError(code, message, CategoryValidation)
}

// NewNotFoundError creates a not found domain error.
func NewNotFoundError(code, message string) *DomainError {
	return NewDomainError(code, message, CategoryNotFound)
}

// NewConflictError creates a conflict domain error.
func NewConflictError(code, message string) *DomainError {
	return NewDomainError(code, message, CategoryConflict)
}

// NewSecurityError creates a security domain error.
func NewSecurityError(code, message string) *DomainError {
	return NewDomainError(code, message, CategorySecurity)
}

// NewInternalError creates an internal domain error.
func NewInternalError(code, message string) *DomainError {
	return NewDomainError(code, message, CategoryInternal)
}

// IsDomainError checks if an error is a *DomainError.
func IsDomainError(err error) bool {
	var de *DomainError
	return errors.As(err, &de)
}

// AsDomainError attempts to unwrap err into a *DomainError.
func AsDomainError(err error) (*DomainError, bool) {
	var de *DomainError
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}

var (
	urlPasswordRegex      = regexp.MustCompile(`(?i)(https?://[^:/@\s]+):([^@\s]+)@`)
	bearerTokenRegex      = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/-]+`)
	querySecretRegex      = regexp.MustCompile(`(?i)([?&](?:token|secret|password|key|auth|api_key|access_token)=)[^&]+`)
	privateKeyRegex       = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`)
	cookieHeaderRegex     = regexp.MustCompile(`(?i)(\bcookie\s*:\s*)[^\r\n]+`)
	secretAssignmentRegex = regexp.MustCompile(`(?i)(["']?(?:api[_-]?key|access[_-]?token|secret|password|token|cookie)["']?\s*[:=]\s*["']?)[^"',\s}&]+`)
	ipv4TokenRegex        = regexp.MustCompile(`(?:^|[^0-9])((?:[0-9]{1,3}\.){3}[0-9]{1,3})(?:$|[^0-9])`)
	ipv6TokenRegex        = regexp.MustCompile(`(?i)[0-9a-f:]{2,}`)
)

// RedactSensitiveInfo masks passwords in URLs, Bearer tokens, query secret params, and private keys.
func RedactSensitiveInfo(input string) string {
	if input == "" {
		return ""
	}

	result := urlPasswordRegex.ReplaceAllString(input, "${1}:***@")
	result = bearerTokenRegex.ReplaceAllString(result, "${1}***")
	result = querySecretRegex.ReplaceAllString(result, "${1}***")
	result = cookieHeaderRegex.ReplaceAllString(result, "${1}[REDACTED]")
	result = secretAssignmentRegex.ReplaceAllString(result, "${1}***")
	result = privateKeyRegex.ReplaceAllString(result, "[REDACTED_PRIVATE_KEY]")
	result = redactCompleteIPs(result)

	return strings.TrimSpace(result)
}

func redactCompleteIPs(input string) string {
	redact := func(match string) string {
		start := 0
		for start < len(match) && !strings.ContainsRune("0123456789abcdefABCDEF:", rune(match[start])) {
			start++
		}
		end := len(match)
		for end > start && !strings.ContainsRune("0123456789abcdefABCDEF:", rune(match[end-1])) {
			end--
		}
		if net.ParseIP(match[start:end]) == nil {
			return match
		}
		return match[:start] + "[REDACTED_IP]" + match[end:]
	}
	result := ipv4TokenRegex.ReplaceAllStringFunc(input, redact)
	return ipv6TokenRegex.ReplaceAllStringFunc(result, func(match string) string {
		if net.ParseIP(match) == nil {
			return match
		}
		return "[REDACTED_IP]"
	})
}
