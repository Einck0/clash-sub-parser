package platform

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"testing"
)

func TestResponseReasonTypedWrappedGolden(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, ""}, {context.Canceled, "cancelled"}, {context.DeadlineExceeded, "deadline_exceeded"},
		{ErrBodyBudget, "budget_not_executed"}, {&net.DNSError{Err: "no such host", Name: "private-secret"}, "dns_error"},
		{x509.UnknownAuthorityError{}, "tls_certificate_error"}, {x509.HostnameError{}, "tls_certificate_error"},
		{syscall.ECONNREFUSED, "connection_refused"}, {syscall.ECONNRESET, "connection_reset"},
		{fmt.Errorf("opaque private password"), "transport_error"},
	}
	for _, tt := range tests {
		err := tt.err
		if err != nil {
			err = &url.Error{Op: "Get", URL: "https://secret/?token=private", Err: fmt.Errorf("wrapped: %w", err)}
		}
		if got := ResponseReason(err); got != tt.want {
			t.Errorf("got %q want %q", got, tt.want)
		}
	}
}
