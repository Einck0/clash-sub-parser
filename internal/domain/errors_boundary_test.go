package domain_test

import (
	"clash-sub-parser/internal/domain"
	"strings"
	"testing"
)

func TestRedactionPreservesWrappedFailureBoundary(t *testing.T) {
	for _, input := range []string{
		`Get "https://alice:pw-secret@example.com/feed?token=query-secret": proxyconnect tcp: connection refused`,
		`Get "https://example.com/feed?access_token=query-secret": net/http: TLS handshake timeout`,
		`Bearer bearer-secret== password="assignment-secret" reason=connection refused`,
	} {
		got := domain.RedactSensitiveInfo(input)
		for _, secret := range []string{"pw-secret", "query-secret", "bearer-secret", "assignment-secret"} {
			if strings.Contains(got, secret) {
				t.Errorf("secret leaked: %q", got)
			}
		}
		if !strings.Contains(got, "connection refused") && !strings.Contains(got, "TLS handshake timeout") {
			t.Errorf("lost failure reason: %q", got)
		}
	}
}
