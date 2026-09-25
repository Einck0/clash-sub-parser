package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"clash-sub-parser/internal/domain"
)

// Default URLs for public identity sources
const (
	DefaultIPsbURL            = "https://api.ip.sb/geoip"
	DefaultCloudflareTraceURL = "https://cloudflare.com/cdn-cgi/trace"
	DefaultIdentityTTL        = 15 * time.Minute
	MaxIdentityResponseBytes  = 64 * 1024 // 64 KB
)

// Status describes whether independent exit observations formed a verified consensus.
type Status string

const (
	StatusVerified    Status = "verified"
	StatusConflicted  Status = "conflicted"
	StatusUnavailable Status = "unavailable"
	StatusError       Status = "error"
)

func (s Status) IsValid() bool {
	return s == StatusVerified || s == StatusConflicted || s == StatusUnavailable || s == StatusError
}

// Candidate is an in-memory observation from one independent identity source.
// The raw IP is strictly in-memory and is never persisted or published into domain.ExitIdentity.
type Candidate struct {
	IP           string `json:"-"`
	CountryCode  string `json:"country_code"`
	ASN          int    `json:"asn,omitempty"`
	Organization string `json:"organization,omitempty"`
}

func (c Candidate) valid() bool {
	ip := net.ParseIP(strings.TrimSpace(c.IP))
	if ip == nil {
		return false
	}
	cc := strings.TrimSpace(c.CountryCode)
	if len(cc) != 2 {
		return false
	}
	for i := 0; i < 2; i++ {
		if (cc[i] < 'A' || cc[i] > 'Z') && (cc[i] < 'a' || cc[i] > 'z') {
			return false
		}
	}
	return true
}

// NormalizedIP returns the canonical string representation of the IP.
func (c Candidate) NormalizedIP() string {
	ip := net.ParseIP(strings.TrimSpace(c.IP))
	if ip == nil {
		return ""
	}
	return ip.String()
}

// NormalizedCountryCode returns uppercase ISO 3166-1 alpha-2 country code.
func (c Candidate) NormalizedCountryCode() string {
	return strings.ToUpper(strings.TrimSpace(c.CountryCode))
}

// Source queries an egress identity endpoint using the provided HTTP client.
type Source interface {
	Name() string
	Probe(ctx context.Context, client *http.Client) (Candidate, error)
}

// Resolution is the outcome of a multi-source exit identity consensus evaluation.
type Resolution struct {
	Status        Status               `json:"status"`
	Identity      *domain.ExitIdentity `json:"identity,omitempty"`
	ExpiresAt     time.Time            `json:"expires_at"`
	Reason        string               `json:"reason,omitempty"`
	AgreedSources []string             `json:"agreed_sources,omitempty"`
}

// Resolver executes pairwise cross-validation across independent sources and maintains a per-node TTL cache.
type Resolver struct {
	mu     sync.RWMutex
	clock  func() time.Time
	ttl    time.Duration
	cache  map[string]cachedResolution
	flight flightGroup
}

type cachedResolution struct {
	value Resolution
}

type flightCall struct {
	wg  sync.WaitGroup
	val Resolution
	err error
}

type flightGroup struct {
	mu sync.Mutex
	m  map[string]*flightCall
}

func (g *flightGroup) Do(key string, fn func() (Resolution, error)) (Resolution, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*flightCall)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(flightCall)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}

// Option configures Resolver behavior.
type Option func(*Resolver)

// WithClock injects a custom clock for deterministic testing.
func WithClock(clock func() time.Time) Option {
	return func(r *Resolver) {
		if clock != nil {
			r.clock = clock
		}
	}
}

// WithTTL sets the cache duration for verified exit identities.
func WithTTL(ttl time.Duration) Option {
	return func(r *Resolver) {
		if ttl > 0 {
			r.ttl = ttl
		}
	}
}

// NewResolver constructs a Resolver with optional configuration.
func NewResolver(options ...Option) *Resolver {
	r := &Resolver{
		clock: time.Now,
		ttl:   DefaultIdentityTTL,
		cache: make(map[string]cachedResolution),
	}
	for _, opt := range options {
		opt(r)
	}
	return r
}

// Resolve queries sources, executes pairwise cross-validation, and returns a verified identity or safe failure.
func (r *Resolver) Resolve(ctx context.Context, client *http.Client, nodeLogicalID string, sources []Source) (Resolution, error) {
	if !domain.IsValidLogicalID(nodeLogicalID) {
		return Resolution{}, domain.NewValidationError("invalid_exit_node_logical_id", "exit identity node logical ID is invalid")
	}
	if client == nil {
		client = http.DefaultClient
	}
	if ctx == nil {
		ctx = context.Background()
	}

	now := r.clock().UTC()

	// Check cache first under read lock
	r.mu.RLock()
	if cached, ok := r.cache[nodeLogicalID]; ok && now.Before(cached.value.ExpiresAt) {
		val := cloneResolution(cached.value)
		r.mu.RUnlock()
		return val, nil
	}
	r.mu.RUnlock()

	// Use singleflight to prevent thundering herd on concurrent probes for the same node
	return r.flight.Do(nodeLogicalID, func() (Resolution, error) {
		// Re-check cache in case another goroutine just completed it
		now = r.clock().UTC()
		r.mu.RLock()
		if cached, ok := r.cache[nodeLogicalID]; ok && now.Before(cached.value.ExpiresAt) {
			val := cloneResolution(cached.value)
			r.mu.RUnlock()
			return val, nil
		}
		r.mu.RUnlock()

		activeSources := make([]Source, 0, len(sources))
		seenNames := make(map[string]struct{}, len(sources))
		for _, s := range sources {
			if s == nil {
				continue
			}
			name := strings.TrimSpace(s.Name())
			if name == "" {
				continue
			}
			if _, seen := seenNames[name]; seen {
				continue
			}
			seenNames[name] = struct{}{}
			activeSources = append(activeSources, s)
		}

		if len(activeSources) < 2 {
			res := Resolution{
				Status:    StatusUnavailable,
				ExpiresAt: now.Add(r.ttl),
				Reason:    "insufficient_sources",
			}
			return res, nil
		}

		// Probe sources concurrently
		type probeResult struct {
			name      string
			candidate Candidate
			err       error
		}
		resultsCh := make(chan probeResult, len(activeSources))
		var wg sync.WaitGroup
		for _, s := range activeSources {
			wg.Add(1)
			go func(src Source) {
				defer wg.Done()
				cand, err := src.Probe(ctx, client)
				resultsCh <- probeResult{name: src.Name(), candidate: cand, err: err}
			}(s)
		}
		wg.Wait()
		close(resultsCh)

		type namedCandidate struct {
			sourceName string
			candidate  Candidate
		}
		var successful []namedCandidate
		var failedCount int
		for res := range resultsCh {
			if res.err == nil && res.candidate.valid() {
				successful = append(successful, namedCandidate{
					sourceName: res.name,
					candidate:  res.candidate,
				})
			} else {
				failedCount++
			}
		}

		// At least two sources must succeed to form consensus
		if len(successful) < 2 {
			res := Resolution{
				Status:    StatusUnavailable,
				ExpiresAt: now.Add(r.ttl),
				Reason:    "insufficient_successful_sources",
			}
			return res, nil
		}

		// Pairwise cross-validation: every pair must agree on IP and CountryCode
		first := successful[0].candidate
		firstIP := net.ParseIP(strings.TrimSpace(first.IP))
		firstCountry := first.NormalizedCountryCode()

		var agreedSources []string
		agreedSources = append(agreedSources, successful[0].sourceName)

		hasDisagreement := false
		var combinedASN *int
		if first.ASN > 0 {
			asnVal := first.ASN
			combinedASN = &asnVal
		}

		for i := 1; i < len(successful); i++ {
			item := successful[i].candidate
			itemIP := net.ParseIP(strings.TrimSpace(item.IP))
			itemCountry := item.NormalizedCountryCode()

			if firstIP == nil || itemIP == nil || !firstIP.Equal(itemIP) || firstCountry != itemCountry {
				hasDisagreement = true
				break
			}

			if item.ASN > 0 {
				if combinedASN != nil && *combinedASN != item.ASN {
					// Disagreement on ASN between sources providing ASN
					hasDisagreement = true
					break
				}
				if combinedASN == nil {
					asnVal := item.ASN
					combinedASN = &asnVal
				}
			}

			agreedSources = append(agreedSources, successful[i].sourceName)
		}

		if hasDisagreement {
			res := Resolution{
				Status:    StatusConflicted,
				ExpiresAt: now.Add(r.ttl),
				Reason:    "identity_consensus_conflict",
			}
			return res, nil
		}

		// Consensus reached: construct ExitIdentity
		digest := ComputeIdentityDigest(first.NormalizedIP(), firstCountry, combinedASN)
		exitIdentity := &domain.ExitIdentity{
			NodeLogicalID:  nodeLogicalID,
			ObservedAt:     now,
			IdentityDigest: digest,
			CountryCode:    firstCountry,
			ASN:            combinedASN,
		}

		if err := exitIdentity.Validate(); err != nil {
			return Resolution{
				Status:    StatusError,
				ExpiresAt: now.Add(r.ttl),
				Reason:    "exit_identity_validation_failed",
			}, err
		}

		res := Resolution{
			Status:        StatusVerified,
			Identity:      exitIdentity,
			ExpiresAt:     now.Add(r.ttl),
			Reason:        "consensus_verified",
			AgreedSources: agreedSources,
		}

		// Cache verified resolution
		r.mu.Lock()
		r.cache[nodeLogicalID] = cachedResolution{value: cloneResolution(res)}
		r.mu.Unlock()

		return res, nil
	})
}

// Get returns the cached resolution for a node if present and unexpired.
func (r *Resolver) Get(nodeLogicalID string) (Resolution, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := r.clock().UTC()
	cached, ok := r.cache[nodeLogicalID]
	if !ok || !now.Before(cached.value.ExpiresAt) {
		return Resolution{}, false
	}
	return cloneResolution(cached.value), true
}

// Set stores a resolution directly into the cache.
func (r *Resolver) Set(nodeLogicalID string, res Resolution) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[nodeLogicalID] = cachedResolution{value: cloneResolution(res)}
}

// Invalidate clears the cache for a specific node logical ID.
func (r *Resolver) Invalidate(nodeLogicalID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, nodeLogicalID)
}

// Clear flushes all entries from the cache.
func (r *Resolver) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]cachedResolution)
}

// Len returns the count of entries in the cache.
func (r *Resolver) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.cache)
}

// ComputeIdentityDigest creates a deterministic, non-reversible SHA-256 digest for an exit identity.
func ComputeIdentityDigest(normalizedIP, countryCode string, asn *int) string {
	asnStr := ""
	if asn != nil && *asn > 0 {
		asnStr = strconv.Itoa(*asn)
	}
	canonical := fmt.Sprintf("exit_identity:%s:%s:%s", normalizedIP, strings.ToUpper(strings.TrimSpace(countryCode)), asnStr)
	sum := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cloneResolution(res Resolution) Resolution {
	cloned := res
	if res.Identity != nil {
		idCopy := *res.Identity
		if res.Identity.ASN != nil {
			asnCopy := *res.Identity.ASN
			idCopy.ASN = &asnCopy
		}
		cloned.Identity = &idCopy
	}
	if res.AgreedSources != nil {
		cloned.AgreedSources = append([]string(nil), res.AgreedSources...)
	}
	return cloned
}

// ExtractCandidate parses JSON or Trace payload bytes into a Candidate.
func ExtractCandidate(payload []byte) (Candidate, error) {
	if len(payload) == 0 {
		return Candidate{}, errors.New("empty payload")
	}

	// 1. Try JSON extraction
	var jsonMap map[string]any
	if err := json.Unmarshal(payload, &jsonMap); err == nil {
		ip := extractString(jsonMap, "ip", "query")
		country := extractString(jsonMap, "country_code", "country", "countryCode")
		org := extractString(jsonMap, "organization", "org", "isp")
		asn := extractASN(jsonMap["asn"])

		cand := Candidate{
			IP:           ip,
			CountryCode:  strings.ToUpper(strings.TrimSpace(country)),
			ASN:          asn,
			Organization: org,
		}
		if cand.valid() {
			return cand, nil
		}
	}

	// 2. Try Cloudflare-style Trace extraction (key=val lines)
	lines := strings.Split(string(payload), "\n")
	traceMap := make(map[string]string)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		key, val, ok := strings.Cut(trimmed, "=")
		if ok {
			traceMap[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(val)
		}
	}

	ip := traceMap["ip"]
	country := traceMap["loc"]
	if country == "" {
		country = traceMap["country"]
	}
	asn := 0
	if rawASN, ok := traceMap["asn"]; ok {
		cleanASN := strings.TrimPrefix(strings.ToUpper(rawASN), "AS")
		asn, _ = strconv.Atoi(cleanASN)
	}

	cand := Candidate{
		IP:          ip,
		CountryCode: strings.ToUpper(strings.TrimSpace(country)),
		ASN:         asn,
	}
	if cand.valid() {
		return cand, nil
	}

	return Candidate{}, errors.New("failed to extract valid exit candidate from payload")
}

func extractString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if val, ok := m[k]; ok {
			if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func extractASN(val any) int {
	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		clean := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(v)), "AS")
		parsed, _ := strconv.Atoi(clean)
		return parsed
	default:
		return 0
	}
}

// HTTPSource fetches exit candidate from an HTTP endpoint.
type HTTPSource struct {
	name string
	url  string
}

// NewHTTPSource creates an HTTP-based exit identity source.
func NewHTTPSource(name, url string) *HTTPSource {
	return &HTTPSource{name: name, url: url}
}

func (s *HTTPSource) Name() string {
	return s.name
}

func (s *HTTPSource) Probe(ctx context.Context, client *http.Client) (Candidate, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return Candidate{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CSP-Probe/2.0)")

	resp, err := client.Do(req)
	if err != nil {
		return Candidate{}, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Candidate{}, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxIdentityResponseBytes))
	if err != nil {
		return Candidate{}, fmt.Errorf("read response body: %w", err)
	}

	return ExtractCandidate(body)
}

// NewIPsbSource creates a standard ip.sb geo source.
func NewIPsbSource() *HTTPSource {
	return NewHTTPSource("ip.sb", DefaultIPsbURL)
}

// NewCloudflareTraceSource creates a standard Cloudflare trace source.
func NewCloudflareTraceSource() *HTTPSource {
	return NewHTTPSource("cloudflare", DefaultCloudflareTraceURL)
}

// DefaultSources returns the standard pair of independent identity sources.
func DefaultSources() []Source {
	return []Source{
		NewIPsbSource(),
		NewCloudflareTraceSource(),
	}
}

// StaticSource is a deterministic mock source for testing.
type StaticSource struct {
	name      string
	candidate Candidate
	err       error
}

// NewStaticSource creates a deterministic source returning a fixed candidate or error.
func NewStaticSource(name string, candidate Candidate) *StaticSource {
	return &StaticSource{name: name, candidate: candidate}
}

// NewFailingStaticSource creates a deterministic source returning an error.
func NewFailingStaticSource(name string, err error) *StaticSource {
	return &StaticSource{name: name, err: err}
}

func (s *StaticSource) Name() string {
	return s.name
}

func (s *StaticSource) Probe(context.Context, *http.Client) (Candidate, error) {
	if s.err != nil {
		return Candidate{}, s.err
	}
	return s.candidate, nil
}
