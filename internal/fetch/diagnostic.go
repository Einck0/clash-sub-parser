package fetch

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"strings"
	"syscall"

	"clash-sub-parser/internal/domain"
)

func networkFailure(err error) *domain.DomainError {
	stage, reason := "connect", "network_error"
	var dns *net.DNSError
	var cert *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	switch {
	case errors.As(err, &dns):
		stage, reason = "dns", "dns_resolution_failed"
	case errors.As(err, &cert), errors.As(err, &unknown):
		stage, reason = "tls", "tls_certificate_failed"
	case errors.Is(err, syscall.ECONNREFUSED):
		reason = "connection_refused"
	case strings.Contains(err.Error(), "TLS handshake timeout"):
		stage, reason = "tls", "tls_handshake_timeout"
	case strings.Contains(err.Error(), "tls:"):
		stage, reason = "tls", "tls_handshake_failed"
	default:
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			reason = "network_timeout"
		}
	}
	if stage == "connect" && strings.Contains(err.Error(), "proxyconnect") {
		stage = "proxy_connect"
	}
	de := domain.NewInternalError("fetch_failed", RedactError(err))
	de.Details = map[string]string{"stage": stage, "reason_code": reason}
	return de
}
