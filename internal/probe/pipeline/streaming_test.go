package pipeline_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/probe/pipeline"
)

func TestCheckYouTube(t *testing.T) {
	t.Run("PremiumUnlockedWithRegion", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><script>var data = {"countryCode": "US"};</script></html>`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckYouTubeWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "US" && !strings.Contains(res, "US") {
			t.Errorf("expected US region, got %s", res)
		}
	})

	t.Run("PremiumNotAvailableInCountry", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body>Premium is not available in your country</body></html>`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckYouTubeWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "No" {
			t.Errorf("expected No, got %s", res)
		}
	})

	t.Run("BlockedOrChallenged", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer ts.Close()

		res, err := pipeline.CheckYouTubeWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "Blocked" {
			t.Errorf("expected Blocked, got %s", res)
		}
	})
}

func TestCheckNetflix(t *testing.T) {
	t.Run("FullUnlock", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			if strings.Contains(r.URL.Path, "70143836") {
				_, _ = w.Write([]byte(`<html><body>Breaking Bad title-70143836 watch-button</body></html>`))
			} else if strings.Contains(r.URL.Path, "80018499") {
				_, _ = w.Write([]byte(`<html><body>Stranger Things title-80018499 Netflix Original</body></html>`))
			}
		}))
		defer ts.Close()

		res, err := pipeline.CheckNetflixWithBaseURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "Full" {
			t.Errorf("expected Full, got %s", res)
		}
	})

	t.Run("OriginalsOnly", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "70143836") {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`<html><body>Stranger Things title-80018499 Netflix Original</body></html>`))
			}
		}))
		defer ts.Close()

		res, err := pipeline.CheckNetflixWithBaseURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "Originals Only" {
			t.Errorf("expected Originals Only, got %s", res)
		}
	})

	t.Run("Blocked", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer ts.Close()

		res, err := pipeline.CheckNetflixWithBaseURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "Blocked" {
			t.Errorf("expected Blocked, got %s", res)
		}
	})
}

func TestCheckDisney(t *testing.T) {
	t.Run("Available", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body>Welcome to Disney+ stream Disney originals</body></html>`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckDisneyWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "Yes" {
			t.Errorf("expected Yes, got %s", res)
		}
	})

	t.Run("UnavailableInRegion", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/unavailable" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`<html><body>Service unavailable in your region</body></html>`))
				return
			}
			http.Redirect(w, r, "/unavailable", http.StatusFound)
		}))
		defer ts.Close()

		res, err := pipeline.CheckDisneyWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "No" {
			t.Errorf("expected No, got %s", res)
		}
	})
}

func TestCheckBilibili(t *testing.T) {
	t.Run("HongKongMacauTaiwan", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code": 0, "message": "0"}`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckBilibiliWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "港澳台限定" {
			t.Errorf("expected 港澳台限定, got %s", res)
		}
	})

	t.Run("MainlandOnly", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code": -10403, "message": "仅限大陆地区"}`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckBilibiliWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "仅大陆" {
			t.Errorf("expected 仅大陆, got %s", res)
		}
	})
}
