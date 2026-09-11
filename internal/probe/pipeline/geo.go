package pipeline

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrNoGeoEndpoints is returned when no IP endpoints are provided.
	ErrNoGeoEndpoints = errors.New("no IP endpoints provided")
	// ErrGeoResolutionFailed is returned when all IP endpoints fail to resolve exit IP.
	ErrGeoResolutionFailed = errors.New("failed to resolve egress IP from candidate providers")
)

type rawIPObservation struct {
	Endpoint     string
	IP           string
	Country      string
	City         string
	ASN          *int64
	Organization string
	Err          error
}

// ResolveGeoIP queries candidate IP/Geo endpoints and computes consensus on exit IP and location.
func ResolveGeoIP(ctx context.Context, client *http.Client, endpoints []string, timeout time.Duration) (*GeoIdentityResult, error) {
	if len(endpoints) == 0 {
		return nil, ErrNoGeoEndpoints
	}
	if client == nil {
		client = http.DefaultClient
	}

	// Limit querying to at most 2 endpoints concurrently for consensus
	numEndpoints := len(endpoints)
	if numEndpoints > 2 {
		numEndpoints = 2
	}
	targetEndpoints := endpoints[:numEndpoints]

	var wg sync.WaitGroup
	obsChan := make(chan rawIPObservation, numEndpoints)

	for _, ep := range targetEndpoints {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			obs := fetchSingleGeoIP(ctx, client, url, timeout)
			obsChan <- obs
		}(ep)
	}

	wg.Wait()
	close(obsChan)

	var validObs []rawIPObservation
	var lastErr error

	for obs := range obsChan {
		if obs.Err == nil && obs.IP != "" {
			validObs = append(validObs, obs)
		} else if obs.Err != nil {
			lastErr = obs.Err
		}
	}

	if len(validObs) == 0 {
		if lastErr == nil {
			lastErr = ErrGeoResolutionFailed
		}
		return nil, lastErr
	}

	// If single observation succeeded
	if len(validObs) == 1 {
		return &GeoIdentityResult{
			IP:           validObs[0].IP,
			Country:      validObs[0].Country,
			City:         validObs[0].City,
			ASN:          validObs[0].ASN,
			Organization: validObs[0].Organization,
			Confidence:   "single_source",
		}, nil
	}

	// If two observations succeeded: check consensus
	o1 := validObs[0]
	o2 := validObs[1]

	ipMatch := strings.EqualFold(o1.IP, o2.IP)
	countryMatch := (o1.Country == "" || o2.Country == "") || strings.EqualFold(o1.Country, o2.Country)

	confidence := "single_source"
	if ipMatch && countryMatch {
		confidence = "verified"
	} else if !ipMatch {
		confidence = "conflicted"
	}

	// Prefer the observation that has more details (e.g. ASN / City)
	best := o1
	if best.ASN == nil && o2.ASN != nil {
		best = o2
	}
	if best.Country == "" && o2.Country != "" {
		best.Country = o2.Country
	}
	if best.City == "" && o2.City != "" {
		best.City = o2.City
	}
	if best.Organization == "" && o2.Organization != "" {
		best.Organization = o2.Organization
	}

	return &GeoIdentityResult{
		IP:           best.IP,
		Country:      best.Country,
		City:         best.City,
		ASN:          best.ASN,
		Organization: best.Organization,
		Confidence:   confidence,
	}, nil
}

func fetchSingleGeoIP(ctx context.Context, client *http.Client, url string, timeout time.Duration) rawIPObservation {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		return rawIPObservation{Endpoint: url, Err: err}
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Connection", "close")

	resp, err := client.Do(req)
	if err != nil {
		return rawIPObservation{Endpoint: url, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return rawIPObservation{Endpoint: url, Err: fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)}
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return rawIPObservation{Endpoint: url, Err: err}
	}

	// Try Cloudflare trace format (key=value lines)
	if strings.Contains(url, "cdn-cgi/trace") || (!strings.HasPrefix(strings.TrimSpace(string(bodyBytes)), "{") && strings.Contains(string(bodyBytes), "ip=")) {
		return parseCloudflareTrace(url, bodyBytes)
	}

	// Try JSON format
	return parseJSONGeo(url, bodyBytes)
}

func parseCloudflareTrace(url string, body []byte) rawIPObservation {
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	obs := rawIPObservation{Endpoint: url}
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.IndexByte(line, '='); idx != -1 {
			k := strings.TrimSpace(line[:idx])
			v := strings.TrimSpace(line[idx+1:])
			switch k {
			case "ip":
				obs.IP = v
			case "loc":
				obs.Country = strings.ToUpper(v)
			}
		}
	}
	if obs.IP == "" {
		obs.Err = errors.New("no ip field found in trace output")
	}
	return obs
}

func parseJSONGeo(url string, body []byte) rawIPObservation {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return rawIPObservation{Endpoint: url, Err: err}
	}

	obs := rawIPObservation{Endpoint: url}

	// IP field variations: "ip", "query"
	if ipVal, ok := m["ip"].(string); ok && ipVal != "" {
		obs.IP = strings.TrimSpace(ipVal)
	} else if qVal, ok := m["query"].(string); ok && qVal != "" {
		obs.IP = strings.TrimSpace(qVal)
	}

	// Country code variations: "country_code", "countryCode"
	if cVal, ok := m["country_code"].(string); ok && cVal != "" {
		obs.Country = strings.ToUpper(strings.TrimSpace(cVal))
	} else if cVal, ok := m["countryCode"].(string); ok && cVal != "" {
		obs.Country = strings.ToUpper(strings.TrimSpace(cVal))
	}

	// City
	if cityVal, ok := m["city"].(string); ok && cityVal != "" {
		obs.City = strings.TrimSpace(cityVal)
	}

	// Organization / ISP
	if orgVal, ok := m["organization"].(string); ok && orgVal != "" {
		obs.Organization = strings.TrimSpace(orgVal)
	} else if orgVal, ok := m["org"].(string); ok && orgVal != "" {
		obs.Organization = strings.TrimSpace(orgVal)
	} else if ispVal, ok := m["isp"].(string); ok && ispVal != "" {
		obs.Organization = strings.TrimSpace(ispVal)
	}

	// ASN
	if asnNum, ok := m["asn"].(float64); ok && asnNum > 0 {
		val := int64(asnNum)
		obs.ASN = &val
	} else if asStr, ok := m["as"].(string); ok && asStr != "" {
		// e.g. "AS15169 Google LLC"
		if strings.HasPrefix(strings.ToUpper(asStr), "AS") {
			fields := strings.Fields(asStr)
			if len(fields) > 0 {
				numPart := strings.TrimPrefix(fields[0], "AS")
				numPart = strings.TrimPrefix(numPart, "as")
				if parsed, err := strconv.ParseInt(numPart, 10, 64); err == nil {
					obs.ASN = &parsed
				}
			}
		}
	}

	if obs.IP == "" {
		obs.Err = errors.New("no valid IP found in JSON response")
	}

	return obs
}
