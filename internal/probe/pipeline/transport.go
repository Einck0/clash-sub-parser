package pipeline

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"time"
)

var (
	// ErrNoCandidateURLs is returned when no candidate 204 URLs are provided.
	ErrNoCandidateURLs = errors.New("no candidate 204 URLs provided")
	// ErrTransportCheckFailed is returned when all candidate 204 endpoints fail.
	ErrTransportCheckFailed = errors.New("transport 204 connectivity check failed")
)

// MeasureTransport executes an HTTP 204 latency probe measuring TCP, TLS, and total RTT.
// It iterates over candidate 204 URLs until the first successful HTTP 204/200 response is observed.
func MeasureTransport(ctx context.Context, client *http.Client, candidateURLs []string, timeout time.Duration) (*TransportResult, error) {
	if len(candidateURLs) == 0 {
		return nil, ErrNoCandidateURLs
	}
	if client == nil {
		client = http.DefaultClient
	}

	overallStart := time.Now()
	var lastErr error
	var lastStatusCode int
	var lastTarget string

	for _, targetURL := range candidateURLs {
		lastTarget = targetURL
		select {
		case <-ctx.Done():
			return &TransportResult{
				Available: false,
				TargetURL: targetURL,
				Error:     ctx.Err(),
			}, ctx.Err()
		default:
		}

		reqStart := time.Now()
		var (
			connectStart, connectDone time.Time
			tlsStart, tlsDone         time.Time
			gotFirstByte              time.Time
		)

		trace := &httptrace.ClientTrace{
			ConnectStart: func(network, addr string) {
				connectStart = time.Now()
			},
			ConnectDone: func(network, addr string, err error) {
				connectDone = time.Now()
			},
			TLSHandshakeStart: func() {
				tlsStart = time.Now()
			},
			TLSHandshakeDone: func(state tls.ConnectionState, err error) {
				tlsDone = time.Now()
			},
			GotFirstResponseByte: func() {
				gotFirstByte = time.Now()
			},
		}

		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		reqCtx = httptrace.WithClientTrace(reqCtx, trace)

		req, err := http.NewRequestWithContext(reqCtx, "GET", targetURL, nil)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", DefaultUserAgent)
		req.Header.Set("Connection", "close")

		resp, err := client.Do(req)
		cancel()

		if err != nil {
			lastErr = err
			continue
		}

		lastStatusCode = resp.StatusCode
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
			rttMs := time.Since(reqStart).Milliseconds()
			if rttMs <= 0 {
				rttMs = 1
			}

			var tcpHandshakeMs int64
			if !connectStart.IsZero() && !connectDone.IsZero() {
				tcpHandshakeMs = connectDone.Sub(connectStart).Milliseconds()
			}

			var tlsHandshakeMs int64
			if !tlsStart.IsZero() && !tlsDone.IsZero() {
				tlsHandshakeMs = tlsDone.Sub(tlsStart).Milliseconds()
			}

			_ = gotFirstByte

			return &TransportResult{
				Available: true,
				Latency: LatencyMetrics{
					TCPHandshakeMs: tcpHandshakeMs,
					TLSHandshakeMs: tlsHandshakeMs,
					RTTMs:          rttMs,
				},
				TargetURL:  targetURL,
				StatusCode: resp.StatusCode,
			}, nil
		}

		lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	totalDuration := time.Since(overallStart).Milliseconds()
	_ = totalDuration

	if lastErr == nil {
		lastErr = ErrTransportCheckFailed
	}

	return &TransportResult{
		Available:  false,
		TargetURL:  lastTarget,
		StatusCode: lastStatusCode,
		Error:      lastErr,
	}, lastErr
}
