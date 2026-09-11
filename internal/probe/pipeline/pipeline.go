package pipeline

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/dialer"
)

var (
	// ErrNilNode is returned when attempting to probe a nil node.
	ErrNilNode = errors.New("node is nil")
	// ErrNilDialer is returned when attempting to probe with a nil dialer.
	ErrNilDialer = errors.New("dialer is nil")
)

// Pipeline coordinates multi-stage proxy capability probing with physical egress isolation and circuit-breaking.
type Pipeline struct {
	cfg Config
}

// New constructs a new Pipeline using the provided configuration.
func New(cfg Config) *Pipeline {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.StageTimeout <= 0 {
		cfg.StageTimeout = 3 * time.Second
	}
	if len(cfg.Transport204URLs) == 0 {
		cfg.Transport204URLs = DefaultConfig().Transport204URLs
	}
	if len(cfg.IPEndpoints) == 0 {
		cfg.IPEndpoints = DefaultConfig().IPEndpoints
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	return &Pipeline{cfg: cfg}
}

// Config returns the pipeline configuration.
func (p *Pipeline) Config() Config {
	return p.cfg
}

// Execute runs the complete probe lifecycle for a single node through its dedicated dialer.
func (p *Pipeline) Execute(ctx context.Context, node *domain.Node, d dialer.Dialer) (*domain.NodeProbeResult, error) {
	if node == nil {
		return nil, ErrNilNode
	}
	if d == nil {
		return nil, ErrNilDialer
	}

	totalCtx, totalCancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer totalCancel()

	nodeKey := node.LogicalID
	if nodeKey == "" {
		nodeKey = node.Name
	}

	result := &domain.NodeProbeResult{
		NodeKey:   nodeKey,
		Name:      node.Name,
		Server:    node.Server,
		Port:      node.Port,
		Type:      string(node.Protocol),
		Status:    domain.ProbeStatusUntested,
		CheckedAt: time.Now().Unix(),
	}

	// Construct strictly isolated HTTP client for this specific dialer
	client := NewIsolatedHTTPClient(d, p.cfg.StageTimeout)
	defer func() {
		if tr, ok := client.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}()

	// Stage 1: Transport 204 low-overhead latency probe
	transportRes, transportErr := MeasureTransport(totalCtx, client, p.cfg.Transport204URLs, p.cfg.StageTimeout)
	if transportErr != nil || (transportRes != nil && !transportRes.Available) {
		// Verify host network baseline to protect against false positives
		if p.cfg.BaselineVerifier != nil && !p.cfg.BaselineVerifier.CheckHostConnectivity(totalCtx) {
			result.Status = domain.ProbeStatusError
			result.Error = "host network unreachable: suppressed false-positive failure"
			return result, nil
		}

		// Distinguish timeout vs handshake failure
		isTimeout := errors.Is(totalCtx.Err(), context.DeadlineExceeded) ||
			(transportErr != nil && strings.Contains(strings.ToLower(transportErr.Error()), "timeout")) ||
			(transportRes != nil && transportRes.Error != nil && strings.Contains(strings.ToLower(transportRes.Error.Error()), "timeout"))

		if isTimeout {
			result.Status = domain.ProbeStatusTimeout
			if transportErr != nil {
				result.Error = transportErr.Error()
			} else if transportRes != nil && transportRes.Error != nil {
				result.Error = transportRes.Error.Error()
			} else {
				result.Error = "transport 204 handshake timed out"
			}
		} else {
			result.Status = domain.ProbeStatusFailed
			if transportErr != nil {
				result.Error = transportErr.Error()
			} else if transportRes != nil && transportRes.Error != nil {
				result.Error = transportRes.Error.Error()
			} else {
				result.Error = "transport 204 handshake failed"
			}
		}

		// CIRCUIT BREAKER: Halt execution immediately to save network bandwidth and host memory
		return result, nil
	}

	// Transport succeeded: mark node online and record RTT
	result.Status = domain.ProbeStatusOK
	rtt := transportRes.Latency.RTTMs
	result.LatencyMs = &rtt

	// Stage 2: Exit IP and Geo identity consensus
	geoRes, _ := ResolveGeoIP(totalCtx, client, p.cfg.IPEndpoints, p.cfg.StageTimeout)
	if geoRes != nil {
		result.IP = geoRes.IP
		result.Country = geoRes.Country
		result.ASN = geoRes.ASN
		result.Organization = geoRes.Organization
	}

	// Stage 3: Streaming unlock evaluation (YouTube, Netflix, Disney+, Bilibili)
	if p.cfg.CheckStreaming {
		streamRes, _ := CheckStreamingWithConfig(totalCtx, client, p.cfg)
		if streamRes != nil {
			result.Media.YouTube = streamRes.YouTube
			result.Media.Netflix = streamRes.Netflix
			result.Media.Disney = streamRes.Disney
			result.Media.Bilibili = streamRes.Bilibili
		}
	}

	// Stage 4: AI service availability evaluation (ChatGPT, Gemini, Claude)
	if p.cfg.CheckAI {
		aiRes, _ := CheckAIWithConfig(totalCtx, client, p.cfg)
		if aiRes != nil {
			result.Media.ChatGPT = aiRes.ChatGPT
			result.Media.Gemini = aiRes.Gemini
			result.Media.Claude = aiRes.Claude
		}
	}

	return result, nil
}
