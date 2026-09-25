package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	uuidv7Regex    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-7[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	logicalIDRegex = regexp.MustCompile(`^(node_)?[0-9a-fA-F]{16,64}$`)
)

// NewUUIDv7 generates a standard RFC 9562 UUIDv7 string.
func NewUUIDv7() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("failed to read random bytes for UUIDv7: %w", err)
	}

	ms := uint64(time.Now().UnixMilli())
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)

	// Version 7: set high 4 bits of byte 6 to 0b0111 (0x7)
	raw[6] = (raw[6] & 0x0f) | 0x70
	// Variant RFC 4122 / RFC 9562: set high 2 bits of byte 8 to 0b10 (0x80)
	raw[8] = (raw[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

// MustNewUUIDv7 generates a UUIDv7 or panics on random source failure.
func MustNewUUIDv7() string {
	id, err := NewUUIDv7()
	if err != nil {
		panic(err)
	}
	return id
}

// IsValidUUIDv7 checks whether a string is a valid RFC 9562 UUIDv7.
func IsValidUUIDv7(s string) bool {
	return uuidv7Regex.MatchString(s)
}

// ValidateUUIDv7 validates a UUIDv7 string and returns a DomainError on failure.
func ValidateUUIDv7(s string) error {
	if !IsValidUUIDv7(s) {
		return NewValidationError("invalid_uuidv7", fmt.Sprintf("invalid UUIDv7 format: %s", s))
	}
	return nil
}

// ExtractTimeFromUUIDv7 extracts the UTC timestamp encoded in the first 48 bits of a UUIDv7.
func ExtractTimeFromUUIDv7(s string) (time.Time, error) {
	if !IsValidUUIDv7(s) {
		return time.Time{}, NewValidationError("invalid_uuidv7", fmt.Sprintf("invalid UUIDv7: %s", s))
	}

	// First 48 bits are in hex chars 0..7 and 9..12
	hexPart := s[0:8] + s[9:13]
	ms, err := strconv.ParseUint(hexPart, 16, 64)
	if err != nil {
		return time.Time{}, NewValidationError("invalid_uuidv7_timestamp", fmt.Sprintf("failed to parse timestamp: %v", err))
	}

	return time.UnixMilli(int64(ms)).UTC(), nil
}

// ComputeNodeLogicalID computes a stable logical identity hash based on normalized non-secret transport params.
func ComputeNodeLogicalID(protocol Protocol, server string, port int, transportParams map[string]string) string {
	normProtocol := strings.ToLower(strings.TrimSpace(string(protocol)))
	normServer := strings.ToLower(strings.TrimSpace(server))

	// Sensitive keys that MUST NEVER affect or be included in logical ID
	sensitiveKeys := map[string]bool{
		"password":    true,
		"secret":      true,
		"token":       true,
		"key":         true,
		"private_key": true,
		"uuid":        true,
		"auth":        true,
		"username":    true,
		"user":        true,
	}

	var keys []string
	for k := range transportParams {
		lowerK := strings.ToLower(strings.TrimSpace(k))
		if !sensitiveKeys[lowerK] {
			keys = append(keys, lowerK)
		}
	}
	sort.Strings(keys)

	var paramParts []string
	for _, k := range keys {
		val := strings.TrimSpace(transportParams[k])
		paramParts = append(paramParts, fmt.Sprintf("%s=%s", k, val))
	}
	sortedParamsStr := strings.Join(paramParts, "&")

	canonical := fmt.Sprintf("%s|%s|%d|%s", normProtocol, normServer, port, sortedParamsStr)
	sum := sha256.Sum256([]byte(canonical))

	return "node_" + hex.EncodeToString(sum[:16])
}

// IsValidLogicalID checks whether a string is a valid node logical ID.
func IsValidLogicalID(s string) bool {
	return logicalIDRegex.MatchString(s)
}
