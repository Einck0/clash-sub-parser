package domain

import (
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxDNSYAMLBytes = 64 * 1024

// DNSSettings is the persisted, user-editable DNS configuration.
type DNSSettings struct {
	Enabled   bool      `json:"enabled"`
	RawYAML   string    `json:"raw_yaml"`
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
	Source    string    `json:"source"`
}

// DefaultDNSSettings returns the historical resolver defaults without claiming the config was saved.
func DefaultDNSSettings() DNSSettings {
	return DNSSettings{
		Enabled: true,
		RawYAML: "enable: true\nnameserver:\n  - 1.1.1.1\n  - 8.8.8.8\n",
		Source:  "default",
	}
}

// NormalizeDNSSettings validates a DNS YAML document and returns its canonical YAML representation.
func NormalizeDNSSettings(enabled bool, raw string) (string, error) {
	cfg, err := ParseDNSSettings(enabled, raw)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", NewValidationError("invalid_dns_yaml", "DNS configuration cannot be normalized")
	}
	return string(data), nil
}

// ParseDNSSettings parses either a DNS mapping or a wrapper containing only a dns mapping.
func ParseDNSSettings(enabled bool, raw string) (map[string]any, error) {
	if len(raw) > maxDNSYAMLBytes {
		return nil, NewValidationError("dns_yaml_too_large", "DNS YAML must not exceed 64 KiB")
	}
	dec := yaml.NewDecoder(strings.NewReader(raw))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, NewValidationError("invalid_dns_yaml", "DNS configuration must be valid YAML")
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, NewValidationError("dns_yaml_multiple_documents", "DNS configuration must contain one YAML document")
		}
		return nil, NewValidationError("invalid_dns_yaml", "DNS configuration must be valid YAML")
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, NewValidationError("invalid_dns_yaml", "DNS configuration must be a YAML mapping")
	}
	root := doc.Content[0]
	if err := validateDNSNode(root, 0, map[*yaml.Node]bool{}); err != nil {
		return nil, err
	}
	if root.Kind != yaml.MappingNode {
		return nil, NewValidationError("invalid_dns_yaml", "DNS configuration must be a YAML mapping")
	}
	if hasMappingKey(root, "dns") {
		if len(root.Content) != 2 || !hasOnlyKeys(root, "dns") {
			return nil, NewValidationError("invalid_dns_yaml", "DNS wrapper must not contain unrelated top-level configuration")
		}
		root = mappingValue(root, "dns")
		if root == nil || root.Kind != yaml.MappingNode {
			return nil, NewValidationError("invalid_dns_yaml", "dns wrapper value must be a mapping")
		}
	}
	value, err := nodeToValue(root)
	if err != nil {
		return nil, err
	}
	cfg, ok := value.(map[string]any)
	if !ok {
		return nil, NewValidationError("invalid_dns_yaml", "DNS configuration must be a mapping")
	}
	if value, exists := cfg["enable"]; exists {
		if _, ok := value.(bool); !ok {
			return nil, NewValidationError("invalid_dns_yaml", "DNS enable must be a boolean")
		}
	}
	cfg["enable"] = enabled
	if err := validateDNSFields(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateDNSNode(node *yaml.Node, depth int, seen map[*yaml.Node]bool) error {
	if depth > 64 {
		return NewValidationError("dns_yaml_too_deep", "DNS YAML nesting must not exceed 64 levels")
	}
	if node == nil || seen[node] {
		return NewValidationError("dns_yaml_alias_cycle", "DNS YAML aliases must not be recursive")
	}
	seen[node] = true
	defer delete(seen, node)
	if node.Kind == yaml.AliasNode {
		return NewValidationError("dns_yaml_alias_unsupported", "DNS YAML aliases are not supported")
	}
	if node.Kind == yaml.MappingNode {
		keys := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return NewValidationError("invalid_dns_yaml", "DNS mapping keys must be strings")
			}
			if _, ok := keys[key.Value]; ok {
				return NewValidationError("dns_yaml_duplicate_key", "DNS configuration contains a duplicate key")
			}
			keys[key.Value] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := validateDNSNode(child, depth+1, seen); err != nil {
			return err
		}
	}
	return nil
}

func nodeToValue(node *yaml.Node) (any, error) {
	if node.Kind == yaml.AliasNode {
		return nil, NewValidationError("dns_yaml_alias_unsupported", "DNS YAML aliases are not supported")
	}
	switch node.Kind {
	case yaml.MappingNode:
		result := make(map[string]any, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			value, err := nodeToValue(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			result[node.Content[i].Value] = value
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, len(node.Content))
		for i, child := range node.Content {
			value, err := nodeToValue(child)
			if err != nil {
				return nil, err
			}
			result[i] = value
		}
		return result, nil
	case yaml.ScalarNode:
		var value any
		if err := node.Decode(&value); err != nil {
			return nil, NewValidationError("invalid_dns_yaml", "DNS scalar value is invalid")
		}
		return value, nil
	default:
		return nil, NewValidationError("invalid_dns_yaml", fmt.Sprintf("unsupported DNS YAML node kind %d", node.Kind))
	}
}

func validateDNSFields(cfg map[string]any) error {
	for _, key := range []string{"ipv6", "use-hosts", "prefer-h3", "respect-rules", "enhanced-mode"} {
		if value, exists := cfg[key]; exists {
			if key == "enhanced-mode" {
				if _, ok := value.(string); !ok {
					return NewValidationError("invalid_dns_yaml", "DNS enhanced-mode must be a string")
				}
			} else if _, ok := value.(bool); !ok {
				return NewValidationError("invalid_dns_yaml", "DNS boolean options must be booleans")
			}
		}
	}
	for _, key := range []string{"nameserver", "fallback", "default-nameserver", "proxy-server-nameserver", "direct-nameserver"} {
		if value, exists := cfg[key]; exists {
			list, ok := value.([]any)
			if !ok || !allStrings(list) {
				return NewValidationError("invalid_dns_yaml", "DNS server fields must be lists of strings")
			}
		}
	}
	for _, key := range []string{"fake-ip-range", "listen", "cache-algorithm"} {
		if value, exists := cfg[key]; exists {
			if _, ok := value.(string); !ok {
				return NewValidationError("invalid_dns_yaml", "DNS string option has an invalid type")
			}
		}
	}
	for _, key := range []string{"store-fake-ip", "use-system-hosts", "direct-nameserver-follow-policy"} {
		if value, exists := cfg[key]; exists {
			if _, ok := value.(bool); !ok {
				return NewValidationError("invalid_dns_yaml", "DNS boolean options must be booleans")
			}
		}
	}
	if value, exists := cfg["fake-ip-filter"]; exists {
		list, ok := value.([]any)
		if !ok || !allStrings(list) {
			return NewValidationError("invalid_dns_yaml", "DNS fake-ip-filter must be a list of strings")
		}
	}
	for _, key := range []string{"fallback-filter", "nameserver-policy", "proxy-server-nameserver-policy", "hosts"} {
		if value, exists := cfg[key]; exists {
			if _, ok := value.(map[string]any); !ok {
				return NewValidationError("invalid_dns_yaml", "DNS mapping option has an invalid type")
			}
		}
	}
	return nil
}

func allStrings(values []any) bool {
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func hasMappingKey(node *yaml.Node, key string) bool { return mappingValue(node, key) != nil }
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
func hasOnlyKeys(node *yaml.Node, allowed ...string) bool {
	set := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		set[key] = true
	}
	for i := 0; i < len(node.Content); i += 2 {
		if !set[node.Content[i].Value] {
			return false
		}
	}
	return true
}
