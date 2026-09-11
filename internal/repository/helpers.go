package repository

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

var timeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05.999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// parseTime parses datetime strings commonly stored by Python/SQLite.
func parseTime(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, nil
	}

	for _, layout := range timeFormats {
		if t, err := time.Parse(layout, trimmed); err == nil {
			return t.UTC(), nil
		}
	}

	// Fallback to standard RFC3339
	return time.Parse(time.RFC3339, trimmed)
}

// formatTime formats a time into standard SQLite timestamp string.
func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

func nullStringFromPtr(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullStringVal(s string) sql.NullString {
	if strings.TrimSpace(s) == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func stringFromNull(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func nullInt64FromPtr(val *int64) sql.NullInt64 {
	if val == nil {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: *val, Valid: true}
}

func int64PtrFromNull(ni sql.NullInt64) *int64 {
	if ni.Valid {
		v := ni.Int64
		return &v
	}
	return nil
}

func nullFloat64FromPtr(val *float64) sql.NullFloat64 {
	if val == nil {
		return sql.NullFloat64{Valid: false}
	}
	return sql.NullFloat64{Float64: *val, Valid: true}
}

func float64PtrFromNull(nf sql.NullFloat64) *float64 {
	if nf.Valid {
		v := nf.Float64
		return &v
	}
	return nil
}

func nullTimeFromPtr(t *time.Time) sql.NullString {
	if t == nil || t.IsZero() {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: formatTime(*t), Valid: true}
}

func timePtrFromNull(ns sql.NullString) *time.Time {
	if !ns.Valid || strings.TrimSpace(ns.String) == "" {
		return nil
	}
	parsed, err := parseTime(ns.String)
	if err != nil {
		return nil
	}
	return &parsed
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int64) bool {
	return i != 0
}

func marshalJSONSafe(v any, fallback string) string {
	if v == nil {
		return fallback
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(data)
}

func unmarshalJSONSafe(raw string, target any) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return json.Unmarshal([]byte(trimmed), target)
}
