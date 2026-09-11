package pipeline

import (
	"context"
	"net"
	"net/http"
	"time"
)

// DefaultBaselineVerifier verifies host network connectivity directly without any proxy.
// It is consulted when a proxy probe fails to distinguish between proxy node failure and host network outage.
type DefaultBaselineVerifier struct {
	timeout time.Duration
	client  *http.Client
}

// NewDefaultBaselineVerifier creates a baseline verifier with a dedicated direct HTTP transport.
func NewDefaultBaselineVerifier(timeout time.Duration) *DefaultBaselineVerifier {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	directDialer := &net.Dialer{
		Timeout: timeout,
	}
	tr := &http.Transport{
		DialContext:       directDialer.DialContext,
		Proxy:             nil,
		DisableKeepAlives: true,
	}
	return &DefaultBaselineVerifier{
		timeout: timeout,
		client: &http.Client{
			Transport: tr,
			Timeout:   timeout,
		},
	}
}

// CheckHostConnectivity tests whether the host itself can reach the public internet directly.
func (b *DefaultBaselineVerifier) CheckHostConnectivity(ctx context.Context) bool {
	// First check direct TCP to authoritative public DNS (1.1.1.1:53 or 8.8.8.8:53)
	d := net.Dialer{Timeout: b.timeout}
	conn, err := d.DialContext(ctx, "tcp", "1.1.1.1:53")
	if err == nil {
		_ = conn.Close()
		return true
	}

	conn8, err8 := d.DialContext(ctx, "tcp", "8.8.8.8:53")
	if err8 == nil {
		_ = conn8.Close()
		return true
	}

	// Fallback to HTTP 204 directly
	req, errReq := http.NewRequestWithContext(ctx, "GET", "http://www.gstatic.com/generate_204", nil)
	if errReq != nil {
		return false
	}
	resp, errResp := b.client.Do(req)
	if errResp == nil {
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK
	}

	return false
}
