package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"clash-sub-parser/internal/domain"
)

type dnsSettingsResponse struct {
	Enabled   bool           `json:"enabled"`
	RawYAML   string         `json:"raw_yaml"`
	Revision  int64          `json:"revision"`
	UpdatedAt *time.Time     `json:"updated_at"`
	Source    string         `json:"source"`
	Config    map[string]any `json:"config"`
}

type updateDNSSettingsRequest struct {
	Enabled          *bool  `json:"enabled"`
	RawYAML          string `json:"raw_yaml"`
	ExpectedRevision *int64 `json:"expected_revision"`
}

type validateDNSRequest struct {
	Enabled *bool  `json:"enabled"`
	RawYAML string `json:"raw_yaml"`
}

type validateDNSResponse struct {
	RawYAML string         `json:"raw_yaml"`
	Config  map[string]any `json:"config"`
}

func settingsGetDNSHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.DNSSettingsRepository == nil {
			WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "DNS settings repository not configured")
			return
		}

		s, err := cfg.DNSSettingsRepository.Get(r.Context())
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}

		cfgMap, err := domain.ParseDNSSettings(s.Enabled, s.RawYAML)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}

		var updatedAt *time.Time
		if !s.UpdatedAt.IsZero() {
			updatedAt = &s.UpdatedAt
		}

		WriteSuccess(w, r, http.StatusOK, dnsSettingsResponse{
			Enabled:   s.Enabled,
			RawYAML:   s.RawYAML,
			Revision:  s.Revision,
			UpdatedAt: updatedAt,
			Source:    s.Source,
			Config:    cfgMap,
		})
	}
}

func settingsPutDNSHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.DNSSettingsRepository == nil {
			WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "DNS settings repository not configured")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		var req updateDNSSettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid request body")
			return
		}

		if req.Enabled == nil {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_dns_settings", "enabled is required")
			return
		}
		if req.ExpectedRevision == nil {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_dns_settings", "expected_revision is required")
			return
		}
		if *req.ExpectedRevision < 0 {
			WriteError(w, r, http.StatusUnprocessableEntity, "invalid_dns_settings", "expected_revision must be non-negative")
			return
		}

		// First parse/normalize to catch any invalid YAML upfront (422) before updating repository
		cfgMap, err := domain.ParseDNSSettings(*req.Enabled, req.RawYAML)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}

		settings := &domain.DNSSettings{
			Enabled: *req.Enabled,
			RawYAML: req.RawYAML,
		}

		if err := cfg.DNSSettingsRepository.Update(r.Context(), settings, *req.ExpectedRevision); err != nil {
			WriteDomainError(w, r, err)
			return
		}

		if cfg.AuditRepository != nil {
			_ = cfg.AuditRepository.Record(r.Context(), &domain.AuditEvent{
				ID:              domain.MustNewUUIDv7(),
				ActorKind:       domain.ActorKindAdmin,
				RequestID:       GetRequestID(r.Context()),
				Action:          "settings.dns.update",
				Result:          domain.AuditResultSuccess,
				RedactedSummary: fmt.Sprintf("Updated DNS settings to revision %d (enabled: %t)", settings.Revision, settings.Enabled),
				CreatedAt:       domain.NowUTC(),
			})
		}

		var updatedAt *time.Time
		if !settings.UpdatedAt.IsZero() {
			updatedAt = &settings.UpdatedAt
		}

		WriteSuccess(w, r, http.StatusOK, dnsSettingsResponse{
			Enabled:   settings.Enabled,
			RawYAML:   settings.RawYAML,
			Revision:  settings.Revision,
			UpdatedAt: updatedAt,
			Source:    settings.Source,
			Config:    cfgMap,
		})
	}
}

func settingsValidateDNSHandler(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		var req validateDNSRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid request body")
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		normalized, err := domain.NormalizeDNSSettings(enabled, req.RawYAML)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}

		cfgMap, err := domain.ParseDNSSettings(enabled, req.RawYAML)
		if err != nil {
			WriteDomainError(w, r, err)
			return
		}

		WriteSuccess(w, r, http.StatusOK, validateDNSResponse{
			RawYAML: normalized,
			Config:  cfgMap,
		})
	}
}
