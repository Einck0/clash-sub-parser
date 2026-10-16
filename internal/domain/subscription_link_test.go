package domain_test

import (
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestSubscriptionLinkLifecycle(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	link := &domain.SubscriptionLink{
		ID:        "link-1",
		Label:     "Default Mihomo",
		Target:    domain.TargetMihomo,
		Token:     "pub_0123456789abcdef0123456789abcdef",
		TokenHash: "hash-1",
		State:     domain.SubscriptionLinkStateActive,
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if !link.IsActive() {
		t.Fatal("expected link to be active")
	}

	masked := link.MaskedToken()
	if masked != "pub_...cdef" {
		t.Fatalf("expected masked token pub_...cdef, got %s", masked)
	}

	shortLink := &domain.SubscriptionLink{Token: "short"}
	if shortLink.MaskedToken() != "******" {
		t.Fatalf("expected short masked token ******, got %s", shortLink.MaskedToken())
	}

	// Rotate token
	rotateTime := now.Add(1 * time.Hour)
	if err := link.RotateToken("pub_newtoken1234567890abcdef", "hash-new", rotateTime); err != nil {
		t.Fatalf("RotateToken failed: %v", err)
	}
	if link.Token != "pub_newtoken1234567890abcdef" || link.TokenHash != "hash-new" {
		t.Fatal("token not rotated properly")
	}
	if link.UpdatedAt != rotateTime {
		t.Fatalf("expected updated_at %v, got %v", rotateTime, link.UpdatedAt)
	}

	// Revoke
	revokeTime := now.Add(2 * time.Hour)
	if err := link.Revoke(revokeTime); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}
	if link.IsActive() {
		t.Fatal("expected link to not be active after revocation")
	}
	if link.RevokedAt == nil || *link.RevokedAt != revokeTime {
		t.Fatalf("expected revoked_at %v, got %v", revokeTime, link.RevokedAt)
	}

	// Repeated revoke should return conflict error
	if err := link.Revoke(revokeTime.Add(1 * time.Minute)); err == nil {
		t.Fatal("expected error on second Revoke call")
	}

	// Rotate on revoked link should fail
	if err := link.RotateToken("another", "hash", revokeTime); err == nil {
		t.Fatal("expected error on RotateToken on revoked link")
	}
}

func TestSettingsPublicBaseURLValidation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "empty is valid", url: "", wantErr: false},
		{name: "valid https", url: "https://sub.example.com", wantErr: false},
		{name: "valid http with port and path", url: "http://127.0.0.1:8080/prefix", wantErr: false},
		{name: "relative url rejected", url: "/path/only", wantErr: true},
		{name: "ftp scheme rejected", url: "ftp://example.com", wantErr: true},
		{name: "userinfo rejected", url: "https://user:pass@example.com", wantErr: true},
		{name: "query parameters rejected", url: "https://example.com/sub?foo=bar", wantErr: true},
		{name: "query mark rejected", url: "https://example.com/sub?", wantErr: true},
		{name: "fragment rejected", url: "https://example.com/sub#frag", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := domain.DefaultSettings()
			s.PublicBaseURL = tt.url
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Settings.Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
