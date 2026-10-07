package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"clash-sub-parser/internal/domain"
)

const (
	DefaultSpeedTestURL = "http://speed.cloudflare.com/__down?bytes=1048576"
	DefaultDownloadMB   = 1
	DefaultSpeedTimeout = 10 * time.Second
)

// NetworkLimitedReader bounds application-level body reads based on an atomic network byte counter or byte limit.
// Note: This enforces an application read budget on resp.Body. Because TCP buffers read ahead at the OS network layer,
// this is not an OS/NIC-level packet cap. When the limit is reached, it returns io.EOF.
type NetworkLimitedReader struct {
	Reader       io.Reader
	BytesCounter *uint64
	StartBytes   uint64
	Limit        uint64
	readBytes    uint64
}

func (r *NetworkLimitedReader) Read(p []byte) (n int, err error) {
	if r.Limit > 0 {
		var networkRead uint64
		if r.BytesCounter != nil {
			currentBytes := atomic.LoadUint64(r.BytesCounter)
			networkRead = currentBytes - r.StartBytes
		} else {
			networkRead = r.readBytes
		}

		if networkRead >= r.Limit {
			return 0, io.EOF
		}

		if remaining := r.Limit - networkRead; remaining < uint64(len(p)) {
			p = p[:remaining]
		}
	}
	n, err = r.Reader.Read(p)
	if n > 0 && r.BytesCounter == nil {
		r.readBytes += uint64(n)
	}
	return n, err
}

// CheckSpeed measures throughput over an HTTP connection with bounded byte budget and monotonic deadline.
func CheckSpeed(ctx context.Context, httpClient *http.Client, bytesCounter *uint64, speedTestURL string, limitBytes uint64, timeout time.Duration) (domain.PlatformCapability, error) {
	now := time.Now().UTC()
	start := time.Now()

	if limitBytes == 0 {
		limitBytes = uint64(DefaultDownloadMB) * 1024 * 1024
	}
	if timeout <= 0 {
		timeout = DefaultSpeedTimeout
	}

	if speedTestURL == "" {
		speedTestURL = fmt.Sprintf("http://speed.cloudflare.com/__down?bytes=%d", limitBytes)
	} else if strings.Contains(speedTestURL, "__down") {
		if parsedURL, parseErr := url.Parse(speedTestURL); parseErr == nil {
			q := parsedURL.Query()
			q.Set("bytes", fmt.Sprintf("%d", limitBytes))
			parsedURL.RawQuery = q.Encode()
			speedTestURL = parsedURL.String()
		}
	}

	speedCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(speedCtx, http.MethodGet, speedTestURL, nil)
	if err != nil {
		lat := time.Since(start).Milliseconds()
		return domain.PlatformCapability{
			Verdict:    domain.VerdictError,
			LatencyMS:  &lat,
			ObservedAt: &now,
			Reason:     "request_build_failed",
		}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Close = true

	var startBytes uint64
	if bytesCounter != nil {
		startBytes = atomic.LoadUint64(bytesCounter)
	}

	downloadStart := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		lat := time.Since(start).Milliseconds()
		if errors.Is(err, context.DeadlineExceeded) || (speedCtx.Err() == context.DeadlineExceeded) {
			return domain.PlatformCapability{
				Verdict:    domain.VerdictUnknown,
				LatencyMS:  &lat,
				ObservedAt: &now,
				Summary:    "speed request timed out waiting for response headers",
				Reason:     "speed_request_timeout",
			}, nil
		}
		return domain.PlatformCapability{
			Verdict:    domain.VerdictError,
			LatencyMS:  &lat,
			ObservedAt: &now,
			Reason:     "speed_request_failed",
		}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		lat := time.Since(start).Milliseconds()
		return domain.PlatformCapability{
			Verdict:    domain.VerdictUnknown,
			LatencyMS:  &lat,
			ObservedAt: &now,
			Summary:    "HTTP 429 target_rate_limited",
			Reason:     "target_rate_limited",
		}, nil
	}
	if resp.StatusCode == http.StatusForbidden {
		lat := time.Since(start).Milliseconds()
		return domain.PlatformCapability{
			Verdict:    domain.VerdictUnknown,
			LatencyMS:  &lat,
			ObservedAt: &now,
			Summary:    "HTTP 403 target_forbidden",
			Reason:     "target_forbidden",
		}, nil
	}
	if resp.StatusCode != http.StatusOK {
		lat := time.Since(start).Milliseconds()
		return domain.PlatformCapability{
			Verdict:    domain.VerdictUnknown,
			LatencyMS:  &lat,
			ObservedAt: &now,
			Summary:    fmt.Sprintf("HTTP %d", resp.StatusCode),
			Reason:     "contract_drift",
		}, nil
	}

	limitedReader := &NetworkLimitedReader{
		Reader:       resp.Body,
		BytesCounter: bytesCounter,
		StartBytes:   startBytes,
		Limit:        limitBytes,
	}

	// Read and discard up to limit
	discardBuf := make([]byte, 32*1024)
	var appBytesRead int64
	var timedOut bool
	for {
		n, rErr := limitedReader.Read(discardBuf)
		if n > 0 {
			appBytesRead += int64(n)
			if bytesCounter != nil {
				// If transport doesn't increment bytesCounter (e.g. standard mock transport), increment locally
				atomic.AddUint64(bytesCounter, uint64(n))
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			// Check if this error is due to timeout / context deadline exceeded
			if errors.Is(rErr, context.DeadlineExceeded) ||
				errors.Is(speedCtx.Err(), context.DeadlineExceeded) ||
				(ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)) {
				timedOut = true
				break
			}
			var netErr net.Error
			if errors.As(rErr, &netErr) && netErr.Timeout() {
				timedOut = true
				break
			}

			// If bytes were read before read error, consider as partial measurement
			if appBytesRead > 0 {
				timedOut = true
				break
			}

			// Other read errors
			lat := time.Since(start).Milliseconds()
			return domain.PlatformCapability{
				Verdict:    domain.VerdictError,
				LatencyMS:  &lat,
				ObservedAt: &now,
				Reason:     "download_read_error",
			}, rErr
		}
	}

	duration := time.Since(downloadStart).Milliseconds()
	if duration <= 0 {
		duration = 1
	}

	var actualBytes int64
	if bytesCounter != nil {
		actualBytes = int64(atomic.LoadUint64(bytesCounter) - startBytes)
	}
	if actualBytes == 0 {
		actualBytes = appBytesRead
	}

	if actualBytes < 1024 {
		lat := time.Since(start).Milliseconds()
		reason := "contract_drift"
		summary := fmt.Sprintf("read %d bytes below minimum 1024", actualBytes)
		if timedOut {
			reason = "speed_budget_limited"
			summary = fmt.Sprintf("read %d bytes below minimum 1024 in %d ms (time limit reached)", actualBytes, duration)
		}
		return domain.PlatformCapability{
			Verdict:    domain.VerdictUnknown,
			LatencyMS:  &lat,
			ObservedAt: &now,
			Summary:    summary,
			Reason:     reason,
		}, nil
	}

	// Throughput in KB/s (1024 bytes/sec): (actualBytes / 1024) / (duration / 1000)
	throughputKBps := (float64(actualBytes) / 1024.0) * 1000.0 / float64(duration)
	latency := time.Since(start).Milliseconds()
	throughputKbpsInt := int64(0)
	if duration > 0 {
		throughputKbpsInt = (actualBytes * 8) / duration
	}

	reason := "speed_test_completed"
	budgetTag := ""
	if timedOut {
		reason = "speed_budget_limited"
		budgetTag = " [speed_budget_limited]"
	}

	summary := fmt.Sprintf("%.1f KB/s (read %d bytes in %d ms) bytes_read=%d throughput_kbps=%d%s",
		throughputKBps, actualBytes, duration, actualBytes, throughputKbpsInt, budgetTag)
	return domain.PlatformCapability{
		Verdict:    domain.VerdictAvailable,
		LatencyMS:  &latency,
		ObservedAt: &now,
		Throughput: &throughputKBps,
		Summary:    summary,
		Reason:     reason,
	}, nil
}
