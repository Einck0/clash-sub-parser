package legacy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
)

var (
	sensitiveQueryKeys = map[string]bool{
		"token":       true,
		"secret":      true,
		"password":    true,
		"pwd":         true,
		"key":         true,
		"auth":        true,
		"token_hash":  true,
		"auth_token":  true,
		"uuid":        true,
		"secret_key":  true,
		"private_key": true,
	}

	subscriptionsAllowedColumns = map[string]bool{
		"id":              true,
		"name":            true,
		"url":             true,
		"update_interval": true,
		"enabled":         true,
		"is_primary":      true,
	}

	nodeGroupsAllowedColumns = map[string]bool{
		"id":                true,
		"name":              true,
		"kind":              true,
		"group_type":        true,
		"sort_order":        true,
		"include_group_ids": true,
	}

	rulesAllowedColumns = map[string]bool{
		"id":         true,
		"name":       true,
		"category":   true,
		"type":       true,
		"value":      true,
		"proxy":      true,
		"sort_order": true,
		"enabled":    true,
	}

	nodesAllowedColumns = map[string]bool{
		"id":       true,
		"name":     true,
		"protocol": true,
		"server":   true,
		"port":     true,
	}
)

// hashRecordID creates a stable, non-reversible fingerprint for a record's legacy identifier.
func hashRecordID(table string, id any) string {
	raw := fmt.Sprintf("%s:%v", table, id)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:8])
}

// isSensitiveKey returns true if the query parameter or field name denotes confidential credentials.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if sensitiveQueryKeys[lower] {
		return true
	}
	for sensitive := range sensitiveQueryKeys {
		if strings.Contains(lower, sensitive) {
			return true
		}
	}
	return false
}

// SanitizeSubscriptionURL parses a legacy URL and scrubs all embedded credentials and secret tokens.
func SanitizeSubscriptionURL(rawURL string) (string, bool, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", false, domain.NewValidationError("empty_url", "subscription URL is empty")
	}

	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false, domain.NewValidationError("invalid_url", fmt.Sprintf("invalid subscription URL: %s", trimmed))
	}

	hadSecrets := false
	if u.User != nil {
		hadSecrets = true
		u.User = nil
	}

	q := u.Query()
	for k := range q {
		if isSensitiveKey(k) {
			hadSecrets = true
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()

	return u.String(), hadSecrets, nil
}

// NormalizeGroupType converts legacy kind/type names to valid domain.GroupType.
func NormalizeGroupType(legacyType string) (domain.GroupType, error) {
	norm := strings.ToLower(strings.TrimSpace(legacyType))
	switch norm {
	case "select":
		return domain.GroupTypeSelect, nil
	case "url-test", "urltest":
		return domain.GroupTypeURLTest, nil
	case "fallback":
		return domain.GroupTypeFallback, nil
	case "load-balance", "loadbalance":
		return domain.GroupTypeLoadBalance, nil
	default:
		return "", domain.NewValidationError("invalid_group_type", fmt.Sprintf("unsupported group type: %s", legacyType))
	}
}

// ComputeOpaqueSecretRef creates a non-secret opaque reference hash for node configurations.
func ComputeOpaqueSecretRef(protocol domain.Protocol, server string, port int) string {
	raw := fmt.Sprintf("%s|%s|%d", protocol, server, port)
	h := sha256.Sum256([]byte(raw))
	return "secret_" + hex.EncodeToString(h[:16])
}

// parseStringID parses an ID interface into a stable string representation.
func parseStringID(val any) string {
	switch v := val.(type) {
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	default:
		return fmt.Sprintf("%v", v)
	}
}
