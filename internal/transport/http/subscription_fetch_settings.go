package http

import (
	"encoding/json"
	"io"
	"net/http"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

type subscriptionFetchResponse struct {
	Mode                        string `json:"mode"`
	EffectiveMode               string `json:"effective_mode"`
	EffectiveSource             string `json:"effective_source"`
	ConfiguredProxyEndpoint     string `json:"configured_proxy_endpoint"`
	EffectiveProxyEndpoint      string `json:"effective_proxy_endpoint"`
	CredentialsPresent          bool   `json:"credentials_present"`
	EffectiveCredentialsPresent bool   `json:"effective_credentials_present"`
}

func subscriptionFetchView(settings *domain.Settings, environmentProxy string) subscriptionFetchResponse {
	configured, credentials := fetch.ProxyEndpoint(settings.SubscriptionFetchProxyURL)
	effectiveURL := settings.EffectiveSubscriptionFetchProxy(environmentProxy)
	effective, effectiveCredentials := fetch.ProxyEndpoint(effectiveURL)
	effectiveMode := "direct"
	if effectiveURL != "" {
		effectiveMode = "proxy"
	}
	source := settings.SubscriptionFetchMode
	if source == "proxy" {
		source = "settings"
	}
	return subscriptionFetchResponse{Mode: settings.SubscriptionFetchMode, EffectiveMode: effectiveMode, EffectiveSource: source,
		ConfiguredProxyEndpoint: configured, EffectiveProxyEndpoint: effective, CredentialsPresent: credentials, EffectiveCredentialsPresent: effectiveCredentials}
}

func settingsGetSubscriptionFetchHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.SettingsRepository == nil {
			WriteError(w, r, 500, "internal_error", "Settings repository unavailable")
			return
		}
		settings, err := cfg.SettingsRepository.Get(r.Context())
		if err != nil {
			WriteError(w, r, 500, "internal_error", "Cannot read subscription fetch settings")
			return
		}
		WriteSuccess(w, r, 200, subscriptionFetchView(settings, cfg.EnvironmentFetchProxy))
	}
}

// Decode explicitly rejects duplicate fields and nulls, so omission alone means retain.
func decodeSubscriptionFetchRequest(r *http.Request) (string, *string, error) {
	dec := json.NewDecoder(r.Body)
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return "", nil, domain.NewValidationError("bad_request", "Expected JSON object")
	}
	seen := map[string]bool{}
	mode := ""
	var proxy *string
	clear := false
	for dec.More() {
		token, err = dec.Token()
		if err != nil {
			return "", nil, err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return "", nil, domain.NewValidationError("bad_request", "Duplicate request field")
		}
		seen[key] = true
		var value json.RawMessage
		if err = dec.Decode(&value); err != nil {
			return "", nil, err
		}
		if string(value) == "null" {
			return "", nil, domain.NewValidationError("bad_request", "Null fields are not supported")
		}
		switch key {
		case "mode":
			err = json.Unmarshal(value, &mode)
		case "proxy_url":
			var url string
			err = json.Unmarshal(value, &url)
			proxy = &url
		case "clear_proxy_url":
			err = json.Unmarshal(value, &clear)
		default:
			return "", nil, domain.NewValidationError("bad_request", "Unknown request field")
		}
		if err != nil {
			return "", nil, err
		}
	}
	if _, err = dec.Token(); err != nil {
		return "", nil, err
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return "", nil, domain.NewValidationError("bad_request", "Unexpected trailing JSON")
	}
	if !seen["mode"] || (clear && proxy != nil) {
		return "", nil, domain.NewValidationError("bad_request", "Mode is required; clear_proxy_url cannot accompany proxy_url")
	}
	if clear {
		empty := ""
		proxy = &empty
	}
	return mode, proxy, nil
}

func settingsPutSubscriptionFetchHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.SettingsRepository == nil {
			WriteError(w, r, 500, "internal_error", "Settings repository unavailable")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		mode, proxy, err := decodeSubscriptionFetchRequest(r)
		if err != nil {
			WriteError(w, r, 400, "bad_request", "Invalid subscription fetch settings payload")
			return
		}
		if err = cfg.SettingsRepository.UpdateSubscriptionFetch(r.Context(), mode, proxy); err != nil {
			if de, ok := domain.AsDomainError(err); ok {
				WriteError(w, r, 400, de.Code, de.Message)
			} else {
				WriteError(w, r, 500, "internal_error", "Cannot save subscription fetch settings")
			}
			return
		}
		if cfg.AuditRepository != nil {
			_ = cfg.AuditRepository.Record(r.Context(), &domain.AuditEvent{ID: domain.MustNewUUIDv7(), ActorKind: domain.ActorKindAdmin, RequestID: GetRequestID(r.Context()), Action: "settings.subscription_fetch.update", Result: domain.AuditResultSuccess, RedactedSummary: "Subscription fetch route updated", CreatedAt: domain.NowUTC()})
		}
		settingsGetSubscriptionFetchHandler(cfg)(w, r)
	}
}
