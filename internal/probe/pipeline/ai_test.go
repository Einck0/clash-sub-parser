package pipeline_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/probe/pipeline"
)

func TestCheckChatGPT(t *testing.T) {
	t.Run("NormalWebAndApp", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status": "normal"}`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckChatGPTWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "Yes (App)" && res != "Yes" {
			t.Errorf("expected Yes (App) or Yes, got %s", res)
		}
	})

	t.Run("BlockedByCloudflare", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<html><body>1020 Access Denied Cloudflare</body></html>`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckChatGPTWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "Blocked" {
			t.Errorf("expected Blocked, got %s", res)
		}
	})
}

func TestCheckGemini(t *testing.T) {
	t.Run("AllowedRegion", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body>Google Gemini,2,1,200,"USA" chat assistant</body></html>`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckGeminiWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "Yes") && !strings.Contains(res, "US") {
			t.Errorf("expected Yes (US), got %s", res)
		}
	})

	t.Run("BlockedRegion", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body>Google Gemini,2,1,200,"CHN" chat assistant</body></html>`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckGeminiWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "No" {
			t.Errorf("expected No, got %s", res)
		}
	})
}

func TestCheckAIStudio(t *testing.T) {
	t.Run("GeofencePassedWithInvalidKey", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": {"code": 400, "message": "API key not valid. Please pass a valid API key.", "status": "INVALID_ARGUMENT", "details": [{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "API_KEY_INVALID"}]}}`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckAIStudioWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "Yes" {
			t.Errorf("expected Yes on geofence pass, got %s", res)
		}
	})

	t.Run("GeofenceBlockedLocation", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": {"code": 400, "message": "User location is not supported for the API use.", "status": "FAILED_PRECONDITION"}}`))
		}))
		defer ts.Close()

		res, err := pipeline.CheckAIStudioWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "No" {
			t.Errorf("expected No on blocked location, got %s", res)
		}
	})
}

func TestCheckClaude(t *testing.T) {
	t.Run("TraceRegionExtraction", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("fl=123\r\nh=claude.ai\r\nip=1.2.3.4\r\nloc=JP\r\n"))
		}))
		defer ts.Close()

		res, err := pipeline.CheckClaudeWithURL(context.Background(), ts.Client(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "Yes (JP)" && res != "JP" {
			t.Errorf("expected Yes (JP) or JP, got %s", res)
		}
	})
}
