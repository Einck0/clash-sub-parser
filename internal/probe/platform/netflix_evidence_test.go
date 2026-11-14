package platform_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
)

func TestCheckNetflixEmptyCDNPreservesFallbackEvidence(t *testing.T) {
	for _, interruption := range []struct {
		name    string
		err     error
		verdict domain.ProbeVerdict
		reason  string
	}{
		{"cancelled", context.Canceled, domain.VerdictSkipped, "cancelled"},
		{"budget", platform.ErrBodyBudget, domain.VerdictSkipped, "budget_not_executed"},
		{"deadline", context.DeadlineExceeded, domain.VerdictError, "deadline_exceeded"},
		{"body_read", io.ErrUnexpectedEOF, domain.VerdictError, "transport_error"},
	} {
		for _, firstStatus := range []int{0, http.StatusNotFound, http.StatusForbidden, http.StatusOK} {
			t.Run(interruption.name+"/"+http.StatusText(firstStatus), func(t *testing.T) {
				titleCalls := 0
				client := mockClient(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host == "api.fast.com" {
						return htmlResponse(http.StatusOK, `{"targets":[]}`)
					}
					if req.URL.Path == "/title/80018499" {
						return nil, interruption.err
					}
					titleCalls++
					if req.URL.Path == "/title/81280792" && firstStatus != 0 {
						return htmlResponse(firstStatus, "title evidence")
					}
					return &http.Response{StatusCode: http.StatusOK, Body: failingBody{err: interruption.err}}, nil
				})
				cap := platform.CheckNetflix(context.Background(), client)
				wantVerdict, wantReason := interruption.verdict, interruption.reason
				if firstStatus == http.StatusForbidden {
					wantVerdict, wantReason = domain.VerdictRestricted, "title_403"
				} else if firstStatus == http.StatusOK {
					wantVerdict, wantReason = domain.VerdictAvailable, "title_non_original_unlocked"
				}
				if titleCalls != 2 || cap.Verdict != wantVerdict || cap.Reason != wantReason {
					t.Fatalf("title calls=%d capability=%+v want %s/%s", titleCalls, cap, wantVerdict, wantReason)
				}
			})
		}
	}
}

func TestCheckNetflixDoesNotUnlockOnUnreadableTitleResponse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		verdict domain.ProbeVerdict
		reason  string
	}{
		{"broken body", io.ErrUnexpectedEOF, domain.VerdictError, "transport_error"},
		{"cancelled", context.Canceled, domain.VerdictSkipped, "cancelled"},
		{"budget exhausted", platform.ErrBodyBudget, domain.VerdictSkipped, "budget_not_executed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			titleCalls := 0
			client := mockClient(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "api.fast.com" {
					return nil, tc.err
				}
				if req.URL.Host != "www.netflix.com" || (req.URL.Path != "/title/81280792" && req.URL.Path != "/title/70143836") {
					t.Fatalf("unexpected request after failed title evidence: %s", req.URL)
				}
				titleCalls++
				return &http.Response{StatusCode: http.StatusOK, Body: failingBody{err: tc.err}}, nil
			})
			cap := platform.CheckNetflix(context.Background(), client)
			if titleCalls != 2 || cap.Verdict != tc.verdict || cap.Reason != tc.reason || cap.SubTier != "" {
				t.Fatalf("title calls=%d capability=%+v, want %s/%s without unlock tier", titleCalls, cap, tc.verdict, tc.reason)
			}
		})
	}
}
