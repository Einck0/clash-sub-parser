package pipeline

import (
	"context"
	"time"
)

const (
	// DefaultUserAgent is the standard modern browser User-Agent used for capability probing.
	DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// LatencyMetrics records detailed handshake and round-trip timing observations.
type LatencyMetrics struct {
	TCPHandshakeMs int64 `json:"tcp_handshake_ms,omitempty"`
	TLSHandshakeMs int64 `json:"tls_handshake_ms,omitempty"`
	RTTMs          int64 `json:"rtt_ms"`
}

// TransportResult captures the outcome of the HTTP 204 low-overhead connectivity check.
type TransportResult struct {
	Available  bool           `json:"available"`
	Latency    LatencyMetrics `json:"latency"`
	TargetURL  string         `json:"target_url"`
	StatusCode int            `json:"status_code"`
	Error      error          `json:"error,omitempty"`
}

// GeoIdentityResult represents exit IP, geographic location, and ASN consensus.
type GeoIdentityResult struct {
	IP           string `json:"ip"`
	Country      string `json:"country"`
	City         string `json:"city,omitempty"`
	ASN          *int64 `json:"asn,omitempty"`
	Organization string `json:"organization,omitempty"`
	Confidence   string `json:"confidence,omitempty"`
}

// StreamingResult encapsulates streaming platform unlock evaluation outcomes.
type StreamingResult struct {
	YouTube  string `json:"youtube,omitempty"`
	Netflix  string `json:"netflix,omitempty"`
	Disney   string `json:"disney,omitempty"`
	Bilibili string `json:"bilibili,omitempty"`
}

// AIResult encapsulates AI platform availability evaluation outcomes.
type AIResult struct {
	ChatGPT string `json:"chatgpt,omitempty"`
	Gemini  string `json:"gemini,omitempty"`
	Claude  string `json:"claude,omitempty"`
}

// BaselineVerifier evaluates host network health to prevent false-positive failures during host outages.
type BaselineVerifier interface {
	CheckHostConnectivity(ctx context.Context) bool
}

// Config specifies timeouts, candidate endpoints, and execution toggles for the probe pipeline.
type Config struct {
	Timeout          time.Duration
	StageTimeout     time.Duration
	Transport204URLs []string
	IPEndpoints      []string
	CheckStreaming   bool
	CheckAI          bool
	BaselineVerifier BaselineVerifier
	UserAgent        string

	// Optional platform target URL overrides (used in offline testing or custom mirrors)
	YouTubeURL     string
	NetflixBaseURL string
	DisneyURL      string
	BilibiliURL    string
	ChatGPTURL     string
	GeminiURL      string
	ClaudeURL      string
}

// DefaultConfig provides recommended production settings conforming to proxy-node-capability-probing.
func DefaultConfig() Config {
	return Config{
		Timeout:      8 * time.Second,
		StageTimeout: 3 * time.Second,
		Transport204URLs: []string{
			"http://cp.cloudflare.com/generate_204",
			"http://www.gstatic.com/generate_204",
			"https://cp.cloudflare.com/generate_204",
			"https://www.google.com/generate_204",
		},
		IPEndpoints: []string{
			"https://api.ip.sb/geoip",
			"https://cloudflare.com/cdn-cgi/trace",
			"https://api.ipify.org?format=json",
		},
		CheckStreaming:   true,
		CheckAI:          true,
		BaselineVerifier: NewDefaultBaselineVerifier(2 * time.Second),
		UserAgent:        DefaultUserAgent,
	}
}
