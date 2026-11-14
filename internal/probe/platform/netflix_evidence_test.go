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

func TestCheckNetflixSeparatesHTTPResponsesFromFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cdnStatus  int
		cdnErr     error
		cdnBodyErr error
		titleErr   error
		verdict    domain.ProbeVerdict
		reason     string
	}{
		{"all404", 404, nil, nil, nil, domain.VerdictError, "status_mismatch"},
		{"cdn503_titles404", 503, nil, nil, nil, domain.VerdictError, "status_mismatch"},
		{"cdn_empty_titles404", 200, nil, nil, nil, domain.VerdictError, "status_mismatch"},
		{"cdn_network_titles404", 0, io.ErrClosedPipe, nil, nil, domain.VerdictError, "transport_error"},
		{"cdn_read_titles404", 200, nil, io.ErrUnexpectedEOF, nil, domain.VerdictError, "transport_error"},
		{"cdn_cancel_titles404", 0, context.Canceled, nil, nil, domain.VerdictSkipped, "cancelled"},
		{"cdn_deadline_titles404", 0, context.DeadlineExceeded, nil, nil, domain.VerdictError, "deadline_exceeded"},
		{"cdn_budget_titles404", 200, nil, platform.ErrBodyBudget, nil, domain.VerdictSkipped, "budget_not_executed"},
		{"cdn404_titles_cancel", 404, nil, nil, context.Canceled, domain.VerdictSkipped, "cancelled"},
		{"cdn503_titles_budget", 503, nil, nil, platform.ErrBodyBudget, domain.VerdictSkipped, "budget_not_executed"},
		{"cdn404_titles_deadline", 404, nil, nil, context.DeadlineExceeded, domain.VerdictError, "deadline_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := mockClient(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "api.fast.com" {
					if tc.cdnErr != nil {
						return nil, tc.cdnErr
					}
					resp, err := htmlResponse(tc.cdnStatus, `{"targets":[]}`)
					if tc.cdnBodyErr != nil {
						resp.Body = failingBody{err: tc.cdnBodyErr}
					}
					return resp, err
				}
				if tc.titleErr != nil {
					return nil, tc.titleErr
				}
				return htmlResponse(http.StatusNotFound, "")
			})
			cap := platform.CheckNetflix(context.Background(), client)
			if cap.Verdict != tc.verdict || cap.Reason != tc.reason {
				t.Fatalf("capability=%+v want %s/%s", cap, tc.verdict, tc.reason)
			}
		})
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
