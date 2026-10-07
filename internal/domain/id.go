package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		"password":       true,
		"secret":         true,
		"token":          true,
		"key":            true,
		"private_key":    true,
		"private-key":    true,
		"public_key":     true,
		"public-key":     true,
		"preshared_key":  true,
		"pre_shared_key": true,
		"pre-shared-key": true,
		"psk":            true,
		"obfs-password":  true,
		"obfs_password":  true,
		"uuid":           true,
		"auth":           true,
		"username":       true,
		"user":           true,
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

type canonicalConnectionIdentity struct {
	Version   string            `json:"version"`
	Protocol  string            `json:"protocol"`
	Server    string            `json:"server"`
	Port      int               `json:"port"`
	Transport map[string]string `json:"transport,omitempty"`
	Auth      canonicalAuth     `json:"auth"`
}

type canonicalAuth struct {
	Username          string   `json:"username,omitempty"`
	Password          string   `json:"password,omitempty"`
	UUID              string   `json:"uuid,omitempty"`
	Method            string   `json:"method,omitempty"`
	AlterID           int      `json:"alter_id,omitempty"`
	PrivateKey        string   `json:"private_key,omitempty"`
	PublicKey         string   `json:"public_key,omitempty"`
	PreSharedKey      string   `json:"pre_shared_key,omitempty"`
	LocalAddress      []string `json:"local_address,omitempty"`
	Reserved          []uint8  `json:"reserved,omitempty"`
	MTU               int      `json:"mtu,omitempty"`
	DNS               []string `json:"dns,omitempty"`
	CongestionControl string   `json:"congestion_control,omitempty"`
	UDPRelayMode      string   `json:"udp_relay_mode,omitempty"`
	ALPN              []string `json:"alpn,omitempty"`
	SNI               string   `json:"sni,omitempty"`
	DisableSNI        bool     `json:"disable_sni,omitempty"`
	ObfsPassword      string   `json:"obfs_password,omitempty"`
}

// ComputeConnectionLogicalID computes a stable logical identity hash based on the full canonical connection configuration,
// including protocol, endpoint, transport options, and credentials.
// Sensitive fields are included in the deterministic SHA-256 digest solely in-memory to prevent collision of distinct
// credentials on the same endpoint, without persisting or exposing plain secrets in the logical ID or transport parameters.
func ComputeConnectionLogicalID(protocol Protocol, server string, port int, transportParams map[string]string, creds InboundProtocolCredential) string {
	normProtocol := strings.ToLower(strings.TrimSpace(string(protocol)))
	normServer := strings.ToLower(strings.TrimSpace(server))

	// Normalize transport map (lowercase keys, trimmed values)
	var normTransport map[string]string
	srcTransport := transportParams
	if len(srcTransport) == 0 && len(creds.Transport) > 0 {
		srcTransport = creds.Transport
	}
	if len(srcTransport) > 0 {
		normTransport = make(map[string]string, len(srcTransport))
		for k, v := range srcTransport {
			trimmedK := strings.ToLower(strings.TrimSpace(k))
			trimmedV := strings.TrimSpace(v)
			if trimmedK == "" || trimmedV == "" {
				continue
			}
			// Skip obfs password from transport dictionary because it is normalized into Auth
			if trimmedK == "obfs-password" || trimmedK == "obfs_password" {
				continue
			}
			normTransport[trimmedK] = trimmedV
		}
	}

	// Normalize auth fields
	var localAddrs []string
	if len(creds.LocalAddress) > 0 {
		localAddrs = make([]string, len(creds.LocalAddress))
		copy(localAddrs, creds.LocalAddress)
		sort.Strings(localAddrs)
	}

	var dnsList []string
	if len(creds.DNS) > 0 {
		dnsList = make([]string, len(creds.DNS))
		copy(dnsList, creds.DNS)
		sort.Strings(dnsList)
	}

	var reservedCopy []uint8
	if len(creds.Reserved) > 0 {
		reservedCopy = make([]uint8, len(creds.Reserved))
		copy(reservedCopy, creds.Reserved)
	}

	var alpnCopy []string
	if len(creds.ALPN) > 0 {
		alpnCopy = make([]string, len(creds.ALPN))
		copy(alpnCopy, creds.ALPN)
	}

	obfsPassword := ""
	if creds.Transport != nil {
		if op, ok := creds.Transport["obfs-password"]; ok {
			obfsPassword = op
		} else if op, ok := creds.Transport["obfs_password"]; ok {
			obfsPassword = op
		}
	}

	auth := canonicalAuth{
		Username:          strings.TrimSpace(creds.Username),
		Password:          creds.Password, // Exact credentials preserved without modifying case
		UUID:              strings.ToLower(strings.TrimSpace(creds.UUID)),
		Method:            strings.ToLower(strings.TrimSpace(creds.Method)),
		AlterID:           creds.AlterID,
		PrivateKey:        strings.TrimSpace(creds.PrivateKey),
		PublicKey:         strings.TrimSpace(creds.PublicKey),
		PreSharedKey:      strings.TrimSpace(creds.EffectivePreSharedKey()),
		LocalAddress:      localAddrs,
		Reserved:          reservedCopy,
		MTU:               creds.MTU,
		DNS:               dnsList,
		CongestionControl: strings.ToLower(strings.TrimSpace(creds.CongestionControl)),
		UDPRelayMode:      strings.ToLower(strings.TrimSpace(creds.UDPRelayMode)),
		ALPN:              alpnCopy,
		SNI:               strings.ToLower(strings.TrimSpace(creds.SNI)),
		DisableSNI:        creds.DisableSNI,
		ObfsPassword:      obfsPassword,
	}

	identity := canonicalConnectionIdentity{
		Version:   "conn_v1",
		Protocol:  normProtocol,
		Server:    normServer,
		Port:      port,
		Transport: normTransport,
		Auth:      auth,
	}

	data, err := json.Marshal(identity)
	if err != nil {
		data = []byte(fmt.Sprintf("%s|%s|%d|%v|%v", normProtocol, normServer, port, normTransport, auth))
	}

	sum := sha256.Sum256(data)
	return "node_" + hex.EncodeToString(sum[:16])
}

// IsValidLogicalID checks whether a string is a valid node logical ID.
func IsValidLogicalID(s string) bool {
	return logicalIDRegex.MatchString(s)
}
