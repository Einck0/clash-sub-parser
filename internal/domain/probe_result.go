package domain

import (
	"strings"
)

// ProbeStatus describes the connectivity observation outcome.
type ProbeStatus string

const (
	ProbeStatusOK       ProbeStatus = "online"
	ProbeStatusFailed   ProbeStatus = "offline"
	ProbeStatusTimeout  ProbeStatus = "timeout"
	ProbeStatusError    ProbeStatus = "error"
	ProbeStatusUntested ProbeStatus = "untested"
)

// MediaUnlockInfo encapsulates regional streaming and AI platform unlock status.
type MediaUnlockInfo struct {
	Netflix  string `json:"netflix,omitempty" yaml:"netflix,omitempty"`
	YouTube  string `json:"youtube,omitempty" yaml:"youtube,omitempty"`
	Disney   string `json:"disney,omitempty" yaml:"disney,omitempty"`
	ChatGPT  string `json:"chatgpt,omitempty" yaml:"chatgpt,omitempty"`
	Gemini   string `json:"gemini,omitempty" yaml:"gemini,omitempty"`
	Claude   string `json:"claude,omitempty" yaml:"claude,omitempty"`
	Spotify  string `json:"spotify,omitempty" yaml:"spotify,omitempty"`
	Bilibili string `json:"bilibili,omitempty" yaml:"bilibili,omitempty"`
}

// NodeProbeResult represents persisted and live capability probe observations.
type NodeProbeResult struct {
	ID           int64           `json:"id" yaml:"id"`
	NodeKey      string          `json:"node_key" yaml:"node_key"`
	Name         string          `json:"name" yaml:"name"`
	Server       string          `json:"server" yaml:"server"`
	Port         int             `json:"port" yaml:"port"`
	Type         string          `json:"type" yaml:"type"`
	Status       ProbeStatus     `json:"status" yaml:"status"`
	LatencyMs    *int64          `json:"latency_ms,omitempty" yaml:"latency_ms,omitempty"`
	SpeedMbps    *float64        `json:"speed_mbps,omitempty" yaml:"speed_mbps,omitempty"`
	IP           string          `json:"ip,omitempty" yaml:"ip,omitempty"`
	Country      string          `json:"country,omitempty" yaml:"country,omitempty"`
	ASN          *int64          `json:"asn,omitempty" yaml:"asn,omitempty"`
	Organization string          `json:"organization,omitempty" yaml:"organization,omitempty"`
	Media        MediaUnlockInfo `json:"media,omitempty" yaml:"media,omitempty"`
	Error        string          `json:"error,omitempty" yaml:"error,omitempty"`
	CheckedAt    int64           `json:"checked_at" yaml:"checked_at"`
}

// Validate checks that probe result invariants are satisfied.
func (p *NodeProbeResult) Validate() error {
	if strings.TrimSpace(p.NodeKey) == "" {
		return ErrMissingNodeKey
	}
	switch p.Status {
	case ProbeStatusOK, ProbeStatusFailed, ProbeStatusTimeout, ProbeStatusError, ProbeStatusUntested:
		// Valid
	default:
		return ErrInvalidProbeStatus
	}
	return nil
}

// IsAvailable indicates whether the proxy successfully passed connectivity probing.
func (p *NodeProbeResult) IsAvailable() bool {
	return p.Status == ProbeStatusOK
}

// HasMediaUnlock checks whether the node unlocks the specified streaming service.
func (p *NodeProbeResult) HasMediaUnlock(service string) bool {
	s := strings.ToLower(strings.TrimSpace(service))
	var val string
	switch s {
	case "netflix":
		val = p.Media.Netflix
	case "youtube":
		val = p.Media.YouTube
	case "disney", "disney+":
		val = p.Media.Disney
	case "spotify":
		val = p.Media.Spotify
	case "bilibili":
		val = p.Media.Bilibili
	default:
		return false
	}
	normVal := strings.ToLower(val)
	return normVal != "" && normVal != "no" && normVal != "none" && normVal != "false" && normVal != "failed"
}

// HasAIUnlock checks whether the node unlocks the specified AI service.
func (p *NodeProbeResult) HasAIUnlock(service string) bool {
	s := strings.ToLower(strings.TrimSpace(service))
	var val string
	switch s {
	case "chatgpt", "openai":
		val = p.Media.ChatGPT
	case "gemini", "google":
		val = p.Media.Gemini
	case "claude", "anthropic":
		val = p.Media.Claude
	default:
		return false
	}
	normVal := strings.ToLower(val)
	return normVal != "" && normVal != "no" && normVal != "none" && normVal != "false" && normVal != "failed"
}
