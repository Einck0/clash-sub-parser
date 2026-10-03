package platform_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
)

// Helper roundTripper for mocking HTTP responses
type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func mockClient(fn roundTripperFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func jsonResponse(status int, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func htmlResponse(status int, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

// 2.1 Alive Tests
func TestCheckAlive(t *testing.T) {
	ctx := context.Background()

	// Valid 204
	client204 := mockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})
	ok, res := platform.CheckAlive(ctx, client204, "")
	if !ok || res.Verdict != domain.VerdictAvailable {
		t.Fatalf("expected available for 204, got ok=%v verdict=%v", ok, res.Verdict)
	}

	// 500 error
	client500 := mockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})
	ok, res = platform.CheckAlive(ctx, client500, "")
	if ok || res.Verdict != domain.VerdictRestricted {
		t.Fatalf("expected restricted for 500, got ok=%v verdict=%v", ok, res.Verdict)
	}
}

// 2.2 OpenAI Tests
func TestCheckOpenAI(t *testing.T) {
	ctx := context.Background()

	// Scenario 1: Full (GPT⁺) with region US
	clientFull := mockClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "api.openai.com":
			return jsonResponse(200, `{"cookies": "ok"}`)
		case "ios.chat.openai.com":
			return jsonResponse(200, `{"status": "ok"}`)
		case "chat.openai.com":
			return htmlResponse(200, "loc=US\nip=1.2.3.4")
		default:
			return jsonResponse(404, "")
		}
	})
	cap := platform.CheckOpenAI(ctx, clientFull)
	if cap.Verdict != domain.VerdictAvailable || cap.SubTier != "full" || cap.Region != "US" {
		t.Fatalf("expected Full(US), got: %+v", cap)
	}

	// Scenario 2: Web (GPT) (cookies pass, iOS client fails with disallowed isp)
	clientWeb := mockClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "api.openai.com":
			return jsonResponse(200, `{"cookies": "ok"}`)
		case "ios.chat.openai.com":
			return jsonResponse(403, `{"error": "disallowed isp"}`)
		case "chat.openai.com":
			return htmlResponse(200, "loc=JP\nip=1.2.3.4")
		default:
			return jsonResponse(404, "")
		}
	})
	cap = platform.CheckOpenAI(ctx, clientWeb)
	if cap.Verdict != domain.VerdictAvailable || cap.SubTier != "web" || cap.Region != "JP" {
		t.Fatalf("expected Web(JP), got: %+v", cap)
	}

	// Scenario 3: App-only (cookies fail with unsupported_country, iOS client passes)
	clientApp := mockClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "api.openai.com":
			return jsonResponse(403, `{"error": "unsupported_country"}`)
		case "ios.chat.openai.com":
			return jsonResponse(200, `{"status": "ok"}`)
		case "chat.openai.com":
			return htmlResponse(200, "loc=SG\nip=1.2.3.4")
		default:
			return jsonResponse(404, "")
		}
	})
	cap = platform.CheckOpenAI(ctx, clientApp)
	if cap.Verdict != domain.VerdictAvailable || cap.SubTier != "app" || cap.Region != "SG" {
		t.Fatalf("expected App(SG), got: %+v", cap)
	}

	// Scenario 4: Banned (both fail)
	clientBanned := mockClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(403, `{"error": "unsupported_country"}`)
	})
	cap = platform.CheckOpenAI(ctx, clientBanned)
	if cap.Verdict != domain.VerdictRestricted || cap.SubTier != "banned" {
		t.Fatalf("expected Banned, got: %+v", cap)
	}
}

// 2.3 Netflix Tests
func TestCheckNetflix(t *testing.T) {
	ctx := context.Background()

	// Scenario 1: Fast.com CDN 403 -> Banned
	clientBanned := mockClient(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Host, "fast.com") {
			return jsonResponse(403, "Forbidden")
		}
		return jsonResponse(404, "")
	})
	cap := platform.CheckNetflix(ctx, clientBanned)
	if cap.Verdict != domain.VerdictRestricted || cap.SubTier != "banned" {
		t.Fatalf("expected Banned from Fast CDN, got: %+v", cap)
	}

	// Scenario 2: Fast.com CDN 200 with country -> Full
	clientFastFull := mockClient(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Host, "fast.com") {
			return jsonResponse(200, `{"targets":[{"location":{"country":"SG"}}]}`)
		}
		return jsonResponse(404, "")
	})
	cap = platform.CheckNetflix(ctx, clientFastFull)
	if cap.Verdict != domain.VerdictAvailable || cap.SubTier != "full" || cap.Region != "SG" {
		t.Fatalf("expected Full(SG), got: %+v", cap)
	}

	// Scenario 3: Fast CDN inconclusive, title fallback: non-original 404, original 200 -> Originals Only
	clientOriginals := mockClient(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Host, "fast.com") {
			return jsonResponse(500, "error")
		}
		if strings.Contains(req.URL.Path, "81280792") {
			return jsonResponse(404, "Not Found")
		}
		if strings.Contains(req.URL.Path, "70143836") {
			return jsonResponse(200, "OK Originals")
		}
		return jsonResponse(404, "")
	})
	cap = platform.CheckNetflix(ctx, clientOriginals)
	if cap.Verdict != domain.VerdictAvailable || cap.SubTier != "originals" {
		t.Fatalf("expected Originals, got: %+v", cap)
	}
}

// 2.4 YouTube Tests
func TestCheckYoutube(t *testing.T) {
	ctx := context.Background()

	// Scenario 1: Redirected to CN
	clientCN := mockClient(func(req *http.Request) (*http.Response, error) {
		return htmlResponse(200, `<html><head><script>location.href="http://www.google.cn";</script></head></html>`)
	})
	cap := platform.CheckYoutube(ctx, clientCN)
	if cap.Verdict != domain.VerdictRestricted || cap.SubTier != "banned" || cap.Region != "CN" {
		t.Fatalf("expected Banned(CN) for sent to China, got: %+v", cap)
	}

	// Scenario 2: Unlocked with GL
	clientUnlocked := mockClient(func(req *http.Request) (*http.Response, error) {
		body := `<html><body><div>YouTube Premium</div><script>"INNERTUBE_CONTEXT_GL":"US"</script></body></html>`
		return htmlResponse(200, body)
	})
	cap = platform.CheckYoutube(ctx, clientUnlocked)
	if cap.Verdict != domain.VerdictAvailable || cap.SubTier != "unlocked" || cap.Region != "US" {
		t.Fatalf("expected Unlocked(US), got: %+v", cap)
	}

	// Scenario 3: Premium not available in country
	clientUnavailable := mockClient(func(req *http.Request) (*http.Response, error) {
		return htmlResponse(200, `<html><body>Premium is not available in your country</body></html>`)
	})
	cap = platform.CheckYoutube(ctx, clientUnavailable)
	if cap.Verdict != domain.VerdictRestricted || cap.SubTier != "banned" {
		t.Fatalf("expected Banned for unavailable, got: %+v", cap)
	}
}

// 2.5 Disney, Claude, and Gemini Tests
func TestCheckDisney(t *testing.T) {
	ctx := context.Background()

	// Scenario 1: Supported location (Unlocked)
	clientUnlocked := mockClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/devices":
			return jsonResponse(200, `{"assertion":"tok123"}`)
		case "/token":
			return jsonResponse(200, `{"refresh_token":"ref123"}`)
		case "/graph/v1/device/graphql":
			return jsonResponse(200, `{"data":{"session":{"countryCode":"US","inSupportedLocation":true}}}`)
		default:
			return jsonResponse(404, "")
		}
	})
	cap := platform.CheckDisney(ctx, clientUnlocked)
	if cap.Verdict != domain.VerdictAvailable || cap.SubTier != "unlocked" || cap.Region != "US" {
		t.Fatalf("expected Unlocked(US), got: %+v", cap)
	}

	// Scenario 2: Location coming soon -> must be Restricted (sub_tier soon)
	clientSoon := mockClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/devices":
			return jsonResponse(200, `{"assertion":"tok123"}`)
		case "/token":
			return jsonResponse(200, `{"refresh_token":"ref123"}`)
		case "/graph/v1/device/graphql":
			return jsonResponse(200, `{"data":{"session":{"countryCode":"TR","inSupportedLocation":false}}}`)
		default:
			return jsonResponse(404, "")
		}
	})
	cap = platform.CheckDisney(ctx, clientSoon)
	if cap.Verdict != domain.VerdictRestricted || cap.SubTier != "soon" || cap.Region != "TR" {
		t.Fatalf("expected Restricted/Soon(TR), got: %+v", cap)
	}

	// Scenario 3: Banned (403 on devices)
	clientBanned := mockClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(403, "Forbidden")
	})
	cap = platform.CheckDisney(ctx, clientBanned)
	if cap.Verdict != domain.VerdictRestricted || cap.SubTier != "banned" {
		t.Fatalf("expected Banned, got: %+v", cap)
	}
}

func TestCheckClaude(t *testing.T) {
	ctx := context.Background()

	// Available in US
	clientUS := mockClient(func(req *http.Request) (*http.Response, error) {
		return htmlResponse(200, "loc=US\nip=1.2.3.4\n")
	})
	cap := platform.CheckClaude(ctx, clientUS)
	if cap.Verdict != domain.VerdictAvailable || cap.SubTier != "unlocked" || cap.Region != "US" {
		t.Fatalf("expected Unlocked(US), got: %+v", cap)
	}

	// Blocked in CN
	clientCN := mockClient(func(req *http.Request) (*http.Response, error) {
		return htmlResponse(200, "loc=CN\nip=1.2.3.4\n")
	})
	cap = platform.CheckClaude(ctx, clientCN)
	if cap.Verdict != domain.VerdictRestricted || cap.SubTier != "banned" || cap.Region != "CN" {
		t.Fatalf("expected Banned(CN), got: %+v", cap)
	}
}

func TestCheckGemini(t *testing.T) {
	ctx := context.Background()

	// Available in USA
	clientUSA := mockClient(func(req *http.Request) (*http.Response, error) {
		return htmlResponse(200, `[null,2,1,200,"USA"]`)
	})
	cap := platform.CheckGemini(ctx, clientUSA)
	if cap.Verdict != domain.VerdictAvailable || cap.SubTier != "unlocked" || cap.Region != "US" {
		t.Fatalf("expected Unlocked(US), got: %+v", cap)
	}

	// Blocked in CHN
	clientCHN := mockClient(func(req *http.Request) (*http.Response, error) {
		return htmlResponse(200, `[null,2,1,200,"CHN"]`)
	})
	cap = platform.CheckGemini(ctx, clientCHN)
	if cap.Verdict != domain.VerdictRestricted || cap.SubTier != "banned" {
		t.Fatalf("expected Banned for CHN, got: %+v", cap)
	}
}

// 2.6 Speed & NetworkLimitedReader Tests
func TestNetworkLimitedReader_EOFAtLimit(t *testing.T) {
	// Source with 100 bytes
	sourceData := bytes.Repeat([]byte("a"), 100)
	var counter uint64

	reader := &platform.NetworkLimitedReader{
		Reader:       bytes.NewReader(sourceData),
		BytesCounter: &counter,
		StartBytes:   0,
		Limit:        30,
	}

	buf := make([]byte, 20)
	// First read: 20 bytes
	n1, err1 := reader.Read(buf)
	if err1 != nil || n1 != 20 {
		t.Fatalf("first read: n=%d err=%v", n1, err1)
	}
	atomic.AddUint64(&counter, uint64(n1))

	// Second read: should only allow remaining 10 bytes to reach limit=30
	n2, err2 := reader.Read(buf)
	if err2 != nil || n2 != 10 {
		t.Fatalf("second read: n=%d err=%v", n2, err2)
	}
	atomic.AddUint64(&counter, uint64(n2))

	// Third read: limit reached, must return 0, io.EOF
	n3, err3 := reader.Read(buf)
	if err3 != io.EOF || n3 != 0 {
		t.Fatalf("third read: expected 0, io.EOF, got n=%d err=%v", n3, err3)
	}
}

func TestCheckSpeed_CalculatesThroughput(t *testing.T) {
	ctx := context.Background()

	// 512 KB payload
	payloadSize := 512 * 1024
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", payloadSize))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, payloadSize))
	}))
	defer server.Close()

	var counter uint64
	cap, err := platform.CheckSpeed(ctx, server.Client(), &counter, server.URL, 1024*1024, 5*time.Second)
	if err != nil {
		t.Fatalf("CheckSpeed failed: %v", err)
	}

	if cap.Verdict != domain.VerdictAvailable {
		t.Fatalf("expected VerdictAvailable, got %v", cap.Verdict)
	}
	if cap.Throughput == nil || *cap.Throughput <= 0 {
		t.Fatalf("expected positive Throughput, got %v", cap.Throughput)
	}
}

func TestCheckIPRisk(t *testing.T) {
	ctx := context.Background()

	clientScam := mockClient(func(req *http.Request) (*http.Response, error) {
		body := `
		Some header
		IP Fraud Risk API
		line 1
		line 2
		line 3
		"score": "12",
		"risk": "low"
		line 6
		`
		return htmlResponse(200, body)
	})

	cap := platform.CheckIPRisk(ctx, clientScam, "1.2.3.4")
	if cap.Verdict != domain.VerdictAvailable || cap.RiskScore != "12% low" {
		t.Fatalf("expected '12%% low', got: %+v", cap)
	}
}
