package http

import (
	"context"
	"net/http"

	"clash-sub-parser/internal/repository/sqlite"
)

// ReadinessCheckerFunc defines the function signature for evaluating service readiness.
type ReadinessCheckerFunc func(ctx context.Context) (*sqlite.ReadinessReport, error)

// HealthzHandler handles GET /healthz liveness requests.
func HealthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteSuccess(w, r, http.StatusOK, map[string]any{
			"status": "ok",
		})
	}
}

// ReadyzHandler handles GET /readyz readiness probe requests.
func ReadyzHandler(checker ReadinessCheckerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if checker == nil {
			WriteSuccess(w, r, http.StatusOK, map[string]any{
				"ready":  true,
				"status": "ready",
			})
			return
		}

		report, err := checker(r.Context())
		if err != nil || report == nil || !report.Ready {
			errMsg := "Database readiness check failed"
			if report != nil && report.Error != "" {
				errMsg = report.Error
			} else if err != nil {
				errMsg = err.Error()
			}
			WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", errMsg)
			return
		}

		WriteSuccess(w, r, http.StatusOK, map[string]any{
			"ready":           true,
			"schema_version":  report.SchemaVersion,
			"required_tables": len(report.RequiredTables),
		})
	}
}
